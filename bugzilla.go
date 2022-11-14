package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gitee.com/bon-ami/eztools/v4"
)

const RestAPIBZStr = "rest/bug/"

// parseTypicalBZNum not used yet
//
//	Return values
//	whether input is in exact x-0 or -0 format.
//		in case of -0, if previous project (x part) found, it is taken.
//		otherwise, false is returned.
//	the non digit part. this is saved as project.
//	the digit part
func parseTypicalBZNum(svr *svrs, num string) (nonDigit,
	digit string, changes, parsed bool) {
	if len(num) < 1 {
		return "", "", false, false
	}
	re := regexp.MustCompile(`^[^-,]+[-][\d]+$`)
	//eztools.ShowStrln("parsing " + num + " 2 typical JIRA")
	pref := re.FindStringSubmatch(num)
	if pref != nil { // "A-1"
		parts := strings.Split(pref[0], typicalJiraSeparator)
		if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 0 {
			saveProj(svr, parts[0])
			return parts[0] + typicalJiraSeparator, parts[1], false, true
		}
	} else {
		if len(svr.Proj) > 0 { // "-1", "1"
			re = regexp.MustCompile(`^[-][\d]+$`)
			pref = re.FindStringSubmatch(num)
			if pref != nil {
				parts := strings.Split(pref[0], typicalJiraSeparator)
				// parts[0]=""
				if len(parts) == 2 && len(parts[1]) > 0 {
					if eztools.Debugging && eztools.Verbose > 1 {
						eztools.ShowStrln("Auto changing to " +
							svr.Proj + typicalJiraSeparator + parts[1])
					}
					return svr.Proj + typicalJiraSeparator, parts[1], true, true
				}
			}
		} // "A-1,B-2" not handled
	}
	return "", "", false, false
}

// bugzillaTransfer transfer an issue to someone else, and additionally to a component
func bugzillaTransfer(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrSummary]) < 1 {
		return nil, eztools.ErrInvalidInput
	}

	var (
		jsonStr []byte
		err     error
	)
	updateMap := map[string]string{
		"assigned_to": issueInfo[IssueinfoStrSummary]}
	if len(issueInfo[IssueinfoStrComments]) > 0 {
		updateMap["component"] = issueInfo[IssueinfoStrComments]
	}
	jsonStr, err = json.Marshal(updateMap)
	if err != nil {
		return nil, err
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log(issueInfo[IssueinfoStrID] + " in transition")
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(eztools.METHOD_PUT,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"?",
			"", authInfo),
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	return nil, err
}

func bugzillaChooseState(svr *svrs, issueInfo issueInfos,
	state string) string {
	resos := makeStates(svr, state)
	var reso string
	switch len(resos) {
	case 0:
		break
	case 1:
		reso = resos[0]
	default:
		var ok bool
		if reso, ok = issueInfo[IssueinfoStrLink]; ok {
			break
		}
		if indx, res := eztools.ChooseStrings(resos); indx != eztools.InvalidID {
			reso = res
		}
	}
	return reso
}

// bugzillaChooseTran uses user input or ask user to choose from a slice
//
//	Parameters:
//	string user input
//	a slice to choose from, if not input already
//	comment required matching the slice. must be of same size as the slice
//
// Return values: selected string and bool, and error
func bugzillaChooseTran(tranName string,
	tranNames []string, tranCmts []bool) (string, bool, error) {
	var (
		tranID     string
		tranCmtReq bool
	)
	if len(tranNames) > 0 {
		if len(tranName) > 0 {
			for i, v := range tranNames {
				//eztools.ShowStrln(v + "=" + tranName + "?")
				if tranName == v {
					return tranName, tranCmts[i], nil
				}
			}
			return tranID, false, eztools.ErrNoValidResults
		} else {
			if uiSilent {
				noInteractionAllowed()
				return "", false, eztools.ErrInvalidInput
			}
			eztools.ShowStrln(
				"There are following transitions available.")
			i, _ := eztools.ChooseStrings(tranNames)
			if i == eztools.InvalidID {
				return "", false, eztools.ErrInvalidInput
			}
			tranID = tranNames[i]
			tranCmtReq = tranCmts[i]
		}
	}
	return tranID, tranCmtReq, nil
}

