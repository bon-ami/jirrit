package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	"gitee.com/bon-ami/eztools/v6"
)

const ( // exit codes
	extCfg = iota + 1
	extAuth
	extConn
	extInpt
	extRslt
	extGram
	extSrvr
)

const (
	module = "jirrit"
	// CategoryJira JIRA server in xml
	CategoryJira = "JIRA"
	// CategoryGerrit Gerrit server in xml
	CategoryGerrit = "Gerrit"
	// CategoryJenkins Jenkins server in xml
	CategoryJenkins = "Jenkins"
	// CategoryBugzilla Bugzilla server in xml
	CategoryBugzilla = "Bugzilla"
	// PassNone no password in xml
	PassNone = "none"
	// PassBasic plain text password in xml
	PassBasic = "basic"
	// PassPlain BASE64ed password in xml
	PassPlain = "plain"
	// PassDigest HTTP password in xml
	PassDigest = "digest"
	// PassToken for headers in xml
	PassToken = "token"
	// intGerritMerge is interval between each status check to merge a submit, in seconds
	intGerritMerge = 15
	// actionSep is the separator for multiple actions
	actionSep = ";"
	// issueSeparator is the separator for multiple IDs
	issueSeparator = ","
)

type sliceFlag []string

func (f *sliceFlag) String() string {
	return fmt.Sprintf("%v", []string(*f))
}

