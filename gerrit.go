package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"gitee.com/bon-ami/eztools"
)

func gerritGetIssuesOrReviews(method, url, magic string,
	authInfo eztools.AuthInfo, fun func(map[string]interface{},
		[]issueInfos) []issueInfos) ([]issueInfos, error) {
	bodySlc, err := restSlc(method, url, authInfo, nil, magic)
	if err != nil || nil == bodySlc || len(bodySlc) < 1 {
		return nil, err
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

// gerritParseIssuesOrReviews parses body from gerrit responses into
// []issueInfos
/*
param:
	m	body
	issues	results are appended to this
	strs	keywords to parse. should be on the first level
	issue	partially parsed fields, usually for looping only
*/
func gerritParseIssuesOrReviews(m map[string]interface{},
	issues []issueInfos, strs []string,
	issue issueInfos) []issueInfos {
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
	return []issueInfos{issue}
}

func gerritParseFileList(m map[string]interface{},
	issues []issueInfos, issue *issueInfos) []issueInfos {
	return nil
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

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
func gerritQuery1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, opt string) ([]issueInfos, error) {
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
		func(m map[string]interface{}, issues []issueInfos) []issueInfos {
			issues = gerritParseIssuesOrReviews(m, issues, issueRevsTxt, nil)
			dlds := gerritParseRecursively(m, []string{"ssh", "commands"},
				func(body map[string]interface{}) []issueInfos {
					retI := body[IssueinfoStrCherry]
					if retI == nil {
						eztools.LogPrint("NOTHING got intead of string!")
						return nil
					}
					if retS, ok := retI.(string); ok {
						return []issueInfos{{IssueinfoStrMergeable: retS}}
					}
					eztools.LogPrint(reflect.TypeOf(retI).String() +
						" got instead of string!")
					return nil
				})

			if len(issues) != 1 || len(dlds) != 1 {
				eztools.LogPrint("Invalid number of revision/downloads!")
				for _, i := range dlds {
					issues = append(issues, i)
				}
			} else {
				issues[0][IssueinfoStrMergeable] = dlds[0][IssueinfoStrMergeable]
			}
			return issues
		})
}

func gerritDetailOnCurrRev(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritQuery1(svr, authInfo, issueInfo, "&o=CURRENT_REVISION")
}

func gerritDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	const RestAPIStr = "changes/"
	return gerritGetDetails(svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/detail",
		svr.Magic, authInfo)
}

// gerritReviews2Scores get all non-approved score items, including "" and "blocked"
func gerritReviews2Scores(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (inf []issueInfos, scores issueInfos, err error) {
	/*id, err := gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		eztools.LogErrPrint(err)
	}*/
	//inf, err = gerritReviews(svr, authInfo, issueInfo)
	inf, err = gerritDetail(svr, authInfo, issueInfo)
	if err != nil {
		return
	}
	for _ /*j*/, inf1 := range inf {
		for _, i := range []string{IssueinfoStrVerified,
			IssueinfoStrCodereview} {
			if len(inf1[i]) > 0 {
				if inf1[i] == " 0" {
					// not parsable bo Atoi
					continue
				}
				scores[i] += inf1[i]
			}
		}
		//inf[j][ISSUEINFO_IND_ID] = id[ISSUEINFO_IND_ID]
	}
	return
}

func gerritParseFiles(body map[string]interface{}) []issueInfos {
	issues := make([]issueInfos, 0)
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
			name, value, indx string
		}
		fldSlc := [...]flds{
			{name: IssueinfoStrBin,
				indx: IssueinfoStrBin},
			{name: IssueinfoStrState,
				indx: IssueinfoStrState},
			{name: IssueinfoStrOldPath,
				indx: IssueinfoStrOldPath},
		}

		for i, v := range fldSlc {
			fldSlc[i].value, ok = func(m map[string]interface{}, str string) (string, bool) {
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
				inf[fld1.indx] = fld1.value
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
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 || len(issueInfo[IssueinfoStrHead]) < 1 {
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
	issueInfo issueInfos) ([]issueInfos, error) {
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
		svr.Magic, authInfo, func(m map[string]interface{}, issues []issueInfos) []issueInfos {
			return gerritParseRecursively(m, []string{"files"}, gerritParseFiles)
		})
}

// gerritParseRecursively loops into the map, returning all results,
// invoking func with the deepest found matches to name of map[string]interface{} into str.
func gerritParseRecursively(m map[string]interface{}, str []string, // issues []issueInfos,
	fun func(body map[string]interface{}) []issueInfos) (issues []issueInfos) {
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
		for _, i := range gerritParseRecursively(fm, str, fun) {
			issues = append(issues, i)
		}
	}
	return issues
}

// no ID will return, since not in replies
func gerritReviews(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/"
	return gerritGetReviews(svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/reviewers/",
		svr.Magic, authInfo)
}

func gerritGetIssuesWtOwner(svr *svrs, authInfo eztools.AuthInfo,
	status string, issueInfo issueInfos) ([]issueInfos, error) {
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
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritGetIssuesWtOwner(svr, authInfo, "merged", issueInfo)
}

func gerritAllOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	eztools.ShowStrln("This may take quite a while...")
	const RestAPIStr = "changes/"
	return gerritGetIssues(svr.URL+RestAPIStr, svr.Magic, authInfo)
}

func gerritSbOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritGetIssuesWtOwner(svr, authInfo, "open", issueInfo)
}

func gerritMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	issueInfo[IssueinfoStrID] = authInfo.User
	return gerritGetIssuesWtOwner(svr, authInfo, "open", issueInfo)
}

func gerritRebase(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/rebase")
}

func gerritRevert(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/revert")
}

func gerritMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return loopIssues(svr, issueInfo, func(issueInfo issueInfos) (issueInfos, error) {
		issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
		if err != nil {
			return issueInfo, err
		}
		// check mergable only, without submittable
		inf, err := gerritDetailOnCurrRev(svr, authInfo, issueInfo)
		if err != nil {
			return issueInfo, err
		}
		if len(inf) < 1 {
			err = eztools.ErrNoValidResults
			return issueInfo, err
		}
		if inf[0][IssueinfoStrSubmittable] != "false" &&
			inf[0][IssueinfoStrMergeable] != "false" {
			// either empty(=not supported or already merged) or true will do
			_, err = gerritActOn1(svr, authInfo, issueInfo, nil, "/submit")
			// TODO: check returned slice
			return issueInfo, err
		}
		return issueInfo, eztools.ErrNoValidResults
	})
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
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrBranch]) < 1 ||
		len(issueInfo[IssueinfoStrRevCur]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	if eztools.Debugging && !uiSilent {
		if !eztools.ChkCfmNPrompt("continue to cherrypick "+
			issueInfo[IssueinfoStrID]+
			" from "+issueInfo[IssueinfoStrRevCur]+
			" to "+issueInfo[IssueinfoStrBranch], "n") {
			return nil, nil
		}
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
	issueInfo issueInfos) ([]issueInfos, error) {
	branch := issueInfo[IssueinfoStrBranch]
	f := func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos,
		res []issueInfos) []issueInfos {
		issueInfo[IssueinfoStrBranch] = branch
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
	return loopIssues(svr, issueInfo, func(issueInfo issueInfos) (issueInfos, error) {
		issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
		if err != nil {
			return issueInfo, err
		}
		_, err = gerritActOn1(svr, authInfo, issueInfo, nil, action)
		// TODO: check returned slice
		return issueInfo, err
	})
}

// gerritActOn1 POST changes/ID/action
// param: issueInfo[ISSUEINFO_IND_ID] unique ID
// TODO: should returned slice mean anything when input slice is nil?
//	Currently all discarded
func gerritActOn1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, issues []issueInfos,
	action string) ([]issueInfos, error) {
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

func gerritScore(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
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
	if len(infWtRev[IssueinfoStrHead]) < 1 {
		eztools.LogPrint("NO revision found!")
		return inf, eztools.ErrNoValidResults
	}

	type map2Marshal map[string]int
	map4Marshal := map2Marshal{IssueinfoStrCodereview: 2}
	if len(svr.Score) > 0 {
		map4Marshal[svr.Score] = 1
	}
	map4Marshal[IssueinfoStrVerified] = 1
	const RestAPIStr = "changes/"
	for {
		var jsonValue []byte
		jsonValue, err = json.Marshal(map[string]map2Marshal{
			"labels": map4Marshal})
		if err != nil {
			eztools.LogErr(err)
			return nil, err
		}
		//eztools.ShowStrln(string(jsonValue))
		if eztools.Debugging && !uiSilent {
			if !eztools.ChkCfmNPrompt("continue to +2/1 to "+
				infWtRev[IssueinfoStrID], "n") {
				return nil, nil
			}
		}
		body, err := restSth("POST", svr.URL+RestAPIStr+
			infWtRev[IssueinfoStrID]+"/revisions/"+
			infWtRev[IssueinfoStrHead]+"/review",
			authInfo, bytes.NewBuffer(jsonValue), svr.Magic)
		// response only contain scores for a success, so it is not parsed
		if err == nil {
			break
		}
		eztools.LogErrWtInfo("failed to", err)
		if body != nil {
			bodyBytes, ok := body.([]byte)
			if ok {
				if map4Marshal[IssueinfoStrVerified] == 0 {
					eztools.LogPrint(bodyBytes)
					break
				}
				if bytes.Contains(bodyBytes, []byte("Verified")) &&
					bytes.Contains(bodyBytes, []byte("restricted")) {
					delete(map4Marshal, IssueinfoStrVerified)
					eztools.Log("Retrying to scrore without verify.")
					continue
				}
			}
		}
		break
	}
	return nil, err
}

func gerritFuncLoopSbOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, fun func(*svrs, eztools.AuthInfo,
		issueInfos) ([]issueInfos, error)) (res []issueInfos, err error) {
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
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritFuncLoopSbOpen(svr, authInfo, issueInfo, gerritWaitNMerge)
}

func gerritWaitNMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		// list ready commits
		inf, err := gerritMyOpen(svr, authInfo, issueInfo)
		if err == nil {
			var choices []string
			if uiSilent {
				defer noInteractionAllowed()
				return nil, eztools.ErrInvalidInput
			}
			for _, v := range inf {
				choices = append(choices,
					v[IssueinfoStrHead]+" <-> "+
						v[IssueinfoStrBranch])
			}
			i := eztools.ChooseStrings(choices)
			if i != eztools.InvalidID {
				issueInfo[IssueinfoStrID] = inf[i][IssueinfoStrID]
			}
		}
	}
	if len(issueInfo[IssueinfoStrID]) < 1 {
		useInputOrPrompt(issueInfo, IssueinfoStrID)
		if len(issueInfo[IssueinfoStrID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
	}
	var (
		err                          error
		inf                          []issueInfos
		scores                       issueInfos
		debugVeri, scored, elsewhere bool
		submitType                   string
	)
	if eztools.Debugging && eztools.Verbose > 1 {
		debugVeri = true
	}
	eztools.ShowStr("waiting for issue to be submittable/mergeable.")
	return loopIssues(svr, issueInfo, func(issueInfo issueInfos) (issueInfos, error) {
	READY2MERGE:
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
			switch inf[0][IssueinfoStrSubmittable] {
			case "true":
				// the only successful break of loop
				break READY2MERGE
			case "":
				//gerritDetails and check
				/*labels:map[
				  Code-Review:map[
				          approved:map  for OK
				          blocking:true for NG
				          rejected:map  also NG
				          values:map[ 0:No score +1:Looks good to me, but someone else must approve +2:Looks good to me, approved -1:I would prefer this is not merged as is -2:This shall not be merged]]*/
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
				submitType = inf[0][IssueinfoStrSubmitType]
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
				eztools.Log("Verified=" + scores[IssueinfoStrVerified])
				// MERGE_IF_NECESSARY/FAST_FORWARD_ONLY
				eztools.Log(IssueinfoStrSubmitType + "=" +
					submitType)
				debugVeri = false
			}
			/*if scores[IssueinfoStrCodereview] < 2 ||
			len(svr.Score) > 0 || scores[IssueinfoStrVerified] < 1 {*/
			if scored {
				if !elsewhere && len(scores[IssueinfoStrVerified]) > 0 {
					eztools.Log("failed to score non-verified field")
					elsewhere = true
				}
			} else {
				_, err = gerritScore(svr, authInfo, inf[0])
				if err != nil {
					eztools.LogErrPrintWtInfo(
						"failed to score and wait for it to be scored by elsewhere.",
						err)
				}
				scored = true
			}
			//}

			time.Sleep(intGerritMerge * time.Second)
			eztools.ShowStr(".")
		}
		eztools.ShowStrln("")
		if err != nil {
			if err == eztools.ErrOutOfBound {
				eztools.LogPrint("Conflict to merge?")
			}
			return issueInfo, err
		}
		// _, err = gerritMerge(svr, authInfo, issueInfo) not used because of redundant steps of checking
		_, err = gerritActOn1(svr, authInfo, issueInfo, nil, "/submit")
		// TODO: check returned slice
		return issueInfo, err
	})
}

func gerritListPrj(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
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
		func(m map[string]interface{}) []issueInfos {
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
			return []issueInfos{issueInfo}
		}), nil
}
