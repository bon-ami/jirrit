package main

import (
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

	"github.com/bon-ami/eztools"
	_ "github.com/go-sql-driver/mysql"
)

var ver string

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

/*type linktypes struct {
	LinkType xml.Name `xml:"linktype"`
	Value    string   `xml:"value,attr"`
	String   string   `xml:",chardata"`
}*/
type fields struct {
	Fld xml.Name `xml:"fields"`
	//Desc    string   `xml:"desc"`
	//LinkType []linktypes `xml:"linktype"`
	TstPre  string `xml:"testpre"`
	TstStep string `xml:"teststep"`
	TstExp  string `xml:"testexp"`
}
type svrs struct {
	Svr   xml.Name  `xml:"server"`
	Type  string    `xml:"type,attr"`
	Name  string    `xml:"name,attr"`
	URL   string    `xml:"url"`
	Pass  passwords `xml:"pass"`
	Magic string    `xml:"magic"`
	Score string    `xml:"score"`
	Flds  fields    `xml:"fields"`
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
		paramHD, paramP, paramS string
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
		"new component when transferring issues, "+
			"or test step comment for JIRA closure")
	flag.StringVar(&paramS, "s", "",
		"linked issue when linking issues")
	flag.StringVar(&paramCfg, "c", "", "config file")
	flag.StringVar(&paramLog, "l", "", "log file")
	flag.Parse()
	if paramH {
		eztools.ShowStrln(module + " v" + ver)
		eztools.ShowStrln("  When inputting ID's, there are following options for some actions.")
		eztools.ShowStrln(" 1. single ID, such as 0 or X-0")
		eztools.ShowStrln(" 2. multiple IDs, such as 0,0,0 or X-0,2,1")
		eztools.ShowStrln(" 3. ID range, such as 0,,2 or X-0,2")
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
	//if eztools.Debugging {
	logger, err := os.OpenFile(cfg.Log,
		os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err == nil {
		if err = eztools.InitLogger(logger); err != nil {
			eztools.LogErrPrint(err)
		}
	} else {
		eztools.LogPrint("Failed to open log file " + cfg.Log)
	}
	//}

	// self upgrade
	db, err := eztools.Connect()
	if err != nil {
		eztools.LogErrFatal(err)
	} else {
		defer db.Close()
	}
	upch := make(chan bool)
	go eztools.AppUpgrade(db, module, ver, nil, upch)

	svr := chooseSvr(cats, cfg.Svrs)
	if svr != nil {
		choices := makeActs2Choose(*svr, cats[svr.Type])
		for {
			fun, issueInfo := chooseAct(svr.Type, choices, cats[svr.Type],
				issueInfos{
					ISSUEINFO_IND_ID:   paramID,
					ISSUEINFO_IND_HEAD: paramHD,
					ISSUEINFO_IND_PROJ: paramP,
					//ISSUEINFO_IND_COMMENT: paramP,
					ISSUEINFO_IND_BRANCH: paramBra,
					ISSUEINFO_IND_STATE:  paramS})
			if fun == nil {
				break
			}
			authInfo, err := cfg2AuthInfo(*svr, cfg)
			if err != nil {
				eztools.LogErrFatal(err)
				break
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
						ISSUEINFO_STR_DISPNAME + /*"/" +
						ISSUEINFO_STR_MANUALTEST + */"=" +
						issue[ISSUEINFO_IND_BRANCH])
					op(ISSUEINFO_STR_STATE + "/" +
						ISSUEINFO_STR_SUBMIT_TYPE + "=" +
						issue[ISSUEINFO_IND_STATE])
				}
			}
		}
	}

	eztools.ShowStrln("waiting for update check...")
	if serverGot := <-upch; serverGot {
		eztools.ShowStrln("waiting for update check to end...")
		<-upch
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

func chooseSvr(cats cat2Act, candidates []svrs) *svrs {
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
		return nil
	}

	return &candidates[si]
}