func (f *sliceFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

var (
	// Ver version
	Ver string
	// Bld build
	Bld       string
	stdOutput bool
	cfgFile   string
	paramS    sliceFlag
	cfg       jirrit
	uiSilent  bool
	step      int
	svrTypes  []string
	errAuth   = errors.New("auth failure")
	errConn   = errors.New("conn failure")
	errCfg    = errors.New("cfg failure")
	errGram   = errors.New("request failure in grammar")
	errSrvr   = errors.New("server error")
)

type passwords struct {
	Cmt  string `xml:",comment"`
	Type string `xml:"type,attr"`
	Pass string `xml:",chardata"`
}

type fields struct {
	Cmt       string   `xml:",comment"`
	RejectRsn string   `xml:"rejectrsn"`
	TstPre    string   `xml:"testpre"`
	TstStep   string   `xml:"teststep"`
	TstExp    string   `xml:"testexp"`
	Solution  []string `xml:"solution"`
}

type states struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type svrs struct {
	Cmt  string `xml:",comment"`
	Type string `xml:"type,attr"`
	Name string `xml:"name,attr"`
	URL  string `xml:"url"`
	// IP is informational only
	IP    string    `xml:"ip"`
	User  string    `xml:"user"`
	Pass  passwords `xml:"pass"`
	Magic string    `xml:"magic"`
	State []states  `xml:"state"`
	Flds  fields    `xml:"fields"`
	Proj  string    `xml:"project"`
	Watch string    `xml:"watch"`
}

type jirrit struct {
	Cmt        string `xml:",comment"`
	EzToolsCfg string `xml:"eztoolscfg"`
	AppUp      struct {
		Interval int    `xml:"interval"`
		Previous string `xml:"previous"`
	} `xml:"appup"`
	Log  string    `xml:"log"`
	User string    `xml:"user"`
	Pass passwords `xml:"pass"`
	Svrs []svrs    `xml:"server"`
}

// LogTypeErr logs failure in type conversion
func LogTypeErr(v any, exp string) {
	Log(stdOutput, false,
		reflect.TypeOf(v).String(),
		"found instead of", exp)
}

// Log wrapped logging and command output
func Log(onscreen, wttime bool, inf ...any) {
	if len(cfg.Log) < 1 {
		switch onscreen {
		case true:
			eztools.ShowStrln(inf...)
		}
		return
	}
	switch onscreen {
	case true:
		switch wttime {
		case true:
			eztools.LogPrintWtTime(inf...)
		case false:
			eztools.LogPrint(inf...)
		}
	case false:
		switch wttime {
		case true:
			eztools.LogWtTime(inf...)
		case false:
			eztools.Log(inf...)
		}
	}
}

type params struct {
	h, ver, v, vv, vvv, reverse, getSvrCfg, setSvrCfg         bool
	r, a, w, k, f, z, i, b, cfg, log, hd, p, l, c, fn, fv, fs string
	Def, CfgSvrOpt                                            string
}

func (p *params) Declare() {
	const ParamDef = "_"
	const cfgSvrOpt = "setsvrcfg"
	p.Def = ParamDef
	p.CfgSvrOpt = cfgSvrOpt
	flag.BoolVar(&p.h, "h", false, "help message")
	flag.BoolVar(&p.ver, "ver", false, "version info")
	flag.BoolVar(&p.ver, "version", false, "version info")
	flag.BoolVar(&p.v, "v", false,
		"log fiLe output")
	flag.BoolVar(&p.vv, "vv", false, "verbose messages")
	flag.BoolVar(&p.vvv, "vvv", false,
		"verbosE messages with network I/O")
	flag.BoolVar(&p.reverse, "reverse", false, "reverse output")
	flag.BoolVar(&p.getSvrCfg, "getsvrcfg", false,
		"get seRver list from config")
	flag.BoolVar(&p.setSvrCfg, cfgSvrOpt, false,
		"set servers as config")
	flag.StringVar(&p.r, "r", "", "server name, to be together with -a")
	flag.StringVar(&p.a, "a", "", "action, to be together with -r. "+
		"multiple actions separated by "+actionSep)
	flag.StringVar(&p.w, "w", ParamDef, "JIRA ID to store in settings, "+
		"to be together with -r. current setting shown, if empty value.")
	flag.StringVar(&p.k, "k", "", "key or description. reject reason for JIRA")
	flag.StringVar(&p.i, "i", "",
		"ID of isSue, change, commit or assignee, or build for Jenkins")
	flag.StringVar(&p.b, "b", "", "branch for JIRA and Gerrit")
	flag.StringVar(&p.hd, "hd", "",
		"new assignee when transferring issues, "+
			"or revision id for cherrypicks")
	flag.StringVar(&p.p, "p", "",
		"project for JIRA or Gerrit, state to trasit to for bugzilla, "+
			"or job ID for Jenkins")
	flag.StringVar(&p.c, "c", "",
		"new component when transferring issues, "+
			"or comment for transitions for JIRA and bugzilla")
	flag.StringVar(&p.l, "l", "",
		"test steps for JIRA, or, "+
			"linked issue when linking issues, "+
			"or resolution of transition in bugzilla, "+
			"or more param for issue listing of Gerrit")
	flag.StringVar(&p.f, "f", "", "file to be sent/saved as, "+
		"or file ID of download in Gerrit")
	flag.StringVar(&p.z, "z", "", "number limit to show Jenkins builds")
	flag.StringVar(&p.cfg, "cfg", "", "config file")
	flag.StringVar(&p.log, "log", "", "log file")
	flag.Var(&paramS, "s", "solution for bugzilla closure. "+
		"If solution field defined in config, "+
		"one string overrides all, while multiple strings are "+
		"appended to each field definition, such as "+
		"\"-s 'guten morgen; bonne soirée'\" is similar to "+
		"\"-s morgen -s tag\" if \"guten\" & \"bonne\" "+
		"defined in config as in example.xml.")
	flag.StringVar(&p.fn, "fn", "", "output filter, name. "+
		"to be used together with fv or fs")
	flag.StringVar(&p.fv, "fv", "", "output filter, value. "+
		"to be used together with fn. results with "+
		"this name-value pair only")
	flag.StringVar(&p.fs, "fs", "", "output filter, python3 script. "+
		"not to be used together with fn or fv. "+
		"results filtered by return value 0 from script fv, "+
		"run by command fn. Note this may be very time-consuming")
}

func (p params) Parse() {
	flag.Parse()
	eztools.AuthInsecureTLS = true
	if p.reverse {
		step = -1
	} else {
		step = 1
	}
}

func mkIssueinfo(p params) IssueInfos {
	inf := make(IssueInfos)
	matrix := [...][]string{
		{p.i, IssueinfoStrID},
		{p.k, IssueinfoStrKey},
		{p.hd, IssueinfoStrSummary},
		{p.p, IssueinfoStrProj},
		{p.b, IssueinfoStrBranch},
		{p.l, IssueinfoStrLink},
		{p.f, IssueinfoStrFile},
		{p.z, IssueinfoStrSize},
		{p.c, IssueinfoStrComments}}
	// paramS to be handled when needed
	for _, i := range matrix {
		if len(i[0]) > 0 {
			inf[i[1]] = i[0]
		}
	}
	return inf
}

// DefLooper to loop one action/function with multiple ID's
type DefLooper struct {
	para          params
	svr           *svrs
	authInfo      eztools.AuthInfo
	funStr1       string       // A string describing the function
	fun1          actionFunc   // Function for action to be called
	issueInfoPrev IssueInfoSlc // issue information slices acumulated among loops
	maxResults    int          // Maximum number of results in each loop
}

// SetFun to set the function to be called in all loops
func (l *DefLooper) SetFun(str string, f actionFunc) {
	l.funStr1 = str
	l.fun1 = f
}

// ResetIssueInfo to reset the issue information slices
func (l *DefLooper) ResetIssueInfo() {
	l.issueInfoPrev = nil
}

// GetIssueInfo to get the issue information slices after all done
func (l *DefLooper) GetIssueInfo() IssueInfoSlc {
	return l.issueInfoPrev
}

// Loop to be used with loopIssues
func (l *DefLooper) Loop(inf IssueInfos) (IssueInfoSlc, error) {
	Log(false, true, l.svr.Name, l.funStr1, inf)
	id := inf[IssueinfoStrID]
	issues, err := l.fun1(l.svr, l.authInfo, inf)
	if err != nil {
		var op bool
		e := err
		if err == eztools.ErrNoValidResults {
			if !uiSilent {
				op = true
			}
			e = eztools.ErrNoValidResults
		}
		Log(op, false, e)
	} else {
		if (l.maxResults > 0) && (len(issues) > l.maxResults) {
			Log(true, false, "limiting to", l.maxResults, "results")
			issues = issues[:l.maxResults]
		}
		issues.Print(id, l.para.fn, l.para.fv, l.para.fs)
		l.issueInfoPrev = append(l.issueInfoPrev, issues...)
	}
	return issues, err
}

func mainLoop(svr *svrs, cats cat2Act, funs []action2Func,
	issueInfo IssueInfos, para params) (err error) {
	var choices []string
	for ; ; svr = nil { // reset nil among loops
		if svr == nil {
			svr = chooseSvr(cats, cfg.Svrs)
			if svr == nil {
				break
			}
		}
		var authInfo eztools.AuthInfo
		authInfo, err = cfg2AuthInfo(*svr, cfg)
		if err != nil {
			Log(false, false, err)
			os.Exit(extCfg)
		}
		if len(svr.Proj) > 0 && !uiSilent {
			eztools.ShowStrln("default project/ID prefix: " +
				svr.Proj)
		}
		var (
			fun1    actionFunc
			funStr1 string
		)
		if funs == nil {
			choices = makeActs2Choose(*svr, cats[svr.Type])
		}
		looper := DefLooper{para, svr, authInfo, funStr1, fun1, nil, -1}
		for funIndx := 0; ; fun1 = nil { // reset fun1 among loops
			var issueInfoCurr IssueInfoSlc
			if funs != nil && funIndx < len(funs) {
				// looping silent actions
				fun1 = funs[funIndx].f
				funStr1 = funs[funIndx].n
				issueInfoCurr = looper.GetIssueInfo()
				funIndx++
				Log(true, false, "actions:", funStr1)
			} else {
				if fun1 == nil { // reset issueInfo among loops
					funStr1, fun1, issueInfoCurr = chooseAct(svr,
						authInfo, choices, cats[svr.Type],
						mkIssueinfo(para), looper.GetIssueInfo())
					if fun1 == nil {
						break
					}
				} else { // first round and silent
					issueInfoCurr = IssueInfoSlc{issueInfo}
				}
			}
			looper.ResetIssueInfo()
			looper.SetFun(funStr1, fun1)
			for _, inf := range issueInfoCurr {
				if _, err = loopIssues(svr, inf, looper.Loop); err != nil {
					Log(false, false, err)
				}
			}
			if choices == nil || len(choices) < 2 || (funs != nil && funIndx == len(funs)) {
				break
			}
		}
		if choices == nil || len(cfg.Svrs) < 2 { // TODO: no loop
			break
		}
	}
	return
}

func flagParse() params {
	var p params
	p.Declare()
	p.Parse()

	eztools.Debugging = p.v || p.vv || p.vvv
	switch {
	case p.vvv:
		eztools.Verbose = 3
	case p.vv:
		eztools.Verbose = 2
	case p.v:
		eztools.Verbose = 1
	}
	return p
}

func flagHelp() {
	eztools.ShowStrln("::Return values::")
	eztools.ShowStrln("", "0", "no error")
	eztools.ShowStrln("", extCfg, "config error")
	eztools.ShowStrln("", extAuth, "auth error")
	eztools.ShowStrln("", extConn, "connection error")
	eztools.ShowStrln("", extInpt, "input error")
	eztools.ShowStrln("", extRslt, "result error")
	eztools.ShowStrln("", extGram, "request error")
	eztools.ShowStrln("", extSrvr, "server error")
	eztools.ShowStrln("::When inputting ID's, there are following",
		"options for some actions::")
	eztools.ShowStrln(" 1. single ID, such as 0 or X-0")
	eztools.ShowStrln(" 2. multiple IDs, such as 0,0,0 or X-0,2,1")
	eztools.ShowStrln(" 3. ID range, such as 0,,2 or X-0,2")
	eztools.ShowStrln("")
	flag.Usage()
	eztools.ShowStrln("")
	eztools.ShowStrln("::Action strings, \"a\", to be used with",
		"server names, \"r\", only, and that will",
		"eliminate interactions in UI::")
	for cat, i := range makeCat2Act() {
		eztools.ShowStrln("\t\t" + cat)
		for _, j := range i {
			eztools.ShowStrln("\t" + j.n)
		}
	}
}

func loadCfg(p params) {
	var errs []error
	cfgFile, errs = eztools.XMLReadDefault(p.cfg, "", "", "", module, &cfg)
	if errs != nil {
		Log(true, false, "failed to open config file", errs[0])
		if len(p.cfg) > 0 {
			cfgFile = p.cfg
		} else {
			home, _ := os.UserHomeDir()
			cfgFile = filepath.Join(home, module+".xml")
		}
		switch eztools.PromptStr("create " +
			cfgFile + "?([Enter]=y)") {
		case "", "y", "Y", "yes", "YES", "Yes":
			break
		default:
			os.Exit(extCfg)
		}
		var changed bool
		cfg.User = chkUsr(cfg.User, false)
		if cfg.Svrs, changed = addSvr(cfg.Svrs, cfg.Pass); !changed {
			os.Exit(extCfg)
		}
		if !saveCfg(true) {
			os.Exit(extCfg)
		}
	}
	if len(p.log) > 0 {
		cfg.Log = p.log
	} else if len(cfg.Log) < 1 && eztools.Debugging {
		cfg.Log = module + ".log"
	}
	if len(cfg.Log) > 0 {
		logger, err := os.OpenFile(cfg.Log,
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			if err = eztools.InitLogger(logger); err != nil {
				Log(true, false, err)
			}
		} else {
			Log(true, false, "Failed to open log file "+cfg.Log)
		}
	}
}

func watchCfg(p params) {
	switch len(p.w) {
	case 0:
		for _, svr := range cfg.Svrs {
			if len(svr.Watch) > 0 {
				Log(true, false, "type:"+svr.Type+", name:"+
					svr.Name+", watch:"+svr.Watch)
			}
		}
	default:
		var svr *svrs
		switch len(p.r) {
		case 0:
			svr = chooseSvrByType(cfg.Svrs, CategoryJira)
		default:
			svr = matchSvr(cfg.Svrs, p.r)
		}
		if svr == nil {
			eztools.LogFatal("NO server matched!")
			return
		}
		// reset previous watch
		for _, svr1 := range cfg.Svrs {
			if len(svr1.Watch) > 0 && svr1.Name != svr.Name {
				svr1.Watch = ""
			}
		}
		svr.Watch = p.w
		if !saveCfg(false) {
			os.Exit(extCfg)
		}
	}
}

func matchFuncFromParam(action string, svr *svrs, cats cat2Act) []action2Func {
	if len(action) <= 0 {
		return nil
	}
	var ret []action2Func
	acts := strings.Split(action, actionSep)
	for _, act := range acts {
		for _, v := range cats[svr.Type] {
			if act == v.n {
				uiSilent = true
				ret = append(ret, action2Func{v.n, v.f})
				break
			}
		}
		if ret == nil {
			Log(true, false, "\""+act+
				"\" NOT recognized as a command")
		}
	}
	return ret
}

func errExit(err error) {
	if err != nil {
		if eztools.Debugging {
			Log(true, false, "exit with \""+err.Error()+"\"")
		}
		switch err {
		case eztools.ErrInvalidInput:
			os.Exit(extInpt)
		case eztools.ErrInExistence,
			eztools.ErrNoValidResults, eztools.ErrOutOfBound:
			os.Exit(extRslt)
		case errAuth:
			os.Exit(extAuth)
		case errCfg:
			os.Exit(extCfg)
		case errConn:
			os.Exit(extConn)
		case errGram:
			os.Exit(extGram)
		case errSrvr:
			os.Exit(extSrvr)
		}
	}
}

func mkSvrFromParam(p string) (svr *svrs, ok bool) {
	switch len(cfg.Svrs) {
	case 0:
		return
		// eztools.LogFatal("NO server configured!")
	case 1:
		svr = &cfg.Svrs[0]
		ok = true
		return
	}
	if len(p) > 0 {
		for i, v := range cfg.Svrs {
			if p == v.Name {
				svr = &cfg.Svrs[i]
				ok = true
				break
			}
		}
		/* 		if svr == nil {
			Log(true, false, "Unknown server "+p.r)
		} */
	} else {
		ok = true
	}
	return
}

func main() {
	svrTypes = []string{CategoryJira, CategoryGerrit, CategoryJenkins, CategoryBugzilla}

	p := flagParse()
	if p.ver {
		eztools.ShowStrln(module + " version " + Ver + " build " + Bld)
		return
	}
	if p.h {
		flagHelp()
		return
	}
	if eztools.Debugging && eztools.Verbose > 1 {
		stdOutput = true
	}
	cats := makeCat2Act()
	loadCfg(p)

	if p.getSvrCfg {
		Log(true, false, "user:"+cfg.User)
		for _, svr := range cfg.Svrs {
			Log(true, false, "type:"+svr.Type+", name:"+
				svr.Name+", url:"+svr.URL+
				", ip:"+svr.IP)
		}
		if !saveCfg(false) {
			os.Exit(extCfg)
		}
		return
	}
	if p.setSvrCfg {
		if uiSilent {
			noInteractionAllowed()
			return
		}
		cfg.User = chkUsr(cfg.User, true)
		var changed bool
		if cfg.Svrs, changed = addSvr(cfg.Svrs, cfg.Pass); !changed {
			os.Exit(extCfg)
		}
		if !saveCfg(false) {
			os.Exit(extCfg)
		}
		return
	}
	if !uiSilent {
		cfg.User = chkUsr(cfg.User, true)
		if !chkSvr(cfg.Svrs, cfg.Pass, p.CfgSvrOpt) {
			failSvrCfg(p.CfgSvrOpt)
			os.Exit(extCfg)
		}
	}
	if p.w != p.Def {
		watchCfg(p)
		return
	}

	// self upgrade
	upch := make(chan bool, 2)
	go chkUpdate(cfg.EzToolsCfg, upch)

	var (
		funs   []action2Func
		funStr []string
	)
	svr, ok := mkSvrFromParam(p.r)
	if !ok {
		eztools.LogFatal("NO server configured or none matched!", p.r)
	}
	issueInfo := mkIssueinfo(p)
	svrParam := "N/A"
	if svr != nil {
		funs = matchFuncFromParam(p.a, svr, cats)
		svrParam = svr.Name
	}
	Log(false, false, "runtime params: server="+
		svrParam+", action=", funStr, ", info array:")
	Log(false, false, issueInfo)
	err := mainLoop(svr, cats, funs, issueInfo, p)

	if eztools.Debugging {
		eztools.ShowStrln("waiting for update check...")
	}
	if <-upch {
		if eztools.Debugging {
			eztools.ShowStrln("waiting for update check to start...")
		}
		if <-upch {
			if eztools.Debugging {
				eztools.ShowStrln("waiting for update check to end...")
			}
			if <-upch {
				if cfg.AppUp.Interval > 0 {
					cfg.AppUp.Previous = eztools.TranDate("")
					saveCfg(false)
				}
			}
		}
	}
	errExit(err)
}

// Print outputs the results
// Parameters: original ID, filter parameters, name, value, script
func (issues IssueInfoSlc) Print(id, fn, fv, fs string) {
	funcScript := func(issueInfo IssueInfos) bool {
		jsonData, err := json.Marshal(issueInfo)
		if err != nil {
			Log(true, false, "Error serializing JSON:", err)
			return true
		}
		if eztools.Debugging && eztools.Verbose > 0 {
			Log(false, false, "filtering with", fn, fs)
		}
		cmd := exec.Command(fn, fs)
		cmd.Stdin = bytes.NewReader(jsonData)
		var out bytes.Buffer
		if eztools.Debugging && eztools.Verbose > 1 {
			cmd.Stdout = &out
		}
		err = cmd.Run()
		if eztools.Debugging && eztools.Verbose > 1 {
			Log(true, false, "script err=", err, "out=", out.String())
		}
		if err == nil {
			return true
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode := status.ExitStatus()
				if eztools.Debugging && eztools.Verbose > 1 {
					Log(true, false, "exit code=", exitCode)
				}
				if exitCode == 0 {
					return true
				}
			}
		}
		return false
	}
	if issues == nil {
		Log(true, false, "No results.")
	} else {
		var i int
		if step == 0 {
			step = 1
		}
		if step < 0 {
			i = len(issues) - 1
		} else {
			i = 0
		}
		var fun func(IssueInfos) bool
		if len(fn) > 0 {
			switch {
			case len(fs) > 0:
				if _, err := os.Stat(fs); err != nil {
					Log(true, false, err)
					break
				}
				fun = funcScript
			case len(fv) > 0:
				fun = func(issueInfo IssueInfos) bool {
					return issueInfo[fn] == fv
				}
			}
		}
		for ; i >= 0 && i < len(issues); i += step {
			if len(issues[i]) < 1 {
				continue
			}
			if fun != nil && !fun(issues[i]) {
				continue
			}
			Log(true, false, "Issue/Reviewer/Comment/File",
				i+1, "(input ID:", id, ")")
			for i1, v1 := range issues[i] {
				Log(true, false, "\t", i1+"="+
					strings.ReplaceAll(v1, "\n", "\n\t\t"))
			}
		}
	}
}