// bugzillaTranExec transition issue {id} to state {tranID}
func bugzillaTranExec(svr *svrs, authInfo eztools.AuthInfo,
	id, cmt, tranID string, cmtReq bool, body any) (err error) {
	if body == nil {
		if cmtReq {
			var cmt string
			if len(cmt) < 1 {
				cmt = eztools.PromptStr(IssueinfoStrComments)
				if len(cmt) < 1 {
					return eztools.ErrInvalidInput
				}
			}
			body = map[string]string{"status": tranID, "comment": cmt}
		} else {
			body = map[string]string{"status": tranID}
		}
	}
	jsonStr, err := json.Marshal(body)
	if err != nil {
		return
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log(id + " in transition")
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(eztools.METHOD_PUT,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			id+"?", "", authInfo),
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	return
}

// bugzillaFuncNTran is transitions for reject & close
func bugzillaFuncNTran(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, steps []string,
	funcBody func(tranID string, tranCmtReq bool) any,
	funcPre func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos) error) (err error) {
	if funcPre != nil {
		if err = funcPre(svr, authInfo, issueInfo); err != nil {
			return err
		}

	}
	var (
		tranNames   []string
		tranCmtReqs []bool
		stt         string
	)
	for i, tran := range steps {
		if eztools.Debugging && eztools.Verbose > 1 {
			eztools.ShowStrln("Trying " + tran)
		}
		if tranNames == nil || len(tranNames) < 1 {
			stt, tranNames, tranCmtReqs, err =
				bugzillaGetTrans(svr, authInfo, issueInfo, stt)
			if err != nil {
				return err
			}
		}
		if tranNames == nil || len(tranNames) < 1 {
			eztools.LogPrint("NO available transitions")
			return eztools.ErrNoValidResults
		}
		tranID, tranCmtReq, err := bugzillaChooseTran(tran,
			tranNames, tranCmtReqs)
		if err != nil {
			if i == len(steps)-1 {
				// return error if the last step fails,
				// since it is a key one
				if err == eztools.ErrNoValidResults {
					eztools.LogPrint("No available transitions. Check permission!")
				}
				return err
			}
			continue
		}
		tranNames = nil
		if funcBody == nil {
			err = bugzillaTranExec(svr, authInfo,
				issueInfo[IssueinfoStrID], issueInfo[IssueinfoStrComments],
				tranID, tranCmtReq, nil)
		} else {
			err = bugzillaTranExec(svr, authInfo,
				issueInfo[IssueinfoStrID], issueInfo[IssueinfoStrComments],
				tranID, tranCmtReq, funcBody(tranID, tranCmtReq))
		}
		if err != nil {
			/*if err == errGram {
				jiraGetTransMustFlds(svr, authInfo,
					issueInfo[IssueinfoStrID])
			}*/
			return err
		}
		stt = tranID
	}
	return err
}

func bugzillaReqCmt(issueInfo issueInfos, must bool) string {
	cmt := issueInfo[IssueinfoStrComments]
	if must {
		if len(cmt) < 1 {
			cmt = eztools.PromptStr(IssueinfoStrComments)
		}
	}
	return cmt
}

// bugzillaReject rejects an issue
//
//	If there are multiple steps, and comment is provided,
//	it is added during all steps!
func bugzillaReject(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	Steps := makeStates(svr, StateTypeTranRej)
	if Steps == nil {
		eztools.LogPrint("No transitions configured for this server!")
		return nil, errCfg
	}
	reso := bugzillaChooseState(svr, issueInfo, StateTypeResolutionRej)
	var makeBody func(string, bool) any
	if len(reso) > 0 {
		makeBody = func(tranID string, cmtReq bool) any {
			return bugzillaBody4Tran(svr,
				issueInfo, reso, false, tranID, cmtReq)
		}
	}
	return nil, bugzillaFuncNTran(svr, authInfo, issueInfo, Steps,
		makeBody, nil)
}

func inputMultiple(cfg, ans []string) (ret string) {
	if cfg == nil {
		return
	}
	switch len(cfg) {
	case 1:
		if len(ans) < 1 { // prompt needed
			break
		}
		// use ans
		fallthrough
	case 0:
		if len(ans) > 0 { // use ans if cfg is < 2
			return strings.Join(ans, "")
		} else { // no config or ans
			return eztools.PromptStr("what to input")
		}
	}
	ansLen := len(ans)
	var ansI int
	for _, cfg1 := range cfg {
		if ansI < ansLen { // use ans
			ret += cfg1 + ans[ansI]
			ansI++
			continue
		}
		ret += cfg1
		ret += eztools.PromptStr("what to append to \"" + cfg1 + "\"")
	}
	for ; ansI < ansLen; ansI++ { // use ans
		ret += ans[ansI]
	}
	return
}

