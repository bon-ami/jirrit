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

func gerritRest4Maps(method, url, magic string,
	authInfo eztools.AuthInfo, fun func(map[string]interface{},
		issueInfoSlc) issueInfoSlc) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	body, err := restSth(method, url, authInfo, nil, magic)
	if err != nil || body == nil {
		return nil, err
	}
	issues := make(issueInfoSlc, 0)
	switch body.(type) {
	case []interface{}:
		bodySlc := body.([]interface{})
		if len(bodySlc) < 1 {
			return nil, err
		}
		for _, v := range bodySlc {
			m, ok := v.(map[string]interface{})
			if !ok {
				LogTypeErr(v, "map string to interface!")
				continue
			}
			issues = fun(m, issues)
		}
	case map[string]interface{}:
		issues = fun(body.(map[string]interface{}), issues)
	default:
		LogTypeErr(body, "slice of or map string to, interface!")
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
		Log(true, true, eztools.GetCaller(1))
	}
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
		if m[str1] == nil {
			if eztools.Debugging && eztools.Verbose > 2 {
				eztools.ShowStrln("unmatching " + str1)
			}
			continue
		}
		switch m[str1].(type) {
		case map[string]interface{}:
			mp := m[str1].(map[string]interface{})
			gerritParseIssuesOrReviews(mp, issues, strs, issue)
		case string:
			str := m[str1].(string)
			if eztools.Debugging && eztools.Verbose > 2 {
				eztools.ShowStrln("matching " +
					str1 + " <- " + str)
			}
			issue[str1] = str
		case float64:
			retF := m[str1].(float64)
			retS := strconv.FormatFloat(retF, 'f', 0, 64)
			issue[str1] = retS
		case bool:
			switch m[str1].(bool) {
			case true:
				issue[str1] = "true"
			case false:
				issue[str1] = "false"
			}
			if eztools.Debugging && eztools.Verbose > 2 {
				eztools.ShowStrln("matched " +
					str1 + "=" + issue[str1])
			}
		default:
			LogTypeErr(m[str1], "unknown type")
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
		Log(false, false, "non string map in author "+
			reflect.TypeOf(i).String())
		return issues
	}
	ni := m[IssueinfoStrName]
	if ni == nil {
		Log(false, false, "no name in author", m)
		return issues
	}
	ns, ok := ni.(string)
	if !ok {
		Log(false, false, "not string of name in author "+
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
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	return gerritRest4Maps(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseIssuesOrReviews(m, issues, reviewInfoTxt, nil)
		})
}

func gerritGetDetails(url, magic string, authInfo eztools.AuthInfo) (
	issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	return gerritRest4Maps(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseIssuesOrReviews(m, issues, issueDetailsTxt, nil)
		})
}

func gerritGetHistory(url, magic string, authInfo eztools.AuthInfo) (
	issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	return gerritRest4Maps(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			i := m["messages"]
			if i == nil {
				Log(false, false, "no history")
				return nil
			}
			a, ok := i.([]interface{})
			if !ok {
				LogTypeErr(i, "slice")
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
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	return gerritRest4Maps(eztools.METHOD_GET, url,
		magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseIssuesOrReviews(m, issues, issueInfoTxt, nil)
		})
}

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
func gerritQuery1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, opt string) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	const RestAPIStr = "changes/?q="
	return gerritGetDetails(svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+opt,
		svr.Magic, authInfo)
}

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
// TODO: remove it if not used
func gerritAnyID2ID(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) {
	if len(issueInfo[IssueinfoStrID]) == 0 {
		return
	}
	inf, err := gerritQuery1(svr, authInfo, issueInfo, "")
	if err != nil {
		return
	}
	if len(inf) != 1 {
		return
	}
	issueInfo[IssueinfoStrID] = inf[0][IssueinfoStrID]
}

func gerritRevs(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	f := func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos, res issueInfoSlc) issueInfoSlc {
		return append(res, issueInfo)
	}
	return gerritProcRevLoopMyOpen(svr, authInfo,
		issueInfo, f)
}

func gerritRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/?q="
	// +"&o=CURRENT_REVISION" to list a commit and *ALL* for all
	ret, err := gerritRest4Maps(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"&o=CURRENT_REVISION&o=DOWNLOAD_COMMANDS",
		svr.Magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			issues = gerritParseIssuesOrReviews(m,
				issues, issueRevsTxt, nil)
			var (
				rev string
				ok  bool
			)
			for _, issue1 := range issues {
				if rev, ok = issue1[IssueinfoStrRevCur]; ok {
					break
				}
			}
			infs := gerritParseRecursively(m,
				[]string{"revisions", rev},
				func(body map[string]interface{}) issueInfoSlc {
					dlds := gerritParseRecursively(m,
						[]string{"fetch", "ssh", "commands"},
						func(body map[string]interface{}) issueInfoSlc {
							retI := body[IssueinfoStrCherry]
							if retI == nil {
								Log(stdOutput, false, "NOTHING got intead of string!")
								return nil
							}
							if retS, ok := retI.(string); ok {
								return issueInfos{IssueinfoStrCherry: retS}.ToSlc()
							}
							LogTypeErr(retI, "string")
							return nil
						})
					retI, ok := body[IssueinfoStr_Nmb]
					if !ok || retI == nil {
						return dlds
					}
					if retF, ok := retI.(float64); ok {
						retS := strconv.FormatFloat(retF, 'f', 0, 64)
						if len(dlds) != 1 {
							Log(true, false, "Invalid number of downloads!")
							dlds = append(dlds, issueInfos{IssueinfoStrNmb: retS})
						} else {
							dlds[0][IssueinfoStrNmb] = retS
						}
					} else {
						LogTypeErr(retI, "float64")
					}
					return dlds
				})

			if len(issues) != 1 || len(infs) != 1 {
				Log(true, false, "Invalid number of revision/downloads!")
				issues = append(issues, infs...)
			} else {
				issues[0][IssueinfoStrNmb] =
					infs[0][IssueinfoStrNmb]
				issues[0][IssueinfoStrCherry] =
					infs[0][IssueinfoStrCherry]
			}
			return issues
		})
	if err != nil {
		return nil, err
	}
	switch len(ret) {
	case 1:
		if len(ret[0][IssueinfoStrRevCur]) < 1 {
			Log(true, false, "NO revision found!")
			err = eztools.ErrNoValidResults
		}
	default:
		Log(true, false, "NO single revision info found!", ret)
		err = eztools.ErrNoValidResults
	}
	return ret, err
}

func gerritDetailOnCurrRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	inf, err := gerritQuery1(svr, authInfo, issueInfo, "&o=CURRENT_REVISION")
	if err != nil {
		return inf, err
	}
	if len(inf) < 1 {
		return nil, eztools.ErrAccess
	}
	switch inf[0][IssueinfoStrState] {
	case IssueinfoStrMerged:
		return inf, nil
	}
	const RestAPIStr = "changes/"
	if more, err := gerritRest4Maps(eztools.METHOD_GET, svr.URL+RestAPIStr+
		inf[0][IssueinfoStrID]+"/revisions/current/mergeable",
		svr.Magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseIssuesOrReviews(m, issues, []string{IssueinfoStrMergeable}, nil)
		}); err == nil {
		if len(more) == 1 {
			inf[0][IssueinfoStrMergeable] = more[0][IssueinfoStrMergeable]
		}
	}
	if more, err := gerritRest4Maps(eztools.METHOD_GET, svr.URL+RestAPIStr+
		inf[0][IssueinfoStrID]+"/revisions/current/actions",
		svr.Magic, authInfo,
		func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			ret := "false"
			if _, ok := m[IssueinfoStrSubmit]; ok {
				ret = "true"
			}
			return issueInfos{IssueinfoStrSubmittable: ret}.ToSlc()
		}); err == nil {
		if len(more) == 1 && len(more[0][IssueinfoStrSubmittable]) > 0 {
			inf[0][IssueinfoStrSubmittable] = more[0][IssueinfoStrSubmittable]
		}
	}
	return inf, err
}