func failSvrCfg(cfgSvrOpt string) {
	Log(true, false, "NO servers defined. Run with param -"+
		cfgSvrOpt+" to add some!")
}

func chooseSvrByType(svr []svrs, tp string) *svrs {
	var (
		indx  []int
		names []string
	)
	for i, s := range svr {
		if s.Type == tp {
			names = append(names, s.Name)
			indx = append(indx, i)
		}
	}
	i, _ := eztools.ChooseStrings(names)
	if i == eztools.InvalidID {
		return nil
	}
	return &svr[indx[i]]
}

func matchSvr(svr []svrs, name string) *svrs {
	for i, s := range svr {
		if s.Name == name {
			return &svr[i]
		}
	}
	return nil
}

func noInteractionAllowed() {
	Log(true, false, "NO interaction allowed in silent mode to provide information!")
}

func chkUsr(user string, save bool) string {
	if len(user) > 0 {
		return user
	}
	un := eztools.PromptStr("config needed. username")
	if len(un) < 1 {
		un = user
	} else {
		if save {
			saveCfg(true)
		}
	}
	return un
}

func chkExistSvr(svr []svrs, name, value string, indx int) bool {
	for i, svr1 := range svr {
		if i == indx {
			continue
		}
		var value2Compare string
		//TODO: can we get a definite member outside the loop?
		switch name {
		case "name":
			value2Compare = svr1.Name
		case "url":
			value2Compare = svr1.URL
		case "ip":
			value2Compare = svr1.IP
		}
		if value2Compare == value {
			return true
		}
	}
	return false
}

func chkNInputSvrFld(svrSlc []svrs, svr1 svrs, field *string, text string,
	indx int) (changed, ok bool) {
	value := *field
	if len(value) < 1 {
		// try to identify this server
		idCandidates := []string{svr1.Name, svr1.URL, svr1.IP, strconv.Itoa(indx)}
		var id string
		for _, v := range idCandidates {
			if len(v) > 0 {
				id = v
				break
			}
		}
		value = eztools.PromptStr(text + " for server " + id)
		if len(value) < 1 {
			return
		}
		*field = value
		changed = true
	}
	if chkExistSvr(svrSlc, text, value, indx) {
		eztools.ShowStrln("name or ip in existence. please enter a new one.")
		*field = ""
		_, ok = chkNInputSvrFld(svrSlc, svr1, field, text, indx)
		return true, ok
	}
	return changed, true
}