// bugzillaBody4Tran constructs body for transitions
// Parameters: reso should exist
func bugzillaBody4Tran(svr *svrs, issueInfo issueInfos, reso string, solutionNeeded bool,
	tranID string, cmtReq bool) any {
	var solu string
	if solutionNeeded {
		if len(paramS) > 0 {
			if str := inputMultiple(svr.Flds.Solution,
				paramS); len(str) > 0 {
				issueInfo[IssueinfoStrSolution] = str
			}
		}
	}
	solu = issueInfo[IssueinfoStrSolution]
	cmt := bugzillaReqCmt(issueInfo, cmtReq)
	if len(cmt) < 1 {
		ret := map[string]string{
			"status":     tranID,
			"resolution": reso,
		}
		if solutionNeeded {
			ret["cf_analysis_solution"] = solu
		}
		return ret
	}
	if !solutionNeeded {
		type tranJsons struct {
			Stt     string `json:"status"`
			Reso    string `json:"resolution"`
			Comment struct {
				Body string `json:"body"`
			} `json:"comment"`
		}
		var tranJSON tranJsons
		tranJSON.Stt = tranID
		tranJSON.Reso = reso
		tranJSON.Comment.Body = cmt
		return tranJSON
	}
	type tranJsons struct {
		Stt     string `json:"status"`
		Reso    string `json:"resolution"`
		Solu    string `json:"cf_analysis_solution"`
		Comment struct {
			Body string `json:"body"`
		} `json:"comment"`
	}
	var tranJSON tranJsons
	tranJSON.Stt = tranID
	tranJSON.Reso = reso
	tranJSON.Solu = solu
	tranJSON.Comment.Body = cmt
	return tranJSON
}

// bugzillaClose closes an issue
//
//	If there are multiple steps, and comment is provided,
//	it is added during all steps!
func bugzillaClose(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	Steps := makeStates(svr, StateTypeTranCls)
	if Steps == nil {
		eztools.LogPrint("No transitions configured for this server!")
		return nil, errCfg
	}
	reso := bugzillaChooseState(svr, issueInfo, StateTypeResolutionRes)
	var makeBody func(string, bool) any
	if len(reso) > 0 {
		makeBody = func(tranID string, cmtReq bool) any {
			return bugzillaBody4Tran(svr,
				issueInfo, reso, true, tranID, cmtReq)
		}
	}
	return nil, bugzillaFuncNTran(svr, authInfo, issueInfo, Steps, makeBody, nil)
}

// bugzillaTransition transitions an issue to a state
func bugzillaTransition(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, names, cmtReqs, err := bugzillaGetTrans(svr, authInfo,
		issueInfo, "")
	if err != nil {
		return nil, err
	}
	tranID, cmtReq, err :=
		bugzillaChooseTran(issueInfo[IssueinfoStrProj], names, cmtReqs)
	if err != nil {
		return nil, err
	}
	return nil, bugzillaTranExec(svr, authInfo,
		issueInfo[IssueinfoStrID], issueInfo[IssueinfoStrComments],
		tranID, cmtReq, nil)
}

// bugzillaLink links two issues
func bugzillaLink(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrLink]) < 1 ||
		issueInfo[IssueinfoStrLink] ==
			issueInfo[IssueinfoStrID] {
		return nil, eztools.ErrInvalidInput
	}
	// TODO: to choose from "blocks" or "depends_on"
	// TODO: to choose from "add", "remove" or "set"
	type blk struct {
		Add []int `json:"add"`
	}
	type tranJsons struct {
		Blocks blk `json:"blocks"`
	}
	var tranJSON tranJsons
	id, err := strconv.Atoi(issueInfo[IssueinfoStrLink])
	if err != nil {
		return nil, err
	}
	tranJSON.Blocks.Add = []int{id}
	jsonStr, err := json.Marshal(tranJSON)
	if err != nil {
		return nil, eztools.ErrOutOfBound
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log(issueInfo[IssueinfoStrID] + " in transition")
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(eztools.METHOD_PUT,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"?", "", authInfo),
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	return nil, err
}

