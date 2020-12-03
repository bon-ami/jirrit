package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"io"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bon-ami/eztools"
)

const (
	module          = "jirrit"
	CATEGORY_JIRA   = "JIRA"
	CATEGORY_GERRIT = "Gerrit"
	PASS_BASIC      = "basic"
	PASS_PLAIN      = "plain"
	PASS_DIGEST     = "digest"
)

type passwords struct {
	Password xml.Name `xml:"pass"`
	Type     string   `xml:"type,attr"`
	Pass     string   `xml:",chardata"`
}
type svrs struct {
	Svr     xml.Name  `xml:"server"`
	Type    string    `xml:"type,attr"`
	Name    string    `xml:"name,attr"`
	URL     string    `xml:"url"`
	Pass    passwords `xml:"pass"`
	Magic   string    `xml:"magic"`
	TstPre  string    `xml:"testpre"`
	TstStep string    `xml:"teststep"`
	TstExp  string    `xml:"testexp"`
}

type cfgs struct {
	Root xml.Name  `xml:"jirrit"`
	Log  string    `xml:"log"`
	User string    `xml:"user"`
	Pass passwords `xml:"pass"`
	Svrs []svrs    `xml:"server"`
}

func main() {
	var (
		paramH, paramV, paramVV, paramVVV bool
		paramID, paramBra, paramCfg, paramLog,
		paramHD, paramP string
	)
	flag.BoolVar(&paramH, "h", false, "help message")
	flag.BoolVar(&paramV, "v", false,
		"log file output")
	flag.BoolVar(&paramVV, "vv", false, "verbose messages")
	flag.BoolVar(&paramVVV, "vvv", false,
		"verbose messages with network I/O")
	flag.StringVar(&paramID, "i", "",
		"ID of issue, change, commit or assignee")
	flag.StringVar(&paramBra, "b", "", "branch")
	flag.StringVar(&paramHD, "hd", "",
		"new assignee when transferring issues, "+
			"or revision id for cherrypicks")
	flag.StringVar(&paramP, "p", "",
		"new component when transferring issues")
	flag.StringVar(&paramCfg, "c", "", "config file")
	flag.StringVar(&paramLog, "l", "", "log file")
	flag.Parse()
	if paramH {
		flag.Usage()
		return
	}
	eztools.Debugging = paramV || paramVV || paramVVV
	switch {
	case paramV:
		eztools.Verbose = 1
	case paramVV:
		eztools.Verbose = 2
	case paramVVV:
		eztools.Verbose = 3
	}
	cats := makeCat2Act()
	var cfg cfgs
	err := eztools.XMLsReadDefault(paramCfg, module, &cfg)
	if err != nil {
		eztools.LogErrFatal(err)
		return
	}
	if len(paramLog) > 0 {
		cfg.Log = paramLog
	} else if len(cfg.Log) < 1 {
		cfg.Log = module + ".log"
	}
	if eztools.Debugging {
		logger, err := os.OpenFile(cfg.Log,
			os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err == nil {
			if err = eztools.InitLogger(logger); err != nil {
				eztools.LogErrPrint(err)
			}
		} else {
			eztools.LogPrint("Failed to open log file " + cfg.Log)
		}
	}
	for {
		svr, fun, issueInfo := chooseSvrNAct(cats, cfg.Svrs,
			issueInfos{
				ISSUEINFO_IND_ID:     paramID,
				ISSUEINFO_IND_HEAD:   paramHD,
				ISSUEINFO_IND_PROJ:   paramP,
				ISSUEINFO_IND_BRANCH: paramBra})
		if svr == nil || fun == nil {
			return
		}
		authInfo, err := cfg2AuthInfo(*svr, cfg)
		if err != nil {
			eztools.LogErrFatal(err)
			return
		}
		issues, err := fun(svr, authInfo, issueInfo)
		if err != nil {
			eztools.LogErrFatal(err)
		}
		var op func(...interface{})
		if eztools.Debugging && eztools.Verbose > 0 {
			op = eztools.LogPrint
		} else {
			op = eztools.ShowSthln
		}
		if issues == nil {
			op("No results.")
		} else {
			for i, issue := range issues {
				op("Issue/Reviewer " +
					strconv.Itoa(i+1))
				op(ISSUEINFO_STR_ID + "=" +
					issue[ISSUEINFO_IND_ID])
				op(ISSUEINFO_STR_ASSIGNEE + "/" +
					ISSUEINFO_STR_KEY + "/" +
					ISSUEINFO_STR_NAME + "/" +
					ISSUEINFO_STR_SUBMITTABLE + "=" +
					issue[ISSUEINFO_IND_SUBMITTABLE])
				op(ISSUEINFO_STR_HEAD + "/" +
					ISSUEINFO_STR_SUMMARY + "/" +
					ISSUEINFO_STR_REV_CUR + "/" +
					ISSUEINFO_STR_VERIFIED + "=" +
					issue[ISSUEINFO_IND_HEAD])
				op(ISSUEINFO_STR_PROJ + "/" +
					ISSUEINFO_STR_CODEREVIEW + "=" +
					issue[ISSUEINFO_IND_PROJ])
				op(ISSUEINFO_STR_BRANCH + "/" +
					ISSUEINFO_STR_DISPNAME + "/" +
					ISSUEINFO_STR_MANUALTEST + "=" +
					issue[ISSUEINFO_IND_BRANCH])
				op(ISSUEINFO_STR_STATE + "/" +
					ISSUEINFO_STR_SUBMIT_TYPE + "=" +
					issue[ISSUEINFO_IND_STATE])
			}
		}
	}
}

func cfg2AuthInfo(svr svrs, cfg cfgs) (authInfo eztools.AuthInfo, err error) {
	pass := svr.Pass
	if len(pass.Pass) < 1 {
		pass = cfg.Pass
	}
	authInfo = eztools.AuthInfo{User: cfg.User}
	switch pass.Type {
	case PASS_DIGEST:
		authInfo.Type = eztools.AUTH_DIGEST
	case PASS_PLAIN:
		authInfo.Type = eztools.AUTH_PLAIN
	case PASS_BASIC:
		authInfo.Type = eztools.AUTH_BASIC
	default:
		authInfo.Type = eztools.AUTH_NONE
		authInfo.Pass = ""
		return
	}
	authInfo.Pass = pass.Pass
	if len(authInfo.Pass) < 1 {
		err = errors.New("NO password configured")
	}
	return
}

/*                   action name -> actionFunc
category name -> []action2Func
cat2Act
*/
type actionFunc func(*svrs, eztools.AuthInfo, issueInfos) ([]issueInfos, error)
type action2Func struct {
	n string
	f actionFunc
}

type cat2Act map[string][]action2Func

type postRESTs func([]interface{})

var postREST postRESTs

func setPostREST(fun postRESTs) {
	postREST = fun
}

func isValidSvr(cats cat2Act, svr *svrs) bool {
	if len(svr.Name) < 1 || len(svr.Type) < 1 || len(svr.URL) < 1 {
		return false
	}
	if _, ok := cats[svr.Type]; !ok {
		return false
	}
	u, err := url.Parse(svr.URL)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func chooseSvrNAct(cats cat2Act, candidates []svrs,
	issueInfo issueInfos) (*svrs, actionFunc, issueInfos) {
	var choices []string
	for _, svr := range candidates {
		if !isValidSvr(cats, &svr) {
			continue
		}
		choices = append(choices, svr.Type+" - "+svr.Name)
	}
	eztools.ShowStrln(" Choose a server")
	si := eztools.ChooseStrings(choices)
	if si == eztools.InvalidID {
		return nil, nil, issueInfo
	}

	svr := candidates[si]
	if svr.Type == CATEGORY_JIRA {
		if len(svr.TstExp+svr.TstPre+svr.TstStep) < 1 {
			cats[svr.Type] = cats[svr.Type][:len(cats[svr.Type])-2]
		}
	}
	choices = make([]string, len(cats[svr.Type]))
	for i, choice := range cats[svr.Type] {
		choices[i] = choice.n
	}
	eztools.ShowStrln(" Choose an action")
	fi := eztools.ChooseStrings(choices)
	if fi == eztools.InvalidID {
		return nil, nil, issueInfo
	}
	inputIssueInfo4Act(svr.Type, cats[svr.Type][fi].n, &issueInfo)
	return &candidates[si], cats[svr.Type][fi].f, issueInfo

}

func restSth(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (body interface{}, err error) {
	body, _ /*errno*/, err = eztools.RestGetOrPostWtMagic(method,
		url, authInfo, bodyReq, []byte(magic))
	if err != nil {
		eztools.LogErrPrint( /*strconv.Itoa(errno), */ err)
		if body != nil {
			bodyBytes, ok := body.([]byte)
			if !ok {
				eztools.LogPrint("REST response type " +
					"not byte slice for error " +
					reflect.TypeOf(body).String())
				if eztools.Debugging && eztools.Verbose > 2 {
					eztools.ShowSthln(body)
				}
			} else {
				//eztools.ShowSthln(bodyBytes)
				eztools.LogPrint(string(bodyBytes))
			}
		}
		return
	}
	return
}

func restSlc(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (bodySlc []interface{}, err error) {
	body, err := restSth(method, url, authInfo, bodyReq, magic)
	if err != nil || body == nil {
		return
	}
	bodySlc, ok := body.([]interface{})
	if !ok {
		eztools.LogPrint("REST response type error for slice " +
			reflect.TypeOf(body).String())
	}
	return
}

func restMap(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (
	bodyMap map[string]interface{}, err error) {
	body, err := restSth(method, url, authInfo, bodyReq, magic)
	if err != nil || body == nil {
		return
	}
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		eztools.LogPrint("REST response type error for map " +
			reflect.TypeOf(body).String())
	} else {
		if err != nil {
			eztools.ShowSthln(bodyMap)
		}
	}
	return
}

const (
	// common use
	ISSUEINFO_IND_ID = iota
	ISSUEINFO_IND_KEY
	ISSUEINFO_IND_HEAD
	ISSUEINFO_IND_PROJ
	ISSUEINFO_IND_BRANCH
	// jira
	ISSUEINFO_IND_DISPNAME = iota - 1
	// common use
	ISSUEINFO_IND_STATE
	ISSUEINFO_IND_MAX

	// gerrit state
	ISSUEINFO_IND_SUBMITTABLE = iota - 1 - ISSUEINFO_IND_MAX
	ISSUEINFO_IND_VERIFIED
	ISSUEINFO_IND_CODEREVIEW
	ISSUEINFO_IND_MANUALTEST
	ISSUEINFO_IND_SUBMIT_TYPE

	ISSUEINFO_STR_ID          = "id"
	ISSUEINFO_STR_SUBMITTABLE = "submittable"      // \
	ISSUEINFO_STR_KEY         = "key"              //
	ISSUEINFO_STR_ASSIGNEE    = "assignee"         //
	ISSUEINFO_STR_NAME        = "name"             // /
	ISSUEINFO_STR_HEAD        = "subject"          // \
	ISSUEINFO_STR_SUMMARY     = "summary"          //
	ISSUEINFO_STR_REV_CUR     = "current_revision" //
	ISSUEINFO_STR_VERIFIED    = "Verified"         // /
	ISSUEINFO_STR_PROJ        = "project"          // \
	ISSUEINFO_STR_CODEREVIEW  = "Code-Review"      // /
	ISSUEINFO_STR_BRANCH      = "branch"           // \
	ISSUEINFO_STR_DISPNAME    = "displayName"      //
	ISSUEINFO_STR_MANUALTEST  = "Manual-Testing"   // /
	// for code-review, verified and manual-testing
	ISSUEINFO_STR_SUBMIT_TYPE = "submit_type" // \
	ISSUEINFO_STR_APPROVALS   = "approvals"   //
	ISSUEINFO_STR_STATE       = "status"      // /
)

type issueInfos [ISSUEINFO_IND_MAX]string
type scoreInfos [ISSUEINFO_IND_SUBMIT_TYPE]int

var issueInfoTxt = issueInfos{
	ISSUEINFO_STR_ID, ISSUEINFO_STR_KEY, ISSUEINFO_STR_HEAD,
	ISSUEINFO_STR_PROJ, ISSUEINFO_STR_BRANCH, ISSUEINFO_STR_STATE}
var issueDetailsTxt = issueInfos{
	ISSUEINFO_STR_ID, ISSUEINFO_STR_SUBMITTABLE, ISSUEINFO_STR_HEAD,
	ISSUEINFO_STR_PROJ, ISSUEINFO_STR_BRANCH, ISSUEINFO_STR_STATE}
var issueRevsTxt = issueInfos{
	ISSUEINFO_STR_ID, ISSUEINFO_STR_NAME, ISSUEINFO_STR_REV_CUR,
	ISSUEINFO_STR_PROJ, /*placeholder*/
	ISSUEINFO_STR_BRANCH, ISSUEINFO_STR_SUBMIT_TYPE}

var reviewInfoTxt = issueInfos{
	ISSUEINFO_STR_ID, ISSUEINFO_STR_NAME, ISSUEINFO_STR_VERIFIED,
	ISSUEINFO_STR_CODEREVIEW, ISSUEINFO_STR_MANUALTEST,
	ISSUEINFO_STR_APPROVALS}

/*var jiraInfoTxt = issueInfos{ISSUEINFO_STR_ID, ISSUEINFO_STR_KEY,
ISSUEINFO_STR_SUMMARY, ISSUEINFO_STR_PROJ, ISSUEINFO_STR_DISPNAME,
ISSUEINFO_STR_STATE}*/

// gerritParseIssuesOrReviews parses body from gerrit responses into
// []issueInfos
/*
param:
	m	body
	issues	results are appended to this
	strs	keywords to parse
	issue	partially parsed fileds, usually for looping only
*/
func gerritParseIssuesOrReviews(m map[string]interface{},
	issues []issueInfos, strs issueInfos,
	issue *issueInfos) []issueInfos {
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowStr("parsing ")
		eztools.ShowSthln(strs)
		eztools.ShowStr("from ")
		eztools.ShowSthln(m)
	}
	if issue == nil {
		issue = new(issueInfos)
	}
	for i := 0; i < ISSUEINFO_IND_MAX; i++ {
		// string array to loop?
		mp, ok := m[strs[i]].(map[string]interface{})
		if ok {
			gerritParseIssuesOrReviews(mp, issues, strs, issue)
			continue
		}
		// try to match one field
		if len(strs[i]) < 1 || m[strs[i]] == nil {
			if eztools.Debugging && eztools.Verbose > 2 {
				if len(strs[i]) > 0 {
					eztools.ShowStrln("unmatching " +
						strs[i])
				}
			}
			continue
		}
		// string?
		str, ok := m[strs[i]].(string)
		if ok {
			if eztools.Debugging && eztools.Verbose > 2 {
				eztools.ShowStrln("matching " +
					strs[i] + " <- " + str)
			}
			issue[i] = str
			continue
		}
		// not a string
		if i == ISSUEINFO_IND_SUBMITTABLE &&
			strs[i] == ISSUEINFO_STR_SUBMITTABLE {
			// bool is different
			bo, ok := m[strs[i]].(bool)
			if !ok {
				if !eztools.Debugging {
					continue
				}
				eztools.LogPrint(
					reflect.TypeOf(
						m[strs[i]]).
						String() +
						" got instead of " +
						"bool for " +
						strs[i] + "!")
			} else {
				switch bo {
				case true:
					issue[i] = "true"
				case false:
					issue[i] = "false"
				}
				if eztools.Debugging && eztools.Verbose > 2 {
					eztools.ShowStrln("matched " +
						ISSUEINFO_STR_SUBMITTABLE +
						"=" + issue[i])
				}
			}
			continue
		}
		// other types
		if eztools.Debugging {
			eztools.Log(strs[i] + " matched with unknown type " +
				reflect.TypeOf(m[strs[i]]).String())
		}
	}
	if issues != nil {
		return append(issues, *issue)
	}
	return []issueInfos{*issue}
}

func gerritGetIssuesOrReviews(method, url, magic string,
	authInfo eztools.AuthInfo, fun func(map[string]interface{},
		[]issueInfos) []issueInfos) ([]issueInfos, error) {
	bodySlc, err := restSlc(method, url, authInfo, nil, magic)
	if err != nil || nil == bodySlc || len(bodySlc) < 1 {
		return nil, err
	}
	if postREST != nil {
		postREST(bodySlc)
	}
	issues := make([]issueInfos, 0)
	for _, v := range bodySlc {
		m, ok := v.(map[string]interface{})
		if !ok {
			eztools.LogPrint(reflect.TypeOf(v).String() +
				" got instead of map string to interface!")
			continue
		}
		issues = fun(m, issues)
	}
	return issues, err
}

// no ID will return, since not in replies
func gerritGetReviews(url, magic string, authInfo eztools.AuthInfo) (
	[]issueInfos, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues []issueInfos) []issueInfos {
			return gerritParseIssuesOrReviews(m, issues, reviewInfoTxt, nil)
		})
}

func gerritGetDetails(url, magic string, authInfo eztools.AuthInfo) (
	[]issueInfos, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues []issueInfos) []issueInfos {
			return gerritParseIssuesOrReviews(m, issues, issueDetailsTxt, nil)
		})
}