func chkSvr(svr []svrs, pass passwords, cfgSvrOpt string) bool {
	if nil == svr || len(svr) < 1 {
		failSvrCfg(cfgSvrOpt)
		return false
	}
	pass4All := len(pass.Type) > 0 &&
		(len(pass.Pass) > 0 || pass.Type == PassNone)
	changed := false
	for i, svr1 := range svr {
		mandatory := []struct {
			addr *string
			text string
		}{
			{&svr[i].Name, "name"},
			{&svr[i].URL, "url"}}
		for _, mand1 := range mandatory {
			c, o := chkNInputSvrFld(svr, svr1, mand1.addr, mand1.text, i)
			if !o {
				return false
			}
			changed = changed || c
		}
		if pass4All {
			continue
		}
		if len(svr1.Pass.Type) < 1 || (svr1.Pass.Type != PassNone && len(svr1.Pass.Pass) < 1) {
			eztools.ShowStrln("NO global password configured. Configure it for " + svr[i].Name)
			passType, passTxt, ok := inputPass4Svr(svr1.Type)
			if !ok {
				return false
			}
			svr[i].Pass.Type = passType
			svr[i].Pass.Pass = passTxt
			changed = true
		}
	}
	if !changed {
		return true
	}
	return saveCfg(false)
}

func inputPass4Svr(svrType string) (passType, passTxt string, ok bool) {
	passTypes := []string{
		PassNone + " - no password",
		PassBasic + " - plain text",
		PassPlain + " - base64'ed",
		PassDigest + " - HTTP password, such as from Settings of Gerrit",
		PassToken + " - token password, such as from API Key settings of Bugzilla"}
	const (
		pref = "Since this server is "
		affi = " is recommended."
	)
	switch svrType {
	case CategoryGerrit:
		eztools.ShowStrln(pref + svrType + ", " + PassDigest + affi)
	case CategoryJira, CategoryJenkins:
		eztools.ShowStrln(pref + svrType + ", " + PassBasic + affi)
	case CategoryBugzilla:
		eztools.ShowStrln(pref + svrType + ", " + PassToken + affi)
	}
	typeInd, _ := eztools.ChooseStrings(passTypes)
	if typeInd == eztools.InvalidID {
		return
	}
	passTypeSlc := strings.Split(passTypes[typeInd], " - ")
	passType = passTypeSlc[0]
	if passType != PassNone {
		passTxt = eztools.PromptStr("password")
		if len(passTxt) < 1 {
			return
		}
	}
	ok = true
	return
}

func addSvr(svrIn []svrs, pass passwords) (svrOut []svrs, ret bool) {
	const MAGIC = ")]}'"
	var name, url, ip, magic string
	svrOut = svrIn
	eztools.ShowStrln("Only mandatory fields for servers will be asked.")
	for {
		typeInd, _ := eztools.ChooseStrings(svrTypes)
		if typeInd == eztools.InvalidID {
			break
		}
		svrType := svrTypes[typeInd]
		for _, i := range []string{"name", "url", "ip"} {
			value := eztools.PromptStr(i)
			if len(value) < 1 {
				break
			}
			if chkExistSvr(svrOut, i, value, -1) {
				eztools.ShowStrln("server already exists")
				break
			}
			switch i {
			case "name":
				name = value
			case "url":
				url = value
			case "ip":
				ip = value
			}
		}
		if len(name) < 1 || len(url) < 1 {
			continue
		}
		if len(pass.Type) > 0 && len(pass.Pass) > 0 {
			eztools.ShowStrln("If you want to use " + pass.Type +
				" " + pass.Pass + " configured for all servers, answer an invalid value.")
		}
		passType, passTxt, _ := inputPass4Svr(svrType)
		if svrType == CategoryGerrit {
			magic = eztools.PromptStr("magic([Y/y=" +
				MAGIC + "])")
			switch magic {
			case "y", "Y":
				magic = MAGIC
			}
		} else {
			magic = eztools.PromptStr("magic")
		}
		svrOut = append(svrOut, svrs{Type: svrType,
			Name: name, URL: url, IP: ip, Magic: magic,
			Pass: passwords{Type: passType, Pass: passTxt}})
		ret = true
	}
	return
}

func saveProj(svr *svrs, proj string) bool {
	if svr == nil && len(proj) < 1 {
		return false
	}
	var ret bool
	if svr.Proj != proj {
		svr.Proj = proj
		ret = true
	}
	if !ret {
		return false
	}
	return saveCfg(false)
}

func saveCfg(creation bool) bool {
	fun := eztools.XMLWrite
	if !creation {
		fun = eztools.XMLWriteNoCreate
	}
	if err := fun(cfgFile, cfg, "\t"); err != nil {
		Log(true, false, err)
		return false
	}
	if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowStrln(cfgFile + " saved.")
	}
	return true
}

/*
upch <-      | false                               | true

	1st. | no check                            | to check
	2nd. | wrong update server config          |
	3rd. | wrong other config or check failure |
*/
func chkUpdate(eztoolscfg string, upch chan bool) {
	if cfg.AppUp.Interval < 1 {
		upch <- false
		return
	}
	if len(cfg.AppUp.Previous) > 0 {
		diff, ok := eztools.DiffDate(cfg.AppUp.Previous,
			eztools.TranDate(""))
		if ok == nil && diff <= cfg.AppUp.Interval {
			upch <- false
			return
		}
	}
	var (
		db  *eztools.Db
		err error
	)
	if len(eztoolscfg) > 0 {
		db, _, err = eztools.MakeDbWtCfgFile("", "", "", "", eztoolscfg)
		if err != nil {
			eztoolscfg = ""
		}
	}
	if len(eztoolscfg) == 0 {
		db, _, err = eztools.MakeDb()
		if err != nil {
			if /*err == os.PathErr ||*/ err == eztools.ErrNoValidResults {
				eztools.ShowStrln("NO configuration for EZtools. Get one to auto update this app!")
			}
			Log(true, false, err)
			upch <- false
			return
		}
	}
	defer db.Close()
	upch <- true
	db.AppUpgrade(db.GetTblDef(), module, Ver, nil, upch)
}

func cfg2AuthInfo(svr svrs, cfg jirrit) (authInfo eztools.AuthInfo, err error) {
	pass := svr.Pass
	if len(pass.Pass) < 1 {
		pass = cfg.Pass
	}
	authInfo = eztools.AuthInfo{User: cfg.User}
	if len(svr.User) > 0 {
		authInfo.User = svr.User
	}
	switch pass.Type {
	case PassDigest:
		authInfo.Type = eztools.AuthDigest
	case PassPlain:
		authInfo.Type = eztools.AuthPlain
	case PassBasic:
		authInfo.Type = eztools.AuthBasic
	default: //PassToken
		authInfo.Type = eztools.AuthNone
		//authInfo.Pass = ""
		//return
	}
	authInfo.Pass = pass.Pass
	if authInfo.Type != eztools.AuthNone && len(authInfo.Pass) < 1 {
		err = errors.New("NO password configured")
	}
	return
}

/*
	action name -> actionFunc

category name -> []action2Func
cat2Act
*/
type actionFunc func(*svrs, eztools.AuthInfo, IssueInfos) (IssueInfoSlc, error)
type action2Func struct {
	n string
	f actionFunc
}

type cat2Act map[string][]action2Func

