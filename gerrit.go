package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gitee.com/bon-ami/eztools/v4"
)

func gerritGetIssuesOrReviews(method, url, magic string,
	authInfo eztools.AuthInfo, fun func(map[string]interface{},
		issueInfoSlc) issueInfoSlc) (issueInfoSlc, error) {
	body, err := restSth(method, url, authInfo, nil, magic)
	if err != nil || body == nil {
		return nil, err
	}
	issues := make(issueInfoSlc, 0)
	bodySlc, ok := body.([]interface{})
	if ok {
		if len(bodySlc) < 1 {
			return nil, err
		}
		for _, v := range bodySlc {
			m, ok := v.(map[string]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of map string to interface!")
				continue
			}
			issues = fun(m, issues)
		}
	} else {
		bodyMap, ok := body.(map[string]interface{})
		if ok {
			issues = fun(bodyMap, issues)
		} else {
			eztools.LogPrint(reflect.TypeOf(body).String() +
				" got instead of slice of or map string to, interface!")
		}
	}
	return issues, err
}

// gerritParseIssuesOrReviews parses body from gerrit responses into
// issueInfoSlc
/*
param:
	m	body
	issues	results are appended to this
	strs	keywords to parse. should be on the first level
	issue	partially parsed fields, usually for looping only
*/
func gerritParseIssuesOrReviews(m map[string]interface{},
	issues issueInfoSlc, strs []string,
	issue issueInfos) issueInfoSlc {
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowStr("parsing ")
		eztools.ShowSthln(strs)
		eztools.ShowStr("from ")
		eztools.ShowSthln(m)
	}
	if issue == nil {
		issue = make(issueInfos)
	}
	for _, str1 := range strs {
		if len(str1) < 1 {
			continue
		}
		// try to match one field
		if m[str1] == nil {
			if eztools.Debugging && eztools.Verbose > 2 {
				eztools.ShowStrln("unmatching " +
					str1)
			}
			continue
		}
		// string array to loop?
		mp, ok := m[str1].(map[string]interface{})
		if ok {
			gerritParseIssuesOrReviews(mp, issues, strs, issue)
			continue
		}
		// string?
		str, ok := m[str1].(string)
		if ok {
			if eztools.Debugging && eztools.Verbose > 2 {
				eztools.ShowStrln("matching " +
					str1 + " <- " + str)
			}
			issue[str1] = str
			continue
		}
		// not a string
		if str1 == IssueinfoStrSubmittable ||
			str1 == IssueinfoStrMergeable {
			// bool is different
			bo, ok := m[str1].(bool)
			if !ok {
				if !eztools.Debugging {
					continue
				}
				eztools.LogPrint(
					reflect.TypeOf(
						m[str1]).
						String() +
						" got instead of " +
						"bool for " +
						str1 + "!")
			} else {
				switch bo {
				case true:
					issue[str1] = "true"
				case false:
					issue[str1] = "false"
				}
				if eztools.Debugging && eztools.Verbose > 2 {
					eztools.ShowStrln("matched " +
						str1 + "=" + issue[str1])
				}
			}
			continue
		}
		// other types
		if eztools.Debugging {
			eztools.Log(str1 + " matched with unknown type " +
				reflect.TypeOf(m[str1]).String())
		}
	}
	if issues != nil {
		return append(issues, issue)
	}
	return issue.ToSlc()
}

func gerritParseAuthor(i interface{}, issues issueInfoSlc) issueInfoSlc {
	if i == nil {
		return issues
	}
	m, ok := i.(map[string]interface{})
	if !ok {
		eztools.Log("non string map in author " +
			reflect.TypeOf(i).String())
		return issues
	}
	ni := m[IssueinfoStrName]
	if ni == nil {
		eztools.Log("no name in author", m)
		return issues
	}
	ns, ok := ni.(string)
	if !ok {
		eztools.Log("not string of name in author " +
			reflect.TypeOf(ni).String())
		return issues
	}
	if issues == nil {
		issues = make(issueInfoSlc, 1)
	}
	issues[len(issues)-1][IssueinfoStrAuthor] = ns
	return issues
}

// no ID will return, since not in replies
func gerritGetReviews(url, magic string, authInfo eztools.AuthInfo) (
	issueInfoSlc, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseIssuesOrReviews(m, issues, reviewInfoTxt, nil)
		})
}