// bugzillaAddComment adds a comment to an issue
func bugzillaAddComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrComments]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	inf, err := bugzillaAddComment1(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	return inf.ToSlc(), err
}

// bugzillaAddComment1 adds a comment to an issue, with no input checking
func bugzillaAddComment1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	jsonStr, err := json.Marshal(map[string]string{
		"comment": issueInfo[IssueinfoStrComments]})
	if err != nil {
		return nil, eztools.ErrOutOfBound
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log("commenting", issueInfo[IssueinfoStrID])
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(eztools.METHOD_POST,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"/comment?", "", authInfo),
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	return nil, err
}

// bugzillaComments lists comments of an issue
func bugzillaComments(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"/comment?",
			"", authInfo), authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return bugzillaParseComments(bodyMap), nil
}

// bugzillaGetTrans get transferable states
//
//	Return values:
//		current state
//		available states
//		whether comment required of a state
//		error
func bugzillaGetTrans(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, stt string) (string, []string, []bool, error) {
	if len(stt) < 1 {
		var ok bool
		slcInf, err := bugzillaDetail(svr, authInfo, issueInfo)
		if err != nil || slcInf == nil || len(slcInf) != 1 {
			return "", nil, nil, eztools.ErrOutOfBound
		}
		stt, ok = slcInf[0][IssueinfoStrState]
		if !ok || len(stt) < 1 {
			return "", nil, nil, eztools.ErrOutOfBound
		}
	}
	const RestAPIBZStr = "rest/field/bug/"
	bodyMap, err := restMap(eztools.METHOD_GET,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			"bug_status?", "", authInfo),
		authInfo, nil, svr.Magic)
	if err != nil {
		return stt, nil, nil, err
	}
	fldsAny, ok := bodyMap["fields"]
	if !ok {
		return stt, nil, nil, eztools.ErrOutOfBound
	}
	fldSlc, ok := fldsAny.([]any)
	if !ok {
		return stt, nil, nil, eztools.ErrOutOfBound
	}
	var (
		retStates []string
		retCmts   []bool
	)
	for _, fld1Any := range fldSlc {
		fld1Map, ok := fld1Any.(map[string]interface{})
		if !ok {
			return stt, nil, nil, eztools.ErrOutOfBound
		}
		/*var (
			name string
			chgMap map[string]any
			chg []string
		)*/
		loopStringMap(fld1Map, "values", nil, func(key string, val any) bool {
			valSlc, ok := val.([]any)
			if !ok {
				eztools.LogPrint(reflect.TypeOf(val).String() +
					" got instead of " +
					"[]interface{}")
				return false
			}
			for _, val1Any := range valSlc {
				val1Map, ok := val1Any.(map[string]any)
				if !ok {
					eztools.LogPrint(reflect.TypeOf(val1Any).String() +
						" got instead of " +
						"map[string]interface{}")
					continue
				}
				nmAny, ok := val1Map["name"]
				if !ok || nmAny == nil {
					continue
				}
				nmStr, ok := nmAny.(string)
				if !ok {
					eztools.LogPrint(reflect.TypeOf(nmAny).String() +
						" got instead of " +
						"string")
					continue
				}
				if nmStr != stt {
					continue
				}
				chgAny, ok := val1Map["can_change_to"]
				if !ok || chgAny == nil {
					continue
				}
				chgSlc, ok := chgAny.([]any)
				if !ok || chgSlc == nil {
					continue
				}
				for _, chg1Any := range chgSlc {
					chg1Map, ok := chg1Any.(map[string]any)
					if !ok || chg1Map == nil {
						continue
					}

					nm1Any, ok := chg1Map["name"]
					if !ok {
						continue
					}
					nm1Str, ok := nm1Any.(string)
					if !ok {
						eztools.LogPrint(reflect.TypeOf(nm1Any).String() +
							" got instead of " +
							"string")
						continue
					}
					retStates = append(retStates, nm1Str)

					var cmt1Bool bool
					cmt1Any, ok := chg1Map["comment_required"]
					if ok {
						cmt1Bool, ok = cmt1Any.(bool)
						if !ok {
							eztools.LogPrint(reflect.TypeOf(cmt1Any).String() +
								" got instead of " +
								"bool")
						}
					}
					retCmts = append(retCmts, cmt1Bool)
				}
			}
			return false
		})
	}
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.LogPrint("can change to", retStates,
			"comment required", retCmts)
	}
	return stt, retStates, retCmts, nil
}