func gerritGetIssues(url, magic string, authInfo eztools.AuthInfo) (
	[]issueInfos, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues []issueInfos) []issueInfos {
			return gerritParseIssuesOrReviews(m, issues, issueInfoTxt, nil)
		})
}

// gerritGetRevs retrieves from URL and parse response into revision info
func gerritGetRevs(url, magic string, authInfo eztools.AuthInfo) (
	[]issueInfos, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues []issueInfos) []issueInfos {
			return gerritParseIssuesOrReviews(m, issues, issueRevsTxt, nil)
		})
}

func jiraParse1Field(m map[string]interface{},
	issueInfo *issueInfos) (changed bool) {
	for i, v := range m {
		switch i {
		case ISSUEINFO_STR_ASSIGNEE:
			changed = chkNLoopStringMap(v, "",
				ISSUEINFO_STR_DISPNAME,
				&issueInfo[ISSUEINFO_IND_DISPNAME]) || changed
		case ISSUEINFO_STR_PROJ:
			changed = chkNLoopStringMap(v, "",
				ISSUEINFO_STR_KEY,
				&issueInfo[ISSUEINFO_IND_PROJ]) || changed
		case ISSUEINFO_STR_STATE:
			changed = chkNLoopStringMap(v, "",
				ISSUEINFO_STR_NAME,
				&issueInfo[ISSUEINFO_IND_STATE]) || changed
		case ISSUEINFO_STR_SUMMARY:
			changed = chkNSetIssueInfo(v, issueInfo,
				ISSUEINFO_IND_HEAD) || changed
		}
	}
	return
}