func gerritHistory(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
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
		LogTypeErr(body,
			"slice of or map string to, interface!")
		return
	}
	labels := bodyMap[IssueinfoStrLabels]
	if labels == nil {
		return
	}
	labelMap, ok := labels.(map[string]interface{})
	if !ok {
		LogTypeErr(labels, "map string to interface")
		return
	}
	scores = make([]scores2Marshal, 0)
	rejected = make(map[string]struct{})
	for labelName, label1 := range labelMap {
		label, ok := label1.(map[string]interface{})
		if !ok {
			LogTypeErr(label1, "map string to interface")
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
				Log(false, false, labelName + " already rejected.")
			}*/
		}
		values := label["values"]
		valueMap, ok := values.(map[string]interface{})
		if !ok {
			LogTypeErr(values,
				"map string to interface for "+
					labelName+"!")
			return nil, nil, nil
		}
		high := 0
		for v := range valueMap {
			i, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				Log(true, false, v+
					" got instead of int for "+
					labelName, err)
				continue
			}
			if i > high {
				high = i
			}
		}
		if high <= 0 {
			Log(true, false, "NO score choices found for "+
				labelName+"!")
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
			LogTypeErr(v, "map string to interface")
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
					LogTypeErr(m[str], "string for "+str)
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
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	useInputOrPrompt(svr, issueInfo, IssueinfoStrRevCur)
	if len(issueInfo[IssueinfoStrSummary]) < 1 {
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
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/?q="
	return gerritRest4Maps(eztools.METHOD_GET,
		svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID]+
			"&o=CURRENT_REVISION&o=CURRENT_FILES",
		svr.Magic, authInfo, func(m map[string]interface{}, issues issueInfoSlc) issueInfoSlc {
			return gerritParseRecursively(m, []string{"files"}, gerritParseFiles)
		})
}

