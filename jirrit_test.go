package main

import (
	"testing"

	"gitee.com/bon-ami/eztools/v6"
)

func runFunc(fun action2Func, svr *svrs) (string, error) {
	authInfo, err := cfg2AuthInfo(*svr, cfg)
	if err != nil {
		return "NO password configured for " + svr.Name, err
	}
	if len(authInfo.User) < 1 {
		return "NO user configured for authinfo", err
	}
	issueInfo := make(issueInfos)
	_ /*issues*/, err = fun.f(svr, authInfo, issueInfo)
	if err != nil {
		//return runtime.FuncForPC(reflect.ValueOf(fun).Pointer()).Name() + " failed", false
		return fun.n + " failed", err
	}
	/*for _, issue := range issues {
		eztools.ShowStrln("Issuse ID=" + issue[IssueinfoIndID])
		eztools.ShowStrln("Issuse HEAD=" + issue[IssueinfoIndHead])
		eztools.ShowStrln("Issuse PROJ=" + issue[IssueinfoIndProj])
		eztools.ShowStrln("Issuse BRANCH=" + issue[IssueinfoIndBranch])
	}*/
	return "", err
}

func test1(t *testing.T, cat, fun string) {
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
			t.Log("Server " + s.Name + ", Func " + f.n)
			errMsg, err := runFunc(f, &s)
			if err != nil && err != eztools.ErrNoValidResults {
				t.Error(errMsg, err)
				t.FailNow()
			}
		}
	}
}

func TestMain(t *testing.T) {
	var err []error
	cfgFile, err = eztools.XMLReadDefault("", "", "", "", module, &cfg)

	if err != nil {
		t.Skip("no config file found")
	}
}

func TestJiraMyOpen(t *testing.T) {
	test1(t, CategoryJira, "list my open cases")
}

func TestGerritMyOpen(t *testing.T) {
	test1(t, CategoryGerrit, "list my open submits")
}

func TestSave(t *testing.T) {
	for i, svr := range cfg.Svrs {
		prjOld := svr.Proj
		prjNew := prjOld + "TST"
		res := saveProj(&cfg.Svrs[i], prjNew)
		if res && len(prjOld) > 0 {
			saveProj(&cfg.Svrs[i], prjOld)
		}
		//break
	}
}
