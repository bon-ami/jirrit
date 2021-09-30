package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"gitee.com/bon-ami/eztools"
)

const RestAPIStr = "rest/api/latest/issue/"

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
	mustStr string, keyStr []string,
	fun func(string, interface{}) bool) (keyVal []string, ret bool) {
	if len(keyStr) > 0 {
		keyVal = make([]string, len(keyStr))
	} else {
		keyVal = nil
	}
	for i, v := range m {
		if eztools.Debugging && eztools.Verbose > 2 {
			eztools.ShowStrln("looping " + i)
		}
		if len(keyStr) > 0 {
			matched := false
			for j, key1 := range keyStr {
				if i == key1 {
					matched = true
					id, ok := v.(string)
					if !ok {
						eztools.LogPrint(
							reflect.TypeOf(v).String() +
								" got instead of string")
						break
					}
					ret = true
					keyVal[j] = id
					if fun == nil {
						break
					}
					//eztools.ShowStrln("id=" + id)
					break
				}
			}
			if matched {
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
	return keyVal, ret
}

func chkNLoopStringMap(m interface{},
	mustStr string, keyStr []string) []string {
	sub, ok := m.(map[string]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(m).String() +
			" got instead of map[string]interface{}")
		return nil
	}
	res, _ := loopStringMap(sub, mustStr, keyStr, nil)
	return res
}

func chkNSetIssueInfo(v interface{}, i string) string {
	if v == nil {
		eztools.Log("nil got, not string")
		return ""
	}
	str, ok := v.(string)
	if !ok {
		eztools.LogPrint(reflect.TypeOf(v).String() +
			" got instead of string")
		return ""
	}
	return str
}

// check map type before looping it
func jiraParse1Field(m map[string]interface{}) (issueInfoOut issueInfos) {
	issueInfoOut = make(issueInfos)
	for i, v := range m {
		if v == nil {
			continue
		}
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
			issueInfoOut[IssueinfoStrState] = chkNSetIssueInfo(v,
				IssueinfoStrHead)
		case IssueinfoStrDesc:
			issueInfoOut[IssueinfoStrDesc] = chkNSetIssueInfo(v,
				IssueinfoStrDesc)
		}
	}
	return
}

func jiraParse1Issue(m map[string]interface{}) (issueInfoOut issueInfos) {
	var id []string
	id, _ = loopStringMap(m, "fields",
		[]string{IssueinfoStrKey},
		func(i string, v interface{}) bool {
			// id, self ignored
			//eztools.ShowStrln("1issue " + i)
			fields, ok := v.(map[string]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of " +
					"map[string]interface{}")
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
				eztools.LogPrint(
					reflect.TypeOf(v).String() +
						" got instead of []interface{}")
				return false
			}
			for _, arr1 := range arrI {
				tran1, ok := arr1.(map[string]interface{})
				if !ok {
					eztools.LogPrint(
						reflect.TypeOf(arr1).
							String() +
							" got instead of " +
							"map[string]interface{}")
					continue
				}
				/*to, ok := tran1["to"].(map[string]interface{})
				if !ok {
					eztools.LogPrint(
						reflect.TypeOf(tran1["to"]).
							String() +
							" got instead of string")
					return false
				}*/
				tranN, ok := tran1["name"].(string)
				if !ok {
					eztools.LogPrint(
						reflect.TypeOf(tran1["name"]).
							String() +
							" got instead of string")
					return false
				}
				tranI, ok := tran1["id"].(string)
				if !ok {
					eztools.LogPrint(
						reflect.TypeOf(tran1["id"]).
							String() +
							" got instead of string")
					return false
				}
				tranNames = append(tranNames, tranN)
				tranIDs = append(tranIDs, tranI)
				//eztools.ShowStrln("ID=" + tranI + ", name=" + tranN)
			}
			return true
		})
	if eztools.Debugging && eztools.Verbose > 2 {
		eztools.ShowSthln(tranNames)
		eztools.ShowSthln(tranIDs)
	}
	return
}

