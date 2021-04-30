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

var (
	ver, cfgFile string
	cfg          cfgs
)

const (
	module = "jirrit"
	// CategoryJira server type in xml
	CategoryJira = "JIRA"
	// CategoryGerrit server type in xml
	CategoryGerrit = "Gerrit"
	// PassBasic password type in xml
	PassBasic = "basic"
	// PassPlain password type in xml
	PassPlain = "plain"
	// PassDigest password type in xml
	PassDigest = "digest"
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
	RejectRsn string `xml:"rejectrsn"`
	TstPre    string `xml:"testpre"`
	TstStep   string `xml:"teststep"`
	TstExp    string `xml:"testexp"`
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
	Proj  string    `xml:"project"`
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
		paramHD, paramP, paramS, paramC string
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
		"project for JIRA issues")
	flag.StringVar(&paramC, "c", "",
		"new component when transferring issues, "+
			"or test step comment for JIRA closure")
	flag.StringVar(&paramS, "s", "",
		"linked issue when linking issues")
	flag.StringVar(&paramCfg, "cfg", "", "config file")
	flag.StringVar(&paramLog, "log", "", "log file")
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
	var err error
	cfgFile, err = eztools.XMLsReadDefaultNoCreate(paramCfg, module, &cfg)
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
	upch := make(chan bool)
	go chkUpdate(upch)

	for {
		svr := chooseSvr(cats, cfg.Svrs)
		if svr == nil {
			break
		}
		eztools.ShowSthln(svr)
		choices := makeActs2Choose(*svr, cats[svr.Type])
		for {
			fun, issueInfo := chooseAct(svr.Type, choices, cats[svr.Type],
				issueInfos{
					IssueinfoIndID:      paramID,
					IssueinfoIndHead:    paramHD,
					IssueinfoIndProj:    paramP,
					IssueinfoIndBranch:  paramBra,
					IssueinfoIndState:   paramS,
					IssueinfoIndComment: paramC})
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
					op(IssueinfoStrID + "=" +
						issue[IssueinfoIndID])
					op(IssueinfoStrAssignee + "/" +
						IssueinfoStrKey + "/" +
						IssueinfoStrName + "/" +
						IssueinfoStrSubmittable + "=" +
						issue[IssueinfoIndSubmittable])
					op(IssueinfoStrHead + "/" +
						IssueinfoStrSummary + "/" +
						IssueinfoStrRevCur + "/" +
						IssueinfoStrVerified + "=" +
						issue[IssueinfoIndHead])
					op(IssueinfoStrProj + "/" +
						IssueinfoStrCodereview + "=" +
						issue[IssueinfoIndProj])
					op(IssueinfoStrBranch + "=" +
						issue[IssueinfoIndBranch])
					op(IssueinfoStrState + "/" +
						IssueinfoStrSubmitType + "=" +
						issue[IssueinfoIndState])
					op(IssueinfoStrMergeable +
						IssueinfoStrDispname + "=" +
						issue[IssueinfoIndMergeable])
				}
			}
		}
	}

	eztools.ShowStrln("waiting for update check...")
	if <-upch {
		eztools.ShowStrln("waiting for update check to end...")
		<-upch
	}
}

func saveProj(svr *svrs, proj string) bool {
	if svr == nil && len(proj) < 1 {
		return false
	}
	var ret bool
	//for i := range cfg.Svrs {
	/*eztools.ShowStrln("saveProj checking " + cfg.Svrs[i].Name + " to " + svr.Name)
	if cfg.Svrs[i] == svr {
		if cfg.Svrs[i].Proj != proj {*/
	if svr.Proj != proj {
		svr.Proj = proj
		ret = true
	}
	/*break
	}*/
	//}
	if !ret {
		return false
	}
	if err := eztools.XMLWriteNoCreate(cfgFile, cfg); err != nil {
		eztools.LogErrPrint(err)
	}
	return true
}

