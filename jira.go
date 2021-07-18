package main

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"gitee.com/bon-ami/eztools"
)

func jiraParse1Field(m map[string]interface{},
	issueInfo *issueInfos) (changed bool) {
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
			issueInfo[IssueinfoIndDispname] = val[0]
			changed = ch || changed
		case IssueinfoStrProj:
			val, ch := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrKey})
			issueInfo[IssueinfoIndProj] = val[0]
			changed = ch || changed
		case IssueinfoStrState:
			val, ch := chkNLoopStringMap(v, "",
				[]string{IssueinfoStrName})
			issueInfo[IssueinfoIndState] = val[0]
			changed = ch || changed
		case IssueinfoStrSummary:
			changed = chkNSetIssueInfo(v, issueInfo,
				IssueinfoIndHead) || changed
		case IssueinfoStrDesc:
			changed = chkNSetIssueInfo(v, issueInfo,
				IssueinfoIndDesc) || changed
		}
	}
	return
}

func jiraParse1Issue(m map[string]interface{},
	issueInfo *issueInfos) (changed bool) {
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
	issueInfo[IssueinfoIndID] = id[0]
	return
}

func jiraParseTrans(m map[string]interface{}) (tranNames, tranIDs []string) {
	f := func(i string, v interface{}) bool {
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
	}
	loopStringMap(m, "transitions", nil, f)
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
	f := func(i string, v interface{}) bool {
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
			var issueInfo issueInfos
			/*if*/ jiraParse1Issue(issue, &issueInfo) // {
			//eztools.ShowSthln(issueInfo)
			results = append(results, issueInfo)
			//}
		}
		return true
	}
	loopStringMap(m, "issues", nil, f)
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
	loopStringMap(m, IssueinfoStrComments,
		nil, func(i string, v interface{}) bool {
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
					[]string{"body", "updated"},
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
						IssueinfoIndComment: inf[0],
						IssueinfoIndBranch:  inf[1],
						IssueinfoIndKey:     author})
				}
			}
			return false
		})
	return issues, nil
}

func jiraTransfer(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	for {
		changed := cfmInputOrPrompt(svr, &issueInfo, IssueinfoIndID)
		changed = cfmInputOrPromptStr(svr, &issueInfo,
			IssueinfoIndHead, "change to assignee") || changed
		changed = cfmInputOrPromptStr(svr, &issueInfo,
			IssueinfoIndComment, "change to component") || changed
		if !changed {
			return nil, nil
		}
		if len(issueInfo[IssueinfoIndID]) < 1 ||
			/*(*/ len(issueInfo[IssueinfoIndHead]) < 1 { //&&
			//len(issueInfo[ISSUEINFO_IND_PROJ]) < 1) {
			return nil, eztools.ErrInvalidInput
		}
		const RestAPIStr = "rest/api/latest/issue/"
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
		if len(issueInfo[IssueinfoIndComment]) > 0 {
			var (
				upCA updateCA
				is   insets
				ss   setss
			)
			is.Name = issueInfo[IssueinfoIndComment]
			ss.Set = append(ss.Set, is)
			upCA.Update.Components = []setss{ss}
			s.Set.Name = issueInfo[IssueinfoIndHead]
			upCA.Update.Assignee = []sets{s}
			jsonStr, err = json.Marshal(upCA)
		} else {
			var upA updateA
			s.Set.Name = issueInfo[IssueinfoIndHead]
			upA.Update.Assignee = []sets{s}
			jsonStr, err = json.Marshal(upA)
		}
		if err != nil {
			return nil, err
		}
		//eztools.ShowByteln(jsonStr)
		bodyMap, err := restMap(eztools.METHOD_PUT,
			svr.URL+RestAPIStr+issueInfo[IssueinfoIndID],
			authInfo, bytes.NewReader(jsonStr), svr.Magic)
		if err != nil {
			return nil, err
		}
		if postREST != nil {
			postREST([]interface{}{bodyMap})
		}
	}
}

func jiraGetTrans(svr *svrs, authInfo eztools.AuthInfo,
	id, tranName string) (tranNames, tranIDs []string, err error) {
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		id+"/transitions", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
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
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_POST, svr.URL+RestAPIStr+
		id+"/transitions", authInfo,
		bytes.NewReader(jsonStr), svr.Magic)
	if err != nil {
		return err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return nil
}