func jiraParse1Issue(m map[string]interface{},
	issueInfo *issueInfos) (changed bool) {
	changed = loopStringMap(m, "fields",
		ISSUEINFO_STR_KEY, &issueInfo[ISSUEINFO_IND_KEY],
		func(i string, v interface{}) bool {
			// id, self ignored
			fields, ok := v.(map[string]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of " +
					"map[string]interface{}")
				return false
			}
			return jiraParse1Field(fields, issueInfo)
		}) || changed
	return
}

func jiraParseTrans(m map[string]interface{}) (tranNames, tranIDs []string) {
	f := func(i string, v interface{}) bool {
		arrI, ok := v.([]interface{})
		if !ok {
			eztools.LogPrint(
				reflect.TypeOf(v).String() +
					" got instead of []interface{}")
			return false
		}
		for _, arr1 := range arrI {
			tran1, ok := arr1.(map[string]interface{})
			if !ok {
				eztools.LogPrint(
					reflect.TypeOf(arr1).
						String() +
						" got instead of " +
						"map[string]interface{}")
				continue
			}
			tranN, ok := tran1["name"].(string)
			if !ok {
				eztools.LogPrint(
					reflect.TypeOf(tran1["name"]).
						String() +
						" got instead of string")
				return false
			}
			tranI, ok := tran1["id"].(string)
			if !ok {
				eztools.LogPrint(
					reflect.TypeOf(tran1["id"]).
						String() +
						" got instead of string")
				return false
			}
			tranNames = append(tranNames, tranN)
			tranIDs = append(tranIDs, tranI)
		}
		return true
	}
	loopStringMap(m, "transitions", "", nil, f)
	return
}

