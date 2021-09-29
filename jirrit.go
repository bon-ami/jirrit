package main

import (
	"database/sql"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"gitee.com/bon-ami/eztools"
	_ "github.com/go-sql-driver/mysql"
)

const (
	module = "jirrit"
	// CategoryJira JIRA server in xml
	CategoryJira = "JIRA"
	// CategoryGerrit Gerrit server in xml
	CategoryGerrit = "Gerrit"
	// PassBasic plain text password in xml
	PassBasic = "basic"
	// PassPlain BASE64ed password in xml
	PassPlain = "plain"
	// PassDigest HTTP password in xml
	PassDigest = "digest"
	// intGerritMerge is interval between each status check to merge a submit, in seconds
	intGerritMerge = 15
)

var (
	ver, cfgFile         string
	cfg                  jirrit
	uiSilent             bool
	step                 int
	dispResultOutputFunc func(...interface{})
	svrTypes             []string
	errAuth              = errors.New("Auth failure")
	errConn              = errors.New("Conn failure")
	errCfg               = errors.New("Cfg failure")
	errGram              = errors.New("Request failure in grammar")
	errSrvr              = errors.New("Server error")
)

type passwords struct {
	Cmt  string `xml:",comment"`
	Type string `xml:"type,attr"`
	Pass string `xml:",chardata"`
}