func jiraParseIssues(m map[string]interface{}) issueInfoSlc {
	/*if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowSthln(strs)
	}*/
	results := make(issueInfoSlc, 0)
	loopStringMap(m, "issues", nil,
		func(i string, v interface{}) bool {
			// https://docs.atlassian.com/software/jira/docs/api/REST/8.12.0/#api/2/search-search
			//eztools.ShowStrln("func " + i)
			issues, ok := v.([]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of " +
					"[]interface{} for " + i)
				return false
			}
			for _, v := range issues {
				//eztools.ShowStrln("Ticket")
				issue, ok := v.(map[string]interface{})
				if !ok {
					eztools.LogPrint(reflect.TypeOf(v).String() +
						" got instead of " +
						"map[string]interface{}")
					continue
				}
				if issueInfo := jiraParse1Issue(issue); issueInfo != nil {
					//eztools.ShowSthln(issueInfo)
					results = append(results, issueInfo)
				}
			}
			return true
		})
	if len(results) < 1 {
		return nil
	}
	return results
}

// jiraParse1Cmt parses
//	IssueinfoStrComments
//	IssueinfoStrBranch=date
//	IssueinfoStrID
//	IssueinfoStrKey=user
func jiraParse1Cmt(m map[string]interface{}) (issueInfos, error) {
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
	return issueInfos{
		IssueinfoStrComments: inf[0],
		IssueinfoStrBranch:   inf[1],
		IssueinfoStrID:       inf[2],
		IssueinfoStrKey:      author}, nil
}

func jiraParseCmts(m map[string]interface{}) (issueInfoSlc, error) {
	var (
		issues issueInfoSlc
	)
	loopStringMap(m, IssueinfoStrComments, nil,
		func(i string, v interface{}) bool {
			cmts, ok := v.([]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(v).String() +
					" got instead of " +
					"[]interface{}")
				return false
			}
			for _, s := range cmts {
				cmt, ok := s.(map[string]interface{})
				if !ok {
					eztools.LogPrint(reflect.TypeOf(s).String() +
						" got instead of " +
						"map[string]interface{}")
					continue
				}
				issue1, err := jiraParse1Cmt(cmt)
				if err != nil {
					// not parsed error only
					eztools.Log("No detail of comment found")
					continue
				}
				issues = append(issues, issue1)
			}
			return false
		})
	return issues, nil
}

func jiraTransfer(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrHead]) < 1 {
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
		s.Set.Name = issueInfo[IssueinfoStrHead]
		upCA.Update.Assignee = []sets{s}
		jsonStr, err = json.Marshal(upCA)
	} else {
		var upA updateA
		s.Set.Name = issueInfo[IssueinfoStrHead]
		upA.Update.Assignee = []sets{s}
		jsonStr, err = json.Marshal(upA)
	}
	if err != nil {
		return nil, err
	}
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.Log(issueInfo[IssueinfoStrID] + " in transition")
		if eztools.Verbose > 2 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restMap(eztools.METHOD_PUT,
		svr.URL+RestAPIStr+issueInfo[IssueinfoStrID],
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	// TODO: parse result
	return nil, err
}