// gerritParseRecursively loops into the map, returning all results,
// invoking func with the deepest found matches to name of map[string]interface{} into str.
func gerritParseRecursively(m map[string]interface{}, str []string,
	fun func(body map[string]interface{}) issueInfoSlc) (
	issues issueInfoSlc) {
	if m[str[0]] != nil {
		m1 := m
		for _, v := range str {
			fm, ok := m1[v].(map[string]interface{})
			if !ok {
				LogTypeErr(m1[v],
					"map string to interface for "+v)
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
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/"
	return gerritGetReviews(svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/reviewers/",
		svr.Magic, authInfo)
}

func gerritGetIssuesWtOwner(svr *svrs, authInfo eztools.AuthInfo,
	status string, issueInfo issueInfos) (issueInfoSlc, error) {
	useInputOrPromptStr(svr, issueInfo,
		IssueinfoStrID, IssueinfoStrAssignee)
	useInputOrPrompt(svr, issueInfo, IssueinfoStrBranch)
	useInputOrPrompt(svr, issueInfo, IssueinfoStrProj)
	useInputOrPromptStr(svr, issueInfo, IssueinfoStrVal,
		"more param(such as \"is:stared+has:star\")")
	var urlAffix string
	strs := [...][2]string{
		{"status:", status},
		{"project:", issueInfo[IssueinfoStrProj]},
		{"branch:", issueInfo[IssueinfoStrBranch]},
		{"owner:", issueInfo[IssueinfoStrID]},
		{"", issueInfo[IssueinfoStrVal]}}
	for _, v := range strs {
		if len(v[1]) > 0 {
			if len(urlAffix) > 0 {
				urlAffix += "+"
			}
			urlAffix += v[0] + v[1]
		}
	}
	if len(urlAffix) < 1 {
		return nil, eztools.ErrInvalidInput
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
	defer func() {
		issueInfo[IssueinfoStrID] = ""
	}()
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
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
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
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	useInputOrPromptStr(svr, issueInfo, IssueinfoStrRevCur,
		"revision(empty for current)")
	useInputOrPrompt(svr, issueInfo, IssueinfoStrBranch)
	if len(issueInfo[IssueinfoStrBranch]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	if len(issueInfo[IssueinfoStrRevCur]) < 1 {
		inf, err := gerritRev(svr, authInfo, issueInfo)
		if err != nil {
			return nil, err
		}
		// should be only one or same among all
		issueInfo[IssueinfoStrRevCur] = inf[0][IssueinfoStrRevCur]
	}
	return gerritPick1(svr, authInfo, issueInfo, nil)
}

func gerritPick1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, res issueInfoSlc) (issueInfoSlc, error) {
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	useInputOrPromptStr(svr, issueInfo, IssueinfoStrRevCur,
		"revision(empty for current)")
	useInputOrPrompt(svr, issueInfo, IssueinfoStrBranch)
	if len(issueInfo[IssueinfoStrBranch]) < 1 ||
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
		Log(false, false, "to cheerypick "+
			issueInfo[IssueinfoStrID]+
			" from "+issueInfo[IssueinfoStrRevCur]+
			" to "+issueInfo[IssueinfoStrBranch])
	}
	const RestAPIStr = "changes/"
	jsonValue, _ := json.Marshal(map[string]string{
		//"message": "testing", // if this is a must, I have to read original submit message
		"destination": issueInfo[IssueinfoStrBranch]})
	bodyMap, err := restMap(eztools.METHOD_POST, svr.URL+
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
			Log(true, false, err)
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
func gerritActOn1WtAnyID(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, issues issueInfoSlc,
	action string) (issueInfoSlc, error) {
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
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
	if eztools.Debugging && !uiSilent {
		if !eztools.ChkCfmNPrompt(action+" "+
			issueInfo[IssueinfoStrID], "n") {
			return nil, nil
		}
	}
	const RestAPIStr = "changes/"
	bodyMap, err := restMap(eztools.METHOD_POST, svr.URL+
		RestAPIStr+issueInfo[IssueinfoStrID]+action,
		authInfo, nil, svr.Magic)
	return gerritParseIssuesOrReviews(bodyMap, issues, issueInfoTxt, nil),
		err
}

func gerritScoreNGetRej(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (rejectedAft map[string]struct{},
	failed map[string]struct{}, err error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
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
			Log(false, false, err)
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
		Log(false, false, errMsg, score1)
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
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	inf, err = gerritRev(svr, authInfo, issueInfo)
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

func gerritRelated(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (inf issueInfoSlc, err error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	inf, err = gerritRev(svr, authInfo, issueInfo)
	if err != nil || len(inf) != 1 {
		return
	}
	const RestAPIStr = "changes/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+
		RestAPIStr+inf[0][IssueinfoStrID]+
		"/revisions/"+inf[0][IssueinfoStrRevCur]+"/related",
		authInfo, nil, svr.Magic)
	if err != nil {
		return
	}
	inf = parseIssues("changes", bodyMap,
		func(m map[string]interface{}) issueInfos {
			inf := make(issueInfos)
			inf[IssueinfoStrProj] = chkNSetIssueInfo(m[IssueinfoStrProj])
			inf[IssueinfoStr_Chg_Nmb] = chkNSetIssueInfo(m[IssueinfoStr_Chg_Nmb])
			inf[IssueinfoStr_Rev_Nmb] = chkNSetIssueInfo(m[IssueinfoStr_Rev_Nmb])
			if m[IssueinfoStrCommit] != nil {
				loopStringMap(m[IssueinfoStrCommit].(map[string]interface{}), "", nil,
					func(k string, v interface{}) bool {
						switch k {
						case IssueinfoStrCommit:
							inf[IssueinfoStrCommit] = v.(string)
						case IssueinfoStrParents:
							for _, parent1 := range v.([]any) {
								for name1, commit1 := range parent1.(map[string]any) {
									switch name1 {
									case IssueinfoStrCommit:
										if len(inf[IssueinfoStrParents]) > 0 {

											inf[IssueinfoStrParents] += ","
										}
										inf[IssueinfoStrParents] += commit1.(string)
									}
								}
							}
						case IssueinfoStrAuthor:
							for nam1, val1 := range v.(map[string]any) {
								switch nam1 {
								case IssueinfoStrName:
									inf[IssueinfoStrAuthor] = val1.(string)
								}
							}
						case IssueinfoStrSubject:
							inf[IssueinfoStrSubject] = v.(string)
						default:
							return false
						}
						return true
					})
			}
			return inf
		})
	return
}

func gerritFuncLoopSbOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, fun func(*svrs, eztools.AuthInfo,
		issueInfos) (issueInfoSlc, error)) (res issueInfoSlc, err error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
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
	if uiSilent || !eztools.Debugging {
		Log(true, false, "bulk wait and merge supported in interaction+debugging mode")
		return nil, eztools.ErrAccess
	}
	return gerritFuncLoopSbOpen(svr, authInfo, issueInfo, gerritWaitNMerge)
}

func gerritWaitNMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	if useInputOrPrompt4ID(svr, authInfo, issueInfo) {
		return nil, eztools.ErrInvalidInput
	}
	var ret issueInfoSlc
	cur, err := gerritDetailOnCurrRev(svr, authInfo, issueInfo)
	if err != nil || len(cur) < 1 {
		Log(true, false, "no details available for", issueInfo[IssueinfoStrID], err)
		return nil, eztools.ErrAccess
	}
	switch cur[0][IssueinfoStrState] {
	case IssueinfoStrMerged:
		return cur, nil
	}
	// match commit number=cur[IssueinfoStr_Nmb] or
	// commit id=gerritRev(svr, authInfo, inf[i])[IssueinfoStrID]
	inf, err := gerritRelated(svr, authInfo, issueInfo)
	if err != nil || len(inf) < 1 {
		Log(true, false, "no related commits for", issueInfo[IssueinfoStrID], err)
		return nil, eztools.ErrAccess
	}
	for i := len(inf) - 1; i >= 0; i-- {
		if inf[i][IssueinfoStr_Chg_Nmb] != cur[0][IssueinfoStr_Nmb] {
			continue
		}
		if len(inf[i][IssueinfoStrParents]) > 0 {
			if parent1, err := gerritWaitNMerge(svr, authInfo,
				issueInfos{IssueinfoStrID: inf[i][IssueinfoStrParents]}); err != nil {
				Log(true, false, "parent",
					inf[i][IssueinfoStrParents],
					"NOT merged")
				break
			} else {
				ret = append(ret, parent1...)
			}
		}
		if ret1, err1 := gerritWaitNMerge1(svr, authInfo,
			issueInfo); err1 != nil {
			err = err1
		} else {
			ret = append(ret, ret1...)
		}
		break
	}
	return ret, err
}

func gerritWaitNMerge1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	var (
		err               error
		inf, rev          issueInfoSlc
		scored            bool
		rejected, rejects map[string]struct{}
		scores            []scores2Marshal
	)
	eztools.ShowStr("waiting for ", issueInfo,
		" to be submittable/mergeable.")
	for err == nil {
		inf, err = gerritDetailOnCurrRev(svr, authInfo, issueInfo)
		if err != nil {
			break
		}
		if len(inf) < 1 {
			err = eztools.ErrNoValidResults
			break
		}
		switch inf[0][IssueinfoStrState] {
		case IssueinfoStrMerged:
			return inf, nil
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
			rev, err = gerritRev(svr, authInfo, issueInfo)
			if err != nil {
				return nil, err
			}
			rejected, _, err = gerritScoreNGetRej(svr, authInfo, rev[0])
			scored = true
			if err != nil {
				Log(false, false,
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
						Log(true, false, i +
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
								Log(true, false, j+
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
			Log(true, false, "Conflict to merge?")
		}
		return nil, err
	}
	// _, err = gerritMerge(svr, authInfo, issueInfo) not used because of redundant steps of checking
	return gerritActOn1(svr, authInfo, issueInfo, nil, "/submit")
}

func gerritListPrj(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if eztools.Debugging && eztools.Verbose > 1 {
		Log(true, true, eztools.GetCaller(1))
	}
	useInputOrPrompt(svr, issueInfo, IssueinfoStrProj)
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
					Log(stdOutput, false, "NOTHING got intead of string for "+
						v+"!")
					continue
				}
				if retS, ok := m[v].(string); !ok {
					LogTypeErr(m[v],
						"string for "+v+"!")
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