func bugzillaDetailExec(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (map[string]interface{}, error) {
	return restMap(eztools.METHOD_GET,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"?",
			"", authInfo), authInfo, nil, svr.Magic)
}

// bugzillaDetail show details of an issue
func bugzillaDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := bugzillaDetailExec(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	return bugzillaParseIssues(bodyMap), nil
}

// bugzillaUriWtToken generate URI of <addr>api_key=<pass>&<prm>
func bugzillaUriWtToken(addr, prm string, authInfo eztools.AuthInfo) string {
	/*addrWtSlash := func() string {
		if strings.HasSuffix(addr, "/") {
			return addr
		}
		return addr + "/"
	}*/
	if authInfo.Type != eztools.AUTH_NONE || len(authInfo.Pass) < 1 {
		/*if len(prm) < 1 {
			return addr
		}*/
		return addr + prm
	}
	var sep string
	if len(prm) > 0 {
		sep = "&"
	}
	// eztools.ShowStrln(addr + /*url.QueryEscape*/
	// 	("Bugzilla_api_key=" + authInfo.Pass + sep + prm))
	return addr + /*url.QueryEscape*/ ("Bugzilla_api_key=" +
		authInfo.Pass + sep + prm)
}

func bugzillaParse1Issue(m map[string]interface{}) (issueInfoOut issueInfos) {
	issueInfoOut = make(issueInfos)
	for i, v := range m {
		if v == nil {
			continue
		}
		//eztools.ShowStrln(i, "-->", v)
		switch i {
		case IssueinfoStrID:
			issueInfoOut[IssueinfoStrID] = chkNSetIssueInfo(v)
		case IssueinfoStrAssigned2:
			val := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrRealName})
			issueInfoOut[IssueinfoStrDispname] = val[0]
		case IssueinfoStrProd:
			issueInfoOut[IssueinfoStrProj] = chkNSetIssueInfo(v)
		case IssueinfoStrState:
			issueInfoOut[IssueinfoStrState] = chkNSetIssueInfo(v)

		case IssueinfoStrSummary:
			issueInfoOut[IssueinfoStrSummary] = chkNSetIssueInfo(v)
		case IssueinfoStrSolution:
			issueInfoOut[IssueinfoStrDesc] = chkNSetIssueInfo(v)
		}
	}
	return
}

func bugzillaParseIssues(m map[string]interface{}) issueInfoSlc {
	return parseIssues("bugs", m, bugzillaParse1Issue)
}

func bugzillaParse1Comment(m map[string]interface{}) (issueInfoOut issueInfos) {
	issueInfoOut = make(issueInfos)
	for i, v := range m {
		if v == nil {
			continue
		}
		//eztools.ShowStrln(i, "-->", v)
		switch i {
		case IssueinfoStrID:
			issueInfoOut[IssueinfoStrID] = chkNSetIssueInfo(v)
		case IssueinfoStrCreator:
			issueInfoOut[IssueinfoStrKey] = chkNSetIssueInfo(v)
		case IssueinfoStrTxt:
			issueInfoOut[IssueinfoStrComments] = chkNSetIssueInfo(v)
		case IssueinfoStrCreatTm:
			issueInfoOut[IssueinfoStrBranch] = chkNSetIssueInfo(v)
		}
	}
	return
}

// bugzillaParseComments only processes 1 bug
func bugzillaParseComments(m map[string]interface{}) issueInfoSlc {
	if m == nil || m["bugs"] == nil {
		return nil
	}
	bugsI := m["bugs"]
	bugsM, ok := bugsI.(map[string]any)
	if !ok {
		eztools.LogPrint(reflect.TypeOf(bugsI).String() +
			" got instead of " +
			"map[string]any for bugs")
		return nil
	}
	for _, bug1I := range bugsM {
		bug1M, ok := bug1I.(map[string]any)
		if !ok {
			eztools.LogPrint(reflect.TypeOf(bug1I).String() +
				" got instead of " +
				"map[string]any for bug1")
			return nil
		}
		return parseIssues("comments", bug1M, bugzillaParse1Comment)
		// only 1 processed
	}
	return nil
}

