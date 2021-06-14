package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/bon-ami/eztools"
)

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

// gerritParseIssuesOrReviews parses body from gerrit responses into
// []issueInfos
/*
param:
	m	body
	issues	results are appended to this
	strs	keywords to parse
	issue	partially parsed fields, usually for looping only
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
	for i := 0; i < IssueinfoIndMax; i++ {
		if len(strs[i]) < 1 {
			continue
		}
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
		if i == IssueinfoIndSubmittable &&
			strs[i] == IssueinfoStrSubmittable {
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
						IssueinfoStrSubmittable +
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

// param: issueInfo[ISSUEINFO_IND_ID] any ID acceptable
func gerritQuery1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, opt string) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/?q="
	return gerritGetDetails(svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+opt,
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
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	const RestAPIStr = "changes/?q="
	// +"&o=CURRENT_REVISION" to list a commit and *ALL* for all
	return gerritGetRevs(svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"&o=ALL_REVISIONS",
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
		for _, i := range []int{IssueinfoIndVerified,
			IssueinfoIndCodereview, IssueinfoIndScore} {
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
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "changes/"
	return gerritGetReviews(svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"/reviewers/",
		svr.Magic, authInfo)
}

func gerritGetIssuesWtOwner(svr *svrs, authInfo eztools.AuthInfo,
	status string, issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	var urlAffix string
	strs := [...][2]string{
		{"status:", status},
		{"branch:", issueInfo[IssueinfoIndBranch]},
		{"owner:", issueInfo[IssueinfoIndID]}}
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
	issueInfo[IssueinfoIndID] = authInfo.User
	return gerritGetIssuesWtOwner(svr, authInfo, "open", issueInfo)
}

func gerritRebase(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return gerritActOn1WtAnyID(svr, authInfo, issueInfo, nil, "/rebase")
}

func gerritMerge(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return loopIssues(svr, issueInfo, func(issueInfo issueInfos) (issueInfos, error) {
		issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
		if err != nil {
			return issueInfo, err
		}
		// check mergable only, without submittable
		inf, err := gerritDetail(svr, authInfo, issueInfo)
		if err != nil {
			return issueInfo, err
		}
		if len(inf) < 1 {
			err = eztools.ErrNoValidResults
			return issueInfo, err
		}
		if inf[0][IssueinfoIndSubmittable] == "true" {
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
	if len(issueInfo[IssueinfoIndID]) < 1 ||
		len(issueInfo[IssueinfoIndBranch]) < 1 ||
		len(issueInfo[IssueinfoIndHead]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	if eztools.Debugging && !uiSilent {
		if !eztools.ChkCfmNPrompt("continue to cherrypick "+
			issueInfo[IssueinfoIndHead]+
			" from "+issueInfo[IssueinfoIndID]+
			" to "+issueInfo[IssueinfoIndBranch], "n") {
			return nil, nil
		}
		eztools.Log("to cheerypick " +
			issueInfo[IssueinfoIndHead] +
			" from " + issueInfo[IssueinfoIndID] +
			" to " + issueInfo[IssueinfoIndBranch])
	}
	const RestAPIStr = "changes/"
	jsonValue, _ := json.Marshal(map[string]string{
		//"message": "testing", // if this is a must, I have to read original submit message
		"destination": issueInfo[IssueinfoIndBranch]})
	bodyMap, err := restMap("POST", svr.URL+
		RestAPIStr+issueInfo[IssueinfoIndID]+
		"/revisions/"+issueInfo[IssueinfoIndHead]+
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
					issueInfo[IssueinfoIndID] + " (" +
					issueInfo[IssueinfoIndID] + ")")
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
	branch := issueInfo[IssueinfoIndBranch]
	f := func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos,
		res []issueInfos) []issueInfos {
		issueInfo[IssueinfoIndBranch] = branch
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
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return issues, eztools.ErrInvalidInput
	}
	if eztools.Debugging && !uiSilent {
		if !eztools.ChkCfmNPrompt(action+" "+
			issueInfo[IssueinfoIndID], "n") {
			return nil, nil
		}
	}
	const RestAPIStr = "changes/"
	bodyMap, err := restMap("POST", svr.URL+
		RestAPIStr+issueInfo[IssueinfoIndID]+action,
		authInfo, nil, svr.Magic)
	return gerritParseIssuesOrReviews(bodyMap, issues, issueInfoTxt, nil),
		err
}

func gerritScore(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
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
	if len(infWtRev[IssueinfoIndHead]) < 1 {
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
		/*// check whether Manual-Testing exists
		inf, _, err = gerritReviews2Scores(svr, authInfo, infWtRev)
		if err != nil {
			return inf, err
		}
		if len(inf) > 0 {
			if len(inf[0][ISSUEINFO_IND_MANUALTEST]) > 0 {
				map4Marshal[ISSUEINFO_STR_MANUALTEST] = 1
			}
			} else {
			eztools.LogPrint("NO review info found!")
			return inf, eztools.ErrNoValidResults
		}*/

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
				infWtRev[IssueinfoIndID], "n") {
				return nil, nil
			}
		}
		body, err := restSth("POST", svr.URL+RestAPIStr+
			infWtRev[IssueinfoIndID]+"/revisions/"+
			infWtRev[IssueinfoIndHead]+"/review",
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
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	var (
		err                          error
		inf                          []issueInfos
		scores                       scoreInfos
		debugVeri, scored, elsewhere bool
		submitType                   string
	)
	if eztools.Debugging && eztools.Verbose > 1 {
		debugVeri = true
	}
	eztools.ShowStr("waiting for issue to be submittable.")
	return loopIssues(svr, issueInfo, func(issueInfo issueInfos) (issueInfos, error) {
		for err == nil {
			inf, err = gerritDetail(svr, authInfo, issueInfo)
			if err != nil {
				break
			}
			if len(inf) < 1 {
				err = eztools.ErrNoValidResults
				break
			}
			if inf[0][IssueinfoIndSubmittable] == "true" {
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
				submitType = inf[0][IssueinfoIndSubmitType]
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
				eztools.Log("Verified=" + strconv.Itoa(scores[IssueinfoIndVerified]))
				// MERGE_IF_NECESSARY/FAST_FORWARD_ONLY
				eztools.Log(IssueinfoStrSubmitType + "=" +
					submitType)
				debugVeri = false
			}
			if scores[IssueinfoIndCodereview] < 2 ||
				(len(svr.Score) > 0 && scores[IssueinfoIndScore] < 1) ||
				scores[IssueinfoIndVerified] < 1 {
				if scored {
					if !elsewhere && scores[IssueinfoIndVerified] > 0 {
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
			}

			time.Sleep(5 * time.Second)
			eztools.ShowStr(".")
		}
		eztools.ShowStrln("")
		if err != nil {
			return issueInfo, err
		}
		_, err = gerritMerge(svr, authInfo, issueInfo)
		// TODO: check returned slice
		return issueInfo, err
	})
}