type fields struct {
	Cmt       string `xml:",comment"`
	RejectRsn string `xml:"rejectrsn"`
	TstPre    string `xml:"testpre"`
	TstStep   string `xml:"teststep"`
	TstExp    string `xml:"testexp"`
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

func main() {
	const ParamDef = "_"
	const (
		extCfg = iota + 1
		extAuth
		extConn
		extInpt
		extRslt
		extGram
		extSrvr
	)
	svrTypes = []string{CategoryJira, CategoryGerrit}
	var (
		paramH, paramV, paramVV, paramVVV,
		paramReverse, paramGetSvrCfg, paramSetSvrCfg bool
		paramR, paramA, paramW, paramK, paramF,
		paramI, paramB, paramCfg, paramLog,
		paramHD, paramP, paramL, paramC string
	)
	const cfgSvrOpt = "setsvrcfg"
	flag.BoolVar(&paramH, "h", false, "help message")
	flag.BoolVar(&paramV, "v", false,
		"log file output")
	flag.BoolVar(&paramVV, "vv", false, "verbose messages")
	flag.BoolVar(&paramVVV, "vvv", false,
		"verbose messages with network I/O")
	flag.BoolVar(&paramReverse, "reverse", false, "reverse output")
	flag.BoolVar(&paramGetSvrCfg, "getsvrcfg", false,
		"get server list from config")
	flag.BoolVar(&paramSetSvrCfg, cfgSvrOpt, false,
		"set servers as config")
	flag.StringVar(&paramR, "r", "", "server name, to be together with -a")
	flag.StringVar(&paramA, "a", "", "action, to be together with -r")
	flag.StringVar(&paramW, "w", ParamDef, "JIRA ID to store in settings, "+
		"to be together with -r. current setting shown, if empty value.")
	flag.StringVar(&paramK, "k", "", "key or description")
	flag.StringVar(&paramI, "i", "",
		"ID of issue, change, commit or assignee")
	flag.StringVar(&paramB, "b", "", "branch")
	flag.StringVar(&paramHD, "hd", "",
		"new assignee when transferring issues, "+
			"or revision id for cherrypicks")
	flag.StringVar(&paramP, "p", "",
		"project for JIRA issues")
	flag.StringVar(&paramC, "c", "",
		"new component when transferring issues, "+
			"or (test step) comment for JIRA (closure)")
	flag.StringVar(&paramL, "l", "",
		"linked issue when linking issues")
	flag.StringVar(&paramF, "f", "", "file to be sent/saved as")
	flag.StringVar(&paramCfg, "cfg", "", "config file")
	flag.StringVar(&paramLog, "log", "", "log file")
	flag.Parse()
	if paramH {
		eztools.ShowStrln(module + " v" + ver)
		eztools.ShowStrln("::Return values::")
		eztools.ShowStrln("", "0", "no error")
		eztools.ShowStrln("", extCfg, "config error")
		eztools.ShowStrln("", extAuth, "auth error")
		eztools.ShowStrln("", extConn, "connection error")
		eztools.ShowStrln("", extInpt, "input error")
		eztools.ShowStrln("", extRslt, "result error")
		eztools.ShowStrln("", extGram, "request error")
		eztools.ShowStrln("", extSrvr, "server error")
		eztools.ShowStrln("::When inputting ID's, there are following options for some actions::")
		eztools.ShowStrln(" 1. single ID, such as 0 or X-0")
		eztools.ShowStrln(" 2. multiple IDs, such as 0,0,0 or X-0,2,1")
		eztools.ShowStrln(" 3. ID range, such as 0,,2 or X-0,2")
		eztools.ShowStrln("")
		flag.Usage()
		eztools.ShowStrln("")
		eztools.ShowStrln("::Action strings, \"a\", to be used with server names, \"r\", only, and that will eliminate interactions in UI::")
		for cat, i := range makeCat2Act() {
			eztools.ShowStrln("\t\t" + cat)
			for _, j := range i {
				eztools.ShowStrln("\t" + j.n)
			}
		}
		return
	}
	eztools.Debugging = paramV || paramVV || paramVVV
	switch {
	case paramV:
		eztools.Verbose = 1
	case paramVV:
		eztools.Verbose = 2
	case paramVVV:
		eztools.Verbose = 3
	}
	cats := makeCat2Act()
	var err error
	cfgFile, err = eztools.XMLsReadDefaultNoCreate(paramCfg, module, &cfg)
	if err != nil {
		eztools.LogErrPrintWtInfo("failed to open config file", err)
		if len(paramCfg) > 0 {
			cfgFile = paramCfg
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
		if !saveCfg() {
			os.Exit(extCfg)
		}
	}
	if len(paramLog) > 0 {
		cfg.Log = paramLog
	} else if len(cfg.Log) < 1 && eztools.Debugging {
		cfg.Log = module + ".log"
	}
	if len(cfg.Log) > 0 {
		logger, err := os.OpenFile(cfg.Log,
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			if err = eztools.InitLogger(logger); err != nil {
				eztools.LogErrPrint(err)
			}
		} else {
			eztools.LogPrint("Failed to open log file " + cfg.Log)
		}
	}

	if paramGetSvrCfg {
		eztools.LogPrint("user:" + cfg.User)
		for _, svr := range cfg.Svrs {
			eztools.LogPrint("type:" + svr.Type + ", name:" +
				svr.Name + ", url:" + svr.URL +
				", ip:" + svr.IP)
		}
		return
	}
	if paramSetSvrCfg {
		if uiSilent {
			noInteractionAllowed()
			return
		}
		cfg.User = chkUsr(cfg.User, true)
		var changed bool
		if cfg.Svrs, changed = addSvr(cfg.Svrs, cfg.Pass); !changed {
			os.Exit(extCfg)
		}
		if !saveCfg() {
			os.Exit(extCfg)
		}
		return
	}
	if !uiSilent {
		cfg.User = chkUsr(cfg.User, true)
		if !chkSvr(cfg.Svrs, cfg.Pass, cfgSvrOpt) {
			failSvrCfg(cfgSvrOpt)
			os.Exit(extCfg)
		}
	}
	var svr *svrs
	if paramW != ParamDef {
		switch len(paramW) {
		case 0:
			for _, svr := range cfg.Svrs {
				if len(svr.Watch) > 0 {
					eztools.LogPrint("type:" + svr.Type + ", name:" +
						svr.Name + ", watch:" + svr.Watch)
				}
			}
		default:
			switch len(paramR) {
			case 0:
				svr = chooseSvrByType(cfg.Svrs, CategoryJira)
			default:
				svr = matchSvr(cfg.Svrs, paramR)
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
			svr.Watch = paramW
			if !saveCfg() {
				os.Exit(extCfg)
			}
		}
		return
	}

	// self upgrade
	upch := make(chan bool)
	go chkUpdate(cfg.EzToolsCfg, upch)

	var (
		fun     actionFunc
		choices []string
	)
	//if eztools.Debugging && eztools.Verbose > 0 {
	dispResultOutputFunc = eztools.LogPrint
	//} else {
	//op = eztools.ShowStrln
	//}
	switch len(cfg.Svrs) {
	case 0:
		eztools.LogFatal("NO server configured!")
	case 1:
		svr = &cfg.Svrs[0]
	}
	if len(paramR) > 0 {
		for i, v := range cfg.Svrs {
			if paramR == v.Name {
				svr = &cfg.Svrs[i]
				break
			}
		}
		if svr == nil {
			eztools.LogPrint("Unknown server " + paramR)
		}
	}
	mkIssueinfo := func() issueInfos {
		inf := make(issueInfos)
		matrix := [...][]string{
			{paramI, IssueinfoStrID},
			{paramK, IssueinfoStrKey},
			{paramHD, IssueinfoStrHead},
			{paramP, IssueinfoStrProj},
			{paramB, IssueinfoStrBranch},
			{paramL, IssueinfoStrLink},
			{paramF, IssueinfoStrFile},
			{paramC, IssueinfoStrComments}}
		for _, i := range matrix {
			if len(i[0]) > 0 {
				inf[i[1]] = i[0]
			}
		}
		return inf
	}
	issueInfo := mkIssueinfo()
	funParam := "N/A"
	svrParam := "N/A"
	if svr != nil {
		if len(paramA) > 0 {
			for _, v := range cats[svr.Type] {
				if paramA == v.n {
					uiSilent = true
					fun = v.f
					funParam = v.n
					break
				}
			}
			if fun == nil {
				eztools.LogPrint("\"" + paramA +
					"\" NOT recognized as a command")
			}
		}
		svrParam = svr.Name
	}
	eztools.Log("runtime params: server=" +
		svrParam + ", action=" + funParam + ", info array:")
	eztools.Log(issueInfo)
	if paramReverse {
		step = -1
	} else {
		step = 1
	}
	err = nil
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
			eztools.LogErr(err)
			os.Exit(extCfg)
		}
		if len(svr.Proj) > 0 && !uiSilent {
			eztools.ShowStrln("default project/ID prefix: " + svr.Proj)
		}
		if fun == nil {
			choices = makeActs2Choose(*svr, cats[svr.Type])
		}
		for ; ; fun = nil { // reset fun among loops
			if fun == nil {
				fun, issueInfo = chooseAct(svr.Type, choices, cats[svr.Type],
					mkIssueinfo())
				if fun == nil {
					break
				}
			}
			_, err = loopIssues(svr, issueInfo,
				func(inf issueInfos) (issueInfos, error) {
					issues, err := fun(svr, authInfo, inf)
					if err != nil {
						if err == eztools.ErrNoValidResults {
							if uiSilent {
								eztools.Log("NO valid results!")
							} else {
								eztools.LogPrint("NO valid results!")
							}
						} else {
							eztools.LogErr(err)
						}
					} else {
						issues.Print(dispResultOutputFunc)
					}
					return inf, err
				})
			if choices == nil { // no loop
				break
			}
		}
		if choices == nil || len(cfg.Svrs) < 2 { // no loop
			break
		}
	}

	if eztools.Debugging {
		eztools.ShowStrln("waiting for update check...")
	}
	if <-upch && <-upch {
		if eztools.Debugging {
			eztools.ShowStrln("waiting for update check to end...")
		}
		if <-upch {
			if cfg.AppUp.Interval > 0 {
				cfg.AppUp.Previous = eztools.TranDate("")
				saveCfg()
			}
		}
	}
	if err != nil {
		if eztools.Debugging {
			eztools.LogPrint("exit with \"" + err.Error() + "\"")
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

func (issues issueInfoSlc) Print(fun func(...interface{})) {
	if issues == nil {
		fun("No results.")
	} else {
		var i int
		if step < 1 {
			i = len(issues) - 1
		} else {
			i = 0
		}
		for ; i >= 0 && i < len(issues); i += step {
			if len(issues[i]) < 1 {
				continue
			}
			fun("Issue/Reviewer/Comment/File " +
				strconv.Itoa(i+1))
			for i1, v1 := range issues[i] {
				fun(i1 + "=" + v1)
			}
		}
	}
}

func failSvrCfg(cfgSvrOpt string) {
	eztools.LogPrint("NO servers defined. Run with param -" +
		cfgSvrOpt + " to add some!")
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
	i := eztools.ChooseStrings(names)
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
	eztools.LogPrint("NO interaction allowed in silent mode to provide information!")
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
			saveCfg()
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
	pass4All := len(pass.Type) > 0 && len(pass.Pass) > 0
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
		if len(svr1.Pass.Type) < 1 || len(svr1.Pass.Pass) < 1 {
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
	return saveCfg()
}

func inputPass4Svr(svrType string) (passType, passTxt string, ok bool) {
	passTypes := []string{PassBasic + " - plain text",
		PassPlain + " - base64'ed",
		PassDigest + " - HTTP password, such as from Settings of Gerrit"}
	const (
		pref = "Since this server is "
		affi = " is recommended."
	)
	switch svrType {
	case CategoryGerrit:
		eztools.ShowStrln(pref + svrType + ", " + PassDigest + affi)
	case CategoryJira:
		eztools.ShowStrln(pref + svrType + ", " + PassBasic + affi)
	}
	typeInd := eztools.ChooseStrings(passTypes)
	if typeInd == eztools.InvalidID {
		return
	}
	passTypeSlc := strings.Split(passTypes[typeInd], " - ")
	passType = passTypeSlc[0]
	passTxt = eztools.PromptStr("password")
	if len(passTxt) < 1 {
		return
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
		typeInd := eztools.ChooseStrings(svrTypes)
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
	return saveCfg()
}

func saveCfg() bool {
	if err := eztools.XMLWriteNoCreate(cfgFile, cfg, "\t"); err != nil {
		eztools.LogErrPrint(err)
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
		if ok && diff <= cfg.AppUp.Interval {
			upch <- false
			return
		}
	}
	var (
		db  *sql.DB
		err error
	)
	if len(eztoolscfg) > 0 {
		db, err = eztools.ConnectWtPath(eztoolscfg)
		if err != nil {
			eztoolscfg = ""
		}
	}
	if len(eztoolscfg) == 0 {
		db, err = eztools.Connect()
		if err != nil {
			if /*err == os.PathErr ||*/ err == eztools.ErrNoValidResults {
				eztools.ShowStrln("NO configuration for EZtools. Get one to auto update this app!")
			}
			eztools.LogErrPrint(err)
			upch <- false
			return
		}
	}
	defer db.Close()
	upch <- true
	eztools.AppUpgrade(db, module, ver, nil, upch)
	return
}

func cfg2AuthInfo(svr svrs, cfg jirrit) (authInfo eztools.AuthInfo, err error) {
	pass := svr.Pass
	if len(pass.Pass) < 1 {
		pass = cfg.Pass
	}
	authInfo = eztools.AuthInfo{User: cfg.User}
	switch pass.Type {
	case PassDigest:
		authInfo.Type = eztools.AUTH_DIGEST
	case PassPlain:
		authInfo.Type = eztools.AUTH_PLAIN
	case PassBasic:
		authInfo.Type = eztools.AUTH_BASIC
	default:
		authInfo.Type = eztools.AUTH_NONE
		authInfo.Pass = ""
		return
	}
	authInfo.Pass = pass.Pass
	if len(authInfo.Pass) < 1 {
		err = errors.New("NO password configured")
	}
	return
}

/*                   action name -> actionFunc
category name -> []action2Func
cat2Act
*/
type actionFunc func(*svrs, eztools.AuthInfo, issueInfos) (issueInfoSlc, error)
type action2Func struct {
	n string
	f actionFunc
}

type cat2Act map[string][]action2Func

func isValidSvr(cats cat2Act, svr *svrs) bool {
	if len(svr.Name) < 1 || len(svr.Type) < 1 || len(svr.URL) < 1 {
		return false
	}
	if _, ok := cats[svr.Type]; !ok {
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
	si := eztools.ChooseStrings(choices)
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

func chooseAct(svrType string, choices []string, funcs []action2Func,
	issueInfo issueInfos) (actionFunc, issueInfos) {
	var fi int
	if uiSilent && len(choices) > 1 {
		noInteractionAllowed()
		return nil, issueInfo
	}
	if len(choices) > 1 {
		eztools.ShowStrln(" Choose an action")
		fi = eztools.ChooseStrings(choices)
		if fi == eztools.InvalidID {
			return nil, issueInfo
		}
	}
	inputIssueInfo4Act(svrType, funcs[fi].n, issueInfo)
	return funcs[fi].f, issueInfo
}

func chkErrRest(body interface{}, errno int, err error) (interface{}, error) {
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
		eztools.LogErrPrintWtInfo("REST error" /*strconv.Itoa(errno) */, err)
		if body != nil {
			bodyBytes, ok := body.([]byte)
			if !ok {
				eztools.LogPrint("REST response type " +
					"not byte slice for error " +
					reflect.TypeOf(body).String())
				if eztools.Debugging && eztools.Verbose > 2 {
					eztools.ShowStrln(body)
				}
			} else {
				//eztools.ShowSthln(bodyBytes)
				eztools.LogPrint("REST body=" + string(bodyBytes))
			}
		}
	}
	return body, err
}

func restFile(method, url string, authInfo eztools.AuthInfo,
	fType, fName string, hdrs map[string]string, magic string) (body interface{}, err error) {
	return chkErrRest(eztools.RestGetOrPostWtMagicNFileNHdr(method,
		url, authInfo, []byte(magic), fType, fName, hdrs))
}

// return nil for 404
func restSth(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (body interface{}, err error) {
	return chkErrRest(eztools.RestGetOrPostWtMagic(method,
		url, authInfo, bodyReq, []byte(magic)))
}

func showRspBody(err error, body interface{}) {
	if err != nil {
		if eztools.Debugging && eztools.Verbose > 1 {
			eztools.ShowStrln("failure with body:")
			eztools.ShowSthln(body)
		}
	}
}

func restSlc(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (bodySlc []interface{}, err error) {
	body, err := restSth(method, url, authInfo, bodyReq, magic)
	if err != nil || body == nil {
		return
	}
	bodySlc, ok := body.([]interface{})
	if !ok {
		eztools.LogPrint("REST response type error for slice " +
			reflect.TypeOf(body).String())
	}
	//showRspBody(err, body)
	return
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
		eztools.LogPrint("REST response type error for map " +
			reflect.TypeOf(body).String())
		/*} else {
		showRspBody(err, body)*/
	}
	return
}

/* get all values from
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
	//eztools.ShowSthln(field)
	fieldMap, ok := field.(map[string]interface{})
	if !ok {
		eztools.Log(reflect.TypeOf(field).String() +
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
					eztools.Log(reflect.TypeOf(v).String() +
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
	if choice := eztools.ChooseStrings(values); choice != eztools.InvalidID {
		return values[choice]
	}
	return ""
}

const (
	//common use
	// IssueinfoStrID ID string
	IssueinfoStrID = "id"
	// IssueinfoStrSubmittable submittable string
	IssueinfoStrSubmittable = "submittable"
	// IssueinfoStrKey key string
	IssueinfoStrKey = "key"
	// IssueinfoStrAssignee assignee string
	IssueinfoStrAssignee = "assignee"
	// IssueinfoStrName name string
	IssueinfoStrName = "name"
	// IssueinfoStrHead subject string
	IssueinfoStrHead = "subject"
	// IssueinfoStrSummary summary string
	IssueinfoStrSummary = "summary"
	// IssueinfoStrDesc description string
	IssueinfoStrDesc = "description"
	// IssueinfoStrRevCur current revision string
	IssueinfoStrRevCur = "current_revision"
	// IssueinfoStrVerified verified string
	IssueinfoStrVerified = "Verified"
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
	// IssueinfoStrMergeable mergable string for details, gerrit
	IssueinfoStrMergeable = "mergeable"
	// IssueinfoStrLabels labels string for scores, gerrit
	IssueinfoStrLabels = "labels"
	// IssueinfoStrComments comment string for details, gerrit
	IssueinfoStrComments = "comments"
	// IssueinfoStrBin binary string for file list, gerrit
	IssueinfoStrBin = "binary"
	// IssueinfoStrBin old path/renamed from string for file list, gerrit
	IssueinfoStrOldPath = "old_path"
	// IssueinfoStrCherry cherry pick string of download for gerrit
	IssueinfoStrCherry = "Cherry Pick"
	// IssueinfoStrLink is the JIRA link string in config of a project, gerrit
	IssueinfoStrLink = "link"
	// IssueinfoStrMatch is the JIRA match pattern string in config of a project, gerrit
	IssueinfoStrMatch = "match"
)

type issueInfos map[string]string
type issueInfoSlc []issueInfos

func (inf issueInfos) ToSlc() issueInfoSlc {
	return issueInfoSlc{inf}
}
func (inf issueInfoSlc) ToMapSlc() (res []map[string]string) {
	for _, i := range inf {
		res = append(res, i)
	}
	return
}

//type scoreInfos [IssueinfoStrScore + 1]int

/*func (issueInfo *issueInfos) String() string {
	var res string
	for _, v := range issueInfo {
		switch len(res) {
		case 0:
			res += "[ "
		default:
			res += ", "

		}
		res += "\"" + v + "\""
	}
	res += " ]"
	return res
}*/

var issueInfoTxt = []string{
	IssueinfoStrID, IssueinfoStrKey, IssueinfoStrHead,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrState}
var issueDetailsTxt = []string{
	IssueinfoStrID, IssueinfoStrSubmittable, IssueinfoStrHead,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrState, IssueinfoStrMergeable}
var issueRevsTxt = []string{
	IssueinfoStrID, IssueinfoStrName, IssueinfoStrRevCur,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrSubmitType}
var issueDldCmds = []string{
	IssueinfoStrID, IssueinfoStrSubmittable, IssueinfoStrHead,
	IssueinfoStrProj, IssueinfoStrBranch, IssueinfoStrState, IssueinfoStrMergeable}
var reviewInfoTxt = []string{
	IssueinfoStrID, IssueinfoStrName, IssueinfoStrVerified,
	IssueinfoStrCodereview, IssueinfoStrDispname, IssueinfoStrApprovals}

/*var jiraInfoTxt = issueInfos{ISSUEINFO_STR_ID, ISSUEINFO_STR_KEY,
ISSUEINFO_STR_SUMMARY, ISSUEINFO_STR_PROJ, ISSUEINFO_STR_DISPNAME,
ISSUEINFO_STR_STATE}*/
/*var jiraDetailTxt = issueInfos{
ISSUEINFO_STR_ID, ISSUEINFO_STR_DESC, ISSUEINFO_STR_SUMMARY,
ISSUEINFO_STR_COMMENT, ISSUEINFO_STR_DISPNAME, ISSUEINFO_STR_STATE}*/

const typicalJiraSeparator = "-"

// return values
//	whether input is in exact x-0 or -0 format.
//		in case of -0, if previous project (x part) found, it is taken.
//		otherwise, false is returned.
//	the non digit part. this is saved as project.
//	the digit part
func parseTypicalJiraNum(svr *svrs, num string) (nonDigit,
	digit string, changes, parsed bool) {
	if len(num) < 1 {
		return "", "", false, false
	}
	re := regexp.MustCompile(`^[^-,]+[-][\d]+$`)
	//eztools.ShowStrln("parsing " + num + " 2 typical JIRA")
	pref := re.FindStringSubmatch(num)
	if pref != nil { // "A-1"
		parts := strings.Split(pref[0], typicalJiraSeparator)
		if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 0 {
			saveProj(svr, parts[0])
			return parts[0] + typicalJiraSeparator, parts[1], false, true
		}
	} else {
		if len(svr.Proj) > 0 { // "-1", "1"
			re = regexp.MustCompile(`^[-][\d]+$`)
			pref = re.FindStringSubmatch(num)
			if pref != nil {
				parts := strings.Split(pref[0], typicalJiraSeparator)
				// parts[0]=""
				if len(parts) == 2 && len(parts[1]) > 0 {
					if eztools.Debugging && eztools.Verbose > 2 {
						eztools.ShowStrln("Auto changing to " +
							svr.Proj + typicalJiraSeparator + parts[1])
					}
					return svr.Proj + typicalJiraSeparator, parts[1], true, true
				}
			}
		} // "A-1,B-2" not handled
	}
	return "", "", false, false
}

// loopIssues runs a function on all numbers between, inclusively,
// X-0 and X-1, or 0,1 from input in format of X-0,1 or 0,1
// If it is not a range, the function's return values are returned.
// Otherwise, no return values.
// IssueinfoStrID is set for each loop of function fun,
// from multiple ID's in one issueInfo,
// while other fields use the former values returned from function fun
func loopIssues(svr *svrs, issueInfo issueInfos, fun func(issueInfos) (
	issueInfos, error)) (issueInfoOut issueInfoSlc, err error) {
	const separator = ","
	printID := func() {
		if err == nil {
			eztools.Log("Done with " + issueInfo[IssueinfoStrID])
		}
	}
	//eztools.Log(strings.Count(issueInfo[IssueinfoStrID], separator))
	switch strings.Count(issueInfo[IssueinfoStrID], separator) {
	case 0: // single ID
		if prefix, lowerBoundStr, _, ok := parseTypicalJiraNum(svr,
			issueInfo[IssueinfoStrID]); ok {
			issueInfo[IssueinfoStrID] = prefix + lowerBoundStr
		}
		issueInfo, err := fun(issueInfo)
		printID()
		return issueInfoSlc{issueInfo}, err
	case 2: // x,,y or x,y,z
		parts := strings.Split(issueInfo[IssueinfoStrID], separator)
		//eztools.Log(parts)
		if len(parts) != 2 || len(parts[0]) < 1 || len(parts[2]) < 1 {
			if len(parts) != 3 {
				eztools.LogPrint("range format needs both parts aside with two \"" +
					separator + "\"" + " or multiple parts, deliminated by \"" +
					separator + "\"")
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
				if prefix, lowerBoundStr, _, ok = parseTypicalJiraNum(svr, parts[0]); !ok {
					eztools.LogPrint("the former part must be in the form of X-0 or 0")
					return
				}
				lowerBound, err = strconv.Atoi(lowerBoundStr)
				if err != nil {
					eztools.LogPrint(lowerBoundStr + " is NOT a number!")
					return
				}
			}
			upperBound, err = strconv.Atoi(parts[2])
			if err != nil {
				eztools.LogPrint("the latter part must be a number")
				return
			}
			if lowerBound >= upperBound {
				eztools.LogPrint("the number in the latter part must be greater than the one in the former part")
				return
			}
			for i := lowerBound; i <= upperBound; i++ {
				issueInfo[IssueinfoStrID] = prefix + strconv.Itoa(i)
				//eztools.ShowStrln("looping " + issueInfo[IssueinfoStrID])
				issueInfo, err = fun(issueInfo)
				/*if err != nil {
					return
				}*/ // let it work for the next
				issueInfoOut = append(issueInfoOut, issueInfo)
				printID()
			}
			return
		} else {
			// x,y,z instead of range
			break
		}
	}
	// x,y[,...]
	parts := strings.Split(issueInfo[IssueinfoStrID], separator)
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
		issueInfo, err = fun(issueInfo)
		/*if err != nil {
			return
		}*/ // let it work for the next
		issueInfoOut = append(issueInfoOut, issueInfo)
		printID()
		if i < len(parts) {
			if prefixNew, currentNo, _, ok = parseTypicalJiraNum(svr, parts[i]); !ok {
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

func cfmInputOrPromptStrMultiLines(inf issueInfos, ind, prompt string) {
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
// return value: whether anything new is input
func cfmInputOrPromptStr(svr *svrs, inf issueInfos, ind, prompt string) bool {
	if uiSilent {
		noInteractionAllowed()
		return false
	}
	const linefeed = " (end with \\ to input multi lines)"
	var def, base string
	var changes, smart bool // no smart affix available by default
	if len(inf[ind]) > 0 {
		var ok bool
		if base, _, changes, ok = parseTypicalJiraNum(svr, inf[ind]); ok {
			smart = true // there is a reference for smart affix
			//eztools.ShowStrln("not int previously")
		}
		def = "=" + inf[ind]
	}
	s := eztools.PromptStr(prompt + linefeed + def)
	if len(s) < 1 || s == inf[ind] {
		return false
	}
	var (
		ok, changesI bool
		sNum, baseI  string
	)
	if baseI, sNum, changesI, ok = parseTypicalJiraNum(svr, s); ok {
		smart = true // there is a reference for smart affix
		s = sNum
		base = baseI
		changes = changes || changesI
		//eztools.ShowStrln("not int previously")
	}
	if smart {
		if _, err := strconv.Atoi(s); err != nil {
			smart = false
			//eztools.ShowStrln("not int currently")
			// we do not care what this number is
		}
	}
	if !smart {
		// input not a number or no previous input to refer to
		if s[len(s)-1] == '\\' {
			inf[ind] = s[:len(s)-1] + "\n"
			cfmInputOrPromptStrMultiLines(inf, ind, prompt)
			return true
		}
		inf[ind] = s
		return true
	}
	// smart affix
	inf[ind] = base + s
	if changes {
		//if eztools.Verbose > 0 {
		eztools.ShowStrln("Auto changed to " + inf[ind])
		//}
	}
	return true
}

// cfmInputOrPrompt does not accept multiple ID format for input
func cfmInputOrPrompt(svr *svrs, inf issueInfos, ind string) bool {
	return cfmInputOrPromptStr(svr, inf, ind, ind)
}

func useInputOrPromptStr(inf issueInfos, ind, prompt string) {
	if len(inf[ind]) > 0 {
		return
	}
	if uiSilent {
		noInteractionAllowed()
		return
	}
	inf[ind] = eztools.PromptStr(prompt)
}

func useInputOrPrompt(inf issueInfos, ind string) {
	useInputOrPromptStr(inf, ind, ind)
}

func inputIssueInfo4Act(svrType, action string, inf issueInfos) {
	switch svrType {
	case CategoryJira:
		switch action {
		case "move status of a case",
			"show details of a case",
			"list comments of a case",
			"list files attached to a case",
			"list watchers of a case",
			"check whether watching a case",
			"watch a case",
			"unwatch a case":
			useInputOrPrompt(inf, IssueinfoStrID)
		case "link a case to another":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrLink,
				"ID to be linked to")
		case "close a case to resolved from any known statuses":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrComments,
				"test step for closure")
		case "remove a file attached to a case":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrKey, "file ID")
		case "add a file to a case":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPrompt(inf, IssueinfoStrFile)
		case "get a file to a case":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrKey, "file ID")
			useInputOrPromptStr(inf, IssueinfoStrFile, "file to be saved as")
		case "change a comment from a case":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrKey, "comment ID")
			useInputOrPromptStr(inf, IssueinfoStrComments, "comment body")
		case "delete a comment from a case":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrKey, "comment ID")
		case "add a comment to a case",
			"reject a case from any known statuses":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPrompt(inf, IssueinfoStrComments)
		case "transfer a case to someone":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrHead, "assignee")
			useInputOrPromptStr(inf, IssueinfoStrComments, "component")
		}
	case CategoryGerrit:
		switch action {
		case "show details of a submit",
			"show reviewers of a submit",
			"show current revision/commit of a submit",
			"list files of a submit":
			useInputOrPrompt(inf, IssueinfoStrID)
		case "list files of a submit by revision":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPrompt(inf, IssueinfoStrRevCur)
		case "cherry pick all my open":
			useInputOrPrompt(inf, IssueinfoStrBranch)
		case "list merged submits of someone",
			"add socres, wait for it to be mergable and merge sb.'s submits",
			"list sb.'s open submits":
			useInputOrPromptStr(inf,
				IssueinfoStrID, IssueinfoStrAssignee)
			useInputOrPrompt(inf, IssueinfoStrBranch)
		case "cherry pick a submit":
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPromptStr(inf, IssueinfoStrRevCur, "revision(empty for current)")
			useInputOrPrompt(inf, IssueinfoStrBranch)
		case "list config of a project":
			useInputOrPrompt(inf, IssueinfoStrProj)
		}
	default:
		eztools.LogPrint("Server type unknown: " + svrType)
	}
	//eztools.ShowSthln(inf)
}

func makeCat2Act() cat2Act {
	return cat2Act{
		CategoryJira: []action2Func{
			{"transfer a case to someone", jiraTransfer},
			{"move status of a case", jiraTransition},
			{"show details of a case", jiraDetail},
			{"list comments of a case", jiraComments},
			{"add a comment to a case", jiraAddComment},
			{"delete a comment from a case", jiraDelComment},
			{"change a comment from a case", jiraModComment},
			{"list my open cases", jiraMyOpen},
			{"link a case to another", jiraLink},
			{"list watchers of a case", jiraWatcherList},
			{"check whether watching a case", jiraWatcherCheck},
			{"watch a case", jiraWatcherAdd},
			{"unwatch a case", jiraWatcherDel},
			{"add a file to a case", jiraAddFile},
			{"list files attached to a case", jiraListFile},
			{"get a file to a case", jiraGetFile},
			{"remove a file attached to a case", jiraDelFile},
			{"reject a case from any known statuses", jiraReject},
			{"close a case to resolved from any known statuses", jiraClose},
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			{"close a case with default design as steps", jiraCloseDef},
			{"close a case with general requirement as steps", jiraCloseGen}},
		CategoryGerrit: []action2Func{
			{"list merged submits of someone", gerritSbMerged},
			{"list my open submits", gerritMyOpen},
			{"list sb.'s open submits", gerritSbOpen},
			{"list all my open revisions/commits", gerritRevs},
			{"list all open submits", gerritAllOpen},
			{"show details of a submit", gerritDetailOnCurrRev},
			{"show reviewers of a submit", gerritReviews},
			{"show current revision/commit of a submit", gerritRev},
			{"rebase a submit", gerritRebase},
			{"merge a submit", gerritMerge},
			{"add scores to a submit", gerritScore},
			{"add socres, wait for it to be mergable and merge a submit", gerritWaitNMerge},
			{"add socres, wait for it to be mergable and merge sb.'s submits", gerritWaitNMergeSb},
			{"abandon all my open submits", gerritAbandonMyOpen},
			{"abandon a submit", gerritAbandon},
			{"cherry pick all my open submits", gerritPickMyOpen},
			{"cherry pick a submit", gerritPick},
			{"revert a submit", gerritRevert},
			{"list files of a submit", gerritListFiles},
			{"list files of a submit by revision", gerritListFilesByRev},
			{"list config of a project", gerritListPrj}}}
}