// bugzillaMyOpen list all open issues of configured user
func bugzillaMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	const RestAPIBZStr = "rest/bug?"
	var states string
	for _, v := range svr.State {
		if v.Type == StateTypeNotOpn {
			if len(v.Text) > 0 {
				states += "&status!=" + v.Text
			}
		}
	}
	bodyMap, err := restMap(eztools.METHOD_GET,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr,
			"assigned_to="+authInfo.User+states,
			authInfo), authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return bugzillaParseIssues(bodyMap), nil
}

func bugzillaWatcherList(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := bugzillaDetailExec(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	var res issueInfoSlc
	loopStringMap(bodyMap, "cc", nil,
		func(_ string, val interface{}) bool {
			str, ok := val.(string)
			if !ok {
				return false
			}
			res = append(res, issueInfos{IssueinfoStrID: str})
			return true
		})
	return res, nil
}

// bugzillaWatcherAdd adds user to cc
func bugzillaWatcherAdd(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	type blk struct {
		Add []string `json:"add"`
	}
	type tranJsons struct {
		Cc blk `json:"cc"`
	}
	var tranJSON tranJsons
	tranJSON.Cc.Add = []string{authInfo.User}
	jsonStr, err := json.Marshal(tranJSON)
	if err != nil {
		return nil, eztools.ErrOutOfBound
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log("watching", issueInfo[IssueinfoStrID])
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(eztools.METHOD_PUT,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"?", "", authInfo),
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	return nil, err
}

// bugzillaWatcherDel removes user from cc
func bugzillaWatcherDel(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	type blk struct {
		Rem []string `json:"remove"`
	}
	type tranJsons struct {
		Cc blk `json:"cc"`
	}
	var tranJSON tranJsons
	tranJSON.Cc.Rem = []string{authInfo.User}
	jsonStr, err := json.Marshal(tranJSON)
	if err != nil {
		return nil, eztools.ErrOutOfBound
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log("unwatching", issueInfo[IssueinfoStrID])
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(eztools.METHOD_PUT,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"?", "", authInfo),
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	return nil, err
}

func bugzillaAddFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrFile]) < 1 ||
		len(issueInfo[IssueinfoStrKey]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	var tranJSON struct {
		ID      []int  `json:"ids"`
		Summary string `json:"summary"`
		FN      string `json:"file_name"`
		Data    string `json:"data"`
		Type    string `json:"content_type"`
	}
	id, err := strconv.Atoi(issueInfo[IssueinfoStrID])
	if err != nil {
		return nil, err
	}
	tranJSON.ID = []int{id}
	tranJSON.Summary = issueInfo[IssueinfoStrKey]
	tranJSON.FN = issueInfo[IssueinfoStrFile]
	tranJSON.Type = eztools.FileType(issueInfo[IssueinfoStrFile])

	buf, err := os.ReadFile(issueInfo[IssueinfoStrFile])
	if err != nil {
		return nil, eztools.ErrAccess
	}
	tranJSON.Data = base64.StdEncoding.EncodeToString(buf)

	jsonStr, err := json.Marshal(tranJSON)
	if err != nil {
		return nil, eztools.ErrOutOfBound
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.Log("attaching to", issueInfo[IssueinfoStrID])
		/*if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}*/
	}
	_, err = restMap(eztools.METHOD_POST,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"/attachment?",
			"", authInfo), authInfo,
		bytes.NewReader(jsonStr), svr.Magic)
	return nil, err
}

func bugzillaListFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			issueInfo[IssueinfoStrID]+"/attachment?",
			"", authInfo), authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return bugzillaParseAttachments(bodyMap, "bugs", false), nil
}