func gerritGetDetails(url, magic string, authInfo eztools.AuthInfo) (
	issueInfoSlc, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseIssuesOrReviews(m, issues, issueDetailsTxt, nil)
		})
}

func gerritGetHistory(url, magic string, authInfo eztools.AuthInfo) (
	issueInfoSlc, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			i := m["messages"]
			if i == nil {
				eztools.Log("no history")
				return nil
			}
			a, ok := i.([]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(i).String() +
					" got instead of slice")
				return nil
			}
			var ret issueInfoSlc
			for _, i := range a {
				m, ok := i.(map[string]interface{})
				if !ok {
					continue
				}
				ret = gerritParseIssuesOrReviews(m, ret, issueHistoryTxt, nil)
				// taking it as granted that a new piece is generated
				//   and added to the end of the slice
				ret = gerritParseAuthor(m[IssueinfoStrAuthor], ret)
			}
			eztools.ShowSthln(ret)
			return ret
		})
}

func gerritGetIssues(url, magic string, authInfo eztools.AuthInfo) (
	issueInfoSlc, error) {
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseIssuesOrReviews(m, issues, issueInfoTxt, nil)
		})
}

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
func gerritQuery1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, opt string) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/?q="
	return gerritGetDetails(svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+opt,
		svr.Magic, authInfo)
}

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
// return value: same as gerritGetDetails[0]
func gerritAnyID2ID(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	inf, err := gerritQuery1(svr, authInfo, issueInfo, "")
	if err != nil {
		return nil, err
	}
	if len(inf) != 1 {
		return nil, eztools.ErrNoValidResults
	}
	return inf[0], nil
}

func gerritRevs(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	f := func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos, res issueInfoSlc) issueInfoSlc {
		return append(res, issueInfo)
	}
	return gerritProcRevLoopMyOpen(svr, authInfo,
		issueInfo, f)
}

func gerritRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	const RestAPIStr = "changes/?q="
	// +"&o=CURRENT_REVISION" to list a commit and *ALL* for all
	return gerritGetIssuesOrReviews(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"&o=CURRENT_REVISION&o=DOWNLOAD_COMMANDS",
		svr.Magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			issues = gerritParseIssuesOrReviews(m, issues, issueRevsTxt, nil)
			dlds := gerritParseRecursively(m, []string{"ssh", "commands"},
				func(body map[string]interface{}) issueInfoSlc {
					retI := body[IssueinfoStrCherry]
					if retI == nil {
						eztools.LogPrint("NOTHING got intead of string!")
						return nil
					}
					if retS, ok := retI.(string); ok {
						return issueInfos{IssueinfoStrMergeable: retS}.ToSlc()
					}
					eztools.LogPrint(reflect.TypeOf(retI).String() +
						" got instead of string!")
					return nil
				})

			if len(issues) != 1 || len(dlds) != 1 {
				eztools.LogPrint("Invalid number of revision/downloads!")
				issues = append(issues, dlds...)
			} else {
				issues[0][IssueinfoStrMergeable] = dlds[0][IssueinfoStrMergeable]
			}
			return issues
		})
}

func gerritDetailOnCurrRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritQuery1(svr, authInfo, issueInfo, "&o=CURRENT_REVISION")
}

func gerritHistory(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/"
	return gerritGetHistory(svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/detail",
		svr.Magic, authInfo)
}

type scores2Marshal map[string]int