func isValidSvr(cats cat2Act, svr *svrs) bool {
	if len(svr.Name) < 1 || len(svr.Type) < 1 || len(svr.URL) < 1 {
		if eztools.Verbose > 0 {
			eztools.LogPrint("Skipping invalid server", svr.Name)
		}
		return false
	}
	if _, ok := cats[svr.Type]; !ok {
		if eztools.Verbose > 0 {
			eztools.LogPrint("Skipping unknown type", svr.Type, "of server", svr.Name, ". must be", cats)
		}
		return false
	}
	u, err := url.Parse(svr.URL)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func chooseSvr(cats cat2Act, candidates []svrs) *svrs {
	var choices []string
	for _, svr := range candidates {
		if !isValidSvr(cats, &svr) {
			continue
		}
		choices = append(choices, svr.Type+" - "+svr.Name)
	}
	if len(choices) == 1 {
		return &candidates[0]
	}
	if uiSilent {
		noInteractionAllowed()
		return nil
	}
	eztools.ShowStrln(" Choose a server")
	si, _ := eztools.ChooseStrings(choices)
	if si == eztools.InvalidID {
		return nil
	}

	return &candidates[si]
}

func makeActs2Choose(svr svrs, funcs []action2Func) []string {
	if svr.Type == CategoryJira {
		if len(svr.Flds.TstExp+svr.Flds.TstPre+svr.Flds.TstStep) < 1 {
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			funcs = funcs[:len(funcs)-2]
		}
	}
	choices := make([]string, len(funcs))
	for i, choice := range funcs {
		choices[i] = choice.n
	}
	return choices
}

func chooseAct(svr *svrs, authInfo eztools.AuthInfo, choices []string,
	funcs []action2Func, issueInfo IssueInfos,
	issueInfoPrev IssueInfoSlc) (string, actionFunc, IssueInfoSlc) {
	var (
		fi  int
		ret IssueInfoSlc
	)
	if uiSilent && len(choices) > 1 {
		noInteractionAllowed()
		return "", nil, IssueInfoSlc{issueInfo}
	}
	switch len(choices) {
	case 0:
		Log(true, false, "NO available actions found for a server")
	case 1:
		Log(false, false, "only action for a server: "+choices[0])
	default:
		eztools.ShowStrln(" Choose an action")
		const wtFormer = "_with former results_"
		for _, choice1 := range [...][]string{append(choices, wtFormer), choices} {
			fi, _ = eztools.ChooseStrings(choice1)
			if fi == eztools.InvalidID {
				return "", nil, IssueInfoSlc{issueInfo}
			}
			if fi < len(choices) {
				break
			}
			issueInfo = nil // to use former results
			ret = issueInfoPrev
		}
	}
	if issueInfo != nil {
		if inputIssueInfo4Act(svr, authInfo, funcs[fi].n, issueInfo) {
			return "", nil, nil
		}
		ret = IssueInfoSlc{issueInfo}
	}
	return funcs[fi].n, funcs[fi].f, ret
}

func chkErrRest(bodyBytes []byte,
	errno int, err error) error {
	var (
		dnsErr *net.DNSError
		urlErr *url.Error
	)
	switch {
	case errors.As(err, &dnsErr):
		err = errConn
	case errors.As(err, &urlErr):
		/*urlErr, ok := err.(*url.Error)
		if ok {
		if urlErr.Timeout() {*/
		err = errCfg
	/*}
	}*/
	default:
		switch errno {
		case http.StatusBadRequest:
			err = errGram
		case http.StatusNotFound:
			err = eztools.ErrNoValidResults
		case http.StatusUnauthorized:
			err = errAuth
		case http.StatusGatewayTimeout:
			err = errConn
		case http.StatusInternalServerError:
			err = errSrvr
		}
	}
	if err != nil {
		Log(true, false, "REST error", err)
		Log(stdOutput, false, "REST body=", string(bodyBytes))
	} else {
		if eztools.Debugging && eztools.Verbose > 2 {
			Log(stdOutput, false, "REST body=", string(bodyBytes))
		}
	}
	return err
}

// chkRespErr checks whether it is an error or log the response
// Return value: whether it is an error
func chkRespErr(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if eztools.Debugging && eztools.Verbose > 2 {
		Log(stdOutput, false, resp)
	}
	return false
}

func logReq(method, url string) {
	if eztools.Debugging && eztools.Verbose > 2 {
		Log(stdOutput, false, method, url)
	}
}

func restFile(method, url string, authInfo eztools.AuthInfo,
	fType, fName string, hdrs map[string]string,
	magic string) (body interface{}, err error) {
	logReq(method, url)
	resp, err := eztools.HTTPSendAuthNHdrNFile(method, url,
		authInfo, fType, fName, hdrs)
	if chkRespErr(resp, err) {
		return
	}
	_, _, bodyBytes, errInt, err :=
		eztools.HTTPParseBody(resp, "", &body, []byte(magic))
	return body, chkErrRest(bodyBytes, errInt, err)
}

// restAttachment sends a request and save the attachement in the response
// parameters: method, url, authInfo, bodyReq, magic(reserved)
func restAttachment(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, _ string) (fileName string, err error) {
	logReq(method, url)
	resp, err := eztools.HTTPSendAuth(method,
		url, "", authInfo, bodyReq)
	if chkRespErr(resp, err) {
		return
	}
	_, fileName, err = eztools.HTTPSaveAttachment(resp, "")
	return fileName, err
}

// return nil for 404
func restSth(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (body interface{}, err error) {
	logReq(method, url)
	/*if eztools.Debugging && eztools.Verbose > 2 && bodyReq != nil {
		Log(stdOutput, false, "resting", bodyReq)
	}*/
	resp, err := eztools.HTTPSendAuth(method,
		url, "", authInfo, bodyReq)
	if chkRespErr(resp, err) {
		return
	}
	_, _, bodyBytes, errInt, err :=
		eztools.HTTPParseBody(resp, "", &body, []byte(magic))
	return body, chkErrRest(bodyBytes, errInt, err)
}

func restMap(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (
	bodyMap map[string]interface{}, err error) {
	body, err := restSth(method, url, authInfo, bodyReq, magic)
	if err != nil || body == nil {
		return
	}
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		LogTypeErr(body, "REST response type error for map")
	}
	return
}

/*
get all values from

	sth. {
		name: value
		[
		sth. {
			name: value
		}
		]
	}
*/
func getValuesFromMaps(name string, field interface{}) string {
	//eztools.ShowStrln(field)
	fieldMap, ok := field.(map[string]interface{})
	if !ok {
		Log(false, false, reflect.TypeOf(field).String()+
			" got instead of map[string]interface{}")
		return ""
	}
	type filterFunc func([]string, map[string]interface{}) []string
	var fF filterFunc
	values := make([]string, 0)
	fF = func(s []string, m map[string]interface{}) []string {
		for i, v := range m {
			if i == name {
				child, ok := v.(string)
				if !ok {
					Log(false, false, reflect.TypeOf(v).String()+
						" got instead of string")
					continue
				}
				s = append(s, child)
				continue
			}
			child, ok := v.([]interface{})
			if ok {
				for _, v := range child {
					child, ok := v.(map[string]interface{})
					if ok {
						s = fF(s, child)
					}
				}
			}
		}
		return s
	}
	values = fF(values, fieldMap)
	if len(values) == 1 {
		return values[0]
	}
	if uiSilent {
		noInteractionAllowed()
		return ""
	}
	if choice, _ := eztools.ChooseStrings(values); choice != eztools.InvalidID {
		return values[choice]
	}
	return ""
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
		/*if eztools.Debugging && eztools.Verbose > 1 {
			eztools.ShowStrln("looping " + i)
		}*/
		if len(keyStr) > 0 {
			matched := false
			for j, key1 := range keyStr {
				if i == key1 {
					matched = true
					id, ok := v.(string)
					if !ok {
						LogTypeErr(v, "string")
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

func chkNSetIssueInfo(v interface{}) string {
	if v == nil {
		Log(false, false, "nil got, not string")
		return ""
	}
	switch v := v.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		Log(false, false,
			"unknown non string/float64 type:",
			fmt.Sprintf("%T", v))
		return ""
	}
}

func chkNLoopStringMap(m interface{},
	mustStr string, keyStr []string) []string {
	sub, ok := m.(map[string]interface{})
	if !ok {
		LogTypeErr(m, "map[string]interface{}")
		return nil
	}
	res, _ := loopStringMap(sub, mustStr, keyStr, nil)
	return res
}

// parseIssues loops a map of string to a slice of map of string
func parseIssues(issueKey string, m map[string]interface{},
	fun func(map[string]interface{}) IssueInfos) IssueInfoSlc {
	/*if eztools.Debugging && eztools.Verbose > 1 {
		eztools.ShowStrln(strs)
	}*/
	results := make(IssueInfoSlc, 0)
	loopStringMap(m, issueKey, nil,
		func(i string, v interface{}) bool {
			//eztools.ShowStrln("func " + i)
			issues, ok := v.([]interface{})
			if !ok {
				LogTypeErr(v, "[]interface{} for "+i)
				return false
			}
			for _, v := range issues {
				//eztools.ShowStrln("Ticket")
				issue, ok := v.(map[string]interface{})
				if !ok {
					LogTypeErr(v, "map[string]interface{}")
					continue
				}
				if issueInfo := fun(issue); issueInfo != nil {
					//eztools.ShowStrln(issueInfo)
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

const (
	// StateTypeTranRej key in cfg
	StateTypeTranRej = "transition reject"
	// StateTypeTranCls StateType key in cfg
	StateTypeTranCls = "transition close"
	// StateTypeNotOpn key in cfg
	StateTypeNotOpn = "not open"
	// StateTypeResolutionRes key in cfg
	StateTypeResolutionRes = "resolved"
	// StateTypeResolutionRej key in cfg
	StateTypeResolutionRej = "rejected"
)

func makeStates(svr *svrs, tp string) (ret []string) {
	for _, v := range svr.State {
		if v.Type == tp {
			if len(v.Text) > 0 {
				ret = append(ret, v.Text)
			}
		}
	}
	return
}

const (
	// IssueinfoStrVal value string for common usages
	IssueinfoStrVal = "value"

	//common use

	// IssueinfoStrID ID string
	IssueinfoStrID = "id"
	// IssueinfoStrSubmittable submittable string
	IssueinfoStrSubmittable = "submittable"
	// IssueinfoStrKey key string
	IssueinfoStrKey = "key"
	// IssueinfoStrAssignee assignee string
	IssueinfoStrAssignee = "assignee"
	// IssueinfoStrAssigned2 assignee string
	IssueinfoStrAssigned2 = "assigned_to_detail"
	// IssueinfoStrRealName real name string
	IssueinfoStrRealName = "real_name"
	// IssueinfoStrName name string
	IssueinfoStrName = "name"
	// IssueinfoStrSummary summary string
	IssueinfoStrSummary = "summary"
	// IssueinfoStrSubject subject string
	IssueinfoStrSubject = "subject"
	// IssueinfoStrSolution solution string
	IssueinfoStrSolution = "cf_analysis_solution"
	// IssueinfoStrDesc description string
	IssueinfoStrDesc = "description"
	// IssueinfoStrRevCur current revision string
	IssueinfoStrRevCur = "current_revision"
	// IssueinfoStrVerified verified string
	IssueinfoStrVerified = "Verified"
	// IssueinfoStrProd product string
	IssueinfoStrProd = "product"
	// IssueinfoStrProj project string
	IssueinfoStrProj = "project"
	// IssueinfoStrCodereview code review string
	IssueinfoStrCodereview = "Code-Review"
	// IssueinfoStrBranch branch string
	IssueinfoStrBranch = "branch"
	// IssueinfoStrDispname display name string
	IssueinfoStrDispname = "displayName"

	// for code-review, verified and manual-testing

	// IssueinfoStrSubmitType submit type string
	IssueinfoStrSubmitType = "submit_type"
	// IssueinfoStrApprovals approvals string
	IssueinfoStrApprovals = "approvals"
	// IssueinfoStrRej rejected string
	IssueinfoStrRej = "rejected"
	// IssueinfoStrState state string
	IssueinfoStrState = "status"
	// IssueinfoStrFile file string
	IssueinfoStrFile = "file"
	// IssueinfoStrSize size string
	IssueinfoStrSize = "size"
	// IssueinfoStrMergeable mergeable string for details, gerrit
	IssueinfoStrMergeable = "mergeable"
	// IssueinfoStrLabels labels string for scores, gerrit
	IssueinfoStrLabels = "labels"
	// IssueinfoStrComments comment string for details, gerrit
	IssueinfoStrComments = "comments"
	// IssueinfoStrBin binary string for file list, gerrit
	IssueinfoStrBin = "binary"
	// IssueinfoStrOldPath old path/renamed from string for file list, gerrit
	IssueinfoStrOldPath = "old_path"
	// IssueinfoStrCherry cherry pick string of download for gerrit
	IssueinfoStrCherry = "Cherry Pick"
	// IssueinfoStrDate date string of history for gerrit
	IssueinfoStrDate = "date"
	// IssueinfoStrMsg message string of history for gerrit
	IssueinfoStrMsg = "message"
	// IssueinfoStrAuthor author string of history for gerrit
	IssueinfoStrAuthor = "author"
	// IssueinfoStr_Nmb number string for gerrit
	IssueinfoStr_Nmb = "_number"
	// IssueinfoStrTopic topic string for gerrit
	IssueinfoStrTopic = "topic"
	// IssueinfoStr_Chg_Nmb change number string for gerrit
	IssueinfoStr_Chg_Nmb = "_change_number"
	// IssueinfoStr_Rev_Nmb revision number string for gerrit
	IssueinfoStr_Rev_Nmb = "_revision_number"
	// IssueinfoStrCreated created string for gerrit
	IssueinfoStrCreated = "created"
	// IssueinfoStrRef ref string for gerrit
	IssueinfoStrRef = "ref"
	// IssueinfoStrAccount accound ID string for gerrit
	IssueinfoStrAccount = "_account_id"
	// IssueinfoStrKind kind string for gerrit
	IssueinfoStrKind = "kind"
	// IssueinfoStrCommit commit string for gerrit
	IssueinfoStrCommit = "commit"
	// IssueinfoStrParents parents string for gerrit
	IssueinfoStrParents = "parents"
	// IssueinfoStrParent parent string for gerrit
	IssueinfoStrParent = "parent"
	// IssueinfoStrMerged merged string for gerrit
	IssueinfoStrMerged = "MERGED"
	// IssueinfoStrSubmit submit string
	IssueinfoStrSubmit = "submit"
	// IssueinfoStrLink is the JIRA link string in config of a project, gerrit
	IssueinfoStrLink = "link"
	// IssueinfoStrMatch is the JIRA match pattern string in config of a project, gerrit
	IssueinfoStrMatch = "match"
	// IssueinfoStrJob job string for Jenkins
	IssueinfoStrJob = "jobs"
	// IssueinfoStrURL url string for Jenkins
	IssueinfoStrURL = "url"
	// IssueinfoStrBld builds string for Jenkins
	IssueinfoStrBld = "builds"
	// IssueinfoStrBldin building string for Jenkins
	IssueinfoStrBldin = "building"
	// IssueinfoStrTimestamp timestamp string for Jenkins
	IssueinfoStrTimestamp = "timestamp"
	// IssueinfoStrNmb number string for Jenkins
	IssueinfoStrNmb = "number"
	// IssueinfoStrResult result string for Jenkins
	IssueinfoStrResult = "result"
	// IssueinfoStrCreator creator string for bugzilla
	IssueinfoStrCreator = "creator"
	// IssueinfoStrTxt text string for bugzilla
	IssueinfoStrTxt = "text"
	// IssueinfoStrCreatTm creation time string for bugzilla
	IssueinfoStrCreatTm = "creation_time"
	// IssueinfoStrFileNm file name string for bugzilla
	IssueinfoStrFileNm = "file_name"
	// IssueinfoStrData data string for bugzilla
	IssueinfoStrData = "data"
	// IssueinfoStrChg changes string for bugzilla
	IssueinfoStrChg = "changes"
	// IssueinfoStrRmvd removed string for changes in bugzilla
	IssueinfoStrRmvd = "removed"
	// IssueinfoStrAdded added string for changes in bugzilla
	IssueinfoStrAdded = "added"
)

type IssueInfos map[string]string
type IssueInfoSlc []IssueInfos

func makeIssueInfo() (inf IssueInfos) {
	return make(IssueInfos)
}

func (inf IssueInfos) ToSlc() IssueInfoSlc {
	return IssueInfoSlc{inf}
}

func (issues IssueInfoSlc) ToMapSlc() (res []map[string]string) {
	for _, i := range issues {
		res = append(res, i)
	}
	return
}

var issueInfoTxt = []string{
	IssueinfoStrID, IssueinfoStrKey, IssueinfoStrSubject,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrState}

// issueDetailsTxt details of a gerrit issue
var issueDetailsTxt = []string{
	IssueinfoStrID, IssueinfoStr_Nmb, IssueinfoStrSubmittable,
	IssueinfoStrSubject, IssueinfoStrProj, IssueinfoStrBranch,
	IssueinfoStrState, IssueinfoStrMergeable, IssueinfoStrTopic}
var issueHistoryTxt = []string{
	IssueinfoStrID, IssueinfoStrDate, IssueinfoStrMsg}

// issueRevsTxt a revision of a gerrit issue
var issueRevsTxt = []string{
	IssueinfoStrID, IssueinfoStr_Nmb, IssueinfoStrRevCur,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrSubmitType,
	IssueinfoStrTopic}

// issueRev1Txt a revision of a gerrit commit
var issueRev1Txt = []string{IssueinfoStr_Nmb, IssueinfoStrKind,
	IssueinfoStrCreated, IssueinfoStrRef, IssueinfoStrAccount}

/*
	 var issueDldCmds = []string{
		IssueinfoStrID, IssueinfoStrSubmittable, IssueinfoStrSummary,
		IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrState, IssueinfoStrMergeable}
*/
var reviewInfoTxt = []string{
	IssueinfoStrID, IssueinfoStrName, IssueinfoStrVerified,
	IssueinfoStrCodereview, IssueinfoStrDispname, IssueinfoStrApprovals}

// loopIssues runs a function on all numbers between, inclusively,
// X-0 and X-1, or 0,1 from input in format of X-0,1 or 0,1
// If it is not a range, the function's return values are returned.
// Otherwise, no return values.
// IssueinfoStrID is set for each loop of function fun,
// from multiple ID's in one issueInfo,
// while other fields use the former values returned from function fun
func loopIssues(svr *svrs, issueInfo IssueInfos, fun func(IssueInfos) (
	IssueInfoSlc, error)) (issueInfoOut IssueInfoSlc, err error) {
	printID := func() {
		if err == nil {
			Log(false, false, "Done with "+
				issueInfo[IssueinfoStrID])
		}
	}
	//Log(false, false,strings.Count(issueInfo[IssueinfoStrID], separator))
	switch strings.Count(issueInfo[IssueinfoStrID], issueSeparator) {
	case 0: // single ID
		if prefix, lowerBoundStr, _, ok := parseTypicalJiraNum(svr,
			issueInfo[IssueinfoStrID]); ok {
			issueInfo[IssueinfoStrID] = prefix + lowerBoundStr
		}
		issueInfo, err := fun(issueInfo)
		printID()
		return issueInfo, err
	case 2: // x,,y or x,y,z
		parts := strings.Split(issueInfo[IssueinfoStrID], issueSeparator)
		//Log(false, false,parts)
		if len(parts) != 2 || len(parts[0]) < 1 || len(parts[2]) < 1 {
			if len(parts) != 3 {
				Log(true, false, "range format needs both parts aside with two \""+
					issueSeparator+"\""+" or multiple parts, deliminated by \""+
					issueSeparator+"\"")
				return nil, eztools.ErrInvalidInput
			}
		}
		if len(parts[1]) < 1 { // x,,y
			var (
				prefix, lowerBoundStr  string
				lowerBound, upperBound int
			)
			lowerBound, err = strconv.Atoi(parts[0])
			if err != nil {
				var ok bool
				if prefix, lowerBoundStr, _, ok =
					parseTypicalJiraNum(svr, parts[0]); !ok {
					Log(true, false, "the former"+
						" part must be in the"+
						" form of X-0 or 0")
					return
				}
				lowerBound, err = strconv.Atoi(lowerBoundStr)
				if err != nil {
					Log(true, false, lowerBoundStr+
						" is NOT a number!")
					return
				}
			}
			upperBound, err = strconv.Atoi(parts[2])
			if err != nil {
				Log(true, false,
					"the latter part must be a number")
				return
			}
			if lowerBound >= upperBound {
				Log(true, false, "the number in the latter"+
					" part must be greater than the one"+
					" in the former part")
				return
			}
			for i := lowerBound; i <= upperBound; i++ {
				issueInfo[IssueinfoStrID] =
					prefix + strconv.Itoa(i)
				//eztools.ShowStrln("looping ",
				//issueInfo[IssueinfoStrID])
				if inf, err := fun(issueInfo); err == nil {
					issueInfoOut = append(issueInfoOut,
						inf...)
				}
				printID()
			}
			return
		}
		// x,y,z instead of range
	}
	// x,y[,...]
	parts := strings.Split(issueInfo[IssueinfoStrID], issueSeparator)
	var (
		prefix, prefixNew, currentNo string
		ok                           bool
	)
	if prefix, currentNo, _, ok = parseTypicalJiraNum(svr, parts[0]); !ok {
		currentNo = parts[0]
	}
	i := 1
	/*eztools.ShowStrln(prefix + "_" + currentNo)
	eztools.ShowStrln(parts)*/
	for {
		issueInfo[IssueinfoStrID] = prefix + currentNo
		//eztools.ShowStrln("looping " + issueInfo[IssueinfoStrID])
		inf, err := fun(issueInfo)
		if err == nil {
			issueInfoOut = append(issueInfoOut, inf...)
		} // let it work for the next
		printID()
		if i < len(parts) {
			if prefixNew, currentNo, _, ok =
				parseTypicalJiraNum(svr, parts[i]); !ok {
				// reuse old prefix
				currentNo = parts[i]
			} else {
				prefix = prefixNew
			}
			i++
		} else {
			break
		}
	}
	return
}

func cfmInputOrPromptStrMultiLines(inf IssueInfos, ind, prompt string) {
	if uiSilent {
		noInteractionAllowed()
		return
	}
	const linefeed = " (end with \\ to continue with more lines. empty to stop)"
	s := eztools.PromptStr(prompt + linefeed)
	if len(s) < 1 {
		return
	}
	if s[len(s)-1] == '\\' {
		inf[ind] += s[:len(s)-1] + "\n"
		cfmInputOrPromptStrMultiLines(inf, ind, prompt)
		return
	}
	inf[ind] += s
}

// cfmInputOrPromptStr does not accept multiple ID format for input
// parse JIRA number format for JIRA servers and ID strings
// return value: whether anything new is input
func cfmInputOrPromptStr(svr *svrs, inf IssueInfos, ind, prompt string) bool {
	const linefeed = " (end with \\ to input multi lines)"
	var def, base string
	var changes, smart bool // no smart affix available by default
	if ind == IssueinfoStrID && len(inf[ind]) > 0 {
		switch {
		case svr.Type == CategoryBugzilla:
			def = "=" + inf[ind]
		case svr.Type == CategoryJira:
			var ok bool
			if base, _, changes, ok = parseTypicalJiraNum(svr,
				inf[ind]); ok {
				smart = true // there is a reference for smart affix
				//eztools.ShowStrln("not int previously")
			}
			def = "=" + inf[ind]
		}
	}
	s := eztools.PromptStr(prompt + linefeed + def)
	if len(s) < 1 || s == inf[ind] {
		return false
	}
	if ind == IssueinfoStrID {
		switch svr.Type {
		case CategoryJira:
			if sChg, ok := changeTypicalJiraNum(svr, s, base,
				smart, changes); ok {
				inf[ind] = sChg
				return true
			}
		}
	}
	// input not a number or no previous input to refer to
	if s[len(s)-1] == '\\' {
		inf[ind] = s[:len(s)-1] + "\n"
		cfmInputOrPromptStrMultiLines(inf, ind, prompt)
		return true
	}
	inf[ind] = s
	return true
}

// useInputOrPromptStr
// Return: whether nothing is provided (either input or as a param)
func useInputOrPromptStr(svr *svrs, inf IssueInfos, ind, prompt string) bool {
	if len(inf[ind]) > 0 {
		return false
	}
	if uiSilent {
		noInteractionAllowed()
		return true
	}
	return !cfmInputOrPromptStr(svr, inf, ind, prompt)
}

// useInputOrPrompt
// Return: whether nothing is provided (either input or as a param)
func useInputOrPrompt(svr *svrs, inf IssueInfos, ind string) bool {
	return useInputOrPromptStr(svr, inf, ind, ind)
}

// useInputOrPrompt4ID lists open cases to choose from
// Parameters: fun=function to list issues for user to choose from
// Return value: true=no ID input; false=sth. input
func useInputOrPrompt4ID(svr *svrs, authInfo eztools.AuthInfo,
	issueInfo IssueInfos) bool {
	/*switch svr.Type {
	case CategoryGerrit:
		defer gerritAnyID2ID(svr, authInfo, issueInfo)
	}*/
	if len(issueInfo[IssueinfoStrID]) > 0 {
		return false
	}
	if uiSilent {
		noInteractionAllowed()
		return true
	}
	var (
		strIndCmp, strIndSum string
		listFunc             func(svr *svrs, authInfo eztools.AuthInfo,
			issueInfo IssueInfos) (IssueInfoSlc, error)
	)
	switch svr.Type {
	case CategoryJira:
		listFunc = JiraMyOpen
		strIndCmp = IssueinfoStrProj
		strIndSum = IssueinfoStrSummary
	case CategoryBugzilla:
		listFunc = BugzillaMyOpen
		strIndCmp = IssueinfoStrProj
		strIndSum = IssueinfoStrSummary
	case CategoryGerrit:
		listFunc = GerritMyOpen
		strIndCmp = IssueinfoStrBranch
		strIndSum = IssueinfoStrSubject
	}
	if listFunc == nil {
		useInputOrPrompt(svr, issueInfo, IssueinfoStrID)
	} else {
		slc, err := listFunc(svr, authInfo, issueInfo)
		var choices []string
		if err == nil && len(slc) > 0 {
			for _, v := range slc {
				//eztools.ShowStrln(v)
				choices = append(choices,
					v[IssueinfoStrID]+" :\t"+
						v[strIndCmp]+" :\t"+
						v[strIndSum])
			}
			i, s := eztools.ChooseStrings(choices)
			if i == eztools.InvalidID {
				issueInfo[IssueinfoStrID] = s
			} else {
				issueInfo[IssueinfoStrID] = slc[i][IssueinfoStrID]
			}
		}
		if len(issueInfo[IssueinfoStrID]) < 1 {
			useInputOrPrompt(svr, issueInfo, IssueinfoStrID)
		}
	}
	if len(issueInfo[IssueinfoStrID]) < 1 {
		return true
	}
	return false
}

// inputIssueInfo4JB is inputIssueInfo4Act for Jira and Bugzilla
func inputIssueInfo4JB(svr *svrs, authInfo eztools.AuthInfo,
	action string, inf IssueInfos) bool {
	switch action {
	case "close a case to resolved from any known statuses":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPrompt(svr, inf, IssueinfoStrComments)
		switch svr.Type {
		case CategoryJira:
			useInputOrPromptStr(svr, inf, IssueinfoStrLink,
				"test step for closure")
		}
	case "move status of a case":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		strCmt := IssueinfoStrComments
		switch svr.Type {
		case CategoryBugzilla:
			strCmt += " (added to all statues during transition)"
		}
		useInputOrPromptStr(svr, inf, IssueinfoStrComments, strCmt)
	case "close a case with default design as steps",
		"close a case with general requirement as steps":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPrompt(svr, inf, IssueinfoStrComments)
	case "show details of a case",
		"list comments of a case",
		"list files attached to a case",
		"list watchers of a case",
		"check whether watching a case",
		"watch a case",
		"unwatch a case":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
	case "link a case to another":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPromptStr(svr, inf, IssueinfoStrLink,
			"ID (not indexes above, if any) this issue blocks")
	case "remove a file attached to a case":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPromptStr(svr, inf,
			IssueinfoStrKey, "file ID")
	case "add a file to a case":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPrompt(svr, inf, IssueinfoStrFile)
		switch svr.Type {
		case CategoryBugzilla:
			useInputOrPromptStr(svr, inf,
				IssueinfoStrKey, "description")
		}
	case "get a file to a case":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPromptStr(svr, inf,
			IssueinfoStrKey, "file ID")
		useInputOrPromptStr(svr, inf,
			IssueinfoStrFile, "file to be saved as")
	case "change a comment from a case":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPromptStr(svr, inf,
			IssueinfoStrKey, "comment ID")
		useInputOrPromptStr(svr, inf,
			IssueinfoStrComments, "comment body")
	case "delete a comment from a case":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPromptStr(svr, inf,
			IssueinfoStrKey, "comment ID")
	case "add a comment to a case":
		if useInputOrPrompt(svr, inf, IssueinfoStrComments) {
			return true
		}
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
	case "reject a case from any known statuses":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPrompt(svr, inf, IssueinfoStrComments)
	case "transfer a case to someone":
		if useInputOrPrompt4ID(svr, authInfo, inf) {
			return true
		}
		useInputOrPromptStr(svr, inf,
			IssueinfoStrSummary, "assignee")
		useInputOrPromptStr(svr, inf,
			IssueinfoStrComments, "component")
	}
	return false
}

// inputIssueInfo4Act asks for input specific to the action and server type, and update
// inf accordingly. Return true if not enough info is given, false otherwise.
func inputIssueInfo4Act(svr *svrs, authInfo eztools.AuthInfo,
	action string, inf IssueInfos) bool {
	switch svr.Type {
	case CategoryGerrit:
		switch action {
		case "rebase a submit",
			"revert a submit",
			"abandon a submit",
			"show reviewers and scores of a submit",
			"add scores to a submit",
			"show revisions of a submit",
			"show history of a submit":
			if useInputOrPrompt4ID(svr, authInfo, inf) {
				return true
			}
		case "list files of a submit by revision":
			if useInputOrPrompt4ID(svr, authInfo, inf) {
				return true
			}
			useInputOrPromptStr(svr, inf, IssueinfoStrRevCur,
				"revision(empty for current)")
		case "download a file of a submit":
			if useInputOrPrompt4ID(svr, authInfo, inf) {
				return true
			}
			useInputOrPromptStr(svr, inf, IssueinfoStrRevCur,
				"revision(empty for current)")
			useInputOrPrompt(svr, inf, IssueinfoStrFile)
		case "cherry pick a submit":
			if useInputOrPrompt4ID(svr, authInfo, inf) {
				return true
			}
			useInputOrPromptStr(svr, inf, IssueinfoStrRevCur,
				"revision(empty for current)")
			useInputOrPrompt(svr, inf, IssueinfoStrBranch)
			if len(inf[IssueinfoStrBranch]) < 1 {
				return true
			}
		}
	case CategoryJira, CategoryBugzilla:
		return inputIssueInfo4JB(svr, authInfo, action, inf)
	case CategoryJenkins:
		switch action {
		case "list jobs", "list builds":
			useInputOrPromptStr(svr, inf, IssueinfoStrSize,
				"max number of results")
		case "get log of a build":
			useInputOrPromptStr(svr, inf, IssueinfoStrFile,
				"log file name to save as")
		}
	default:
		Log(true, false, "Server type unknown: "+svr.Type)
		return true
	}
	//eztools.ShowStrln(inf)
	return false
}

func makeCat2Act() cat2Act {
	return cat2Act{
		CategoryJira: []action2Func{
			{"transfer a case to someone", JiraTransfer},
			{"move status of a case", JiraTransition},
			{"show details of a case", JiraDetail},
			{"list comments of a case", JiraComments},
			{"add a comment to a case", JiraAddComment},
			{"delete a comment from a case", JiraDelComment},
			{"change a comment from a case", JiraModComment},
			{"list my open cases", JiraMyOpen},
			{"link a case to another", JiraLink},
			{"list watchers of a case", JiraWatcherList},
			{"check whether watching a case", JiraWatcherCheck},
			{"watch a case", JiraWatcherAdd},
			{"unwatch a case", JiraWatcherDel},
			{"add a file to a case", JiraAddFile},
			{"list files attached to a case", JiraListFile},
			{"get a file to a case", JiraGetFile},
			{"remove a file attached to a case", JiraDelFile},
			{"reject a case from any known statuses", JiraReject},
			{"close a case to resolved from any known statuses", JiraClose},
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			{"close a case with default design as steps", JiraCloseDef},
			{"close a case with general requirement as steps", JiraCloseGen}},
		CategoryGerrit: []action2Func{
			{"list merged submits of someone", GerritSbMerged},
			{"list my open submits", GerritMyOpen},
			{"list sbs open submits", GerritSbOpen},
			{"list all open submits", GerritAllOpen},
			{"list my open commits", GerritMyOpenCmts},
			{"show details of a submit", GerritDetailOnCurrRev},
			{"show revisions of a submit", GerritRevs},
			{"show history of a submit", GerritHistory},
			{"show reviewers and scores of a submit", GerritReviews},
			{"show current revision or commit of a submit", GerritRev},
			{"rebase a submit", GerritRebase},
			{"merge a submit", GerritMerge},
			{"show related submits of one", GerritRelated},
			{"add scores to a submit", GerritScore},
			{"add scores, wait for it to be mergable and merge a submit", GerritWaitNMerge},
			{"wait for mergable and merge sbs submits", GerritWaitNMergeSb},
			{"abandon all my open submits", GerritAbandonMyOpen},
			{"abandon a submit", GerritAbandon},
			{"cherry pick all my open submits", GerritPickMyOpen},
			{"cherry pick a submit", GerritPick},
			{"revert a submit", GerritRevert},
			{"list files of a submit by revision", GerritListFilesByRev},
			{"list config of a project", GerritListPrj},
			{"download a file of a submit", GerritGetFile}},
		CategoryJenkins: []action2Func{
			{"list jobs", JenkinsListJobs},
			{"show details of a build", JenkinsDetailOnBld},
			{"get log of a build", JenkinsLogOfBld},
			{"list builds", JenkinsListBlds}},
		CategoryBugzilla: []action2Func{
			{"transfer a case to someone", BugzillaTransfer},
			{"move status of a case", BugzillaTransition},
			{"show details of a case", BugzillaDetail},
			{"list comments of a case", BugzillaComments},
			{"add a comment to a case", BugzillaAddComment},
			{"list my open cases", BugzillaMyOpen},
			{"link a case to another", BugzillaLink},
			{"list watchers of a case", BugzillaWatcherList},
			{"watch a case", BugzillaWatcherAdd},
			{"unwatch a case", BugzillaWatcherDel},
			{"add a file to a case", BugzillaAddFile},
			{"list files attached to a case", BugzillaListFile},
			{"get a file to a case", BugzillaGetFile},
			{"reject a case from any known statuses", BugzillaReject},
			{"close a case to resolved from any known statuses", BugzillaClose},
		}}
}
