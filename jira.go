package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gitee.com/bon-ami/eztools/v6"
)

const urlAPI4JR = "rest/api/latest/issue/"

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

// changeTypicalJiraNum changes abbreviated input into full numbers
func changeTypicalJiraNum(svr *svrs, num, base string, smart, changes bool) (string, bool) {
	if baseI, sNum, changesI, ok := parseTypicalJiraNum(svr, num); ok {
		smart = true // there is a reference for smart affix
		num = sNum
		base = baseI
		changes = changes || changesI
		//eztools.ShowStrln("not int previously")
	}
	if smart {
		if _, err := strconv.Atoi(num); err != nil {
			smart = false
			//eztools.ShowStrln("not int currently")
			// we do not care what this number is
		}
	}
	if smart {
		inf := base + num
		if changes {
			//if eztools.Verbose > 0 {
			eztools.ShowStrln("Auto changed to " + inf)
			//}
		}
		return inf, true
	}
	return "", false
}

// return values
//
//	whether input is in exact x-0 or -0 format.
//		in case of -0, if previous project (x part) found, it is taken.
//		otherwise, false is returned.
//	the non digit part. this is saved as project.
//	the digit part
func parseTypicalJiraNum(svr *svrs, num string) (nonDigit,
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

// check map type before looping it
func jiraParse1Field(m map[string]interface{}) (issueInfoOut IssueInfos) {
	issueInfoOut = make(IssueInfos)
	for i, v := range m {
		if v == nil {
			continue
		}
		//eztools.ShowStrln(i, "-->", v)
		/*if len(svr.Flds.Desc) > 0 && i == svr.Flds.Desc {
			changed = chkNSetIssueInfo(v, issueInfo,
				ISSUEINFO_IND_DESC) || changed
			continue
		}*/
		switch i {
		case IssueinfoStrAssignee:
			val := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrDispname})
			issueInfoOut[IssueinfoStrDispname] = val[0]
		case IssueinfoStrProj:
			val := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrKey})
			issueInfoOut[IssueinfoStrProj] = val[0]
		case IssueinfoStrState:
			val := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrName})
			issueInfoOut[IssueinfoStrState] = val[0]
		case IssueinfoStrSummary:
			issueInfoOut[IssueinfoStrSummary] = chkNSetIssueInfo(v)
		case IssueinfoStrDesc:
			issueInfoOut[IssueinfoStrDesc] = chkNSetIssueInfo(v)
		}
	}
	return
}

func jiraParse1Issue(m map[string]interface{}) (issueInfoOut IssueInfos) {
	var id []string
	id, _ = loopStringMap(m, "fields",
		[]string{IssueinfoStrKey},
		func(i string, v interface{}) bool {
			// id, self ignored
			//eztools.ShowStrln("1issue "+i, v)
			fields, ok := v.(map[string]interface{})
			if !ok {
				LogTypeErr(v, "map[string]interface{}")
				return false
			}
			issueInfoOut = jiraParse1Field(fields)
			return true
		})
	if len(id) > 0 && issueInfoOut != nil {
		issueInfoOut[IssueinfoStrID] = id[0]
	}
	return
}

func jiraParseTrans(m map[string]interface{}) (tranNames, tranIDs []string) {
	loopStringMap(m, "transitions", nil,
		func(i string, v interface{}) bool {
			arrI, ok := v.([]interface{})
			if !ok {
				LogTypeErr(v, "[]interface{}")
				return false
			}
			for _, arr1 := range arrI {
				tran1, ok := arr1.(map[string]interface{})
				if !ok {
					LogTypeErr(arr1,
						"map[string]interface{}")
					continue
				}
				/*to, ok := tran1["to"].(map[string]interface{})
				if !ok { // to contains description, id, name and statusCategory=id+key+name
					// TODO: dynamically calculate path to resolved status
					LogTypeErr(stdOutput, false,
						reflect.TypeOf(tran1["to"]).
							String() +
							" got instead of string")
					return false
				}*/
				tranN, ok := tran1["name"].(string)
				if !ok {
					LogTypeErr(tran1["name"], "string")
					return false
				}
				tranI, ok := tran1["id"].(string)
				if !ok {
					LogTypeErr(tran1["id"], "string")
					return false
				}
				tranNames = append(tranNames, tranN)
				tranIDs = append(tranIDs, tranI)
				//eztools.ShowStrln("ID=" + tranI + ", name=" + tranN)
			}
			return true
		})
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowSthln(tranNames)
		eztools.ShowSthln(tranIDs)
	}
	return
}

