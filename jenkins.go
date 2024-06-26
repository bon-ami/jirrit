package main

import (
	"fmt"
	"reflect"
	"strconv"

	"gitee.com/bon-ami/eztools/v4"
)

// jenkinsParseBlds get "name" & "url" from "jobs" or sth.
func jenkinsParseBlds(i interface{}) (issueInfoSlc, error) {
	if i == nil {
		Log(false, false, "NO builds got")
		return nil, nil
	}
	a, ok := i.([]interface{})
	if !ok {
		LogTypeErr(i, "slice")
		return nil, nil
	}
	var issues issueInfoSlc
	for _, e := range a {
		m, ok := e.(map[string]interface{})
		if !ok {
			Log(false, false, reflect.TypeOf(e).String()+
				" got instead of map string to interface!")
			continue
		}
		ni := m[IssueinfoStrNmb]
		if ni == nil {
			Log(false, false, "NO "+IssueinfoStrNmb+" found")
			continue
		}
		ns, ok := ni.(float64)
		if !ok {
			Log(false, false, reflect.TypeOf(ni).String()+
				" got instead of string!")
			continue
		}
		ui := m[IssueinfoStrURL]
		if ui == nil {
			Log(false, false, "NO "+IssueinfoStrURL+" found")
			continue
		}
		us, ok := ui.(string)
		if !ok {
			Log(false, false, reflect.TypeOf(ui).String()+
				" got instead of string!")
			continue
		}
		issues = append(issues, issueInfos{
			IssueinfoStrID:  strconv.FormatFloat(ns, 'f', 0, 64),
			IssueinfoStrURL: us,
		})
	}
	return issues, nil
}