// gerritGetScores run detail on the issue to list all fields needing scores
func gerritGetScores(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (scores []scores2Marshal,
	rejected map[string]struct{}, err error) {
	issueInfo, err = gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	const RestAPIStr = "changes/"
	body, err := restSth(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/detail",
		authInfo, nil, svr.Magic)
	if err != nil || body == nil {
		return
	}
	err = eztools.ErrNoValidResults
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(body).String() +
			" got instead of slice of or map string to, interface!")
		return
	}
	labels := bodyMap[IssueinfoStrLabels]
	if labels == nil {
		return
	}
	labelMap, ok := labels.(map[string]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(labels).String() +
			" got instead of map string to interface!")
		return
	}
	scores = make([]scores2Marshal, 0)
	rejected = make(map[string]struct{})
	for labelName, label1 := range labelMap {
		label, ok := label1.(map[string]interface{})
		if !ok {
			eztools.LogPrint(reflect.TypeOf(label1).String() +
				" got instead of map string to interface!")
			continue
		}
		//eztools.ShowStrln(labelName + "=")
		if label["approved"] != nil {
			if eztools.Debugging && eztools.Verbose > 2 {
				eztools.ShowStrln(labelName, " already approved.")
			}
			continue
		}
		if label["rejected"] != nil {
			rejected[labelName] = struct{}{}
			/*if eztools.Debugging && eztools.Verbose > 2 {
				eztools.Log(labelName + " already rejected.")
			}*/
		}
		values := label["values"]
		valueMap, ok := values.(map[string]interface{})
		if !ok {
			eztools.LogPrint(reflect.TypeOf(values).String() +
				" got instead of map string to interface for " +
				labelName + "!")
			return nil, nil, nil
		}
		high := 0
		for v := range valueMap {
			i, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				eztools.LogPrint(v+
					" got instead of int for "+
					labelName, err)
				continue
			}
			if i > high {
				high = i
			}
		}
		if high <= 0 {
			eztools.LogPrint("NO score choices found for " +
				labelName + "!")
			continue
		}
		scores = append(scores, scores2Marshal{labelName: high})
	}
	if len(scores) < 1 {
		return nil, rejected, eztools.ErrInExistence
	}
	err = nil
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowSthln(scores)
	}
	return
}

func gerritParseFiles(body map[string]interface{}) issueInfoSlc {
	issues := make(issueInfoSlc, 0)
	for file1, v := range body {
		if file1 == "/COMMIT_MSG" {
			continue
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			eztools.LogPrint(reflect.TypeOf(v).String() +
				" got instead of map string to interface!")
			continue
		}
		type flds struct {
			name, value string
		}
		fldSlc := [...]flds{
			{name: IssueinfoStrBin},
			{name: IssueinfoStrState},
			{name: IssueinfoStrOldPath},
		}

		for i, v := range fldSlc {
			fldSlc[i].value, _ = func(m map[string]interface{}, str string) (string, bool) {
				if m[str] == nil {
					return "", true
				}
				fb, ok := m[str].(bool)
				if ok {
					return strconv.FormatBool(fb), true
				}
				fn, ok := m[str].(string)
				if !ok {
					eztools.LogPrint(reflect.TypeOf(m[str]).String() +
						" got instead of string for " + str)
					return "", false

				}
				return fn, true
			}(m, v.name)
		}
		inf := issueInfos{IssueinfoStrFile: file1}
		for _, fld1 := range fldSlc {
			if len(fld1.value) > 0 {
				inf[fld1.name] = fld1.value
			}
		}
		issues = append(issues, inf)
		if eztools.Debugging && eztools.Verbose > 2 {
			eztools.ShowStrln(file1 + " checked")
		}
	}
	return issues
}

func gerritListFilesByRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 || len(issueInfo[IssueinfoStrSummary]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/"
	bodyMap, err := restMap(eztools.METHOD_GET,
		svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID]+"/revisions/"+
			issueInfo[IssueinfoStrRevCur]+"/files/",
		authInfo, nil, svr.Magic)
	if err != nil || nil == bodyMap || len(bodyMap) < 1 {
		return nil, err
	}
	return gerritParseFiles(bodyMap), nil
}

func gerritListFiles(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	/*inf, err := gerritRev(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(inf) != 1 {
		eztools.LogPrint("NO single revision info found!")
		return inf, eztools.ErrNoValidResults
	}
	infWtRev := inf[0]
	if len(infWtRev[IssueinfoStrHead]) < 1 {
		eztools.LogPrint("NO revision found!")
		return inf, eztools.ErrNoValidResults
	}
	issueInfo[IssueinfoStrHead] = infWtRev[IssueinfoStrHead]
	return gerritListFilesByRev(svr, authInfo, issueInfo)*/
	/*const RestAPIStr = "changes/?q="
	bodyMap, err := restMap(eztools.METHOD_GET,
		svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID]+
			"&o=CURRENT_REVISION&o=CURRENT_FILES",
		authInfo, nil, svr.Magic)
	if err != nil || nil == bodyMap || len(bodyMap) < 1 {
		return nil, err
	}
	f := bodyMap["files"]
	if f == nil {
		return nil, nil
	}
	fm, ok := f.(map[string]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(f).String() +
			" got instead of map string to interface!")
		return nil, nil
	}
	return gerritParseFiles(fm), nil*/
	const RestAPIStr = "changes/?q="
	return gerritGetIssuesOrReviews(eztools.METHOD_GET,
		svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID]+
			"&o=CURRENT_REVISION&o=CURRENT_FILES",
		svr.Magic, authInfo, func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseRecursively(m, []string{"files"}, gerritParseFiles)
		})
}