func chkUpdate(upch chan bool) {
	db, err := eztools.Connect()
	if err != nil {
		upch <- false
		eztools.LogErr(err)
	} else {
		defer db.Close()
	}
	eztools.AppUpgrade(db, module, ver, nil, upch)
}

func cfg2AuthInfo(svr svrs, cfg cfgs) (authInfo eztools.AuthInfo, err error) {
	pass := svr.Pass
	if len(pass.Pass) < 1 {
		pass = cfg.Pass
	}
	authInfo = eztools.AuthInfo{User: cfg.User}
	switch pass.Type {
	case PassDigest:
		authInfo.Type = eztools.AUTH_DIGEST
	case PassPlain:
		authInfo.Type = eztools.AUTH_PLAIN
	case PassBasic:
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
	if svr.Type == CategoryJira {
		if len(svr.Flds.TstExp+svr.Flds.TstPre+svr.Flds.TstStep) < 1 {
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
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

/* get all values from
sth. {
	name: value
	[
	sth. {
		name: value
	}
	]
}
*/
func getValuesFromMaps(name string, field interface{}) string {
	//eztools.ShowSthln(field)
	fieldMap, ok := field.(map[string]interface{})
	if !ok {
		eztools.Log(reflect.TypeOf(field).String() +
			" got instead of map[string]interface{}")
		return ""
	}
	type filterFunc func([]string, map[string]interface{}) []string
	var fF filterFunc
	values := make([]string, 0)
	fF = func(s []string, m map[string]interface{}) []string {
		for i, v := range m {
			if i == name {
				child, ok := v.(string)
				if !ok {
					eztools.Log(reflect.TypeOf(v).String() +
						" got instead of string")
					continue
				}
				s = append(s, child)
				continue
			}
			child, ok := v.([]interface{})
			if ok {
				for _, v := range child {
					child, ok := v.(map[string]interface{})
					if ok {
						s = fF(s, child)
					}
				}
			}
		}
		return s
	}
	values = fF(values, fieldMap)
	if choice := eztools.ChooseStrings(values); choice != eztools.InvalidID {
		return values[choice]
	}
	return ""
}

const (
	//common use

	// IssueinfoIndID ID for issueInfos
	IssueinfoIndID = iota
	// IssueinfoIndKey key for issueInfos
	IssueinfoIndKey
	// IssueinfoIndHead head/title for issueInfos
	IssueinfoIndHead
	// IssueinfoIndProj project for issueInfos
	IssueinfoIndProj
	// IssueinfoIndBranch branch for issueInfos
	IssueinfoIndBranch
	// IssueinfoIndState state for issueInfos
	IssueinfoIndState
	// IssueinfoIndExt extension/placeholder for issueInfos
	IssueinfoIndExt // placeholder for mergable of gerrit and comment of jira
	// IssueinfoIndMax number of issueInfos indexes
	IssueinfoIndMax

	// gerrit state

	// placeholder for ID

	// IssueinfoIndSubmittable submittable of issueInfos for gerrit
	IssueinfoIndSubmittable = iota - IssueinfoIndMax
	// IssueinfoIndVerified verified of issueInfos for gerrit
	IssueinfoIndVerified
	// IssueinfoIndCodereview codereview of issueInfos for gerrit
	IssueinfoIndCodereview
	// IssueinfoIndScore configured score of issueInfos for gerrit
	IssueinfoIndScore // upper bound of scoreInfos
	// IssueinfoIndSubmitType submit type of issueInfos for gerrit
	IssueinfoIndSubmitType
	// IssueinfoIndMergeable mergable of issueInfos for gerrit
	IssueinfoIndMergeable

	// jira details

	// placeholder for ID

	// IssueinfoIndDesc description of issueInfos for Jira
	IssueinfoIndDesc = iota + 1 - IssueinfoIndMax*2
	// no id for summary, jira

	// IssueinfoIndDispname display name of issueInfos for Jira
	IssueinfoIndDispname = iota + 3 - IssueinfoIndMax*2
	// IssueinfoIndComment comment of issueInfos for Jira
	IssueinfoIndComment = iota + 4 - IssueinfoIndMax*2

	// IssueinfoStrID ID string for issueInfos
	IssueinfoStrID = "id"
	// IssueinfoStrSubmittable submittable string for issueInfos
	IssueinfoStrSubmittable = "submittable" // \
	// IssueinfoStrKey key string for issueInfos
	IssueinfoStrKey = "key" //
	// IssueinfoStrAssignee assignee string for issueInfos
	IssueinfoStrAssignee = "assignee" //
	// IssueinfoStrName name string for issueInfos
	IssueinfoStrName = "name" // /
	// IssueinfoStrHead subject string for issueInfos
	IssueinfoStrHead = "subject" // \
	// IssueinfoStrSummary summary string for issueInfos
	IssueinfoStrSummary = "summary" //
	// IssueinfoStrDesc description string for issueInfos
	IssueinfoStrDesc = "description" //
	// IssueinfoStrRevCur current revision string for issueInfos
	IssueinfoStrRevCur = "current_revision" //
	// IssueinfoStrVerified verified string for issueInfos
	IssueinfoStrVerified = "Verified" // /
	// IssueinfoStrProj project string for issueInfos
	IssueinfoStrProj = "project" // \
	// IssueinfoStrCodereview code review string for issueInfos
	IssueinfoStrCodereview = "Code-Review" // /
	// IssueinfoStrBranch branch string for issueInfos
	IssueinfoStrBranch = "branch" // \
	// IssueinfoStrDispname display name string for issueInfos
	IssueinfoStrDispname = "displayName" // /
	// for code-review, verified and manual-testing

	// IssueinfoStrSubmitType submit type string for issueInfos
	IssueinfoStrSubmitType = "submit_type" // \
	// IssueinfoStrApprovals approvals string for issueInfos
	IssueinfoStrApprovals = "approvals" //
	// IssueinfoStrState state string for issueInfos
	IssueinfoStrState = "status" // /
	// gerrit details

	// IssueinfoStrMergeable mergable string for issueInfos
	IssueinfoStrMergeable = "mergeable"
)

type issueInfos [IssueinfoIndMax]string
type scoreInfos [IssueinfoIndScore + 1]int

var issueInfoTxt = issueInfos{
	IssueinfoStrID, IssueinfoStrKey, IssueinfoStrHead,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrState, ""}
var issueDetailsTxt = issueInfos{
	IssueinfoStrID, IssueinfoStrSubmittable, IssueinfoStrHead,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrState, IssueinfoStrMergeable}
var issueRevsTxt = issueInfos{
	IssueinfoStrID, IssueinfoStrName, IssueinfoStrRevCur,
	IssueinfoStrProj, /*placeholder*/
	IssueinfoStrBranch, IssueinfoStrSubmitType, ""}

var reviewInfoTxt = issueInfos{
	IssueinfoStrID, IssueinfoStrName, IssueinfoStrVerified,
	IssueinfoStrCodereview, IssueinfoStrDispname, /*placeholder for SCORE*/
	IssueinfoStrApprovals, ""}

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

func custFld(jsonStr, fldKey, fldVal string) string {
	if len(fldKey) > 0 {
		if len(jsonStr) > 0 {
			jsonStr += `,
`
		}
		return jsonStr + `        "` +
			fldKey + `": "` + fldVal + `"`
	}
	return jsonStr
}

const typicalJiraSeparator = "-"

// return values
//	whether input is in exact x-0 or -0 format.
//		in case of -0, if previous project (x part) found, it is taken.
//		otherwise, false is returned.
//	the non digit part. this is saved as project.
//	the digit part
func parseTypicalJiraNum(svr *svrs, num string) (ret bool, nonDigit, digit string) {
	re := regexp.MustCompile(`^[^\d]+[-][\d]+$`)
	pref := re.FindStringSubmatch(num)
	if pref != nil {
		parts := strings.Split(pref[0], typicalJiraSeparator)
		if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 0 {
			saveProj(svr, parts[0])
			return true, parts[0] + typicalJiraSeparator, parts[1]
		}
	} else {
		if len(svr.Proj) > 0 {
			re = regexp.MustCompile(`[-][\d]+$`)
			pref = re.FindStringSubmatch(num)
			if pref != nil {
				parts := strings.Split(pref[0], typicalJiraSeparator)
				// parts[0]=""
				if len(parts) == 2 && len(parts[1]) > 0 {
					eztools.ShowStrln("Auto changed to " +
						svr.Proj + typicalJiraSeparator + parts[1])
					return true, svr.Proj + typicalJiraSeparator, parts[1]
				}
			}
		}
	}
	return false, "", ""
}

// loopIssues runs a function on all numbers between, inclusively,
// X-0 and X-1, or 0,1 from input in format of X-0,1 or 0,1
// If it is not a range, the function's return values are returned.
// Otherwise, no return values.
func loopIssues(svr *svrs, issueInfo issueInfos, fun func(issueInfos) (
	issueInfos, error)) (issueInfoOut []issueInfos, err error) {
	const separator = ","
	printID := func() {
		if err == nil {
			eztools.LogPrint("Done with " + issueInfo[IssueinfoIndID])
		}
	}
	switch strings.Count(issueInfo[IssueinfoIndID], separator) {
	case 0: // single ID
		if ok, prefix, lowerBoundStr := parseTypicalJiraNum(svr,
			issueInfo[IssueinfoIndID]); ok {
			issueInfo[IssueinfoIndID] = prefix + lowerBoundStr
		}
		issueInfo, err := fun(issueInfo)
		printID()
		return []issueInfos{issueInfo}, err
	case 2: // x,,y or x,y,z
		parts := strings.Split(issueInfo[IssueinfoIndID], separator)
		if len(parts) != 2 || len(parts[0]) < 1 || len(parts[2]) < 1 {
			if len(parts) == 3 {
				// x,y,z instead of range
				break
			}
			eztools.LogPrint("range format needs both parts aside with two \"" +
				separator + "\"" + " or multiple parts, deliminated by \"" +
				separator + "\"")
			return nil, eztools.ErrInvalidInput
		}
		if len(parts[1]) < 1 { // x,,y
			var (
				prefix, lowerBoundStr  string
				lowerBound, upperBound int
			)
			lowerBound, err = strconv.Atoi(parts[0])
			if err != nil {
				var ok bool
				if ok, prefix, lowerBoundStr = parseTypicalJiraNum(svr, parts[0]); !ok {
					eztools.LogPrint("the former part must be in the form of X-0 or 0")
					return
				}
				lowerBound, err = strconv.Atoi(lowerBoundStr)
				if err != nil {
					eztools.LogPrint(lowerBoundStr + " is NOT a number!")
					return
				}
			}
			upperBound, err = strconv.Atoi(parts[1])
			if err != nil {
				eztools.LogPrint("the latter part must be a number")
				return
			}
			if lowerBound >= upperBound {
				eztools.LogPrint("the number in the latter part must be greater than the one in the former part")
				return
			}
			for i := lowerBound; i <= upperBound; i++ {
				issueInfo[IssueinfoIndID] = prefix + strconv.Itoa(i)
				//eztools.ShowStrln("looping " + issueInfo[ISSUEINFO_IND_ID])
				issueInfo, err = fun(issueInfo)
				if err != nil {
					return
				}
				issueInfoOut = append(issueInfoOut, issueInfo)
				printID()
			}
			return
		}
	}
	// x,y[,...]
	parts := strings.Split(issueInfo[IssueinfoIndID], separator)
	var (
		prefix, prefixNew, currentNo string
		ok                           bool
	)
	if ok, prefix, currentNo = parseTypicalJiraNum(svr, parts[0]); !ok {
		currentNo = parts[0]
	}
	i := 1
	for {
		issueInfo[IssueinfoIndID] = prefix + currentNo
		//eztools.ShowStrln("looping " + issueInfo[ISSUEINFO_IND_ID])
		issueInfo, err = fun(issueInfo)
		if err != nil {
			return
		}
		issueInfoOut = append(issueInfoOut, issueInfo)
		printID()
		if i < len(parts) {
			if ok, prefixNew, currentNo = parseTypicalJiraNum(svr, parts[i]); !ok {
				// reuse old prefix
				currentNo = parts[i]
			} else {
				prefix = prefixNew
			}
			i++
		} else {
			break
		}
	}
	return
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
func cfmInputOrPromptStr(svr *svrs, inf *issueInfos, ind int, prompt string) bool {
	const linefeed = " (end with \\ to input multi lines)"
	var def, base string
	var smart bool // no smart affix available by default
	if len(inf[ind]) > 0 {
		var ok bool
		if ok, base, _ = parseTypicalJiraNum(svr, inf[ind]); ok {
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
	eztools.ShowStrln("Auto changed to " + inf[ind])
	//}
	return true
}

func cfmInputOrPrompt(svr *svrs, inf *issueInfos, ind int) bool {
	return cfmInputOrPromptStr(svr, inf, ind, issueInfoTxt[ind])
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
	case CategoryJira:
		switch action {
		case "show details of a case",
			"list comments of a case":
			useInputOrPrompt(inf, IssueinfoIndID)
		case "close a case to resolved from any known statues":
			useInputOrPromptStr(inf, IssueinfoIndComment,
				"test step for closure")
		case "search for comments by project and user":
			useInputOrPrompt(inf, IssueinfoIndProj)
		}
	case CategoryGerrit:
		switch action {
		case "show details of a submit",
			"show reviewers of a submit",
			"rebase a submit",
			"abandon a submit",
			"show revision/commit of a submit",
			"add scores to a submit",
			"reject a case from any known statues",
			"merge a submit",
			"add socres, wait for it to be mergable and merge a submit":
			useInputOrPrompt(inf, IssueinfoIndID)
		case "cherry pick all my open":
			useInputOrPromptStr(inf,
				IssueinfoIndHead, IssueinfoStrRevCur)
			useInputOrPrompt(inf, IssueinfoIndBranch)
		case "list merged submits of someone",
			"add socres, wait for it to be mergable and merge sb.'s submits",
			"list sb.'s open submits":
			useInputOrPromptStr(inf,
				IssueinfoIndID, IssueinfoStrAssignee)
			useInputOrPrompt(inf, IssueinfoIndBranch)
		case "cherry pick a submit":
			eztools.ShowStrln("Please input an ID that can make it " +
				"distinguished, such as commit, instead of Change " +
				"ID, which is reused among cherrypicks.")
			useInputOrPromptStr(inf,
				IssueinfoIndID, IssueinfoStrID)
			useInputOrPromptStr(inf,
				IssueinfoIndHead, IssueinfoStrRevCur)
			useInputOrPrompt(inf, IssueinfoIndBranch)
		}
	default:
		eztools.LogPrint("Server type unknown: " + svrType)
	}
	//eztools.ShowSthln(inf)
}

func makeCat2Act() cat2Act {
	return cat2Act{
		CategoryJira: []action2Func{
			{"transfer a case to someone", jiraTransfer},
			{"move status of a case", jiraTransition},
			{"show details of a case", jiraDetail},
			{"list comments of a case", jiraComments},
			{"add a comment to a case", jiraAddComment},
			{"search for comments by project and user", jiraSearchCommentsByProjNUser},
			{"list my open cases", jiraMyOpen},
			{"link a case to the other", jiraLink},
			{"reject a case from any known statues", jiraReject},
			{"close a case to resolved from any known statues", jiraClose},
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			{"close a case with default design as steps", jiraCloseDef},
			{"close a case with general requirement as steps", jiraCloseGen},
		},
		CategoryGerrit: []action2Func{
			{"list merged submits of someone", gerritSbMerged},
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
}
