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
	"path/filepath"
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

func read1Cfg(fn string, cfg *cfgs) (err error) {
	if _, err = os.Stat(fn); os.IsNotExist(err) {
		return err
	}
	err = eztools.XMLRead(fn, cfg)
	return
}

func readCfg(fn string, cfg *cfgs) (err error) {
	if len(fn) > 0 {
		err = read1Cfg(fn, cfg)
		if err == nil {
			return
		}
	}
	home, _ := os.UserHomeDir()
	cfgPaths := [...]string{".", home}
	for _, path1 := range cfgPaths {
		err = read1Cfg(filepath.Join(path1, module+".xml"), cfg)
		if err == nil {
			break
		}
	}
	return
}

func main() {
	var (
		paramH, paramV, paramVV, paramVVV bool
		paramID, paramBra, paramCfg, paramLog,
		paramHD, paramP string
	)
	flag.BoolVar(&paramH, "h", false, "Help Message")
	flag.BoolVar(&paramV, "v", false,
		"Log file output. Most actions need this.")
	flag.BoolVar(&paramVV, "vv", false, "verbose messages")
	flag.BoolVar(&paramVVV, "vvv", false,
		"verbose messages with network I/O")
	flag.StringVar(&paramID, "i", "", "Issue ID or assignee")
	flag.StringVar(&paramBra, "b", "", "Branch")
	flag.StringVar(&paramHD, "hd", "",
		"new assignee when transferring issues")
	flag.StringVar(&paramP, "p", "",
		"new component when transferring issues")
	flag.StringVar(&paramCfg, "c", "", "Config File")
	flag.StringVar(&paramLog, "l", "", "Log File")
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
	err := readCfg(paramCfg, &cfg)
	if err != nil {
		eztools.LogErrFatal(err)
		return
	}
	if len(paramLog) > 0 {
		cfg.Log = paramLog
	} else if len(cfg.Log) < 1 {
		cfg.Log = module + ".log"
	}
	logger, err := os.OpenFile(cfg.Log,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err == nil {

		if err = eztools.InitLogger(logger); err != nil {
			eztools.LogErrPrint(err)
		}
	} else {
		eztools.LogPrint("Failed to open log file " + cfg.Log)
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
		if eztools.Debugging && eztools.Verbose > 0 {
			if issues == nil {
				eztools.Log("No results.")
			} else {
				for i, issue := range issues {
					eztools.Log("Issue/Reviewer " +
						strconv.Itoa(i+1))
					eztools.Log("ID/reviewer/submittable=" +
						issue[ISSUEINFO_IND_ID])
					eztools.Log("HEAD/verified=" +
						issue[ISSUEINFO_IND_HEAD])
					eztools.Log("PROJ/code-review=" +
						issue[ISSUEINFO_IND_PROJ])
					eztools.Log("BRANCH/manual-testing/owner=" +
						issue[ISSUEINFO_IND_BRANCH])
					eztools.Log("(approval) State=" +
						issue[ISSUEINFO_IND_APPROVAL])
				}
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
	inputIssueInfo4Act(cats[svr.Type][fi].n, &issueInfo)
	return &candidates[si], cats[svr.Type][fi].f, issueInfo

}

func restSth(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (body interface{}, err error) {
	body, errno, err := eztools.RestGetOrPostWtMagic(method,
		url, authInfo, bodyReq, []byte(magic))
	if err != nil {
		eztools.LogErrPrintWtInfo(strconv.Itoa(errno), err)
		if eztools.Debugging && eztools.Verbose > 2 &&
			body != nil {
			eztools.ShowSthln(body)
		}
		return
	}
	return
}

func restSlc(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (bodySlc []interface{}, err error) {
	body, err := restSth(method, url, authInfo, bodyReq, magic)
	if body == nil {
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
	if body == nil {
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
	ISSUEINFO_IND_ID          = iota     // \
	ISSUEINFO_IND_SUBMITTABLE = iota - 1 //
	ISSUEINFO_IND_KEY         = iota - 2 // /
	ISSUEINFO_IND_HEAD                   // \
	ISSUEINFO_IND_SUMMARY     = iota - 3 // /
	ISSUEINFO_IND_PROJ
	ISSUEINFO_IND_BRANCH              // \
	ISSUEINFO_IND_DISPNAME = iota - 4 // /
	ISSUEINFO_IND_APPROVAL            // \
	ISSUEINFO_IND_STATE    = iota - 5 // /
	ISSUEINFO_IND_MAX
	ISSUEINFO_STR_ID          = "change_id"      // \
	ISSUEINFO_STR_SUBMITTABLE = "submittable"    //
	ISSUEINFO_STR_ASSIGNEE    = "assignee"       //
	ISSUEINFO_STR_KEY         = "key"            //
	ISSUEINFO_STR_NAME        = "name"           // /
	ISSUEINFO_STR_HEAD        = "subject"        // \
	ISSUEINFO_STR_SUMMARY     = "summary"        //
	ISSUEINFO_STR_VERIFIED    = "Verified"       // /
	ISSUEINFO_STR_PROJ        = "project"        // \
	ISSUEINFO_STR_CODEREVIEW  = "Code-Review"    // /
	ISSUEINFO_STR_BRANCH      = "branch"         // \
	ISSUEINFO_STR_DISPNAME    = "displayName"    //
	ISSUEINFO_STR_MANUALTEST  = "Manual-Testing" // /
	ISSUEINFO_STR_APPROVAL    = "approvals"      // \ for code-review, verified and manual-testing
	ISSUEINFO_STR_STATE       = "status"         // /
)

type issueInfos [ISSUEINFO_IND_MAX]string

var issueInfoTxt = [ISSUEINFO_IND_MAX]string{
	ISSUEINFO_STR_ID, ISSUEINFO_STR_HEAD, ISSUEINFO_STR_PROJ,
	ISSUEINFO_STR_BRANCH, ISSUEINFO_STR_STATE}
var issueDetailsTxt = [ISSUEINFO_IND_MAX]string{
	ISSUEINFO_STR_SUBMITTABLE, ISSUEINFO_STR_HEAD, ISSUEINFO_STR_PROJ,
	ISSUEINFO_STR_BRANCH, ISSUEINFO_STR_STATE}

// do we also need mergable and submit_type=MERGE_IF_NECESSARY/Fast Forward Only?
var reviewInfoTxt = [ISSUEINFO_IND_MAX]string{
	ISSUEINFO_STR_NAME, ISSUEINFO_STR_VERIFIED, ISSUEINFO_STR_CODEREVIEW,
	ISSUEINFO_STR_MANUALTEST, ISSUEINFO_STR_APPROVAL}

//var jiraInfoTxt = [ISSUEINFO_IND_MAX]string{ISSUEINFO_STR_KEY, ISSUEINFO_STR_SUMMARY, ISSUEINFO_STR_PROJ, ISSUEINFO_STR_DISPNAME, ISSUEINFO_STR_STATE}

func gerritParseIssuesOrReviews(m map[string]interface{},
	issues []issueInfos, strs [ISSUEINFO_IND_MAX]string,
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
		if len(strs[i]) < 1 || m[strs[i]] == nil {
			continue
		}
		str, ok := m[strs[i]].(string)
		if !ok {
			if i == ISSUEINFO_IND_SUBMITTABLE &&
				strs[i] == ISSUEINFO_STR_SUBMITTABLE {
				bo, ok := m[strs[i]].(bool)
				if !ok {
					if eztools.Debugging {
						eztools.LogPrint(reflect.TypeOf(m[strs[i]]).String() +
							" got instead of bool for " + strs[i] + "!")
					}
				} else {
					switch bo {
					case true:
						issue[i] = "true"
					case false:
						issue[i] = "false"
					}
				}
				continue
			}
			if eztools.Debugging {
				if i != ISSUEINFO_IND_APPROVAL &&
					strs[i] != ISSUEINFO_STR_APPROVAL {
					eztools.Log(strs[i] + " matched without string value!")
				}
			}
			mp, ok := m[strs[i]].(map[string]interface{})
			if !ok {
				if eztools.Debugging {
					eztools.LogPrint(reflect.TypeOf(m[strs[i]]).String() +
						" got instead of map string to interface!")
				}
				continue
			}
			gerritParseIssuesOrReviews(mp, issues, strs, issue)
			continue
		}
		if eztools.Debugging && eztools.Verbose > 2 {
			eztools.ShowStrln("matching " + strs[i] + " <> " + str)
		}
		issue[i] = str
	}
	if issues != nil {
		return append(issues, *issue)
	}
	return []issueInfos{*issue}
}

func gerritParseReviews(m map[string]interface{}, issues []issueInfos) []issueInfos {
	return gerritParseIssuesOrReviews(m, issues, reviewInfoTxt, nil)
}

func gerritParseDetails(m map[string]interface{}, issues []issueInfos) []issueInfos {
	return gerritParseIssuesOrReviews(m, issues, issueDetailsTxt, nil)
}

func gerritParseIssues(m map[string]interface{}, issues []issueInfos) []issueInfos {
	return gerritParseIssuesOrReviews(m, issues, issueInfoTxt, nil)
}

func gerritGetIssuesOrReviews(method, url, magic string, authInfo eztools.AuthInfo,
	fun func(map[string]interface{}, []issueInfos) []issueInfos) ([]issueInfos, error) {
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

func gerritGetReviews(url, magic string, authInfo eztools.AuthInfo) (
	[]issueInfos, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo, gerritParseReviews)
}

func gerritGetDetails(url, magic string, authInfo eztools.AuthInfo) (
	[]issueInfos, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo, gerritParseDetails)
}

func gerritGetIssues(url, magic string, authInfo eztools.AuthInfo) (
	[]issueInfos, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo, gerritParseIssues)
}

func jiraParse1Field(m map[string]interface{}, issueInfo *issueInfos) (changed bool) {
	for i, v := range m {
		// description,
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
				ISSUEINFO_IND_SUMMARY) || changed
		}
	}
	return
}

func jiraParse1Issue(m map[string]interface{}, issueInfo *issueInfos) (changed bool) {
	changed = loopStringMap(m, "fields",
		ISSUEINFO_STR_KEY, &issueInfo[ISSUEINFO_IND_KEY],
		func(i string, v interface{}) bool {
			// id, self ignored
			fields, ok := v.(map[string]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of map[string]interface{}")
				return false
			}
			return jiraParse1Field(fields, issueInfo)
		}) || changed
	return
}

func jiraParseTrans(m map[string]interface{}) (tranNames, tranIDs []string) {
	loopStringMap(m, "transitions", "", nil,
		func(i string, v interface{}) bool {
			arrI, ok := v.([]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of []interface{}")
				return false
			}
			for _, arr1 := range arrI {
				tran1, ok := arr1.(map[string]interface{})
				if !ok {
					eztools.LogPrint(reflect.TypeOf(arr1).String() +
						" got instead of map[string]interface{}")
					continue
				}
				tranN, ok := tran1["name"].(string)
				if !ok {
					eztools.LogPrint(reflect.TypeOf(tran1["name"]).String() +
						" got instead of string")
					return false
				}
				tranI, ok := tran1["id"].(string)
				if !ok {
					eztools.LogPrint(reflect.TypeOf(tran1["id"]).String() +
						" got instead of string")
					return false
				}
				tranNames = append(tranNames, tranN)
				tranIDs = append(tranIDs, tranI)
			}
			return true
		})
	return
}

func jiraParseIssues(m map[string]interface{}) []issueInfos {
	/*if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowSthln(strs)
	}*/
	results := make([]issueInfos, 0)
	loopStringMap(m, "issues", "", nil,
		func(i string, v interface{}) bool {
			// https://docs.atlassian.com/software/jira/docs/api/REST/8.12.0/#api/2/search-search
			issues, ok := v.([]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of []interface{} for " + i)
				return false
			}
			for _, v := range issues {
				//eztools.ShowStrln("Ticket")
				issue, ok := v.(map[string]interface{})
				if !ok {
					eztools.LogPrint(reflect.TypeOf(v).String() +
						" got instead of map[string]interface{}")
					continue
				}
				var issueInfo issueInfos
				if jiraParse1Issue(issue, &issueInfo) {
					results = append(results, issueInfo)
				}
			}
			return true
		})
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
If key matches keyStr, put value into keyVal in case of string or skip otherwise.
If key does not match mustStr, skip.
Invoke fun with key and value.
Both return values of fun and this means whether any item ever processed successfully.
*/
func loopStringMap(m map[string]interface{},
	mustStr, keyStr string, keyVal *string,
	fun func(string, interface{}) bool) (ret bool) {
	for i, v := range m {
		if len(keyStr) > 0 {
			if i == keyStr {
				id, ok := v.(string)
				if !ok {
					eztools.LogPrint(reflect.TypeOf(v).String() +
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

func gerritDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	// change ID or commit/revision
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const REST_API_STR = "changes/?q=" // +"&o=CURRENT_REVISION" to list a commit and *ALL* for all
	return gerritGetDetails(svr.URL+REST_API_STR+issueInfo[ISSUEINFO_IND_ID],
		svr.Magic, authInfo)
}

func gerritReviews(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const REST_API_STR = "changes/"
	return gerritGetReviews(svr.URL+REST_API_STR+
		issueInfo[ISSUEINFO_IND_ID]+"/reviewers/", svr.Magic, authInfo)
}

func gerritSbBraMerged(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	const REST_API_STR = "changes/?q="
	return gerritGetIssues(svr.URL+REST_API_STR+
		"status:merged+branch:"+issueInfo[ISSUEINFO_IND_BRANCH]+
		"+owner:"+issueInfo[ISSUEINFO_IND_ID],
		svr.Magic, authInfo)
}

func gerritAllOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
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

func gerritMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const REST_API_STR = "changes/"
	_, err := restSth("POST", svr.URL+
		REST_API_STR+issueInfo[ISSUEINFO_IND_ID],
		authInfo, nil, svr.Magic)
	return nil, err
}

func gerritWaitNMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	var (
		err error
		inf []issueInfos
	)
	eztools.ShowStr("waiting for issue to be mergable.")
	for err == nil {
		inf, err = gerritDetail(svr, authInfo, issueInfo)
		if err != nil {
			break
		}
		if len(inf) != 1 {
			eztools.ShowStrln("")
			eztools.ShowStr("NO unique submit found!")
			err = eztools.ErrInvalidInput
			break
		}
		if inf[0][ISSUEINFO_IND_SUBMITTABLE] == "true" {
			break
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
			eztools.ShowStrln("There are following transitions available.")
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
			// since all fields are dynamic, construct the json manually
			jsonStr :=
				`{
  "fields": {
`
			sth := false
			jsonStr = custFld(jsonStr, svr.TstPre, "none", &sth)
			jsonStr = custFld(jsonStr, svr.TstStep, qa, &sth)
			jsonStr = custFld(jsonStr, svr.TstExp, "none", &sth)
			if !sth {
				eztools.LogPrint("NO Tst* fields defined for this server")
			} else {
				jsonStr = jsonStr + `
  }
}`
				eztools.ShowStrln(jsonStr)
				const REST_API_STR = "rest/api/latest/issue/"
				bodyMap, err := restMap(eztools.METHOD_PUT,
					svr.URL+REST_API_STR+issueInfo[ISSUEINFO_IND_ID],
					authInfo, strings.NewReader(jsonStr), svr.Magic)
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
				issueInfo[ISSUEINFO_IND_ID], tran); err != nil && err != eztools.ErrNoValidResults {
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
	if len(inf[ind]) > 0 {
		s := eztools.PromptStr(prompt + "=" + inf[ind])
		if len(s) > 0 {
			if _, err := strconv.Atoi(s); err == nil {
				// input a number
				if _, err := strconv.Atoi(inf[ind]); err != nil {
					// it was not a number
					re := regexp.MustCompile(`^[^\d]+`)
					pref := re.FindStringSubmatch(inf[ind])
					if pref != nil {
						// old non-number part + new number
						inf[ind] = pref[0] + s
						if eztools.Debugging {
							eztools.ShowStrln("auto changed to " +
								inf[ind])
						}
					}
				}
			} else {
				inf[ind] = s
			}
		}
		return
	} else {
		inf[ind] = eztools.PromptStr(prompt)
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

func inputIssueInfo4Act(action string, inf *issueInfos) {
	switch action {
	case "detail Gerrit",
		"reviewers Gerrit",
		"merge Gerrit",
		"detail JIRA",
		"wait and merge Gerrit":
		useInputOrPrompt(inf, ISSUEINFO_IND_ID)
	case "sb.'s all Gerrit":
		useInputOrPromptStr(inf,
			ISSUEINFO_IND_ID, ISSUEINFO_STR_ASSIGNEE)
		useInputOrPrompt(inf, ISSUEINFO_IND_BRANCH)
	}
	//eztools.ShowSthln(inf)
}

func makeCat2Act() cat2Act {
	c := cat2Act{
		CATEGORY_JIRA: []action2Func{
			{"transfer JIRA", jiraTransfer},
			{"transition JIRA", jiraTransition},
			{"detail JIRA", jiraDetail},
			{"my open JIRA", jiraMyOpen},
			{"close JIRA", jiraClose},
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			{"close default design JIRA", jiraCloseDef},
			{"close general requirement JIRA", jiraCloseGen},
		},
		CATEGORY_GERRIT: []action2Func{
			{"sb.'s all Gerrit", gerritSbBraMerged},
			{"my open Gerrit", gerritMyOpen},
			{"all open Gerrit", gerritAllOpen},
			{"detail Gerrit", gerritDetail},
			{"reviewers Gerrit", gerritReviews},
			{"merge Gerrit", gerritMerge},
			{"wait and merge Gerrit", gerritWaitNMerge},
		}}
	return c
}