func jiraParseIssues(m map[string]interface{}) IssueInfoSlc {
	return parseIssues("issues", m, jiraParse1Issue)
}

// jiraParse1Cmt parses
//
//	IssueinfoStrComments
//	IssueinfoStrBranch=date
//	IssueinfoStrID
//	IssueinfoStrKey=user
func jiraParse1Cmt(m map[string]interface{}) (IssueInfos, error) {
	var author string
	inf, ok := loopStringMap(m, "author",
		[]string{"body", "updated", "id"},
		func(i string, v interface{}) bool {
			id := chkNLoopStringMap(v,
				"", []string{IssueinfoStrKey})
			if id == nil {
				return false
			}
			author = id[0]
			return false
		})
	if !ok || len(inf) < 1 {
		return nil, eztools.ErrNoValidResults
	}
	return IssueInfos{
		IssueinfoStrComments: inf[0],
		IssueinfoStrBranch:   inf[1],
		IssueinfoStrID:       inf[2],
		IssueinfoStrKey:      author}, nil
}

func jiraParseCmts(m map[string]interface{}) (IssueInfoSlc, error) {
	var (
		issues IssueInfoSlc
	)
	loopStringMap(m, IssueinfoStrComments, nil,
		func(i string, v interface{}) bool {
			cmts, ok := v.([]interface{})
			if !ok {
				LogTypeErr(v, "[]interface{}")
				return false
			}
			for _, s := range cmts {
				cmt, ok := s.(map[string]interface{})
				if !ok {
					LogTypeErr(s, "map[string]interface{}")
					continue
				}
				issue1, err := jiraParse1Cmt(cmt)
				if err != nil {
					// not parsed error only
					Log(false, false, "No detail of comment found")
					continue
				}
				issues = append(issues, issue1)
			}
			return false
		})
	return issues, nil
}

