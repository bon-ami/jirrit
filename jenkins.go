package main

import (
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
		Log(true, false, reflect.TypeOf(i).String()+
			" got instead of slice!")
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
		ui := m[IssueinfoStrUrl]
		if ui == nil {
			Log(false, false, "NO "+IssueinfoStrUrl+" found")
			continue
		}
		us, ok := ui.(string)
		if !ok {
			Log(false, false, reflect.TypeOf(ui).String()+
				" got instead of string!")
			continue
		}
		issues = append(issues, issueInfos{
			IssueinfoStrKey: strconv.FormatFloat(ns, 'f', 0, 64),
			IssueinfoStrUrl: us,
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
		svr.URL+"job/"+issueInfo[IssueinfoStrID]+RestAPIStr, authInfo, nil, svr.Magic)
	if err != nil || nil == bodyMap || len(bodyMap) < 1 {
		return nil, err
	}
	return jenkinsParseBlds(bodyMap[IssueinfoStrBld])
}

func jenkinsChooseBld(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	issueInfo, err := jenkinsChooseJob(svr, authInfo, issueInfo)
	if err != nil || len(issueInfo[IssueinfoStrKey]) > 0 {
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
			return issues[ind][IssueinfoStrKey]
		}, "Which build?")
	if ind == eztools.InvalidID {
		return issueInfo, eztools.ErrInvalidInput
	}
	issueInfo[IssueinfoStrKey] = issues[ind][IssueinfoStrKey]
	return issueInfo, nil
}

func jenkinsChooseJob(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfos, error) {
	if len(issueInfo[IssueinfoStrID]) > 0 {
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
	issueInfo[IssueinfoStrID] = issues[ind][IssueinfoStrName]
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
		Log(true, false, reflect.TypeOf(i).String()+
			" got instead of slice!")
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
		ui := m[IssueinfoStrUrl]
		if ui == nil {
			Log(false, false, "NO "+IssueinfoStrUrl+" found")
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
			IssueinfoStrUrl:  us,
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

func jenkinsParseDtlBld(bodyMap map[string]interface{}) (issueInfos, error) {
	actInt := bodyMap["actions"]
	if actInt == nil {
		Log(false, false, "NO actions found")
		return nil, eztools.ErrNoValidResults
	}
	actSlc, ok := actInt.([]interface{})
	if !ok {
		Log(false, false, "actions NOT a slice!")
		return nil, eztools.ErrNoValidResults
	}
	issueInfo := make(issueInfos)
	for _, act1Int := range actSlc {
		act1Map, ok := act1Int.(map[string]interface{})
		if !ok {
			continue
		}
		clsInt := act1Map["_class"]
		if clsInt == nil {
			continue
		}
		clsStr, ok := clsInt.(string)
		if !ok {
			Log(false, false, "class NOT a string!")
			continue
		}
		if clsStr != "hudson.model.ParametersAction" {
			continue
		}
		parInt := act1Map["parameters"]
		if parInt == nil {
			Log(false, false, "parameters NOT found")
			continue
		}
		parSlc, ok := parInt.([]interface{})
		if !ok || parSlc == nil {
			Log(false, false, "parameters NOT a slice")
			continue
		}
		for _, par1Int := range parSlc {
			par1Map, ok := par1Int.(map[string]interface{})
			if !ok {
				continue
			}
			// check class
			clsInt := par1Map["_class"]
			if clsInt == nil {
				continue
			}
			clsStr, ok := clsInt.(string)
			if !ok {
				Log(false, false, "class NOT a string!")
				continue
			}
			if clsStr != "hudson.model.StringParameterValue" {
				continue
			}
			// check name
			clsInt = par1Map["name"]
			if clsInt == nil {
				continue
			}
			nmStr, ok := clsInt.(string)
			if !ok {
				Log(false, false, "name NOT a string!")
				continue
			}
			if nmStr == "" {
				continue
			}
			// get value
			clsInt = par1Map["value"]
			if clsInt == nil {
				continue
			}
			vlStr, ok := clsInt.(string)
			if !ok {
				Log(false, false, "value NOT a string!")
				continue
			}
			if vlStr != "" {
				issueInfo[nmStr] = vlStr
			}
		}
	}
	return issueInfo, nil
}

func jenkinsDetailOnBld(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo issueInfos) (issueInfoSlc, error) {
	issueInfo, err := jenkinsChooseBld(svr, authInfo, issueInfo)
	if err != nil {
		return nil, err
	}
	if len(issueInfo[IssueinfoStrID]) < 1 || len(issueInfo[IssueinfoStrKey]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "/api/json"
	bodyMap, err := restMap(eztools.METHOD_GET,
		svr.URL+"job/"+issueInfo[IssueinfoStrID]+"/"+
			issueInfo[IssueinfoStrKey]+RestAPIStr,
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
	if len(issueInfo[IssueinfoStrID]) < 1 || len(issueInfo[IssueinfoStrKey]) < 1 {
		return nil, eztools.ErrInvalidInput
	}
	const RestAPIStr = "/consoleText"
	body, err := restSth(eztools.METHOD_GET,
		svr.URL+"job/"+issueInfo[IssueinfoStrID]+
			"/"+issueInfo[IssueinfoStrKey]+RestAPIStr,
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