func jiraParseIssues(m map[string]interface{}) []issueInfos {
	/*if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowSthln(strs)
	}*/
	results := make([]issueInfos, 0)
	f := func(i string, v interface{}) bool {
		// https://docs.atlassian.com/software/jira/docs/api/REST/8.12.0/#api/2/search-search
		issues, ok := v.([]interface{})
		if !ok {
			eztools.LogPrint(reflect.TypeOf(v).String() +
				" got instead of " +
				"[]interface{} for " + i)
			return false
		}
		for _, v := range issues {
			//eztools.ShowStrln("Ticket")
			issue, ok := v.(map[string]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of " +
					"map[string]interface{}")
				continue
			}
			var issueInfo issueInfos
			if jiraParse1Issue(issue, &issueInfo) {
				results = append(results, issueInfo)
			}
		}
		return true
	}
	loopStringMap(m, "issues", "", nil, f)
	if len(results) < 1 {
		return nil
	}
	return results
}

func chkNSetIssueInfo(v interface{}, issueInfo *issueInfos, i int) bool {
	str, ok := v.(string)
	if !ok {
		eztools.LogPrint(reflect.TypeOf(v).String() +
			" got instead of string")
		return false
	}
	issueInfo[i] = str
	return true
}

// check map type before looping it
func chkNLoopStringMap(m interface{},
	mustStr, keyStr string, keyVal *string) bool {
	sub, ok := m.(map[string]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(m).String() +
			" got instead of map[string]interface{}")
		return false
	}
	return loopStringMap(sub, mustStr, keyStr, keyVal, nil)
}

/*
loop map.
	If key matches keyStr, put value into keyVal
		in case of string or skip otherwise.
	If key does not match mustStr, skip.
Invoke fun with key and value.
Both return values of fun and this means whether
	any item ever processed successfully.
*/
func loopStringMap(m map[string]interface{},
	mustStr, keyStr string, keyVal *string,
	fun func(string, interface{}) bool) (ret bool) {
	for i, v := range m {
		if len(keyStr) > 0 {
			if i == keyStr {
				id, ok := v.(string)
				if !ok {
					eztools.LogPrint(
						reflect.TypeOf(v).String() +
							" got instead of string")
					continue
				}
				ret = true
				if keyVal != nil {
					*keyVal = id
					if fun == nil {
						break
					}
				}
				continue
			}
		}
		if len(mustStr) > 0 && i != mustStr {
			//eztools.ShowStrln("skipping " + i)
			continue
		}
		if fun != nil {
			ret = fun(i, v) || ret
		}
	}
	return ret
}

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
func gerritQuery1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, opt string) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const REST_API_STR = "changes/?q="
	return gerritGetDetails(svr.URL+REST_API_STR+
		issueInfo[ISSUEINFO_IND_ID]+opt,
		svr.Magic, authInfo)
}

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
// return value: same as gerritGetDetails[0]
func gerritAnyID2ID(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	inf, err := gerritQuery1(svr, authInfo, issueInfo, "")
	if err != nil {
		return issueInfos{}, err
	}
	if len(inf) != 1 {
		return issueInfos{}, eztools.ErrNoValidResults
	}
	return inf[0], nil
}