// gerritParseRecursively loops into the map, returning all results,
// invoking func with the deepest found matches to name of map[string]interface{} into str.
func gerritParseRecursively(m map[string]interface{}, str []string, // issues issueInfoSlc,
	fun func(body map[string]interface{}) issueInfoSlc) (issues issueInfoSlc) {
	if m[str[0]] != nil {
		m1 := m
		for _, v := range str {
			fm, ok := m1[v].(map[string]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(m1[v]).String() +
					" got instead of map string to interface for " + v)
				return nil
			}
			m1 = fm
		}
		return fun(m1)
	}
	for _, v := range m {
		fm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		issues = append(issues, gerritParseRecursively(fm, str, fun)...)
	}
	return issues
}

// no ID will return, since not in replies
func gerritReviews(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	// so that commit ID works, too, besides ChangeID and URL
	issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	const RestAPIStr = "changes/"
	return gerritGetReviews(svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/reviewers/",
		svr.Magic, authInfo)
}

func gerritGetIssuesWtOwner(svr *svrs, authInfo eztools.AuthInfo,
	status string, issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	var urlAffix string
	strs := [...][2]string{
		{"status:", status},
		{"branch:", issueInfo[IssueinfoStrBranch]},
		{"owner:", issueInfo[IssueinfoStrID]}}
	for _, v := range strs {
		if len(v[1]) > 0 {
			if len(urlAffix) > 0 {
				urlAffix += "+"
			}
			urlAffix += v[0] + v[1]
		}
	}
	const RestAPIStr = "changes/?q="
	return gerritGetIssues(svr.URL+RestAPIStr+urlAffix, svr.Magic, authInfo)
}

func gerritSbMerged(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritGetIssuesWtOwner(svr, authInfo, "merged", issueInfo)
}

func gerritAllOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if !uiSilent {
		if cfm := eztools.PromptStr("This may take quite a while..." +
			"Confirm to continue. ([Enter]=Y/y)"); cfm == "N" || cfm == "n" {
			return nil, eztools.ErrInvalidInput
		}
	}
	const RestAPIStr = "changes/"
	return gerritGetIssues(svr.URL+RestAPIStr, svr.Magic, authInfo)
}

func gerritSbOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritGetIssuesWtOwner(svr, authInfo, "open", issueInfo)
}

func gerritMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	issueInfo[IssueinfoStrID] = authInfo.User
	return gerritGetIssuesWtOwner(svr, authInfo, "open", issueInfo)
}

func gerritRebase(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/rebase")
}

func gerritRevert(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/revert")
}

func gerritMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	issueInfo, err := gerritChooseMyOpen(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	// check mergable only, without submittable
	inf, err := gerritDetailOnCurrRev(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(inf) < 1 {
		err = eztools.ErrNoValidResults
		return nil, err
	}
	if inf[0][IssueinfoStrSubmittable] != "false" &&
		inf[0][IssueinfoStrMergeable] != "false" {
		// either empty(=not supported or already merged) or true will do
		return gerritActOn1(svr, authInfo, issueInfo, nil, "/submit")
	}
	return nil, eztools.ErrNoValidResults
}

func gerritAbandon(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/abandon")
}

func gerritAbandonMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritActOnMyOpen(svr, authInfo, issueInfo, "/abandon")
}

func gerritPick(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrBranch]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	if len(issueInfo[IssueinfoStrRevCur]) < 1 {
		inf, err := gerritRev(svr, authInfo, issueInfo)
		if err != nil {
			return nil, err
		}
		if len(inf) < 1 {
			return nil, eztools.ErrNoValidResults
		}
		// should be only one or same among all
		issueInfo[IssueinfoStrRevCur] = inf[0][IssueinfoStrRevCur]
	}
	return gerritPick1(svr, authInfo, issueInfo, nil)
}