func jiraGetTrans(svr *svrs, authInfo eztools.AuthInfo,
	id, tranName string) (tranNames, tranIDs []string, err error) {
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		id+"/transitions", authInfo, nil, svr.Magic)
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
				//eztools.ShowStrln(v + "=?" + tranName)
				if tranName == v {
					tranID = tranIDs[i]
					//eztools.ShowStrln("tran ID=" + tranID)
					break
				}
			}
		} else {
			if uiSilent {
				defer noInteractionAllowed()
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
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.Log(id + " in transition")
		if eztools.Verbose > 2 {
			eztools.ShowByteln(jsonStr)
		}
	}
	_, err = restSth(eztools.METHOD_POST, svr.URL+RestAPIStr+
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

// jiraFuncNTran is transitions for reject & close
func jiraFuncNTran(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, steps []string,
	fun func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos) error) error {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		slc, err := jiraMyOpen(svr, authInfo, issueInfo)
		var choices []string
		if err == nil {
			if len(slc) > 0 {
				for _, v := range slc {
					choices = append(choices,
						v[IssueinfoStrID]+":"+v[IssueinfoStrState])
				}
			}
		}
		i, s := eztools.ChooseStrings(choices)
		if i == eztools.InvalidID {
			if s == "" {
				return eztools.ErrInvalidInput
			}
			issueInfo[IssueinfoStrID] = s
		} else {
			issueInfo[IssueinfoStrID] = slc[i][IssueinfoStrID]
		}

	}
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return eztools.ErrInvalidInput
	}
	if fun != nil {
		if err := fun(svr, authInfo, issueInfo); err != nil {
			return err
		}

	}
	var (
		tranNames, tranIDs []string
		err                error
		//res                issueInfoSlc
	)
	for i, tran := range steps {
		if eztools.Debugging && eztools.Verbose > 2 {
			eztools.ShowStrln("Trying " + tran)
		}
		if len(tranNames) < 1 || len(tranIDs) < 1 {
			tranNames, tranIDs, err = jiraGetTrans(svr, authInfo,
				issueInfo[IssueinfoStrID], tran)
			if err != nil {
				return err
			}
		}
		tranID, err := jiraChooseTran(tran, tranNames, tranIDs)
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
		tranIDs = nil
		err = jiraTranExec(svr, authInfo,
			issueInfo[IssueinfoStrID], tranID)
		if err != nil {
			return err
		}
		//res = append(res, inf)
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
	issueInfo issueInfos, jsonInner string) error {
	jsonStr := jiraConstructFields(jsonInner)
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.Log("Processing " + issueInfo[IssueinfoStrID])
	}
	//eztools.ShowStrln(jsonStr)
	_, err := restSth(eztools.METHOD_PUT,
		svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID],
		authInfo, strings.NewReader(jsonStr),
		svr.Magic)
	return err
}

func jiraEditMeta(svr *svrs, authInfo eztools.AuthInfo, id, filter string) (interface{}, error) {
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
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

func jiraGetDesc(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (jsonStr string) {
	if len(svr.Flds.RejectRsn) > 0 {
		if len(issueInfo[IssueinfoStrDesc]) < 1 {
			// get all possible reasons
			field, err := jiraEditMeta(svr, authInfo, issueInfo[IssueinfoStrID],
				svr.Flds.RejectRsn)
			if err != nil {
				eztools.LogErr(err)
			} else {
				issueInfo[IssueinfoStrDesc] = getValuesFromMaps("value", field)
			}
			if len(issueInfo[IssueinfoStrDesc]) < 1 {
				eztools.Log("NO choices found for " + svr.Flds.RejectRsn)
				cfmInputOrPromptStr(svr, issueInfo,
					IssueinfoStrDesc, "reject reason")
			}
		}
		if len(issueInfo[IssueinfoStrDesc]) > 0 {
			for i, v := range map[string]string{
				"value": issueInfo[IssueinfoStrDesc]} {
				jsonStr = custFld(jsonStr, i, v)
			}
			if len(jsonStr) < 1 {
				eztools.LogPrint("NO RejectRsn field " +
					"defined for this server")
				issueInfo[IssueinfoStrDesc] = ""
			} else {
				jsonStr = `        "` + svr.Flds.RejectRsn + `": {` + jsonStr + `}`
			}
		}
	} else {
		issueInfo[IssueinfoStrDesc] = ""
	}
	return
}

func jiraReject(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	Steps := []string{"Back to process",
		"Resolved", "SCM integrating", "Verify failed", "Reopen",
		"Implementing", "Reject"}
	var jsonStr string
	firstRun := true
	return nil, jiraFuncNTran(svr, authInfo, issueInfo, Steps,
		func(svr *svrs, authInfo eztools.AuthInfo,
			issueInfo issueInfos) error {
			if len(issueInfo[IssueinfoStrComments]) > 0 {
				_, err := jiraAddComment1(svr, authInfo, issueInfo)
				if err != nil {
					eztools.LogErrPrint(err)
				}
			}
			/* senarios
			| entrance time | issueInfo | jsonStr | to process |
			| ------------- | --------- | ------- | ---------- |
			| 0             | ""        | ""      | choose, make jsonStr and send |
			| 0             | sth.      | ""      | make jsonStr and send |
			| n             | ""        | ""      | N |
			| n             | sth.      | sth.    | send only |
			*/
			if firstRun {
				jsonStr = jiraGetDesc(svr, authInfo, issueInfo)
				firstRun = false
			}
			if len(jsonStr) > 0 {
				return jiraEditWtFields(svr, authInfo, issueInfo, jsonStr)

			}
			return nil
		})
}

func jiraCloseWtQA(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, qa string) (issueInfoSlc, error) {
	useInputOrPromptStr(issueInfo, IssueinfoStrComments,
		"test step for closure")
	Steps := []string{"Verify failed",
		"Reopen", "Implementing", "Back to process",
		"Assign owner", "Resolved"}
	if len(qa) < 1 {
		return nil, jiraFuncNTran(svr, authInfo, issueInfo, Steps, nil)
	}
	var jsonStr string
	for i, v := range map[string]string{
		svr.Flds.TstPre:  "none",
		svr.Flds.TstStep: qa,
		svr.Flds.TstExp:  "none"} {
		jsonStr = custFld(jsonStr, i, v)
	}
	if len(jsonStr) < 1 {
		eztools.LogPrint("NO Tst* fields " +
			"defined for this server")
	}
	return nil, jiraFuncNTran(svr, authInfo, issueInfo, Steps,
		func(svr *svrs, authInfo eztools.AuthInfo,
			issueInfo issueInfos) error {
			return jiraEditWtFields(svr, authInfo, issueInfo, jsonStr)
		})
}

func jiraClose(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return jiraCloseWtQA(svr, authInfo, issueInfo, issueInfo[IssueinfoStrComments])
}

func jiraCloseDef(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "default AOSP/vendor/design")
}

func jiraCloseGen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "general requirement")
}