func gerritRevs(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	f := func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos, res []issueInfos) []issueInfos {
		return append(res, issueInfo)
	}
	return gerritProcRevLoopMyOpen(svr, authInfo,
		issueInfo, f)
}

func gerritRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	const REST_API_STR = "changes/?q="
	// +"&o=CURRENT_REVISION" to list a commit and *ALL* for all
	return gerritGetRevs(svr.URL+REST_API_STR+
		issueInfo[ISSUEINFO_IND_ID]+"&o=ALL_REVISIONS",
		svr.Magic, authInfo)
}

func gerritDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritQuery1(svr, authInfo, issueInfo, "&o=CURRENT_REVISION")
}

// gerritReviews2Scores get all reviews and combine into one set of scores
func gerritReviews2Scores(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (inf []issueInfos, scores scoreInfos, err error) {
	/*id, err := gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		eztools.LogErrPrint(err)
	}*/
	inf, err = gerritReviews(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	for _ /*j*/, inf1 := range inf {
		for _, i := range []int{ISSUEINFO_IND_VERIFIED,
			ISSUEINFO_IND_CODEREVIEW, ISSUEINFO_IND_MANUALTEST} {
			if len(inf1[i]) > 0 {
				if inf1[i] == " 0" {
					// not parsable bo Atoi
					continue
				}
				score1, err := strconv.Atoi(inf1[i])
				if err != nil {
					/*if eztools.Debugging && eztools.Verbose > 0 {
						eztools.ShowStrln(inf1[i] + " is NOT a number!")
					}*/
					continue
				}
				scores[i] += score1
			}
		}
		//inf[j][ISSUEINFO_IND_ID] = id[ISSUEINFO_IND_ID]
	}
	return
}

// no ID will return, since not in replies
func gerritReviews(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const REST_API_STR = "changes/"
	return gerritGetReviews(svr.URL+REST_API_STR+
		issueInfo[ISSUEINFO_IND_ID]+"/reviewers/",
		svr.Magic, authInfo)
}

func gerritSbBraMerged(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_BRANCH]) < 1 ||
		len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const REST_API_STR = "changes/?q="
	return gerritGetIssues(svr.URL+REST_API_STR+
		"status:merged+branch:"+issueInfo[ISSUEINFO_IND_BRANCH]+
		"+owner:"+issueInfo[ISSUEINFO_IND_ID],
		svr.Magic, authInfo)
}

func gerritAllOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	eztools.ShowStrln("This may take quite a while...")
	const REST_API_STR = "changes/"
	return gerritGetIssues(svr.URL+REST_API_STR, svr.Magic, authInfo)
}

func gerritMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	const REST_API_STR = "changes/?q="
	return gerritGetIssues(svr.URL+REST_API_STR+
		/*url.QueryEscape*/ ("status:open+owner:"+authInfo.User),
		svr.Magic, authInfo)
}

func gerritRebase(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/rebase")
}

func gerritMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/submit")
}

func gerritAbandon(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/abandon")
}

func gerritAbandonMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritActOnMyOpen(svr, authInfo, issueInfo, "/abandon")
}

func gerritPick(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritPick1(svr, authInfo, issueInfo, nil)
}

func gerritPick1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, res []issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 ||
		len(issueInfo[ISSUEINFO_IND_BRANCH]) < 1 ||
		len(issueInfo[ISSUEINFO_IND_HEAD]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	if eztools.Debugging {
		if !eztools.ChkCfmNPrompt("continue to cherrypick "+
			issueInfo[ISSUEINFO_IND_HEAD]+
			" from "+issueInfo[ISSUEINFO_IND_ID]+
			" to "+issueInfo[ISSUEINFO_IND_BRANCH], "n") {
			return nil, nil
		}
		eztools.Log("to cheerypick " +
			issueInfo[ISSUEINFO_IND_HEAD] +
			" from " + issueInfo[ISSUEINFO_IND_ID] +
			" to " + issueInfo[ISSUEINFO_IND_BRANCH])
	}
	const REST_API_STR = "changes/"
	jsonValue, _ := json.Marshal(map[string]string{
		//"message": "testing", // if this is a must, I have to read original submit message
		"destination": issueInfo[ISSUEINFO_IND_BRANCH]})
	bodyMap, err := restMap("POST", svr.URL+
		REST_API_STR+issueInfo[ISSUEINFO_IND_ID]+
		"/revisions/"+issueInfo[ISSUEINFO_IND_HEAD]+
		"/cherrypick",
		authInfo, bytes.NewBuffer(jsonValue), svr.Magic)
	if len(bodyMap) < 1 {
		return nil, nil
	}
	return gerritParseIssuesOrReviews(bodyMap, res, issueInfoTxt, nil),
		err
}

