package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gitee.com/bon-ami/eztools/v4"
)

const RestAPIBZStr = "rest/bug/"

// return values
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
	type updateA struct {
		Assignee string `json:"assigned_to"`
	}
	type updateCA struct {
		Components string `json:"product"`
		Assignee   string `json:"assigned_to"`
	}

	var (
		jsonStr []byte
		err     error
	)
	if len(issueInfo[IssueinfoStrComments]) > 0 {
		var upCA updateCA
		upCA.Components = issueInfo[IssueinfoStrComments]
		upCA.Assignee = issueInfo[IssueinfoStrSummary]
		jsonStr, err = json.Marshal(upCA)
	} else {
		var upA updateA
		upA.Assignee = issueInfo[IssueinfoStrSummary]
		jsonStr, err = json.Marshal(upA)
	}
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

func bugzillaChooseTran(tranName string, tranNames []string) (string, error) {
	var tranID string
	if len(tranNames) > 0 {
		if len(tranName) > 0 {
			for _, v := range tranNames {
				//eztools.ShowStrln(v + "=" + tranName + "?")
				if tranName == v {
					tranID = v
					//eztools.ShowStrln("tran ID=" + tranID)
					return tranID, nil
				}
			}
			return tranID, eztools.ErrNoValidResults
		} else {
			if uiSilent {
				noInteractionAllowed()
				return "", eztools.ErrInvalidInput
			}
			eztools.ShowStrln(
				"There are following transitions available.")
			i, _ := eztools.ChooseStrings(tranNames)
			if i == eztools.InvalidID {
				return "", eztools.ErrInvalidInput
			}
			tranID = tranNames[i]
		}
	}
	return tranID, nil
}

// bugzillaTranExec transition issue {id} to state {tranID}
func bugzillaTranExec(svr *svrs, authInfo eztools.AuthInfo,
	id, tranID string) (err error) {
	type tranJsons struct {
		Stt string `json:"status"`
	}
	var tranJSON tranJsons
	tranJSON.Stt = tranID
	jsonStr, err := json.Marshal(tranJSON)
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
	fun func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos) error) (err error) {
	if fun != nil {
		if err = fun(svr, authInfo, issueInfo); err != nil {
			return err
		}

	}
	var (
		tranNames []string
		stt       string
	)
	for i, tran := range steps {
		if eztools.Debugging && eztools.Verbose > 1 {
			eztools.ShowStrln("Trying " + tran)
		}
		if tranNames == nil || len(tranNames) < 1 {
			stt, tranNames, err = bugzillaGetTrans(svr, authInfo, issueInfo, stt)
			if err != nil {
				return err
			}
		}
		if tranNames == nil || len(tranNames) < 1 {
			eztools.LogPrint("NO available transitions")
			return eztools.ErrNoValidResults
		}
		tranID, err := bugzillaChooseTran(tran, tranNames)
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
		err = bugzillaTranExec(svr, authInfo,
			issueInfo[IssueinfoStrID], tranID)
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

// bugzillaReject rejects an issue
func bugzillaReject(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	Steps := makeStates(svr, "transition reject")
	if Steps == nil {
		eztools.LogPrint("No transitions configured for this server!")
		return nil, errCfg
	}
	return nil, bugzillaFuncNTran(svr, authInfo, issueInfo, Steps,
		func(svr *svrs, authInfo eztools.AuthInfo,
			issueInfo issueInfos) error {
			if len(issueInfo[IssueinfoStrComments]) > 0 {
				_, err := bugzillaAddComment1(svr, authInfo, issueInfo)
				if err != nil {
					eztools.LogPrint(err)
				}
			}
			return nil
		})
}

// bugzillaClose closes an issue
func bugzillaClose(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	Steps := makeStates(svr, "transition close")
	if Steps == nil {
		eztools.LogPrint("No transitions configured for this server!")
		return nil, errCfg
	}
	return nil, bugzillaFuncNTran(svr, authInfo, issueInfo, Steps, nil)
}

// bugzillaTransition transitions an issue to a state
func bugzillaTransition(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, names, err := bugzillaGetTrans(svr, authInfo,
		issueInfo, "")
	if err != nil {
		return nil, err
	}
	tranID, err := bugzillaChooseTran("", names)
	if err != nil {
		return nil, err
	}
	return nil, bugzillaTranExec(svr, authInfo,
		issueInfo[IssueinfoStrID], tranID)
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
	var tranJSON struct {
		Comment string `json:"comment"`
	}
	tranJSON.Comment = issueInfo[IssueinfoStrComments]
	jsonStr, err := json.Marshal(tranJSON)
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

func bugzillaGetTrans(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, stt string) (string, []string, error) {
	if len(stt) < 1 {
		var ok bool
		slcInf, err := bugzillaDetail(svr, authInfo, issueInfo)
		if err != nil || slcInf == nil || len(slcInf) != 1 {
			return "", nil, eztools.ErrOutOfBound
		}
		stt, ok = slcInf[0][IssueinfoStrState]
		if !ok || len(stt) < 1 {
			return "", nil, eztools.ErrOutOfBound
		}
	}
	const RestAPIBZStr = "rest/field/bug/"
	bodyMap, err := restMap(eztools.METHOD_GET,
		bugzillaUriWtToken(svr.URL+RestAPIBZStr+
			"bug_status?", "", authInfo),
		authInfo, nil, svr.Magic)
	if err != nil {
		return stt, nil, err
	}
	fldsAny, ok := bodyMap["fields"]
	if !ok {
		return stt, nil, eztools.ErrOutOfBound
	}
	fldSlc, ok := fldsAny.([]any)
	if !ok {
		return stt, nil, eztools.ErrOutOfBound
	}
	var ret []string
	for _, fld1Any := range fldSlc {
		fld1Map, ok := fld1Any.(map[string]interface{})
		if !ok {
			return stt, nil, eztools.ErrOutOfBound
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
					ret = append(ret, nm1Str)
				}
			}
			return false
		})
	}
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.LogPrint("can change to", ret)
	}
	return stt, ret, nil
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
	eztools.ShowStrln(addr + /*url.QueryEscape*/
		("Bugzilla_api_key=" + authInfo.Pass + sep + prm))
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

func bugzillaParseComments(m map[string]interface{}) issueInfoSlc {
	return parseIssues("comments", m, bugzillaParse1Comment)
}

// bugzillaMyOpen list all open issues of configured user
func bugzillaMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	const RestAPIBZStr = "rest/bug?"
	var states string
	for _, v := range svr.State {
		if v.Type == "not open" {
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

	buf, err := ioutil.ReadFile(issueInfo[IssueinfoStrFile])
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