func jiraTransition(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	names, ids, err := jiraGetTrans(svr, authInfo,
		issueInfo[IssueinfoStrID], "")
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

func jiraLink(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
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
	if uiSilent { // TODO: command params
		defer noInteractionAllowed()
		return nil, eztools.ErrInvalidInput
	}
	linkType := eztools.ChooseStringsWtIDs(
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
	_, err = restMap(eztools.METHOD_PUT, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID],
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	// TODO: parse result
	return nil, err
}

func jiraModComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrComments]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	if len(issueInfo[IssueinfoStrKey]) < 1 {
		inf, err := jiraComments(svr, authInfo, issueInfo)
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
	_, err = restMap(eztools.METHOD_PUT,
		svr.URL+RestAPIStr+issueInfo[IssueinfoStrID]+"/comment/"+
			issueInfo[IssueinfoStrKey], authInfo,
		bytes.NewReader(jsonStr), svr.Magic)
	// TODO: parse result
	return nil, err
}

func jiraDelComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	// TODO: select key
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrKey]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restMap(eztools.METHOD_DEL, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/comment/"+issueInfo[IssueinfoStrKey],
		authInfo, nil, svr.Magic)
	// TODO: parse result
	return nil, err
}

func jiraAddComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
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
	if eztools.Debugging && eztools.Verbose > 2 {
		eztools.ShowByteln(jsonStr)
	}
	bodyMap, err = restMap(eztools.METHOD_POST, svr.URL+RestAPIStr+
		id+"/"+urlSuffix,
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	if err != nil {
		return
	}
	return
}

func jiraAddComment1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
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

func jiraComments(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/comment",
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return jiraParseCmts(bodyMap)
}

func jiraDetailExec(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (map[string]interface{}, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID], authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return bodyMap, err
}

func jiraDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
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

func jiraMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	const RestAPIStr = "rest/api/latest/search?jql="
	var states string
	for _, v := range svr.State {
		if v.Type == "not open" {
			if len(v.Text) > 0 {
				states += "&status!=" + v.Text
			}
		}
	}
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		url.QueryEscape("assignee="+authInfo.User+states),
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return jiraParseIssues(bodyMap), err
}

func jiraWatcherList(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	var res issueInfoSlc
	loopStringMap(bodyMap, "watchers", nil,
		func(_ string, watchersI interface{}) bool {
			watchersS, ok := watchersI.([]interface{})
			if !ok {
				eztools.LogPrint(reflect.TypeOf(watchersS).String() +
					" got instead of " +
					"[]interface{}")
				return false
			}
			for _, watcherI := range watchersS {
				inf := chkNLoopStringMap(watcherI, "",
					[]string{"name", "displayName"})
				if inf == nil {
					return false
				}
				res = append(res, issueInfos{
					IssueinfoStrDispname: inf[1],
					IssueinfoStrID:       inf[0]})
			}
			return true
		})
	return res, nil
}