// gerritProcRevLoopMyOpen run a func on all my open issues
// with current revision/commit info
func gerritProcRevLoopMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos,
	f func(*svrs, eztools.AuthInfo, issueInfos,
		[]issueInfos) []issueInfos) (res []issueInfos,
	err error) {
	issues, err := gerritMyOpen(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	for _, issueInfo := range issues {
		inf, err := gerritRev(svr, authInfo, issueInfo)
		if err != nil {
			eztools.LogErrPrint(err)
			continue
		}
		if len(inf) != 1 {
			eztools.ShowStrln("NO unique revision info found?")
			if eztools.Debugging {
				eztools.Log("NO unique revision found for " +
					issueInfo[ISSUEINFO_IND_ID] + " (" +
					issueInfo[ISSUEINFO_IND_ID] + ")")
			}
			continue
		}
		res = f(svr, authInfo, inf[0], res)
		// error should have been handled by gerritPick1
	}
	return
}

// gerritPickMyOpen cherry picks all my open submits
func gerritPickMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	branch := issueInfo[ISSUEINFO_IND_BRANCH]
	f := func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos,
		res []issueInfos) []issueInfos {
		issueInfo[ISSUEINFO_IND_BRANCH] = branch
		resO, _ := gerritPick1(svr, authInfo, issueInfo, res)
		return resO
	}
	return gerritProcRevLoopMyOpen(svr, authInfo,
		issueInfo, f)
}

func gerritActOnMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, action string) (res []issueInfos, err error) {
	issues, err := gerritMyOpen(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	for _, issueInfo := range issues {
		res, err = gerritActOn1(svr, authInfo, issueInfo, res, action)
		if err != nil {
			return
		}
	}
	return
}

// gerritActOn1WtAnyID POST changes/ID from input/action
// param: issueInfo[ISSUEINFO_IND_ID] unique ID
func gerritActOn1WtAnyID(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, issues []issueInfos,
	action string) ([]issueInfos, error) {
	issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	return gerritActOn1(svr, authInfo, issueInfo, nil, action)
}

// gerritActOn1 POST changes/ID/action
// param: issueInfo[ISSUEINFO_IND_ID] unique ID
func gerritActOn1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, issues []issueInfos,
	action string) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return issues, eztools.ErrInvalidInput
	}
	if eztools.Debugging {
		if !eztools.ChkCfmNPrompt(action+" "+
			issueInfo[ISSUEINFO_IND_ID], "n") {
			return nil, nil
		}
	}
	const REST_API_STR = "changes/"
	bodyMap, err := restMap("POST", svr.URL+
		REST_API_STR+issueInfo[ISSUEINFO_IND_ID]+action,
		authInfo, nil, svr.Magic)
	return gerritParseIssuesOrReviews(bodyMap, issues, issueInfoTxt, nil),
		err
}

func gerritScore(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	inf, err := gerritRev(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(inf) != 1 {
		eztools.LogPrint("NO single revision info found!")
		return inf, eztools.ErrNoValidResults
	}
	infWtRev := inf[0]
	if len(infWtRev[ISSUEINFO_IND_HEAD]) < 1 {
		eztools.LogPrint("NO revision found!")
		return inf, eztools.ErrNoValidResults
	}

	// check whether Manual-Testing exists
	inf, _, err = gerritReviews2Scores(svr, authInfo, infWtRev)
	if err != nil {
		return inf, err
	}
	if len(inf) < 1 {
		eztools.LogPrint("NO review info found!")
		return inf, eztools.ErrNoValidResults
	}
	type map2Marshal map[string]int
	map4Marshal := map2Marshal{ISSUEINFO_STR_CODEREVIEW: 2, ISSUEINFO_STR_VERIFIED: 1}
	if len(inf[0][ISSUEINFO_IND_MANUALTEST]) > 0 {
		map4Marshal[ISSUEINFO_STR_MANUALTEST] = 1
	}

	const REST_API_STR = "changes/"
	var jsonValue []byte
	jsonValue, err = json.Marshal(map[string]map2Marshal{
		"labels": map4Marshal})
	if err != nil {
		eztools.LogErr(err)
		return nil, err
	}
	if eztools.Debugging {
		if !eztools.ChkCfmNPrompt("continue to +2/1 to "+
			infWtRev[ISSUEINFO_IND_ID], "n") {
			return nil, nil
		}
	}
	eztools.ShowStrln(string(jsonValue))
	_, err = restMap("POST", svr.URL+REST_API_STR+
		infWtRev[ISSUEINFO_IND_ID]+"/revisions/"+
		infWtRev[ISSUEINFO_IND_HEAD]+"/review",
		authInfo, bytes.NewBuffer(jsonValue), svr.Magic)
	// response only contain scores for a success, so it is not parsed
	return nil, err
}

func gerritWaitNMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	var (
		err               error
		inf               []issueInfos
		scores            scoreInfos
		debugVeri, scored bool
		submit_type       string
	)
	if eztools.Debugging && eztools.Verbose > 1 {
		debugVeri = true
	}
	eztools.ShowStr("waiting for issue to be mergable.")
	for err == nil {
		// check submittable
		inf, err = gerritDetail(svr, authInfo, issueInfo)
		if err != nil {
			break
		}
		if len(inf) < 1 {
			err = eztools.ErrNoValidResults
			break
		}
		if inf[0][ISSUEINFO_IND_SUBMITTABLE] == "true" {
			break
		}

		if debugVeri {
			// get submit_type
			inf, err = gerritRev(svr, authInfo, issueInfo)
			if err != nil {
				break
			}
			if len(inf) != 1 {
				err = eztools.ErrNoValidResults
				break
			}
			submit_type = inf[0][ISSUEINFO_IND_SUBMIT_TYPE]
		}

		// get scores
		_ /*inf*/, scores, err = gerritReviews2Scores(svr, authInfo, issueInfo)
		if err != nil {
			break
		}
		/*if len(inf) < 1 {
			err = eztools.ErrNoValidResults
			break
		}*/

		if debugVeri {
			eztools.Log("Verified=" + strconv.Itoa(scores[ISSUEINFO_IND_VERIFIED]))
			// MERGE_IF_NECESSARY/FAST_FORWARD_ONLY
			eztools.Log(ISSUEINFO_STR_SUBMIT_TYPE + "=" +
				submit_type)
			debugVeri = false
		}
		if scores[ISSUEINFO_IND_CODEREVIEW] < 2 ||
			scores[ISSUEINFO_IND_MANUALTEST] < 1 ||
			scores[ISSUEINFO_IND_VERIFIED] < 1 {
			if scored {
				err = errors.New("failed to score")
				break
			}
			_, err = gerritScore(svr, authInfo, inf[0])
			if err != nil {
				break
			}
			scored = true
		}

		time.Sleep(5 * time.Second)
		eztools.ShowStr(".")
	}
	eztools.ShowStrln("")
	if err != nil {
		return nil, err
	}
	return gerritMerge(svr, authInfo, issueInfo)
}