func makeActs2Choose(svr svrs, funcs []action2Func) []string {
	if svr.Type == CATEGORY_JIRA {
		if len(svr.Flds.TstExp+svr.Flds.TstPre+svr.Flds.TstStep) < 0 {
			funcs = funcs[:len(funcs)-2]
		}
	}
	choices := make([]string, len(funcs))
	for i, choice := range funcs {
		choices[i] = choice.n
	}
	return choices
}

func chooseAct(svrType string, choices []string, funcs []action2Func,
	issueInfo issueInfos) (actionFunc, issueInfos) {
	eztools.ShowStrln(" Choose an action")
	fi := eztools.ChooseStrings(choices)
	if fi == eztools.InvalidID {
		return nil, issueInfo
	}
	inputIssueInfo4Act(svrType, funcs[fi].n, &issueInfo)
	return funcs[fi].f, issueInfo
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

func showRspBody(err error, body interface{}) {
	if err != nil {
		if eztools.Debugging && eztools.Verbose > 1 {
			eztools.ShowStrln("failure with body:")
			eztools.ShowSthln(body)
		}
	}
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
	//showRspBody(err, body)
	return
}

func restMapOrSth(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (body interface{},
	bodyMap map[string]interface{}, err error) {
	body, err = restSth(method, url, authInfo, bodyReq, magic)
	if err != nil || body == nil {
		return
	}
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		eztools.LogPrint("REST response type error for map " +
			reflect.TypeOf(body).String())
	} else {
		showRspBody(err, body)
	}
	return
}

func restMap(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (
	bodyMap map[string]interface{}, err error) {
	_, bodyMap, err = restMapOrSth(method, url, authInfo, bodyReq, magic)
	return
}

const (
	// common use
	ISSUEINFO_IND_ID = iota
	ISSUEINFO_IND_KEY
	ISSUEINFO_IND_HEAD
	ISSUEINFO_IND_PROJ
	ISSUEINFO_IND_BRANCH
	ISSUEINFO_IND_STATE
	ISSUEINFO_IND_MAX

	// gerrit state
	// placeholder for ID
	ISSUEINFO_IND_SUBMITTABLE = iota - ISSUEINFO_IND_MAX
	ISSUEINFO_IND_VERIFIED
	ISSUEINFO_IND_CODEREVIEW
	ISSUEINFO_IND_SCORE
	ISSUEINFO_IND_SUBMIT_TYPE

	// jira details
	// placeholder for ID
	ISSUEINFO_IND_DESC = iota + 1 - ISSUEINFO_IND_MAX*2
	// no id for summary, jira
	ISSUEINFO_IND_COMMENT = iota + 2 - ISSUEINFO_IND_MAX*2
	ISSUEINFO_IND_DISPNAME

	ISSUEINFO_STR_ID          = "id"
	ISSUEINFO_STR_SUBMITTABLE = "submittable"      // \
	ISSUEINFO_STR_KEY         = "key"              //
	ISSUEINFO_STR_ASSIGNEE    = "assignee"         //
	ISSUEINFO_STR_NAME        = "name"             // /
	ISSUEINFO_STR_HEAD        = "subject"          // \
	ISSUEINFO_STR_SUMMARY     = "summary"          //
	ISSUEINFO_STR_DESC        = "description"      //
	ISSUEINFO_STR_REV_CUR     = "current_revision" //
	ISSUEINFO_STR_VERIFIED    = "Verified"         // /
	ISSUEINFO_STR_PROJ        = "project"          // \
	ISSUEINFO_STR_CODEREVIEW  = "Code-Review"      // /
	ISSUEINFO_STR_BRANCH      = "branch"           // \
	ISSUEINFO_STR_DISPNAME    = "displayName"      // /
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
	ISSUEINFO_STR_CODEREVIEW, ISSUEINFO_STR_DISPNAME, /*placeholder for SCORE*/
	ISSUEINFO_STR_APPROVALS}

/*var jiraInfoTxt = issueInfos{ISSUEINFO_STR_ID, ISSUEINFO_STR_KEY,
ISSUEINFO_STR_SUMMARY, ISSUEINFO_STR_PROJ, ISSUEINFO_STR_DISPNAME,
ISSUEINFO_STR_STATE}*/
/*var jiraDetailTxt = issueInfos{
ISSUEINFO_STR_ID, ISSUEINFO_STR_DESC, ISSUEINFO_STR_SUMMARY,
ISSUEINFO_STR_COMMENT, ISSUEINFO_STR_DISPNAME, ISSUEINFO_STR_STATE}*/

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

// return values
//	whether input is in exact x-0 format
//	the non digit part
//	the digit part
func parseTypicalJiraNum(num string) (bool, string, string) {
	re := regexp.MustCompile(`^[^\d]+[-][\d]+$`)
	pref := re.FindStringSubmatch(num)
	if pref != nil {
		parts := strings.Split(pref[0], typicalJiraSeparator)
		if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 0 {
			return true, parts[0] + typicalJiraSeparator, parts[1]
		}
	}
	return false, "", ""
}

// loopIssues runs a function on all numbers between, inclusively,
// X-0 and X-1, or 0,1 from input in format of X-0,1 or 0,1
// If it is not a range, the function's return values are returned.
// Otherwise, no return values.
func loopIssues(issueInfo issueInfos, fun func(issueInfos) (
	[]issueInfos, error)) ([]issueInfos, error) {
	const separator = ","
	switch strings.Count(issueInfo[ISSUEINFO_IND_ID], separator) {
	case 0:
		return fun(issueInfo)
	case 2:
		parts := strings.Split(issueInfo[ISSUEINFO_IND_ID], separator)
		if len(parts) != 2 || len(parts[0]) < 1 || len(parts[2]) < 1 {
			eztools.LogPrint("range format needs both parts aside with two \"" +
				separator + "\"" + " or multiple parts, deliminated by \"" +
				separator + "\"")
			break
		}
		if len(parts[1]) < 0 {
			var (
				prefix, lowerBoundStr  string
				lowerBound, upperBound int
				err                    error
			)
			lowerBound, err = strconv.Atoi(parts[0])
			if err != nil {
				var ok bool
				if ok, prefix, lowerBoundStr = parseTypicalJiraNum(parts[0]); !ok {
					eztools.LogPrint("the former part must be in the form of X-0")
					break
				}
				lowerBound, err = strconv.Atoi(lowerBoundStr)
				if err != nil {
					eztools.LogPrint(lowerBoundStr + " is NOT a number!")
					break
				}
			}
			upperBound, err = strconv.Atoi(parts[1])
			if err != nil {
				eztools.LogPrint("the latter part must be a number")
				break
			}
			if lowerBound >= upperBound {
				eztools.LogPrint("the number in the latter part must be greater than the one in the former part")
				break
			}
			for i := lowerBound; i <= upperBound; i++ {
				issueInfo[ISSUEINFO_IND_ID] = prefix + strconv.Itoa(i)
				_, err := fun(issueInfo)
				if err != nil {
					return nil, err
				}
			}
			return nil, nil
		}
	}
	parts := strings.Split(issueInfo[ISSUEINFO_IND_ID], separator)
	var (
		prefix    string
		currentNo string
		ok        bool
	)
	if ok, prefix, currentNo = parseTypicalJiraNum(parts[0]); !ok {
		currentNo = parts[0]
	}
	i := 1
	for {
		issueInfo[ISSUEINFO_IND_ID] = prefix + currentNo
		_, err := fun(issueInfo)
		if err != nil {
			return nil, err
		}
		if i < len(parts) {
			currentNo = parts[i]
			i++
		} else {
			break
		}
	}
	return nil, nil
}

func cfmInputOrPromptStrMultiLines(inf *issueInfos, ind int, prompt string) {
	const linefeed = " (end with \\ to continue with more lines. empty to stop)"
	s := eztools.PromptStr(prompt + linefeed)
	if len(s) < 1 {
		return
	}
	if s[len(s)-1] == '\\' {
		inf[ind] += s[:len(s)-1] + "\n"
		cfmInputOrPromptStrMultiLines(inf, ind, prompt)
		return
	}
	inf[ind] += s
}

// return value: whether anything new is input
func cfmInputOrPromptStr(inf *issueInfos, ind int, prompt string) bool {
	const linefeed = " (end with \\ to input multi lines)"
	var def, base string
	var smart bool // no smart affix available by default
	if len(inf[ind]) > 0 {
		var ok bool
		if ok, base, _ = parseTypicalJiraNum(inf[ind]); ok {
			smart = true // there is a reference for smart affix
			//eztools.ShowStrln("not int previously")
		}
		def = "=" + inf[ind]
	}
	s := eztools.PromptStr(prompt + linefeed + def)
	if len(s) < 1 || s == inf[ind] {
		return false
	}
	if smart {
		if _, err := strconv.Atoi(s); err != nil {
			smart = false
			//eztools.ShowStrln("not int currently")
			// we do not care what this number is
		}
	}
	if !smart {
		// input not a number or no previous input to refer to
		if s[len(s)-1] == '\\' {
			inf[ind] = s[:len(s)-1] + "\n"
			cfmInputOrPromptStrMultiLines(inf, ind, prompt)
			return true
		}
		inf[ind] = s
		return true
	}
	//eztools.ShowStrln("smart changing")
	// smart affix
	inf[ind] = base + s
	//if eztools.Debugging {
	eztools.ShowStrln("auto changed to " + inf[ind])
	//}
	return true
}

func cfmInputOrPrompt(inf *issueInfos, ind int) bool {
	return cfmInputOrPromptStr(inf, ind, issueInfoTxt[ind])
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
		case "show details of a case",
			"list comments of a case":
			useInputOrPrompt(inf, ISSUEINFO_IND_ID)
		case "close a case to resolved from any known statues":
			useInputOrPromptStr(inf, ISSUEINFO_IND_COMMENT,
				"test step for closure")
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
		case "list merged submits of someone",
			"add socres, wait for it to be mergable and merge sb.'s submits",
			"list sb.'s open submits":
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
			{"list comments of a case", jiraComments},
			{"add a comment to a case", jiraAddComment},
			{"list my open cases", jiraMyOpen},
			{"link a case to the other", jiraLink},
			{"close a case to resolved from any known statues", jiraClose},
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			{"close a case with default design as steps", jiraCloseDef},
			{"close a case with general requirement as steps", jiraCloseGen},
		},
		CATEGORY_GERRIT: []action2Func{
			{"list merged submits of someone", gerritSbBraMerged},
			{"list my open submits", gerritMyOpen},
			{"list sb.'s open submits", gerritSbOpen},
			{"list all my open revisions/commits", gerritRevs},
			{"list all open submits", gerritAllOpen},
			{"show details of a submit", gerritDetail},
			{"show reviewers of a submit", gerritReviews},
			{"show revision/commit of a submit", gerritRev},
			{"rebase a submit", gerritRebase},
			{"merge a submit", gerritMerge},
			{"add scores to a submit", gerritScore},
			{"add socres, wait for it to be mergable and merge a submit", gerritWaitNMerge},
			{"add socres, wait for it to be mergable and merge sb.'s submits", gerritWaitNMergeSb},
			{"abandon all my open submits", gerritAbandonMyOpen},
			{"abandon a submit", gerritAbandon},
			{"cherry pick all my open submits", gerritPickMyOpen},
			{"cherry pick a submit", gerritPick},
		}}
	return c
}
