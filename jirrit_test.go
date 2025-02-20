package main

import (
	"os"
	"sync"
	"testing"

	"gitee.com/bon-ami/eztools/v6"
	"github.com/stretchr/testify/suite"
)

const (
	infoSep    = ":"
	maxResults = 2
)

var (
	ParamsTest params
	actionsAll cat2Act
	cfgLoad    sync.Once
)

type TestSuite struct {
	suite.Suite
	category, action, dependsOnDesc, dependsOnFunc string
	svr                                            *svrs
	authInfo                                       eztools.AuthInfo
	funs                                           []action2Func
	issueInfo                                      IssueInfos
	skipped, log2testing                           bool
}

func (s *TestSuite) Skip(args ...any) {
	s.skipped = true
	s.T().Skip(args...)
}

func (s *TestSuite) IsSkipped() bool {
	return s.skipped
}

// SetupTest loads cfg on the first run, and runs the depended test
func (s *TestSuite) SetDependency(desc, funcName string) {
	s.dependsOnDesc = desc
	s.dependsOnFunc = funcName
}

func (s *TestSuite) SetupTest() {
	// to use testing for logging
	if s.log2testing {
		cfgLoad.Do(func() {
			eztools.SetLogFunc(func(l ...any) {
				func(m ...any) {
					s.T().Log(m)
				}(eztools.GetCallerLog(), l)
			})
		})
	}
	if s.issueInfo == nil {
		s.issueInfo = mkIssueinfo(ParamsTest)
	}
	if s.svr == nil {
		var ok bool
		s.svr, ok = mkSvrByType(ParamsTest.r, s.category)
		if !ok {
			s.Skip("no server specified")
		}
		var err error
		s.authInfo, err = cfg2AuthInfo(*s.svr, cfg)
		if err != nil {
			s.Skip("NO password configured for", infoSep, s.svr.Name, infoSep, err)
		}
		if len(s.authInfo.User) < 1 {
			s.Skip("NO user configured for authinfo", infoSep, err)
		}
	}
	if len(s.dependsOnDesc) > 0 {
		if len(ParamsTest.i) < 1 {
			// dependent test only when ID is not provided
			sui := TestSuite{category: s.category,
				action: s.dependsOnDesc, issueInfo: s.issueInfo}
			s.Run(s.dependsOnFunc, func() {
				suite.Run(s.T(), &sui)
			})
			if sui.IsSkipped() {
				s.T().SkipNow()
			}
		}
	}
	if s.funs == nil {
		s.funs = matchFuncFromParam(s.action, s.svr, actionsAll)
	}
}

// Test1 is the main test function
func (s *TestSuite) Test1() {
	looper := DefLooper{para: ParamsTest, svr: s.svr,
		authInfo: s.authInfo, maxResults: maxResults}
	errs := LoopActions(s.svr, s.funs, s.issueInfo, &looper, nil)
	if errs != nil {
		// Log(false, false, err)
		for _, err1 := range errs {
			s.T().Error(err1)
		}
		s.T().FailNow()
	}
}

func mkSvrByType(name, category string) (*svrs, bool) {
	svr, ok := mkSvrFromParam(name)
	if ok && svr != nil {
		if svr.Type != category {
			ok = false
		}
		return svr, ok
	}
	for i, svr1 := range cfg.Svrs {
		if svr1.Type == category {
			return &cfg.Svrs[i], true
		}
	}
	return nil, false
}

func TestSave(t *testing.T) {
	for i, svr := range cfg.Svrs {
		prjOld := svr.Proj
		prjNew := prjOld + "TST"
		res := saveProj(&cfg.Svrs[i], prjNew)
		if res && len(prjOld) > 0 {
			saveProj(&cfg.Svrs[i], prjOld)
		}
	}
}

func init() {
	ParamsTest.Declare()
}

func TestMain(m *testing.M) {
	ParamsTest.Parse()
	// test/action related info, such as svrs, is loaded later
	var errs []error
	cfgFile, errs = eztools.XMLReadDefault(ParamsTest.cfg, "", "", "", module, &cfg)
	if errs != nil {
		eztools.ShowStrln("NO config file for tests")
		os.Exit(0)
	}
	actionsAll = makeCat2Act()
	os.Exit(m.Run())
}