func jiraTransfer(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	for {
		cfmInputOrPrompt(&issueInfo, ISSUEINFO_IND_ID)
		cfmInputOrPromptStr(&issueInfo,
			ISSUEINFO_IND_HEAD, "change to assignee")
		cfmInputOrPromptStr(&issueInfo,
			ISSUEINFO_IND_PROJ, "change to component")
		if len(issueInfo[ISSUEINFO_IND_ID]) < 1 ||
			len(issueInfo[ISSUEINFO_IND_HEAD]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		const REST_API_STR = "rest/api/latest/issue/"
		type insets struct {
			Name string `json:"name"`
		}
		type sets struct {
			Set insets `json:"set"`
		}
		type setss struct {
			Set []insets `json:"set"`
		}
		type updateA struct {
			Update struct {
				Assignee []sets `json:"assignee"`
			} `json:"update"`
		}
		type updateCA struct {
			Update struct {
				Components []setss `json:"components"`
				Assignee   []sets  `json:"assignee"`
			} `json:"update"`
		}

		var (
			jsonStr []byte
			err     error
			s       sets
		)
		if len(issueInfo[ISSUEINFO_IND_PROJ]) > 0 {
			var (
				upCA updateCA
				is   insets
				ss   setss
			)
			is.Name = issueInfo[ISSUEINFO_IND_PROJ]
			ss.Set = append(ss.Set, is)
			upCA.Update.Components = []setss{ss}
			s.Set.Name = issueInfo[ISSUEINFO_IND_HEAD]
			upCA.Update.Assignee = []sets{s}
			jsonStr, err = json.Marshal(upCA)
		} else {
			var upA updateA
			s.Set.Name = issueInfo[ISSUEINFO_IND_HEAD]
			upA.Update.Assignee = []sets{s}
			jsonStr, err = json.Marshal(upA)
		}
		if err != nil {
			return nil, err
		}
		eztools.ShowByteln(jsonStr)
		bodyMap, err := restMap(eztools.METHOD_PUT,
			svr.URL+REST_API_STR+issueInfo[ISSUEINFO_IND_ID],
			authInfo, bytes.NewReader(jsonStr), svr.Magic)
		if err != nil {
			return nil, err
		}
		if postREST != nil {
			postREST([]interface{}{bodyMap})
		}
	}
}

func jiraTran1(svr *svrs, authInfo eztools.AuthInfo,
	id, tranName string) error {
	const REST_API_STR = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+REST_API_STR+
		id+"/transitions", authInfo, nil, svr.Magic)
	if err != nil {
		return err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	var tranID string
	tranNames, tranIDs := jiraParseTrans(bodyMap)
	if len(tranNames) > 0 && len(tranIDs) > 0 {
		if len(tranName) > 0 {
			for i, v := range tranNames {
				if tranName == string(v) {
					tranID = tranIDs[i]
					break
				}
			}
		} else {
			eztools.ShowStrln(
				"There are following transitions available.")
			i := eztools.ChooseStrings(tranNames)
			if i == eztools.InvalidID {
				return eztools.ErrInvalidInput
			}
			tranID = tranIDs[i]
		}
	}
	if len(tranID) < 1 {
		return eztools.ErrNoValidResults
	}

	type tranJsons struct {
		Transition struct {
			Id string `json:"id"`
		} `json:"transition"`
	}
	var tranJson tranJsons
	tranJson.Transition.Id = tranID
	jsonStr, err := json.Marshal(tranJson)
	if err != nil {
		return err
	}
	eztools.ShowByteln(jsonStr)
	bodyMap, err = restMap(eztools.METHOD_POST, svr.URL+REST_API_STR+
		id+"/transitions", authInfo,
		bytes.NewReader(jsonStr), svr.Magic)
	if err != nil {
		return err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return nil
}

func jiraClose(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return jiraCloseWtQA(svr, authInfo, issueInfo, "")
}

func jiraCloseDef(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "default AOSP/vendor/design")
}

func jiraCloseGen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "general requirement")
}

func custFld(jsonStr, fldKey, fldVal string, sth *bool) string {
	if len(fldKey) > 0 {
		if *sth {
			jsonStr += `,
`
		}
		*sth = true
		return jsonStr + `        "` +
			fldKey + `": "` + fldVal + `"`
	}
	return jsonStr
}

func jiraCloseWtQA(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, qa string) ([]issueInfos, error) {
	trans := [...]string{"Implementing", "Assign owner", "Resolved"}
	for {
		cfmInputOrPrompt(&issueInfo, ISSUEINFO_IND_ID)
		if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		if len(qa) > 0 {
			// since all fields are dynamic,
			// construct the json manually
			jsonStr :=
				`{
  "fields": {
`
			sth := false
			jsonStr = custFld(jsonStr, svr.TstPre, "none", &sth)
			jsonStr = custFld(jsonStr, svr.TstStep, qa, &sth)
			jsonStr = custFld(jsonStr, svr.TstExp, "none", &sth)
			if !sth {
				eztools.LogPrint("NO Tst* fields " +
					"defined for this server")
			} else {
				jsonStr = jsonStr + `
  }
}`
				eztools.ShowStrln(jsonStr)
				const REST_API_STR = "rest/api/latest/issue/"
				bodyMap, err := restMap(eztools.METHOD_PUT,
					svr.URL+REST_API_STR+
						issueInfo[ISSUEINFO_IND_ID],
					authInfo, strings.NewReader(jsonStr),
					svr.Magic)
				if err != nil {
					return nil, err
				}
				if postREST != nil {
					postREST([]interface{}{bodyMap})
				}
			}
		}
		for _, tran := range trans {
			if err := jiraTran1(svr, authInfo,
				issueInfo[ISSUEINFO_IND_ID], tran); err != nil &&
				err != eztools.ErrNoValidResults {
				return nil, err
			}
		}
	}
}