func gerritPick1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, res issueInfoSlc) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrBranch]) < 1 ||
		len(issueInfo[IssueinfoStrRevCur]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	//if eztools.Debugging && !uiSilent {
	/*if !eztools.ChkCfmNPrompt("continue to cherrypick "+
		issueInfo[IssueinfoStrID]+
		" from "+issueInfo[IssueinfoStrRevCur]+
		" to "+issueInfo[IssueinfoStrBranch], "n") {
		return nil, nil
	}*/
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log("to cheerypick " +
			issueInfo[IssueinfoStrID] +
			" from " + issueInfo[IssueinfoStrRevCur] +
			" to " + issueInfo[IssueinfoStrBranch])
	}
	const RestAPIStr = "changes/"
	jsonValue, _ := json.Marshal(map[string]string{
		//"message": "testing", // if this is a must, I have to read original submit message
		"destination": issueInfo[IssueinfoStrBranch]})
	bodyMap, err := restMap("POST", svr.URL+
		RestAPIStr+issueInfo[IssueinfoStrID]+
		"/revisions/"+issueInfo[IssueinfoStrRevCur]+
		"/cherrypick",
		authInfo, bytes.NewBuffer(jsonValue), svr.Magic)
	if len(bodyMap) < 1 {
		return nil, nil
	}
	return gerritParseIssuesOrReviews(bodyMap, res, issueInfoTxt, nil), err
}

// gerritProcRevLoopMyOpen run a func on all my open issues
// with current revision/commit info
func gerritProcRevLoopMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos,
	f func(*svrs, eztools.AuthInfo, issueInfos,
		issueInfoSlc) issueInfoSlc) (res issueInfoSlc,
	err error) {
	issues, err := gerritMyOpen(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	for _, issueInfo := range issues {
		inf, err := gerritRev(svr, authInfo, issueInfo)
		if err != nil {
			eztools.LogPrint(err)
			continue
		}
		if len(inf) != 1 {
			eztools.ShowStrln("NO unique revision info found?")
			if eztools.Debugging {
				eztools.Log("NO unique revision found for " +
					issueInfo[IssueinfoStrID] + " (" +
					issueInfo[IssueinfoStrID] + ")")
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
	issueInfo issueInfos) (issueInfoSlc, error) {
	branch := issueInfo[IssueinfoStrBranch]
	f := func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos,
		res issueInfoSlc) issueInfoSlc {
		issueInfo[IssueinfoStrBranch] = branch
		resO, _ := gerritPick1(svr, authInfo, issueInfo, res)
		return resO
	}
	return gerritProcRevLoopMyOpen(svr, authInfo,
		issueInfo, f)
}

func gerritActOnMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, action string) (res issueInfoSlc, err error) {
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
	issueInfo issueInfos, issues issueInfoSlc,
	action string) (issueInfoSlc, error) {
	issueInfo, err := gerritChooseMyOpen(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	return gerritActOn1(svr, authInfo, issueInfo, nil, action)
}

// gerritActOn1 POST changes/ID/action
// param: issueInfo[ISSUEINFO_IND_ID] unique ID
// TODO: should returned slice mean anything when input slice is nil?
// Currently all discarded
func gerritActOn1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, issues issueInfoSlc,
	action string) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return issues, eztools.ErrInvalidInput
	}
	if eztools.Debugging && !uiSilent {
		if !eztools.ChkCfmNPrompt(action+" "+
			issueInfo[IssueinfoStrID], "n") {
			return nil, nil
		}
	}
	const RestAPIStr = "changes/"
	bodyMap, err := restMap("POST", svr.URL+
		RestAPIStr+issueInfo[IssueinfoStrID]+action,
		authInfo, nil, svr.Magic)
	return gerritParseIssuesOrReviews(bodyMap, issues, issueInfoTxt, nil),
		err
}

func gerritScoreChkRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (inf issueInfoSlc, err error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	inf, err = gerritRev(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(inf) != 1 {
		eztools.LogPrint("NO single revision info found!")
		return inf, eztools.ErrNoValidResults
	}
	if len(inf[0][IssueinfoStrRevCur]) < 1 {
		eztools.LogPrint("NO revision found!")
		return inf, eztools.ErrNoValidResults
	}
	return
}

func gerritScoreNGetRej(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (rejectedAft map[string]struct{},
	failed map[string]struct{}, err error) {
	scores, rejectedB4, err := gerritGetScores(svr, authInfo, issueInfo)
	if err != nil {
		if err == eztools.ErrInExistence {
			return nil, nil, nil
		}
		return
	}
	const RestAPIStr = "changes/"
	for _, score1 := range scores {
		if len(score1) < 1 {
			continue
		}
		var jsonValue []byte
		jsonValue, err = json.Marshal(map[string]scores2Marshal{
			IssueinfoStrLabels: score1})
		if err != nil {
			eztools.Log(err)
			break
		}
		//eztools.ShowStrln(string(jsonValue))
		/*if eztools.Debugging && !uiSilent {
			if !eztools.ChkCfmNPrompt("continue to +2/1 to "+
				infWtRev[IssueinfoStrID], "n") {
				break
			}
		}*/
		body, err1 := restSth("POST", svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID]+"/revisions/"+
			issueInfo[IssueinfoStrRevCur]+"/review",
			authInfo, bytes.NewBuffer(jsonValue), svr.Magic)
		if err1 == nil {
			// response only contain scores for a success, so it is not parsed
			continue
		}
		var errMsg string
		if body == nil {
			errMsg = "no body got"
		} else {
			bodyBytes, ok := body.([]byte)
			if !ok {
				errMsg = reflect.TypeOf(body).String() +
					" got instead of slice of bytes"
			} else {
				if bytes.Contains(bodyBytes, []byte("restricted")) {
					errMsg = "no right to scrore"
				} else {
					errMsg = string(bodyBytes)
				}
			}
		}
		eztools.Log(errMsg, score1)
		rejectedAft = make(map[string]struct{})
		failed = make(map[string]struct{})
		for i := range score1 {
			rejectedAft[i] = rejectedB4[i]
			failed[i] = struct{}{}
		}
		// only one error can be returned?
		err = err1
	}
	return
}

// gerritScore add unapproved scores
// return values:
//
//	inf = nil if success
//	inf = info of current revision if more than one / no revisions found?
//	inf = rejected fields that needs to approve but failed
func gerritScore(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (inf issueInfoSlc, err error) {
	issueInfo, err = gerritChooseMyOpen(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	inf, err = gerritScoreChkRev(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	_, failed, err := gerritScoreNGetRej(svr, authInfo, inf[0])
	if failed != nil {
		inf = nil
		for i := range failed {
			inf = append(inf, issueInfos{IssueinfoStrRej: i})
		}
	}
	return
}

func gerritFuncLoopSbOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, fun func(*svrs, eztools.AuthInfo,
		issueInfos) (issueInfoSlc, error)) (res issueInfoSlc, err error) {
	issues, err := gerritSbOpen(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	for _, issueInfo := range issues {
		res, err = fun(svr, authInfo, issueInfo)
		if err != nil {
			return
		}
	}
	return
}

func gerritWaitNMergeSb(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return gerritFuncLoopSbOpen(svr, authInfo, issueInfo, gerritWaitNMerge)
}

func gerritChooseMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		if uiSilent {
			noInteractionAllowed()
			return nil, eztools.ErrInvalidInput
		}
		// list ready commits
		inf, err := gerritMyOpen(svr, authInfo, issueInfo)
		issueInfo[IssueinfoStrID] = "" // dirty to be user ID by above function
		if err != nil {
			return issueInfo, err
		}
		var choices []string
		for _, v := range inf {
			choices = append(choices,
				v[IssueinfoStrSummary]+" <-> "+
					v[IssueinfoStrBranch]+
					" ("+v[IssueinfoStrID]+")")
		}
		if len(choices) > 0 {
			i, _ := eztools.ChooseStrings(choices)
			if i != eztools.InvalidID {
				issueInfo[IssueinfoStrID] = inf[i][IssueinfoStrID]
			}
		} else {
			eztools.ShowStrln("NO open submits found!")
		}
	}
	if len(issueInfo[IssueinfoStrID]) < 1 {
		useInputOrPrompt(svr, issueInfo, IssueinfoStrID)
		if len(issueInfo[IssueinfoStrID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
	}
	return gerritAnyID2ID(svr, authInfo, issueInfo) // necessary?
}

func gerritWaitNMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	issueInfo, err := gerritChooseMyOpen(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	var (
		inf, rev          issueInfoSlc
		scored            bool
		rejected, rejects map[string]struct{}
		scores            []scores2Marshal
	)
	eztools.ShowStr("waiting for issue to be submittable/mergeable.")
	for err == nil {
		inf, err = gerritDetailOnCurrRev(svr, authInfo, issueInfo)
		if err != nil {
			break
		}
		if len(inf) < 1 {
			err = eztools.ErrNoValidResults
			break
		}
		if inf[0][IssueinfoStrMergeable] == "false" {
			// conflict
			err = eztools.ErrOutOfBound
			break
		}
		if inf[0][IssueinfoStrSubmittable] != "false" &&
			len(inf[0][IssueinfoStrMergeable])+
				len(inf[0][IssueinfoStrSubmittable]) > 0 {
			// the only successful break of loop
			break
			// if neither got, we need to check scores with more details
		}
		//if inf[0][IssueinfoStrSubmittable] == ""
		//get details and check
		/*labels:map[
		  Code-Review:map[
		          approved:map  for OK
		          blocking:true for NG
		          rejected:map  also NG
		          values:map[ 0:No score +1:Looks good to me, but someone else must approve +2:Looks good to me, approved -1:I would prefer this is not merged as is -2:This shall not be merged]]*/

		if !scored {
			rev, err = gerritScoreChkRev(svr, authInfo, issueInfo)
			if err != nil {
				return nil, err
			}
			rejected, _, err = gerritScoreNGetRej(svr, authInfo, rev[0])
			scored = true
			if err != nil {
				eztools.Log(
					"failed to score and wait for it to be scored elsewhere", err)
				err = nil
			} else {
				continue
			}
		} else {
			scores, rejects, err = gerritGetScores(svr, authInfo, rev[0])
			/* 			if rejects != nil {
				for i := range rejects {
					if _, ok := rejected[i]; !ok {
						eztools.LogPrint(i +
							" got newly rejected. exiting.")
						return issueInfoSlc{
								issueInfos{
									IssueinfoStrRej: i}},
							eztools.ErrAccess
					}
				}
			} */
			if scores != nil {
				if rejects != nil {
					for _, i := range scores {
						for j := range i {
							if _, ok := rejected[j]; !ok {
								eztools.LogPrint(j +
									" got de-scored. exiting.")
								return issueInfoSlc{
										issueInfos{
											IssueinfoStrRej: j}},
									eztools.ErrAccess
							}
						}
					}
				}
			}
		}
		time.Sleep(intGerritMerge * time.Second)
		eztools.ShowStr(".")
	}
	eztools.ShowStrln("")
	if err != nil && err != eztools.ErrInExistence {
		// when no scores got, ErrInExistence. Then will try to submit it.
		if err == eztools.ErrOutOfBound {
			eztools.LogPrint("Conflict to merge?")
		}
		return nil, err
	}
	// _, err = gerritMerge(svr, authInfo, issueInfo) not used because of redundant steps of checking
	return gerritActOn1(svr, authInfo, issueInfo, nil, "/submit")
}

func gerritListPrj(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrProj]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "projects/"
	bodyMap, err := restMap(eztools.METHOD_GET,
		svr.URL+RestAPIStr+
			url.QueryEscape(issueInfo[IssueinfoStrProj])+"/config",
		authInfo, nil, svr.Magic)
	if err != nil || nil == bodyMap || len(bodyMap) < 1 {
		return nil, err
	}
	return gerritParseRecursively(bodyMap, []string{"jira"},
		func(m map[string]interface{}) issueInfoSlc {
			issueInfo := make(issueInfos)
			sth := false
			for _, v := range [...]string{
				IssueinfoStrLink, IssueinfoStrMatch} {
				if m[v] == nil {
					eztools.LogPrint("NOTHING got intead of string for " +
						v + "!")
					continue
				}
				if retS, ok := m[v].(string); !ok {
					eztools.LogPrint(reflect.TypeOf(m[v]).String() +
						" got instead of string for " + v + "!")
				} else {
					issueInfo[v] = retS
					sth = true
				}
			}
			if !sth {
				return nil
			}
			return issueInfo.ToSlc()
		}), nil
}