func jiraWatcherCheck(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	var res issueInfoSlc
	watchingI := bodyMap["isWatching"]
	watching, ok := watchingI.(bool)
	if !ok {
		eztools.LogPrint(reflect.TypeOf(watchingI).String() +
			" got instead of " +
			"bool")
		return nil, err
	}
	res = append(res, issueInfos{
		IssueinfoStrState: strconv.FormatBool(watching)})
	return res, nil
}

func jiraWatcherAdd(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restMap(eztools.METHOD_POST, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/watchers",
		authInfo, strings.NewReader("\""+cfg.User+"\""), svr.Magic)
	return nil, err
}

func jiraWatcherDel(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restMap(eztools.METHOD_DEL, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/watchers?username="+cfg.User,
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	return nil, err
}

func jiraParseAttachments(bodyMap map[string]interface{}) (issues issueInfoSlc) {
	bodyInt := bodyMap["fields"]
	if bodyInt == nil {
		eztools.LogPrint("NO fields to parse")
		return
	}
	bodyFlds, ok := bodyInt.(map[string]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(bodyInt).String() +
			" got instead of map of string to interface{}")
		return
	}
	if bodyFlds["attachment"] == nil {
		eztools.Log("NO attachment to parse")
		return
	}
	bodySlc, ok := bodyFlds["attachment"].([]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(bodyFlds["attachment"]).String() +
			" got instead of slice of interface{}")
		return
	}
	for _, slc1 := range bodySlc {
		map1, ok := slc1.(map[string]interface{})
		if !ok {
			eztools.LogPrint(reflect.TypeOf(slc1).String() +
				" got instead of map of string to interface{}")
			continue
		}
		inf, _ := loopStringMap(map1, "", []string{
			"self", "content", "filename", "id", "mimeType"}, nil)
		var sz string
		if map1[IssueinfoStrSize] != nil {
			szI, ok := map1[IssueinfoStrSize].(float64)
			if !ok {
				eztools.LogPrint(reflect.TypeOf(map1[IssueinfoStrSize]).String() +
					" got instead of float64")
			} else {
				sz = eztools.TranSize(int64(szI), 1, false)
			}
		}
		issues = append(issues, issueInfos{
			IssueinfoStrBin:  inf[0],
			IssueinfoStrLink: inf[1],
			IssueinfoStrFile: inf[2],
			IssueinfoStrKey:  inf[3],
			IssueinfoStrDesc: inf[4],
			IssueinfoStrSize: sz})
	}
	return
}

func jiraAddFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 ||
		len(issueInfo[IssueinfoStrFile]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restFile(eztools.METHOD_POST, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/attachments",
		authInfo, "file", issueInfo[IssueinfoStrFile],
		map[string]string{"X-Atlassian-Token": "nocheck"}, svr.Magic)
	if err != nil {
		return nil, err
	}
	return nil, err
}

func jiraListFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	bodyMap, err := jiraDetailExec(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	return jiraParseAttachments(bodyMap), nil
}

func jiraGetFileInf(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	inf, err := jiraListFile(svr, authInfo, issueInfo)
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

func jiraGetFile(svr *svrs, authInfo eztools.AuthInfo,
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
	issueInfo, err = jiraGetFileInf(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(issueInfo[IssueinfoStrLink]) < 1 {
		return nil, eztools.ErrNoValidResults
	}
	if len(issueInfo[IssueinfoStrFile]) < 1 || isDir {
		issueInfo[IssueinfoStrFile] = filepath.Join(issueInfo[IssueinfoStrFile],
			issueInfo[IssueinfoStrName])
	}
	errNo, err := eztools.RestGetOrPostSaveFile(eztools.METHOD_GET,
		issueInfo[IssueinfoStrLink], authInfo, []byte(svr.Magic),
		issueInfo[IssueinfoStrFile])
	issueInfo[IssueinfoStrState] = strconv.Itoa(errNo)
	return issueInfo.ToSlc(), err
}

func jiraDelFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
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
	_, err := restSth(eztools.METHOD_DEL,
		svr.URL+RestAPIStr+issueInfo[IssueinfoStrKey],
		authInfo, nil, svr.Magic)
	return nil, err
}