func jiraFuncNTran(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos, steps []string,
	fun func(svr *svrs, authInfo eztools.AuthInfo,
		issueInfo *issueInfos) error) ([]issueInfos, error) {
	firstRun := true
	for {
		if !cfmInputOrPrompt(svr, &issueInfo, IssueinfoIndID) && !firstRun {
			return nil, nil
		}
		firstRun = false
		if len(issueInfo[IssueinfoIndID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		_, err := loopIssues(svr, issueInfo, func(issueInfo issueInfos) (
			issueInfos, error) {
			if eztools.Debugging && eztools.Verbose > 1 {
				eztools.Log("Processing " + issueInfo[IssueinfoIndID])
			}
			if fun != nil {
				if err := fun(svr, authInfo, &issueInfo); err != nil {
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
						issueInfo[IssueinfoIndID], tran)
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
					issueInfo[IssueinfoIndID], tranID)
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
	issueInfo *issueInfos, jsonInner string) error {
	jsonStr := jiraConstructFields(jsonInner)
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.Log("Processing " + issueInfo[IssueinfoIndID])
	}
	//eztools.ShowStrln(jsonStr)
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_PUT,
		svr.URL+RestAPIStr+
			issueInfo[IssueinfoIndID],
		authInfo, strings.NewReader(jsonStr),
		svr.Magic)
	if err != nil {
		return err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return nil
}

func jiraEditMeta(svr *svrs, authInfo eztools.AuthInfo, id, filter string) (interface{}, error) {
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		id+"/editmeta", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
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
	issueInfo *issueInfos) (jsonStr string) {
	if len(svr.Flds.RejectRsn) > 0 {
		if len(issueInfo[IssueinfoIndDesc]) < 1 {
			// get all possible reasons
			field, err := jiraEditMeta(svr, authInfo, issueInfo[IssueinfoIndID],
				svr.Flds.RejectRsn)
			if err != nil {
				eztools.LogErr(err)
			} else {
				issueInfo[IssueinfoIndDesc] = getValuesFromMaps("value", field)
			}
			if len(issueInfo[IssueinfoIndDesc]) < 1 {
				eztools.Log("NO choices found for " + svr.Flds.RejectRsn)
				cfmInputOrPromptStr(svr, issueInfo,
					IssueinfoIndDesc, "reject reason")
			}
		}
		if len(issueInfo[IssueinfoIndDesc]) > 0 {
			for i, v := range map[string]string{
				"value": issueInfo[IssueinfoIndDesc]} {
				jsonStr = custFld(jsonStr, i, v)
			}
			if len(jsonStr) < 1 {
				eztools.LogPrint("NO RejectRsn field " +
					"defined for this server")
				issueInfo[IssueinfoIndDesc] = ""
			} else {
				jsonStr = `        "` + svr.Flds.RejectRsn + `": {` + jsonStr + `}`
			}
		}
	} else {
		issueInfo[IssueinfoIndDesc] = ""
	}
	return
}

func jiraReject(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	Steps := []string{"Back to process",
		"Resolved", "SCM integrating", "Verify failed", "Reopen",
		"Implementing", "Reject"}
	cfmInputOrPromptStr(svr, &issueInfo,
		IssueinfoIndComment, "comment to add")
	var jsonStr string
	firstRun := true
	return jiraFuncNTran(svr, authInfo, issueInfo, Steps,
		func(svr *svrs, authInfo eztools.AuthInfo,
			issueInfo *issueInfos) error {
			if len(issueInfo[IssueinfoIndComment]) > 0 {
				_, err := jiraAddComment1(svr, authInfo, *issueInfo)
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
			issueInfo *issueInfos) error {
			return jiraEditWtFields(svr, authInfo, issueInfo, jsonStr)
		})
}

func jiraClose(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	return jiraCloseWtQA(svr, authInfo, issueInfo, issueInfo[IssueinfoIndComment])
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
		cfmInputOrPrompt(svr, &issueInfo, IssueinfoIndID)
		if len(issueInfo[IssueinfoIndID]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		names, ids, err := jiraGetTrans(svr, authInfo,
			issueInfo[IssueinfoIndID], "")
		if err != nil {
			return nil, err
		}
		tranID, err := jiraChooseTran("", names, ids)
		if err != nil {
			continue
		}
		err = jiraTranExec(svr, authInfo,
			issueInfo[IssueinfoIndID], tranID)
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
		changed := cfmInputOrPrompt(svr, &issueInfo, IssueinfoIndID)
		if len(issueInfo[IssueinfoIndID]) < 1 {
			return nil, nil
		}
		if len(issueInfo[IssueinfoIndState]) < 1 {
			issueInfo[IssueinfoIndState] = issueInfo[IssueinfoIndID]
		}
		changed = cfmInputOrPromptStr(svr, &issueInfo,
			IssueinfoIndState, "id to relate to") || changed
		if len(issueInfo[IssueinfoIndState]) < 1 ||
			issueInfo[IssueinfoIndState] ==
				issueInfo[IssueinfoIndID] {
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
		if len(issueInfo[IssueinfoIndID]) < 1 ||
			len(issueInfo[IssueinfoIndState]) < 1 {
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
		il.Add.II[IssueinfoStrKey] = issueInfo[IssueinfoIndState]
		jstru.Update.IL = append(jstru.Update.IL, il)
		jsonStr, err = json.Marshal(jstru)
		if err != nil {
			return nil, err
		}
		//eztools.ShowByteln(jsonStr)
		const RestAPIStr = "rest/api/latest/issue/"
		bodyMap, err := restMap(eztools.METHOD_PUT, svr.URL+RestAPIStr+
			issueInfo[IssueinfoIndID],
			authInfo, bytes.NewReader(jsonStr), svr.Magic)
		if err != nil {
			return nil, err
		}
		if postREST != nil {
			postREST([]interface{}{bodyMap})
		}
	}
}

func jiraAddComment(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	for {
		changed := cfmInputOrPrompt(svr, &issueInfo, IssueinfoIndID)
		changed = cfmInputOrPromptStr(svr, &issueInfo,
			IssueinfoIndComment, "comment to add") || changed
		if !changed {
			return nil, nil
		}
		if len(issueInfo[IssueinfoIndID]) < 1 ||
			len(issueInfo[IssueinfoIndComment]) < 1 {
			return nil, eztools.ErrInvalidInput
		}
		if _, err := jiraAddComment1(svr, authInfo, issueInfo); err != nil {
			return nil, err
		}
	}
}

func jiraPostSth(svr *svrs, urlSuffix string, authInfo eztools.AuthInfo,
	stru interface{}, id string) ([]issueInfos, error) {
	var (
		jsonStr []byte
		err     error
	)
	jsonStr, err = json.Marshal(stru)
	if err != nil {
		return nil, err
	}
	if eztools.Debugging && eztools.Verbose > 2 {
		eztools.ShowByteln(jsonStr)
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_POST, svr.URL+RestAPIStr+
		id+"/"+urlSuffix,
		authInfo, bytes.NewReader(jsonStr), svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return nil, nil
}

func jiraAddComment1(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	type comment1 struct {
		Comment1 string `json:"body"`
	}
	var (
		cmt comment1
	)
	cmt.Comment1 = issueInfo[IssueinfoIndComment]
	return jiraPostSth(svr, "comment", authInfo, cmt, issueInfo[IssueinfoIndID])
}

func jiraComments(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"/comment",
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return jiraParseCmts(bodyMap)
}

// TODO: input param not used
func jiraSearchCommentsByProjNUser(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndComment]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"/comment",
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return nil, err
}

func jiraDetail(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID], authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	jiraParse1Issue(bodyMap, &issueInfo)
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
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return jiraParseIssues(bodyMap), err
}

func jiraWatcherList(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
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
					IssueinfoIndDispname: inf[1],
					IssueinfoIndID:       inf[0]})
			}
			return true
		})
	return res, nil
}

func jiraWatcherCheck(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_GET, svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"/watchers", authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
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
		IssueinfoIndState: strconv.FormatBool(watching)})
	return res, nil
}

func jiraWatcherAdd(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_POST, svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"/watchers",
		authInfo, strings.NewReader("\""+cfg.User+"\""), svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return nil, nil
}

func jiraWatcherDel(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) ([]issueInfos, error) {
	if len(issueInfo[IssueinfoIndID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "rest/api/latest/issue/"
	bodyMap, err := restMap(eztools.METHOD_DEL, svr.URL+RestAPIStr+
		issueInfo[IssueinfoIndID]+"/watchers?username="+cfg.User,
		authInfo, nil, svr.Magic)
	if err != nil {
		return nil, err
	}
	if postREST != nil {
		postREST([]interface{}{bodyMap})
	}
	return nil, err
}
