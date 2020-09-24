package main

import (
	"strconv"
	"testing"

	"github.com/bon-ami/eztools"
)

const debugging = 0

func runFunc(fun action2Func, svr *svrs, cfg cfgs) (string, bool) {
	authInfo, err := cfg2AuthInfo(*svr, cfg)
	if err != nil {
		return "NO password configured for " + svr.Name, false
	}
	eztools.ShowStrln("Server " + svr.Name + ", Func " + fun.n)
	var issueInfo issueInfos
	issues, err := fun.f(svr, authInfo, issueInfo)
	if err != nil {
		//return runtime.FuncForPC(reflect.ValueOf(fun).Pointer()).Name() + " failed", false
		return fun.n + " failed", false
	}
	for _, issue := range issues {
		eztools.ShowStrln("Issuse ID=" + issue[ISSUEINFO_IND_ID])
		eztools.ShowStrln("Issuse HEAD=" + issue[ISSUEINFO_IND_HEAD])
		eztools.ShowStrln("Issuse PROJ=" + issue[ISSUEINFO_IND_PROJ])
		eztools.ShowStrln("Issuse BRANCH=" + issue[ISSUEINFO_IND_BRANCH])
	}
	return "", true
}

func test1(t *testing.T, cat, fun string) {
	var cfg cfgs
	err := readCfg("", &cfg)

	if err != nil {
		t.Error("test.xml fails")
		t.FailNow()
	}
	setPostREST(func(bodySlc []interface{}) {
		if debugging < 3 {
			return
		}
		for i, v := range bodySlc {
			eztools.ShowStrln("Result " + strconv.Itoa(i))
			eztools.RangeStrMap(v, func(k string, v interface{}) bool {
				eztools.ShowStr(k + "=")
				eztools.ShowSthln(v)
				return false
			})
		}
	})
	cats := makeCat2Act()
	for _, s := range cfg.Svrs {
		if len(cat) > 0 && cat != s.Type {
			continue
		}
		if !isValidSvr(cats, &s) {
			t.Error("Wrong server configured " + s.Name)
			continue
		}
		for _, f := range cats[s.Type] {
			if len(fun) > 0 && f.n != fun {
				continue
			}
			errMsg, ok := runFunc(f, &s, cfg)
			if !ok {
				t.Error(errMsg)
			}
		}
	}
}

func TestJira(t *testing.T) {
	test1(t, CATEGORY_JIRA, "")
}

func TestGerrit(t *testing.T) {
	test1(t, CATEGORY_GERRIT, "")
}

func TestGerritAllOpen(t *testing.T) {
	test1(t, CATEGORY_GERRIT, "all open Gerrit")
}

func TestGerritMyOpen(t *testing.T) {
	test1(t, CATEGORY_GERRIT, "my open Gerrit")
}

func TestMain(t *testing.T) {
	test1(t, "", "")
}
