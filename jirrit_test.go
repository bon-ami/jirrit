package main

import (
	"os"
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
	// cfgLoad    sync.Once
)

type TestSuite struct {
	suite.Suite
	category, action, dependsOnDesc, dependsOnFunc string
	svr                                            *svrs
	authInfo                                       eztools.AuthInfo
	funs                                           []action2Func
	issueInfo                                      IssueInfos
	skipped                                        bool
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
	/* 	// to use testing for logging
		cfgLoad.Do(func() {
		eztools.SetLogFunc(func(l ...any) {
			func(m ...any) {
				t.T().Log(m)
			}(eztools.GetCallerLog(), l)
		})
	}) */
	if s.issueInfo == nil {
		s.issueInfo = mkIssueinfo(ParamsTest)
	}
	t := s.T()
	if len(s.dependsOnDesc) > 0 {
		if len(ParamsTest.i) < 1 {
			// dependent test only when ID is not provided
			sui := TestSuite{category: s.category,
				action: s.dependsOnDesc, issueInfo: s.issueInfo}
			s.Run(s.dependsOnFunc, func() {
				suite.Run(t, &sui)
			})
			if sui.IsSkipped() {
				t.SkipNow()
			}
		}
	}
	cat := s.category
	action := s.action
	if s.svr == nil {
		var ok bool
		s.svr, ok = mkSvrByType("", cat)
		if !ok {
			t.Skip("no server specified")
		}
		var err error
		s.authInfo, err = cfg2AuthInfo(*s.svr, cfg)
		if err != nil {
			t.Skip("NO password configured for", infoSep, s.svr.Name, infoSep, err)
		}
		if len(s.authInfo.User) < 1 {
			t.Skip("NO user configured for authinfo", infoSep, err)
		}
	}
	if s.funs == nil {
		s.funs = matchFuncFromParam(action, s.svr, actionsAll)
	}
}

// Test1 is the main test function
func (s *TestSuite) Test1() {
	// t := s.T()
	for _, fun1 := range s.funs {
		if inputIssueInfo4Act(s.svr, s.authInfo, fun1.n, s.issueInfo) {
			s.Skip(fun1.n, infoSep, "NOT enough info to run")
		}
		looper := DefLooper{ParamsTest, s.svr,
			s.authInfo, fun1.n, fun1.f, nil, maxResults}
		if _, err := loopIssues(s.svr, s.issueInfo, looper.Loop); err != nil {
			Log(false, false, err)
		}
		inf := looper.GetIssueInfo()
		// only IDs are stored for next rounds
		for _, info := range inf {
			if len(s.issueInfo[IssueinfoStrID]) > 0 {
				s.issueInfo[IssueinfoStrID] += issueSeparator
			}
			s.issueInfo[IssueinfoStrID] += info[IssueinfoStrID]
		}
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
	loadCfg(ParamsTest)
	actionsAll = makeCat2Act()
	os.Exit(m.Run())
}
