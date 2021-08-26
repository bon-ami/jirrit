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

func jiraParse1Field(m map[string]interface{},
	issueInfo issueInfos) (changed bool) {
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
			val, ch := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrDispname})
			issueInfo[IssueinfoStrDispname] = val[0]
			changed = ch || changed
		case IssueinfoStrProj:
			val, ch := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrKey})
			issueInfo[IssueinfoStrProj] = val[0]
			changed = ch || changed
		case IssueinfoStrState:
			val, ch := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrName})
			issueInfo[IssueinfoStrState] = val[0]
			changed = ch || changed
		case IssueinfoStrSummary:
			changed = chkNSetIssueInfo(v, issueInfo,
				IssueinfoStrHead) || changed
		case IssueinfoStrDesc:
			changed = chkNSetIssueInfo(v, issueInfo,
				IssueinfoStrDesc) || changed
		}
	}
	return
}

func jiraParse1Issue(m map[string]interface{},
	issueInfo issueInfos) (changed bool) {
	var id []string
	id, changed = loopStringMap(m, "fields",
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
			return jiraParse1Field(fields, issueInfo)
		})
	issueInfo[IssueinfoStrID] = id[0]
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
	/*if eztools.Debugging && eztools.Verbose > 2 {
		eztools.ShowSthln(tranNames)
		eztools.ShowSthln(tranIDs)
	}*/
	return
}

func jiraParseIssues(m map[string]interface{}) []issueInfos {
	/*if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowSthln(strs)
	}*/
	results := make([]issueInfos, 0)
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
				issueInfo := make(issueInfos)
				/*if*/ jiraParse1Issue(issue, issueInfo) // {
				//eztools.ShowSthln(issueInfo)
				results = append(results, issueInfo)
				//}
			}
			return true
		})
	if len(results) < 1 {
		return nil
	}
	return results
}

func jiraParseCmts(m map[string]interface{}) ([]issueInfos, error) {
	var (
		author string
		issues []issueInfos
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
				inf, _ := loopStringMap(cmt, "author",
					[]string{"body", "updated", "id"},
					func(i string, v interface{}) bool {
						id, _ := chkNLoopStringMap(v,
							"", []string{IssueinfoStrKey})
						if id == nil {
							return false
						}
						author = id[0]
						return false
					})
				if len(inf[0]) > 0 {
					issues = append(issues, issueInfos{
						IssueinfoStrComments: inf[0],
						IssueinfoStrBranch:   inf[1],
						IssueinfoStrID:       inf[2],
						IssueinfoStrKey:      author})
				}
			}
			return false
		})
	return issues, nil
}