func bugzillaParseAttachments(bodyMap map[string]interface{}, tp string, dataNeeded bool) (issues issueInfoSlc) {
	bodyInt := bodyMap[tp]
	if bodyInt == nil {
		eztools.LogPrint("NO bugs to parse")
		return
	}
	bugAny, ok := bodyInt.(map[string]any)
	if !ok {
		eztools.LogPrint(reflect.TypeOf(bodyInt).String() +
			" got instead of map[string]any")
		return
	}
	for _, bug1Any := range bugAny {
		bugSlc, ok := bug1Any.([]any)
		if !ok {
			continue
		}
		for _, bug1Slc := range bugSlc {
			file1Map, ok := bug1Slc.(map[string]any)
			if !ok {
				continue
			}
			keyF, ok := file1Map[IssueinfoStrID].(float64)
			if !ok {
				eztools.LogPrint("skipping id for an attachment",
					reflect.TypeOf(file1Map[IssueinfoStrID]).String())
				continue
			}
			key := strconv.Itoa(int(keyF))
			sizeF, ok := file1Map[IssueinfoStrSize].(float64)
			if !ok {
				eztools.LogPrint("skipping size for an attachment",
					reflect.TypeOf(file1Map[IssueinfoStrSize]).String())
				continue
			}
			size := eztools.TranSize(int64(sizeF), 0, false)
			file, ok := file1Map[IssueinfoStrFileNm].(string)
			if !ok {
				eztools.LogPrint("skipping file for an attachment",
					reflect.TypeOf(file1Map[IssueinfoStrFileNm]).String())
				continue
			}
			desc, ok := file1Map[IssueinfoStrSummary].(string)
			if !ok {
				eztools.LogPrint("skipping desc for an attachment",
					reflect.TypeOf(file1Map[IssueinfoStrSummary]).String())
				continue
			}
			inf := issueInfos{
				IssueinfoStrKey:  key,
				IssueinfoStrSize: size,
				IssueinfoStrDesc: desc,
				IssueinfoStrFile: file,
			}
			if dataNeeded {
				inf[IssueinfoStrVal] = file1Map[IssueinfoStrData].(string)
			}
			issues = append(issues, inf)
		}
	}
	return
}

func bugzillaGetFileInf(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	inf, err := bugzillaListFile(svr, authInfo, issueInfo)
	if err != nil {
		return issueInfo, err
	}
	if len(inf) < 1 {
		return issueInfo, eztools.ErrNoValidResults
	}
	if len(issueInfo[IssueinfoStrKey]) > 0 {
		for _, v := range inf {
			if v[IssueinfoStrKey] == issueInfo[IssueinfoStrKey] {
				issueInfo[IssueinfoStrName] = v[IssueinfoStrFile]
				break
			}
		}
	} else {
		i := eztools.ChooseMaps(inf.ToMapSlc(), " (",
			IssueinfoStrFile, IssueinfoStrSize)
		if i == eztools.InvalidID {
			return issueInfo, eztools.ErrInvalidInput
		}
		issueInfo[IssueinfoStrName] = inf[i][IssueinfoStrFile]
		issueInfo[IssueinfoStrKey] = inf[i][IssueinfoStrKey]
	}
	return issueInfo, nil
}

// bugzillaGetFile saves an attachment
func bugzillaGetFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	isDir := false
	fi, err := os.Stat(issueInfo[IssueinfoStrFile])
	if err == nil || !os.IsNotExist(err) {
		if !fi.IsDir() {
			eztools.LogPrint(issueInfo[IssueinfoStrFile] + " in EXISTENCE and will NOT be overwritten!")
			return nil, err
		}
		isDir = true
	}
	issueInfo, err = bugzillaGetFileInf(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(issueInfo[IssueinfoStrKey]) < 1 {
		return nil, eztools.ErrNoValidResults
	}
	if len(issueInfo[IssueinfoStrFile]) < 1 || isDir {
		issueInfo[IssueinfoStrFile] = filepath.Join(issueInfo[IssueinfoStrFile],
			issueInfo[IssueinfoStrName])
	}
	bodyMap, err := restMap(eztools.METHOD_GET,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+"attachment/"+
			issueInfo[IssueinfoStrKey]+"?",
			"", authInfo), authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	inf := bugzillaParseAttachments(bodyMap, "attachment", true)
	var ret issueInfoSlc
	for _, f1 := range inf {
		if len(f1[IssueinfoStrFile]) < 1 || len(f1[IssueinfoStrVal]) < 1 {
			continue
		}
		bufI := []byte(f1[IssueinfoStrVal])
		bufO := make([]byte, len(bufI))
		ln, err := base64.StdEncoding.Decode(bufO, bufI)
		if err != nil {
			eztools.LogPrint("failed to parse for", f1[IssueinfoStrFile])
			continue
		}
		err = eztools.FileWrite(f1[IssueinfoStrFile], bufO[:ln])
		if err != nil {
			eztools.LogPrint("failed to save", f1[IssueinfoStrFile])
			continue
		}
		if eztools.Debugging && eztools.Verbose > 0 {
			eztools.Log(f1[IssueinfoStrFile], "saved")
		}
		delete(f1, IssueinfoStrVal)
		ret = append(ret, f1)
	}
	return ret, nil
}