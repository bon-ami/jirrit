package main

import (
	"strconv"
	"testing"

	"gitee.com/bon-ami/eztools"
)

const debugging = 0

func runFunc(fun action2Func, svr *svrs, cfg jirrit) (string, bool) {
	authInfo, err := cfg2AuthInfo(*svr, cfg)
	if err != nil {
		return "NO password configured for " + svr.Name, false
	}
	eztools.ShowStrln("Server " + svr.Name + ", Func " + fun.n)
	var issueInfo issueInfos
	_ /*issues*/, err = fun.f(svr, authInfo, issueInfo)
	if err != nil {
		//return runtime.FuncForPC(reflect.ValueOf(fun).Pointer()).Name() + " failed", false
		return fun.n + " failed", false
	}
	/*for _, issue := range issues {
		eztools.ShowStrln("Issuse ID=" + issue[IssueinfoIndID])
		eztools.ShowStrln("Issuse HEAD=" + issue[IssueinfoIndHead])
		eztools.ShowStrln("Issuse PROJ=" + issue[IssueinfoIndProj])
		eztools.ShowStrln("Issuse BRANCH=" + issue[IssueinfoIndBranch])
	}*/
	return "", true
}

func test1(t *testing.T, cat, fun string) {
	_, err := eztools.XMLsReadDefaultNoCreate("", module, &cfg)

	if err != nil {
		eztools.ShowStrln("no config file found")
		return
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
			t.FailNow()
		}
		for _, f := range cats[s.Type] {
			if len(fun) > 0 && f.n != fun {
				continue
			}
			errMsg, ok := runFunc(f, &s, cfg)
			if !ok {
				t.Error(errMsg)
				t.FailNow()
			}
		}
	}
}

func TestMain(t *testing.T) {
	//test1(t, "", "") it will fail defintely
	testGerritMyOpen(t)
	testJiraMyOpen(t)
}

/*func TestJira(t *testing.T) {
	test1(t, CategoryJira, "")
}*/

func testJiraMyOpen(t *testing.T) {
	test1(t, CategoryJira, "list my open cases")
}

/*func TestGerrit(t *testing.T) {
	test1(t, CategoryGerrit, "")
}*/

/*func TestGerritAllOpen(t *testing.T) {
	test1(t, CategoryGerrit, "all open")
}*/

func testGerritMyOpen(t *testing.T) {
	test1(t, CategoryGerrit, "list my open submits")
}

func TestSave(t *testing.T) {
	var err error
	cfgFile, err = eztools.XMLsReadDefaultNoCreate("", module, &cfg)

	if err != nil {
		eztools.ShowStrln("no config file found")
		return
	}
	for i, svr := range cfg.Svrs {
		prjOld := svr.Proj
		prjNew := prjOld + "TST"
		res := saveProj(&cfg.Svrs[i], prjNew)
		if res && len(prjOld) > 0 {
			saveProj(&cfg.Svrs[i], prjOld)
		}
		break
	}
}