func JiraTransfer(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrSummary]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
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
	if len(issueInfo[IssueinfoStrComments]) > 0 {
		var (
			upCA updateCA
			is   insets
			ss   setss
		)
		is.Name = issueInfo[IssueinfoStrComments]
		ss.Set = append(ss.Set, is)
		upCA.Update.Components = []setss{ss}
		s.Set.Name = issueInfo[IssueinfoStrSummary]
		upCA.Update.Assignee = []sets{s}
		jsonStr, err = json.Marshal(upCA)
	} else {
		var upA updateA
		s.Set.Name = issueInfo[IssueinfoStrSummary]
		upA.Update.Assignee = []sets{s}
		jsonStr, err = json.Marshal(upA)
	}
	if err != nil {
		return nil, err
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		Log(false, false, issueInfo[IssueinfoStrID]+" in transition")
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(http.MethodPut,
		svr.URL+urlAPI4JR+issueInfo[IssueinfoStrID],
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	// result/body is []uint8, if success
	return nil, err
}

type mustFlds struct {
	name    string
	choices IssueInfoSlc
}

func jiraParseAllowedVal(vals interface{}) (musts []mustFlds) {
	valsSlc, ok := vals.([]interface{})
	if !ok {
		LogTypeErr(vals, "slice")
		return
	}
	for _, val1 := range valsSlc {
		val1Map, ok := val1.(map[string]interface{})
		if !ok {
			LogTypeErr(val1, "map of string to interface")
			continue
		}
		for _, i := range [...]string{"id", "value"} {
			val1Any := val1Map[i]
			if val1Any == nil {
				Log(stdOutput, false, "NO "+i+" found")
				break
			}
			val1Str, ok := val1Any.(string)
			if !ok {
				Log(stdOutput, false, reflect.TypeOf(val1Any).String()+
					" got instead of string")
				break
			}
			eztools.ShowStrln(i + "=" + val1Str)
			//musts = append(musts, issueInfoSlc)//here
		}
	}
	return
}

func jiraGetTransMustFlds(svr *svrs, authInfo eztools.AuthInfo,
	id string) (mustMap []mustFlds, err error) {
	bodyMap, err := jiraGetTransExpanded(svr, authInfo, id, "?expand=transitions.fields")
	if err != nil {
		return nil, err
	}
	loopStringMap(bodyMap, "transitions", nil,
		func(i string, v interface{}) bool {
			// id, self ignored
			//eztools.ShowStrln("1issue " + i)
			arrI, ok := v.([]interface{})
			if !ok {
				LogTypeErr(v, "[]interface{}")
				return false
			}
			for _, arr1 := range arrI {
				arr1Map, ok := arr1.(map[string]interface{})
				if !ok {
					LogTypeErr(arr1,
						"map[string]interface{}")
					continue
				}
				fields := arr1Map["fields"]
				if fields == nil {
					Log(false, false, "NO fields got "+
						"in reply of expanded fields!")
					return false
				}
				fieldMap, ok := fields.(map[string]interface{})
				if !ok {
					LogTypeErr(fieldMap,
						"map[string]interface{}")
					return false
				}
			FLD_MAP_IN_TRAN_MUST_FLDS:
				for _ /*fldName*/, fldVal := range fieldMap { //here
					//eztools.ShowStrln(fldName)
					fldValMap, ok := fldVal.(map[string]interface{})
					if !ok {
						LogTypeErr(fldVal,
							"map[string]interface{}")
						continue
					}
					fldReq := fldValMap["required"]
					if fldReq == nil {
						continue
					}
					fldReqBl, ok := fldReq.(bool)
					if !ok {
						LogTypeErr(fldReq, "bool")
						continue
					}
					if !fldReqBl {
						continue
					}
					var mustFld1 mustFlds
					//var pairs issueInfoSlc //here
					for fld1Name, fld1Val := range fldValMap {
						switch fld1Name {
						case "name":
							fld1ValStr, ok := fld1Val.(string)
							if !ok {
								LogTypeErr(fld1Val,
									"string for "+fld1Name)
								continue FLD_MAP_IN_TRAN_MUST_FLDS
							}
							mustFld1.name = fld1ValStr
						case "allowedValues":
							jiraParseAllowedVal(fld1Val)
						}
					}
					if len(mustFld1.name) < 1 || len(mustFld1.choices) < 1 {
						continue
					}
					mustMap = append(mustMap, mustFld1)
				}
			}
			return true
		})
	return mustMap, err
}

func jiraGetTransExpanded(svr *svrs, authInfo eztools.AuthInfo,
	id, exp string) (bodyMap map[string]interface{}, err error) {
	return restMap(http.MethodGet, svr.URL+urlAPI4JR+
		id+"/transitions"+exp, authInfo, nil, svr.Magic)
}

func jiraGetTrans(svr *svrs, authInfo eztools.AuthInfo,
	id string) (tranNames, tranIDs []string, err error) {
	bodyMap, err := jiraGetTransExpanded(svr, authInfo, id, "")
	if err != nil {
		return nil, nil, err
	}
	tranNames, tranIDs = jiraParseTrans(bodyMap)
	return tranNames, tranIDs, nil
}

func jiraChooseTran(tranName string, tranNames, tranIDs []string) (string, error) {
	var tranID string
	if len(tranNames) > 0 && len(tranIDs) > 0 {
		if len(tranName) > 0 {
			for i, v := range tranNames {
				//eztools.ShowStrln(v + "=" + tranName + "?")
				if tranName == v {
					tranID = tranIDs[i]
					//eztools.ShowStrln("tran ID=" + tranID)
					break
				}
			}
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
			tranID = tranIDs[i]
		}
	}
	if len(tranID) < 1 {
		return "", eztools.ErrNoValidResults
	}
	return tranID, nil
}

// jiraTranExec transition issue {id} to state {tranID}
func jiraTranExec(svr *svrs, authInfo eztools.AuthInfo,
	id, tranID string) (err error) {
	type tranJsons struct {
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
	}
	var tranJSON tranJsons
	tranJSON.Transition.ID = tranID
	jsonStr, err := json.Marshal(tranJSON)
	if err != nil {
		return
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		Log(false, false, id+" in transition")
		if eztools.Verbose > 1 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(http.MethodPost, svr.URL+urlAPI4JR+
		id+"/transitions", authInfo,
		bytes.NewReader(jsonStr), svr.Magic)
	// replies to transitions contains no body
	/*tranNames, tranIDs := jiraParseTrans(bodyMap)
	//for i := 0; i < len(tranNames) && i < len(tranIDs); i++ {
	if len(tranNames) > 0 && len(tranIDs) > 0 { // multiple?
		issueInfo[IssueinfoStrKey] = tranIDs[0]
		issueInfo[IssueinfoStrState] = tranNames[0]
	}
	//}*/
	return
}

// jiraCmtNTran is transitions for reject & close, adding comments
func jiraCmtNTran(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos, steps []string) (err error) {
	if len(issueInfo[IssueinfoStrComments]) > 0 {
		_, err := jiraAddComment1(svr, authInfo, issueInfo)
		if err != nil {
			Log(true, false, err)
		}
	}
	var tranNames, tranIDs []string
	for i, tran := range steps {
		if eztools.Debugging && eztools.Verbose > 1 {
			eztools.ShowStrln("Trying " + tran)
		}
		if tranNames == nil || tranIDs == nil ||
			len(tranNames) < 1 || len(tranIDs) < 1 {
			tranNames, tranIDs, err = jiraGetTrans(svr, authInfo,
				issueInfo[IssueinfoStrID])
			if err != nil {
				return err
			}
		}
		if tranNames == nil || tranIDs == nil ||
			len(tranNames) < 1 || len(tranIDs) < 1 {
			Log(true, false, "NO available transitions")
			return eztools.ErrNoValidResults
		}
		tranID, err := jiraChooseTran(tran, tranNames, tranIDs)
		if err != nil {
			if i == len(steps)-1 {
				// return error if the last step fails,
				// since it is a key one
				if err == eztools.ErrNoValidResults {
					Log(true, false, "No available transitions. Check permission!")
				}
				return err
			}
			continue
		}
		tranNames = nil
		tranIDs = nil
		err = jiraTranExec(svr, authInfo,
			issueInfo[IssueinfoStrID], tranID)
		if err != nil {
			if err == errGram {
				flds, err := jiraGetTransMustFlds(svr, authInfo,
					issueInfo[IssueinfoStrID])
				if err != nil {
					Log(true, false, err)
				} else {
					Log(true, false, "must fields are", flds)
				}
			}
			return err
		}
	}
	return err
}

func jiraConstructFields(in string) string {
	return `{
  "fields": {
` + in + `
  }
}`
}

func jiraEditWtFields(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos, jsonInner string) error {
	jsonStr := jiraConstructFields(jsonInner)
	if eztools.Debugging && eztools.Verbose > 0 {
		Log(false, false, "Processing "+issueInfo[IssueinfoStrID])
	}
	//eztools.ShowStrln(jsonStr)
	_, err := restSth(http.MethodPut,
		svr.URL+urlAPI4JR+
			issueInfo[IssueinfoStrID],
		authInfo, strings.NewReader(jsonStr),
		svr.Magic)
	return err
}

func jiraEditMeta(svr *svrs, authInfo eztools.AuthInfo, id, filter string) (interface{}, error) {
	bodyMap, err := restMap(http.MethodGet, svr.URL+urlAPI4JR+
		id+"/editmeta", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	type filterFunc func(map[string]interface{}) interface{}
	var fF filterFunc
	fF = func(m map[string]interface{}) interface{} {
		for i, v := range m {
			if i == filter {
				return v
			}
			child, ok := v.(map[string]interface{})
			if ok {
				if r := fF(child); r != nil {
					return r
				}
			}
		}
		return nil
	}
	return fF(bodyMap), nil
}

// jiraEditMeta get possible reject reasons
// reject reason stored in IssueinfoStrKey
func jiraGetDesc(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (jsonStr string) {
	if len(svr.Flds.RejectRsn) > 0 {
		if len(issueInfo[IssueinfoStrKey]) < 1 {
			// get all possible reasons
			field, err := jiraEditMeta(svr, authInfo, issueInfo[IssueinfoStrID],
				svr.Flds.RejectRsn)
			if err != nil {
				Log(false, false, err)
			} else {
				issueInfo[IssueinfoStrKey] = getValuesFromMaps("value", field)
			}
			if len(issueInfo[IssueinfoStrKey]) < 1 {
				Log(false, false, "NO choices found for "+svr.Flds.RejectRsn)
				useInputOrPromptStr(svr, issueInfo,
					IssueinfoStrKey, "reject reason")
			}
		}
		if len(issueInfo[IssueinfoStrKey]) > 0 {
			for i, v := range map[string]string{
				"value": issueInfo[IssueinfoStrKey]} {
				jsonStr = custFld(jsonStr, i, v)
			}
			if len(jsonStr) < 1 {
				Log(true, false, "NO RejectRsn field "+
					"defined for this server")
				issueInfo[IssueinfoStrKey] = ""
			} else {
				jsonStr = `        "` + svr.Flds.RejectRsn + `": {` + jsonStr + `}`
			}
		}
	}
	return
}

func JiraReject(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	Steps := makeStates(svr, StateTypeTranRej)
	if Steps == nil {
		Log(true, false, "No transitions configured for this server!")
		return nil, errCfg
	}
	// set reject reason
	if jsonStr := jiraGetDesc(svr, authInfo, issueInfo); len(jsonStr) > 0 {
		if err := jiraEditWtFields(svr,
			authInfo, issueInfo,
			jsonStr); err != nil {
			return nil, err
		}
	}
	return nil, jiraCmtNTran(svr, authInfo, issueInfo, Steps)
}

func jiraCloseWtQA(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos, qa string) (IssueInfoSlc, error) {
	Steps := makeStates(svr, StateTypeTranCls)
	if Steps == nil {
		Log(true, false, "No transitions configured for this server!")
		return nil, errCfg
	}
	if len(qa) > 0 {
		var jsonStr string
		for i, v := range map[string]string{
			svr.Flds.TstPre:  "none",
			svr.Flds.TstStep: qa,
			svr.Flds.TstExp:  "none"} {
			jsonStr = custFld(jsonStr, i, v)
		}
		if len(jsonStr) < 1 {
			Log(true, false,
				"NO Tst* fields defined for this server")
		} else {
			if err := jiraEditWtFields(svr, authInfo,
				issueInfo, jsonStr); err != nil {
				return nil, err
			}
		}
	}
	return nil, jiraCmtNTran(svr, authInfo, issueInfo, Steps)
}

func JiraClose(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	return jiraCloseWtQA(svr, authInfo, issueInfo, issueInfo[IssueinfoStrLink])
}

func JiraCloseDef(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "default AOSP/vendor/design")
}

func JiraCloseGen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "general requirement")
}

func JiraTransition(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	names, ids, err := jiraGetTrans(svr, authInfo,
		issueInfo[IssueinfoStrID])
	if err != nil {
		return nil, err
	}
	tranID, err := jiraChooseTran("", names, ids)
	if err != nil {
		return nil, err
	}
	err = jiraTranExec(svr, authInfo,
		issueInfo[IssueinfoStrID], tranID)
	if err != nil {
		return nil, err
	}
	return nil, err
}

func JiraLink(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	// TODO: get these from server
	linkChoices := []struct {
		name, inward, outward string
	}{
		{inward: "is blocked by",
			name:    "Blocks",
			outward: "blocks"}}
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrLink]) < 1 ||
		issueInfo[IssueinfoStrLink] ==
			issueInfo[IssueinfoStrID] {
		return nil, eztools.ErrInvalidInput
	}
	var linkType int
	switch len(linkChoices) {
	case 0:
		return nil, eztools.ErrOutOfBound
	case 1:
		linkType = 0
	default:
		if uiSilent { // TODO: command params
			noInteractionAllowed()
			return nil, eztools.ErrInvalidInput
		}
		linkType = eztools.ChooseStringsWtIDs(
			func() int {
				return len(linkChoices)
			},
			func(i int) int {
				return i
			},
			func(i int) string {
				return linkChoices[i].name
			}, "link type")
		if linkType == eztools.InvalidID {
			return nil, eztools.ErrInvalidInput
		}
	}
	type ils struct {
		Add struct {
			Type map[string]string `json:"type"`
			II   map[string]string `json:"inwardIssue"`
		} `json:"add"`
	}
	type jsonStru struct {
		Update struct {
			IL []ils `json:"issuelinks"`
		} `json:"update"`
	}
	var (
		jsonStr []byte
		err     error
		jstru   jsonStru
		il      ils
	)
	il.Add.II = make(map[string]string)
	il.Add.Type = make(map[string]string)
	//il.Add.Type["id"] = svr.Flds.LinkType[linkType].Value
	il.Add.Type["inward"] = linkChoices[linkType].inward
	il.Add.Type["name"] = linkChoices[linkType].name
	il.Add.Type["outward"] = linkChoices[linkType].outward
	il.Add.II[IssueinfoStrKey] = issueInfo[IssueinfoStrLink]
	jstru.Update.IL = append(jstru.Update.IL, il)
	jsonStr, err = json.Marshal(jstru)
	if err != nil {
		return nil, err
	}
	//eztools.ShowByteln(jsonStr)
	_, err = restMap(http.MethodPut, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID],
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	// TODO: parse result
	return nil, err
}

func JiraModComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrComments]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	if len(issueInfo[IssueinfoStrKey]) < 1 {
		inf, err := JiraComments(svr, authInfo, issueInfo)
		if err != nil {
			return nil, err
		}
		/*var a []map[string]string
		a = ((interface{})(inf)).([]map[string]string)*/
		i := eztools.ChooseMaps(inf.ToMapSlc(), " (",
			IssueinfoStrComments, IssueinfoStrID)
		if i == eztools.InvalidID {
			return nil, eztools.ErrInvalidInput
		}
		issueInfo[IssueinfoStrKey] = inf[i][IssueinfoStrKey]
		if len(issueInfo[IssueinfoStrKey]) < 1 {
			return nil, eztools.ErrNoValidResults
		}
	}
	var tranJSON struct {
		Body string `json:"body"`
	}
	tranJSON.Body = issueInfo[IssueinfoStrComments]
	jsonStr, err := json.Marshal(tranJSON)
	if err != nil {
		return nil, err
	}
	_, err = restMap(http.MethodPut,
		svr.URL+urlAPI4JR+issueInfo[IssueinfoStrID]+"/comment/"+
			issueInfo[IssueinfoStrKey], authInfo,
		bytes.NewReader(jsonStr), svr.Magic)
	// TODO: parse result
	return nil, err
}

func JiraDelComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	// TODO: select key
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrKey]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restMap(http.MethodDelete, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID]+"/comment/"+issueInfo[IssueinfoStrKey],
		authInfo, nil, svr.Magic)
	// TODO: parse result
	return nil, err
}

func JiraAddComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrComments]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	inf, err := jiraAddComment1(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	return inf.ToSlc(), err
}

func jiraPostSth(svr *svrs, urlSuffix string, authInfo eztools.AuthInfo,
	stru interface{}, id string) (bodyMap map[string]interface{}, err error) {
	var jsonStr []byte
	jsonStr, err = json.Marshal(stru)
	if err != nil {
		return
	}
	if eztools.Debugging && eztools.Verbose > 0 {
		eztools.ShowByteln(jsonStr)
	}
	bodyMap, err = restMap(http.MethodPost, svr.URL+urlAPI4JR+
		id+"/"+urlSuffix,
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	if err != nil {
		return
	}
	return
}

func jiraAddComment1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfos, error) {
	type comment1 struct {
		Comment1 string `json:"body"`
	}
	var (
		cmt comment1
	)
	cmt.Comment1 = issueInfo[IssueinfoStrComments]
	body, err := jiraPostSth(svr, "comment", authInfo, cmt, issueInfo[IssueinfoStrID])
	if err != nil {
		return nil, err
	}
	return jiraParse1Cmt(body)
}

