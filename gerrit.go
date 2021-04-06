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
			ISSUEINFO_IND_CODEREVIEW, ISSUEINFO_IND_SCORE} {
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

func gerritCheckIDNGetIssues(svr *svrs, authInfo eztools.AuthInfo,
	url string, issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[ISSUEINFO_IND_BRANCH]) < 1 ||
		len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	return gerritGetIssues(svr.URL+url, svr.Magic, authInfo)
}

func gerritSbBraMerged(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	const REST_API_STR = "changes/?q="
	return gerritCheckIDNGetIssues(svr, authInfo, REST_API_STR+
		"status:merged+branch:"+issueInfo[ISSUEINFO_IND_BRANCH]+
		"+owner:"+issueInfo[ISSUEINFO_IND_ID], issueInfo)
}

func gerritAllOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	eztools.ShowStrln("This may take quite a while...")
	const REST_API_STR = "changes/"
	return gerritGetIssues(svr.URL+REST_API_STR, svr.Magic, authInfo)
}

func gerritSbOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	const REST_API_STR = "changes/?q="
	return gerritCheckIDNGetIssues(svr, authInfo, REST_API_STR+
		"status:open+branch:"+issueInfo[ISSUEINFO_IND_BRANCH]+
		"+owner:"+issueInfo[ISSUEINFO_IND_ID], issueInfo)
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
	return loopIssues(issueInfo, func(issueInfo issueInfos) ([]issueInfos, error) {
		issueInfo, err := gerritAnyID2ID(svr, authInfo, issueInfo)
		if err != nil {
			return nil, err
		}
		return gerritActOn1(svr, authInfo, issueInfo, nil, action)
	})
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

	type map2Marshal map[string]int
	map4Marshal := map2Marshal{ISSUEINFO_STR_CODEREVIEW: 2}
	if len(svr.Score) > 0 {
		map4Marshal[svr.Score] = 1
	}
	map4Marshal[ISSUEINFO_STR_VERIFIED] = 1
	const REST_API_STR = "changes/"
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
		if eztools.Debugging {
			if !eztools.ChkCfmNPrompt("continue to +2/1 to "+
				infWtRev[ISSUEINFO_IND_ID], "n") {
				return nil, nil
			}
		}
		body, err := restSth("POST", svr.URL+REST_API_STR+
			infWtRev[ISSUEINFO_IND_ID]+"/revisions/"+
			infWtRev[ISSUEINFO_IND_HEAD]+"/review",
			authInfo, bytes.NewBuffer(jsonValue), svr.Magic)
		// response only contain scores for a success, so it is not parsed
		if err == nil {
			break
		}
		eztools.LogErrWtInfo("failed to", err)
		if body != nil {
			bodyBytes, ok := body.([]byte)
			if ok {
				if map4Marshal[ISSUEINFO_STR_VERIFIED] == 0 {
					eztools.LogPrint(bodyBytes)
					break
				}
				if bytes.Contains(bodyBytes, []byte("Verified")) &&
					bytes.Contains(bodyBytes, []byte("restricted")) {
					delete(map4Marshal, ISSUEINFO_STR_VERIFIED)
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
	if len(issueInfo[ISSUEINFO_IND_ID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	var (
		err                          error
		inf                          []issueInfos
		scores                       scoreInfos
		debugVeri, scored, elsewhere bool
		submit_type                  string
	)
	if eztools.Debugging && eztools.Verbose > 1 {
		debugVeri = true
	}
	eztools.ShowStr("waiting for issue to be mergable.")
	return loopIssues(issueInfo, func(issueInfo issueInfos) ([]issueInfos, error) {
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
				(len(svr.Score) > 0 && scores[ISSUEINFO_IND_SCORE] < 1) ||
				scores[ISSUEINFO_IND_VERIFIED] < 1 {
				if scored {
					if !elsewhere && scores[ISSUEINFO_IND_VERIFIED] > 0 {
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
			return nil, err
		}
		return gerritMerge(svr, authInfo, issueInfo)
	})
}