func jiraTransition(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	for {
		cfmInputOrPrompt(&issueInfo, ISSUEINFO_IND_ID)
		if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		if err := jiraTran1(svr, authInfo,
			issueInfo[ISSUEINFO_IND_ID], ""); err != nil && err != eztools.ErrNoValidResults {
			return nil, err
		}
	}
}

func jiraDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const REST_API_STR = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+REST_API_STR+
		issueInfo[ISSUEINFO_IND_ID], authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return jiraParseIssues(bodyMap), err
}

func jiraMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	const REST_API_STR = "rest/api/latest/search?jql="
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+REST_API_STR+
		url.QueryEscape("assignee=")+authInfo.User,
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return jiraParseIssues(bodyMap), err
}

func cfmInputOrPromptStr(inf *issueInfos, ind int, prompt string) {
	if len(inf[ind]) < 1 {
		inf[ind] = eztools.PromptStr(prompt)
		return
	}
	s := eztools.PromptStr(prompt + "=" + inf[ind])
	if len(s) < 1 {
		return
	}
	if _, err := strconv.Atoi(s); err != nil {
		inf[ind] = s
		return
	}
	// input a number
	if _, err := strconv.Atoi(inf[ind]); err == nil {
		return
	}
	// it was not a number
	re := regexp.MustCompile(`^[^\d]+`)
	pref := re.FindStringSubmatch(inf[ind])
	if pref != nil {
		// old non-number part + new number
		inf[ind] = pref[0] + s
		if eztools.Debugging {
			eztools.ShowStrln("auto changed to " + inf[ind])
		}
	}
}

func cfmInputOrPrompt(inf *issueInfos, ind int) {
	cfmInputOrPromptStr(inf, ind, issueInfoTxt[ind])
}

func useInputOrPromptStr(inf *issueInfos, ind int, prompt string) {
	if len(inf[ind]) > 0 {
		return
	}
	inf[ind] = eztools.PromptStr(prompt)
}

func useInputOrPrompt(inf *issueInfos, ind int) {
	useInputOrPromptStr(inf, ind, issueInfoTxt[ind])
}

func inputIssueInfo4Act(svrType, action string, inf *issueInfos) {
	switch svrType {
	case CATEGORY_JIRA:
		switch action {
		case "show details of a case":
			useInputOrPrompt(inf, ISSUEINFO_IND_ID)
		}
	case CATEGORY_GERRIT:
		switch action {
		case "show details of a submit",
			"show reviewers of a submit",
			"rebase a submit",
			"abandon a submit",
			"show revision/commit of a submit",
			"add scores to a submit",
			"merge a submit",
			"add socres, wait for it to be mergable and merge a submit":
			useInputOrPrompt(inf, ISSUEINFO_IND_ID)
		case "cherry pick all my open":
			useInputOrPromptStr(inf,
				ISSUEINFO_IND_HEAD, ISSUEINFO_STR_REV_CUR)
			useInputOrPrompt(inf, ISSUEINFO_IND_BRANCH)
		case "list open submits of someone":
			useInputOrPromptStr(inf,
				ISSUEINFO_IND_ID, ISSUEINFO_STR_ASSIGNEE)
			useInputOrPrompt(inf, ISSUEINFO_IND_BRANCH)
		case "cherry pick a submit":
			eztools.ShowStrln("Please input an ID that can make it " +
				"distinguished, such as commit, instead of Change " +
				"ID, which is reused among cherrypicks.")
			useInputOrPromptStr(inf,
				ISSUEINFO_IND_ID, ISSUEINFO_STR_ID)
			useInputOrPromptStr(inf,
				ISSUEINFO_IND_HEAD, ISSUEINFO_STR_REV_CUR)
			useInputOrPrompt(inf, ISSUEINFO_IND_BRANCH)
		}
	default:
		eztools.LogPrint("Server type unknown: " + svrType)
	}
	//eztools.ShowSthln(inf)
}

func makeCat2Act() cat2Act {
	c := cat2Act{
		CATEGORY_JIRA: []action2Func{
			{"transfer a case to someone", jiraTransfer},
			{"move status of a case", jiraTransition},
			{"show details of a case", jiraDetail},
			{"list my open cases", jiraMyOpen},
			{"close a case to resolved from any known statues", jiraClose},
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			{"close a case with default design as steps", jiraCloseDef},
			{"close a case with general requirement as steps", jiraCloseGen},
		},
		CATEGORY_GERRIT: []action2Func{
			{"list open submits of someone", gerritSbBraMerged},
			{"list my open submits", gerritMyOpen},
			{"list all my open revisions/commits", gerritRevs},
			{"list all open submits", gerritAllOpen},
			{"show details of a submit", gerritDetail},
			{"show reviewers of a submit", gerritReviews},
			{"show revision/commit of a submit", gerritRev},
			{"rebase a submit", gerritRebase},
			{"merge a submit", gerritMerge},
			{"add scores to a submit", gerritScore},
			{"add socres, wait for it to be mergable and merge a submit", gerritWaitNMerge},
			{"abandon all my open submits", gerritAbandonMyOpen},
			{"abandon a submit", gerritAbandon},
			{"cherry pick all my open submits", gerritPickMyOpen},
			{"cherry pick a submit", gerritPick},
		}}
	return c
}