func jiraTransfer(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	firstRun := true
	for {
		changed := cfmInputOrPrompt(svr, issueInfo, IssueinfoStrID)
		changed = cfmInputOrPromptStr(svr, issueInfo,
			IssueinfoStrHead, "change to assignee") || changed
		changed = cfmInputOrPromptStr(svr, issueInfo,
			IssueinfoStrComments, "change to component") || changed
		if !firstRun {
			if !changed {
				return nil, nil
			}
		} else {
			firstRun = false
		}
		if len(issueInfo[IssueinfoStrID]) < 1 ||
			/*(*/ len(issueInfo[IssueinfoStrHead]) < 1 { //&&
			//len(issueInfo[ISSUEINFO_IND_PROJ]) < 1) {
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
		//eztools.ShowByteln(jsonStr)
		_, err = restMap(eztools.METHOD_PUT,
			svr.URL+RestAPIStr+issueInfo[IssueinfoStrID],
			authInfo, bytes.NewReader(jsonStr), svr.Magic)
		if err != nil {
			return nil, err
		}
	}
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
				if tranName == string(v) {
					tranID = tranIDs[i]
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
			i := eztools.ChooseStrings(tranNames)
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
	id, tranID string) error {
	type tranJsons struct {
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
	}
	var tranJSON tranJsons
	tranJSON.Transition.ID = tranID
	jsonStr, err := json.Marshal(tranJSON)
	if err != nil {
		return err
	}
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.Log("Processing " + id)
	}
	//eztools.ShowByteln(jsonStr)
	_, err = restMap(eztools.METHOD_POST, svr.URL+RestAPIStr+
		id+"/transitions", authInfo,
		bytes.NewReader(jsonStr), svr.Magic)
	return err
}

func jiraFuncNTran(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, steps []string,
	fun func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo issueInfos) error) ([]issueInfos, error) {
	firstRun := true
	for {
		if !cfmInputOrPrompt(svr, issueInfo, IssueinfoStrID) && !firstRun {
			return nil, nil
		}
		firstRun = false
		if len(issueInfo[IssueinfoStrID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		_, err := loopIssues(svr, issueInfo, func(issueInfo issueInfos) (
			issueInfos, error) {
			if eztools.Debugging && eztools.Verbose > 1 {
				eztools.Log("Processing " + issueInfo[IssueinfoStrID])
			}
			if fun != nil {
				if err := fun(svr, authInfo, issueInfo); err != nil {
					return issueInfo, err
				}

			}
			var (
				tranNames, tranIDs []string
				err                error
			)
			for _, tran := range steps {
				if eztools.Debugging && eztools.Verbose > 2 {
					eztools.ShowStrln("Trying " + tran)
				}
				if len(tranNames) < 1 || len(tranIDs) < 1 {
					tranNames, tranIDs, err = jiraGetTrans(svr, authInfo,
						issueInfo[IssueinfoStrID], tran)
					if err != nil {
						return issueInfo, err
					}
				}
				tranID, err := jiraChooseTran(tran, tranNames, tranIDs)
				if err != nil {
					if err == eztools.ErrNoValidResults {
						tranNames = nil
						tranIDs = nil
					}
					continue
				}
				tranNames = nil
				tranIDs = nil
				err = jiraTranExec(svr, authInfo,
					issueInfo[IssueinfoStrID], tranID)
				if err != nil {
					return issueInfo, err
				}
			}
			return issueInfo, nil
		})
		if err != nil {
			return nil, err
		}
	}
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
	_, err := restMap(eztools.METHOD_PUT,
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
	issueInfo issueInfos) ([]issueInfos, error) {
	Steps := []string{"Back to process",
		"Resolved", "SCM integrating", "Verify failed", "Reopen",
		"Implementing", "Reject"}
	cfmInputOrPromptStr(svr, issueInfo,
		IssueinfoStrComments, "comment to add")
	var jsonStr string
	firstRun := true
	return jiraFuncNTran(svr, authInfo, issueInfo, Steps,
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
	issueInfo issueInfos, qa string) ([]issueInfos, error) {
	Steps := []string{"Verify failed",
		"Reopen", "Implementing", "Back to process",
		"Assign owner", "Resolved"}
	if len(qa) < 1 {
		return jiraFuncNTran(svr, authInfo, issueInfo, Steps, nil)
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
	return jiraFuncNTran(svr, authInfo, issueInfo, Steps,
		func(svr *svrs, authInfo eztools.AuthInfo,
			issueInfo issueInfos) error {
			return jiraEditWtFields(svr, authInfo, issueInfo, jsonStr)
		})
}

func jiraClose(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return jiraCloseWtQA(svr, authInfo, issueInfo, issueInfo[IssueinfoStrComments])
}

func jiraCloseDef(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "default AOSP/vendor/design")
}

func jiraCloseGen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return jiraCloseWtQA(svr, authInfo,
		issueInfo, "general requirement")
}

func jiraTransition(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	for {
		cfmInputOrPrompt(svr, issueInfo, IssueinfoStrID)
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
			continue
		}
		err = jiraTranExec(svr, authInfo,
			issueInfo[IssueinfoStrID], tranID)
		if err != nil {
			return nil, err
		}
	}
}

func jiraLink(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	linkChoices := []struct {
		name, inward, outward string
	}{
		{inward: "is blocked by",
			name:    "Blocks",
			outward: "blocks"}}
	linkType := eztools.InvalidID
	for {
		changed := cfmInputOrPrompt(svr, issueInfo, IssueinfoStrID)
		if len(issueInfo[IssueinfoStrID]) < 1 {
			return nil, nil
		}
		if len(issueInfo[IssueinfoStrState]) < 1 {
			issueInfo[IssueinfoStrState] = issueInfo[IssueinfoStrID]
		}
		changed = cfmInputOrPromptStr(svr, issueInfo,
			IssueinfoStrState, "id to relate to") || changed
		if len(issueInfo[IssueinfoStrState]) < 1 ||
			issueInfo[IssueinfoStrState] ==
				issueInfo[IssueinfoStrID] {
			return nil, nil
		}
		if uiSilent {
			defer noInteractionAllowed()
			return nil, eztools.ErrInvalidInput
		}
		i := eztools.ChooseStringsWtIDs(
			func() int {
				//return len(svr.Flds.LinkType)
				return len(linkChoices)
			},
			func(i int) int {
				return i
			},
			func(i int) string {
				//return svr.Flds.LinkType[i].String
				return linkChoices[i].name
			}, "link type")
		if i == eztools.InvalidID ||
			(!changed &&
				i == linkType &&
				/* not the first run*/
				linkType != eztools.InvalidID) {
			return nil, nil
		}
		linkType = i
		if len(issueInfo[IssueinfoStrID]) < 1 ||
			len(issueInfo[IssueinfoStrState]) < 1 {
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
		il.Add.II[IssueinfoStrKey] = issueInfo[IssueinfoStrState]
		jstru.Update.IL = append(jstru.Update.IL, il)
		jsonStr, err = json.Marshal(jstru)
		if err != nil {
			return nil, err
		}
		//eztools.ShowByteln(jsonStr)
		_, err = restMap(eztools.METHOD_PUT, svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID],
			authInfo, bytes.NewReader(jsonStr), svr.Magic)
		if err != nil {
			return nil, err
		}
	}
}

func jiraModComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	firstRun := true
	for {
		changed := cfmInputOrPrompt(svr, issueInfo, IssueinfoStrID)
		changed = cfmInputOrPromptStr(svr, issueInfo,
			IssueinfoStrKey, "ID of comment to change") || changed
		changed = cfmInputOrPromptStr(svr, issueInfo,
			IssueinfoStrComments, "body of comment to change") || changed
		if !firstRun {
			if !changed {
				return nil, nil
			}
		} else {
			firstRun = false
		}
		if len(issueInfo[IssueinfoStrID]) < 1 ||
			len(issueInfo[IssueinfoStrComments]) < 1 ||
			len(issueInfo[IssueinfoStrKey]) < 1 {
			return nil, eztools.ErrInvalidInput
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
		if err != nil {
			return nil, err
		}
	}
}

func jiraDelComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	firstRun := true
	for {
		changed := cfmInputOrPrompt(svr, issueInfo, IssueinfoStrID)
		changed = cfmInputOrPromptStr(svr, issueInfo,
			IssueinfoStrKey, "ID of comment to delete") || changed
		if !firstRun {
			if !changed {
				return nil, nil
			}
		} else {
			firstRun = false
		}
		if len(issueInfo[IssueinfoStrID]) < 1 ||
			len(issueInfo[IssueinfoStrKey]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		_, err := restMap(eztools.METHOD_DEL, svr.URL+RestAPIStr+
			issueInfo[IssueinfoStrID]+"/comment/"+issueInfo[IssueinfoStrKey],
			authInfo, nil, svr.Magic)
		if err != nil {
			return nil, err
		}
	}
}

func jiraAddComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	firstRun := true
	for {
		changed := cfmInputOrPrompt(svr, issueInfo, IssueinfoStrID)
		changed = cfmInputOrPromptStr(svr, issueInfo,
			IssueinfoStrComments, "comment to add") || changed
		if !firstRun {
			if !changed {
				return nil, nil
			}
		} else {
			firstRun = false
		}
		if len(issueInfo[IssueinfoStrID]) < 1 ||
			len(issueInfo[IssueinfoStrComments]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		issues, err := jiraAddComment1(svr, authInfo, issueInfo)
		if err != nil {
			return nil, err
		}
		dispResults(issues)
	}
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
	issueInfo issueInfos) ([]issueInfos, error) {
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
	return jiraParseCmts(body)
}

func jiraComments(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
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
	issueInfo issueInfos) ([]issueInfos, error) {
	bodyMap, err := jiraDetailExec(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	jiraParse1Issue(bodyMap, issueInfo)
	return []issueInfos{issueInfo}, nil
	//return jiraParseIssues(svr, bodyMap), err
}

func jiraMyOpen(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
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
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	var res []issueInfos
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
				inf, _ := chkNLoopStringMap(watcherI, "",
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
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	var res []issueInfos
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
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	_, err := restMap(eztools.METHOD_POST, svr.URL+RestAPIStr+
		issueInfo[IssueinfoStrID]+"/watchers",
		authInfo, strings.NewReader("\""+cfg.User+"\""), svr.Magic)
	return nil, err
}

func jiraWatcherDel(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
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

func jiraParseAttachments(bodyMap map[string]interface{}) (issues []issueInfos) {
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
	issueInfo issueInfos) ([]issueInfos, error) {
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
	issueInfo issueInfos) ([]issueInfos, error) {
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
	var choices []string
	for _, v := range inf {
		if len(issueInfo[IssueinfoStrKey]) > 0 {
			if v[IssueinfoStrKey] == issueInfo[IssueinfoStrKey] {
				issueInfo[IssueinfoStrLink] = v[IssueinfoStrLink]
				issueInfo[IssueinfoStrName] = v[IssueinfoStrFile]
				break
			}
			continue
		}
		choices = append(choices,
			v[IssueinfoStrFile]+"("+v[IssueinfoStrSize])
	}
	if choices != nil {
		i := eztools.ChooseStrings(choices)
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
	issueInfo issueInfos) ([]issueInfos, error) {
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
	return []issueInfos{issueInfo}, err
}

func jiraDelFile(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
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