func jenkinsListBlds(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	issueInfo, err := jenkinsChooseJob(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	//https://www.jianshu.com/p/ae7e003dfb2c
	const NumOfRes = "10"
	if len(issueInfo[IssueinfoStrSize]) < 1 {
		issueInfo[IssueinfoStrSize] = NumOfRes
	}
	var RestAPIStr = "/api/json?tree=builds[number,url]{," +
		issueInfo[IssueinfoStrSize] + "}"
	bodyMap, err := restMap(eztools.METHOD_GET,
		svr.URL+"job/"+issueInfo[IssueinfoStrProj]+RestAPIStr, authInfo, nil, svr.Magic)
	if err != nil || nil == bodyMap || len(bodyMap) < 1 {
		return nil, err
	}
	return jenkinsParseBlds(bodyMap[IssueinfoStrBld])
}

func jenkinsChooseBld(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	issueInfo, err := jenkinsChooseJob(svr, authInfo, issueInfo)
	if err != nil || len(issueInfo[IssueinfoStrID]) > 0 {
		return issueInfo, err
	}

	issues, err := jenkinsListBlds(svr, authInfo, issueInfo)
	if err != nil {
		return issueInfo, eztools.ErrNoValidResults
	}
	ind := eztools.ChooseStringsWtIDs(
		func() int {
			return len(issues)
		},
		func(ind int) int {
			return ind
		},
		func(ind int) string {
			return issues[ind][IssueinfoStrID]
		}, "Which build?")
	if ind == eztools.InvalidID {
		return issueInfo, eztools.ErrInvalidInput
	}
	issueInfo[IssueinfoStrID] = issues[ind][IssueinfoStrID]
	return issueInfo, nil
}

func jenkinsChooseJob(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	if len(issueInfo[IssueinfoStrProj]) > 0 {
		return issueInfo, nil
	}

	issues, err := jenkinsListJobs(svr, authInfo, issueInfo)
	if err != nil {
		return issueInfo, eztools.ErrNoValidResults
	}
	ind := eztools.ChooseStringsWtIDs(
		func() int {
			return len(issues)
		},
		func(ind int) int {
			return ind
		},
		func(ind int) string {
			return issues[ind][IssueinfoStrName]
		}, "Which job?")
	if ind == eztools.InvalidID {
		return issueInfo, eztools.ErrInvalidInput
	}
	issueInfo[IssueinfoStrProj] = issues[ind][IssueinfoStrName]
	return issueInfo, nil
}

// jenkinsParseJobs get "name" & "url" from "jobs" or sth.
func jenkinsParseJobs(i interface{}) (issueInfoSlc, error) {
	if i == nil {
		Log(false, false, "NO jobs got")
		return nil, nil
	}
	a, ok := i.([]interface{})
	if !ok {
		LogTypeErr(i, "slice")
		return nil, nil
	}
	var issues issueInfoSlc
	for _, e := range a {
		m, ok := e.(map[string]interface{})
		if !ok {
			Log(false, false, reflect.TypeOf(e).String()+
				" got instead of map string to interface!")
			continue
		}
		ni := m[IssueinfoStrName]
		if ni == nil {
			Log(false, false, "NO "+IssueinfoStrName+" found")
			continue
		}
		ns, ok := ni.(string)
		if !ok {
			Log(false, false, reflect.TypeOf(ni).String()+
				" got instead of string!")
			continue
		}
		ui := m[IssueinfoStrURL]
		if ui == nil {
			Log(false, false, "NO "+IssueinfoStrURL+" found")
			continue
		}
		us, ok := ui.(string)
		if !ok {
			Log(false, false, reflect.TypeOf(ui).String()+
				" got instead of string!")
			continue
		}
		issues = append(issues, issueInfos{
			IssueinfoStrName: ns,
			IssueinfoStrURL:  us,
		})
	}
	return issues, nil
}

func jenkinsListJobs(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	const RestAPIStr = "api/json"
	bodyMap, err := restMap(eztools.METHOD_GET,
		svr.URL+RestAPIStr, authInfo, nil, svr.Magic)
	if err != nil || nil == bodyMap || len(bodyMap) < 1 {
		return nil, err
	}
	return jenkinsParseJobs(bodyMap[IssueinfoStrJob])
}

func jenkinsParseBldActParams(parIn any, issueInfo issueInfos) bool {
	par1Map, ok := parIn.(map[string]interface{})
	if !ok {
		return false
	}
	// check class
	clsAny := par1Map["_class"]
	if clsAny == nil {
		return false
	}
	clsStr, ok := clsAny.(string)
	if !ok {
		Log(false, false, "class NOT a string!")
		return false
	}
	switch clsStr {
	case "hudson.model.StringParameterValue", "hudson.model.TextParameterValue":
		break
	default:
		return false
	}
	// check name
	clsAny = par1Map["name"]
	if clsAny == nil {
		return false
	}
	nmStr, ok := clsAny.(string)
	if !ok {
		Log(false, false, "name NOT a string!")
		return false
	}
	if nmStr == "" {
		return false
	}
	// get value
	clsAny = par1Map["value"]
	if clsAny == nil {
		return false
	}
	vlStr, ok := clsAny.(string)
	if !ok {
		Log(false, false, "value NOT a string!")
		return false
	}
	if vlStr != "" {
		issueInfo[nmStr] = vlStr
	}
	return true
}

func jenkinsParseBldParams(act1Map map[string]any, issueInfo issueInfos) bool {
	parAny := act1Map["parameters"]
	if parAny == nil {
		Log(false, false, "parameters NOT found")
		return false
	}
	parSlc, ok := parAny.([]interface{})
	if !ok || parSlc == nil {
		Log(false, false, "parameters NOT a slice")
		return false
	}
	for _, par1 := range parSlc {
		jenkinsParseBldActParams(par1, issueInfo)
	}
	return true
}

func jenkinsParseBldActCause(parIn any, issueInfo issueInfos) bool {
	par1Map, ok := parIn.(map[string]interface{})
	if !ok {
		return false
	}
	// check class
	clsAny := par1Map["_class"]
	if clsAny == nil {
		return false
	}
	clsStr, ok := clsAny.(string)
	if !ok {
		Log(false, false, "class NOT a string!")
		return false
	}
	switch clsStr {
	case "hudson.model.Cause$UserIdCause":
		break
	default:
		return false
	}
	// check userName
	clsAny = par1Map["userName"]
	if clsAny == nil {
		return false
	}
	nmStr, ok := clsAny.(string)
	if !ok {
		Log(false, false, "name NOT a string!")
		return false
	}
	if nmStr == "" {
		return false
	}
	issueInfo[IssueinfoStrAuthor] = nmStr
	return true
}

func jenkinsParseBldCause(act1Map map[string]any, issueInfo issueInfos) bool {
	parAny := act1Map["causes"]
	if parAny == nil {
		Log(false, false, "parameters NOT found")
		return false
	}
	parSlc, ok := parAny.([]interface{})
	if !ok || parSlc == nil {
		Log(false, false, "parameters NOT a slice")
		return false
	}
	for _, par1 := range parSlc {
		jenkinsParseBldActCause(par1, issueInfo)
	}
	return true
}

func jenkinsParseBldDisp(act1Map map[string]any, issueInfo issueInfos) bool {
	parAny := act1Map[IssueinfoStrBldin]
	if parAny == nil {
		Log(false, false, "parameters NOT found")
		return false
	}
	parBool, ok := parAny.(bool)
	if !ok {
		Log(false, false, "parameters NOT a bool, but a ", fmt.Sprintf("%T", parAny))
		return false
	}
	issueInfo[IssueinfoStrBldin] = strconv.FormatBool(parBool)
	return true
}

func jenkinsParseDtlBld(bodyMap map[string]interface{}) (issueInfos, error) {
	issueInfo := make(issueInfos)
	if bodyMap["actions"] == nil {
		Log(false, false, "NO actions found")
		return nil, eztools.ErrNoValidResults
	}
	actSlc, ok := bodyMap["actions"].([]interface{})
	if !ok {
		Log(false, false, "actions NOT a slice!")
		return nil, eztools.ErrNoValidResults
	}
	for _, act1Any := range actSlc {
		act1Map, ok := act1Any.(map[string]interface{})
		if !ok {
			continue
		}
		clsAny := act1Map["_class"]
		if clsAny == nil {
			continue
		}
		clsStr, ok := clsAny.(string)
		if !ok {
			Log(false, false, "class NOT a string!")
			continue
		}
		switch clsStr {
		case "hudson.model.ParametersAction":
			jenkinsParseBldParams(act1Map, issueInfo)
		case "hudson.model.CauseAction":
			jenkinsParseBldCause(act1Map, issueInfo)
		}
	}
	jenkinsParseBldDisp(bodyMap, issueInfo)
	if len(issueInfo) < 1 {
		return nil, eztools.ErrNoValidResults
	}
	return issueInfo, nil
}

func jenkinsDetailOnBld(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	issueInfo, err := jenkinsChooseBld(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(issueInfo[IssueinfoStrProj]) < 1 || len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "/api/json"
	bodyMap, err := restMap(eztools.METHOD_GET,
		svr.URL+"job/"+issueInfo[IssueinfoStrProj]+"/"+
			issueInfo[IssueinfoStrID]+RestAPIStr,
		authInfo, nil, svr.Magic)
	if err != nil || nil == bodyMap || len(bodyMap) < 1 {
		return nil, err
	}
	issueInfo, err = jenkinsParseDtlBld(bodyMap) // input info no need further
	return issueInfo.ToSlc(), err
}

func jenkinsLogOfBld(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	issueInfo, err := jenkinsChooseBld(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(issueInfo[IssueinfoStrProj]) < 1 || len(issueInfo[IssueinfoStrID]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "/consoleText"
	body, err := restSth(eztools.METHOD_GET,
		svr.URL+"job/"+issueInfo[IssueinfoStrProj]+
			"/"+issueInfo[IssueinfoStrID]+RestAPIStr,
		authInfo, nil, svr.Magic)
	if err != nil || nil == body {
		return nil, err
	}
	bodyBytes, ok := body.([]byte)
	if !ok {
		Log(false, false, reflect.TypeOf(body).String()+
			" got instead of slice of bytes!")
		return nil, eztools.ErrOutOfBound
	}
	if len(issueInfo[IssueinfoStrFile]) > 0 {
		err = eztools.FileWrite(issueInfo[IssueinfoStrFile],
			bodyBytes)
		if err == nil {
			return nil, err
		}
	}
	eztools.ShowStrln(string(bodyBytes))
	return nil, err
}