func JiraComments(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(http.MethodGet, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID]+"/comment",
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return jiraParseCmts(bodyMap)
}

func jiraDetailExec(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (map[string]interface{}, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(http.MethodGet, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID], authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return bodyMap, err
}

func JiraDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	bodyMap, err := jiraDetailExec(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	issueInfo = jiraParse1Issue(bodyMap)
	if issueInfo == nil {
		return nil, nil
	}
	return issueInfo.ToSlc(), nil
	//return jiraParseIssues(svr, bodyMap), err
}

func JiraMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	const RestAPIStr = "rest/api/latest/search?jql="
	var states string
	for _, v := range svr.State {
		if v.Type == StateTypeNotOpn {
			if len(v.Text) > 0 {
				states += "&status!=" + v.Text
			}
		}
	}
	bodyMap, err := restMap(http.MethodGet, svr.URL+RestAPIStr+
		url.QueryEscape("assignee="+authInfo.User+states),
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return jiraParseIssues(bodyMap), err
}

func JiraWatcherList(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(http.MethodGet, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	var res IssueInfoSlc
	loopStringMap(bodyMap, "watchers", nil,
		func(_ string, watchersI interface{}) bool {
			watchersS, ok := watchersI.([]interface{})
			if !ok {
				LogTypeErr(watchersS, "[]interface{}")
				return false
			}
			for _, watcherI := range watchersS {
				inf := chkNLoopStringMap(watcherI, "",
					[]string{"name", "displayName"})
				if inf == nil {
					return false
				}
				res = append(res, IssueInfos{
					IssueinfoStrDispname: inf[1],
					IssueinfoStrID:       inf[0]})
			}
			return true
		})
	return res, nil
}

func JiraWatcherCheck(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(http.MethodGet, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	var res IssueInfoSlc
	watchingI := bodyMap["isWatching"]
	watching, ok := watchingI.(bool)
	if !ok {
		LogTypeErr(watchingI, "bool")
		return nil, err
	}
	res = append(res, IssueInfos{
		IssueinfoStrState: strconv.FormatBool(watching)})
	return res, nil
}

func JiraWatcherAdd(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restMap(http.MethodPost, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID]+"/watchers",
		authInfo, strings.NewReader("\""+cfg.User+"\""), svr.Magic)
	return nil, err
}

func JiraWatcherDel(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restMap(http.MethodDelete, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID]+"/watchers?username="+cfg.User,
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return nil, err
}

func jiraParseAttachments(bodyMap map[string]interface{}) (issues IssueInfoSlc) {
	bodyInt := bodyMap["fields"]
	if bodyInt == nil {
		Log(stdOutput, false, "NO fields to parse")
		return
	}
	bodyFlds, ok := bodyInt.(map[string]interface{})
	if !ok {
		LogTypeErr(bodyInt, "map of string to interface{}")
		return
	}
	if bodyFlds["attachment"] == nil {
		Log(false, false, "NO attachment to parse")
		return
	}
	bodySlc, ok := bodyFlds["attachment"].([]interface{})
	if !ok {
		LogTypeErr(bodyFlds["attachment"], "slice of interface{}")
		return
	}
	for _, slc1 := range bodySlc {
		map1, ok := slc1.(map[string]interface{})
		if !ok {
			LogTypeErr(slc1, "map of string to interface{}")
			continue
		}
		inf, _ := loopStringMap(map1, "", []string{
			"self", "content", "filename", "id", "mimeType"}, nil)
		var sz string
		if map1[IssueinfoStrSize] != nil {
			szI, ok := map1[IssueinfoStrSize].(float64)
			if !ok {
				LogTypeErr(map1[IssueinfoStrSize], "float64")
			} else {
				sz = eztools.TranSize(int64(szI), 1, false)
			}
		}
		issues = append(issues, IssueInfos{
			IssueinfoStrBin:  inf[0],
			IssueinfoStrLink: inf[1],
			IssueinfoStrFile: inf[2],
			IssueinfoStrKey:  inf[3],
			IssueinfoStrDesc: inf[4],
			IssueinfoStrSize: sz})
	}
	return
}

func JiraAddFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrFile]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restFile(http.MethodPost, svr.URL+urlAPI4JR+
		issueInfo[IssueinfoStrID]+"/attachments",
		authInfo, "file", issueInfo[IssueinfoStrFile],
		map[string]string{"X-Atlassian-Token": "nocheck"}, svr.Magic)
	if err != nil {
		return nil, err
	}
	return nil, err
}

func JiraListFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	bodyMap, err := jiraDetailExec(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	return jiraParseAttachments(bodyMap), nil
}

func jiraGetFileInf(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfos, error) {
	inf, err := JiraListFile(svr, authInfo, issueInfo)
	if err != nil {
		return issueInfo, err
	}
	if len(inf) < 1 {
		return issueInfo, eztools.ErrNoValidResults
	}
	if len(issueInfo[IssueinfoStrKey]) > 0 {
		for _, v := range inf {
			if v[IssueinfoStrKey] == issueInfo[IssueinfoStrKey] {
				issueInfo[IssueinfoStrLink] = v[IssueinfoStrLink]
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
		issueInfo[IssueinfoStrLink] = inf[i][IssueinfoStrLink]
		issueInfo[IssueinfoStrName] = inf[i][IssueinfoStrFile]
		issueInfo[IssueinfoStrKey] = inf[i][IssueinfoStrKey]
	}
	return issueInfo, nil
}

func JiraGetFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	/*isDir := false
	fi, err := os.Stat(issueInfo[IssueinfoStrFile])
	if err == nil || !os.IsNotExist(err) {
		if !fi.IsDir() {
			Log(stdOutput, false, issueInfo[IssueinfoStrFile]+" in EXISTENCE and will NOT be overwritten!")
			return nil, err
		}
		isDir = true
	}*/
	issueInfo, err := jiraGetFileInf(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(issueInfo[IssueinfoStrLink]) < 1 {
		return nil, eztools.ErrNoValidResults
	}
	_, err = restAttachment(http.MethodGet, issueInfo[IssueinfoStrLink], authInfo, nil, svr.Magic)
	/*if len(issueInfo[IssueinfoStrFile]) < 1 || isDir {
		issueInfo[IssueinfoStrFile] = filepath.Join(issueInfo[IssueinfoStrFile],
			issueInfo[IssueinfoStrName])
	}
	resp, err := eztools.HTTPSendAuth(http.MethodGet,
		issueInfo[IssueinfoStrLink], "", authInfo, nil)
	if err != nil {
		return nil, err
	}
	recognized, bodyType, bodyBytes, errNo, err := eztools.HTTPParseBody(resp,
		issueInfo[IssueinfoStrFile], nil, []byte(svr.Magic))
	if err == nil {
		if recognized != eztools.BodyTypeFile {
			if eztools.Debugging && eztools.Verbose > 0 {
				Log(false, false, "body type", bodyType, "forced to be saved as file!")
			}
			if err := eztools.FileWrite(issueInfo[IssueinfoStrFile], bodyBytes); err != nil {
				Log(true, false, "failed to save file", issueInfo[IssueinfoStrFile], err)
			}
		}
	}
	issueInfo[IssueinfoStrState] = strconv.Itoa(errNo)*/
	return issueInfo.ToSlc(), err
}

func JiraDelFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) (IssueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrKey]) < 1 {
		if len(issueInfo[IssueinfoStrID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		var err error
		issueInfo, err = jiraGetFileInf(svr, authInfo, issueInfo)
		if err != nil {
			return nil, err
		}
	}
	// https://developer.atlassian.com/static/rest/jira/5.1.6.html#id127779
	const RestAPIStr = "rest/api/latest/attachment/"
	_, err := restSth(http.MethodDelete,
		svr.URL+RestAPIStr+issueInfo[IssueinfoStrKey],
		authInfo, nil, svr.Magic)
	return nil, err
}
