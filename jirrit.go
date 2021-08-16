package main

import (
	"database/sql"
	"errors"
	"flag"
	"io"
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
	Score string    `xml:"score"`
	Flds  fields    `xml:"fields"`
	Proj  string    `xml:"project"`
	Watch string    `xml:"watch"`
}

type jirrit struct {
	Cmt        string    `xml:",comment"`
	EzToolsCfg string    `xml:"eztoolscfg"`
	Log        string    `xml:"log"`
	User       string    `xml:"user"`
	Pass       passwords `xml:"pass"`
	Svrs       []svrs    `xml:"server"`
}

func main() {
	const ParamDef = "_"
	svrTypes = []string{CategoryJira, CategoryGerrit}
	var (
		paramH, paramV, paramVV, paramVVV,
		paramReverse, paramGetSvrCfg, paramSetSvrCfg bool
		paramR, paramA, paramW, paramK,
		paramI, paramB, paramCfg, paramLog,
		paramHD, paramP, paramS, paramC string
	)
	flag.BoolVar(&paramH, "h", false, "help message")
	flag.BoolVar(&paramV, "v", false,
		"log file output")
	flag.BoolVar(&paramVV, "vv", false, "verbose messages")
	flag.BoolVar(&paramVVV, "vvv", false,
		"verbose messages with network I/O")
	flag.BoolVar(&paramReverse, "reverse", false, "reverse output")
	flag.BoolVar(&paramGetSvrCfg, "getsvrcfg", false,
		"get server list from config")
	flag.BoolVar(&paramSetSvrCfg, "setsvrcfg", false,
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
	flag.StringVar(&paramS, "s", "",
		"linked issue when linking issues")
	flag.StringVar(&paramCfg, "cfg", "", "config file")
	flag.StringVar(&paramLog, "log", "", "log file")
	flag.Parse()
	if paramH {
		eztools.ShowStrln(module + " v" + ver)
		eztools.ShowStrln("  When inputting ID's, there are following options for some actions.")
		eztools.ShowStrln(" 1. single ID, such as 0 or X-0")
		eztools.ShowStrln(" 2. multiple IDs, such as 0,0,0 or X-0,2,1")
		eztools.ShowStrln(" 3. ID range, such as 0,,2 or X-0,2")
		eztools.ShowStrln("")
		flag.Usage()
		eztools.ShowStrln("")
		eztools.ShowStrln("action strings, to be used with server name, \"r\", only, and that will eliminate interactions in UI:")
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
		eztools.LogErrFatal(err)
		var changed bool
		if cfg.Svrs, changed = addSvr(cfg.Svrs, cfg.Pass); !changed {
			return
		}
		home, _ := os.UserHomeDir()
		cfgFile = filepath.Join(home, module+".xml")
		if !saveCfg() {
			return
		}
	}
	if len(paramLog) > 0 {
		cfg.Log = paramLog
	} else if len(cfg.Log) < 1 && eztools.Debugging {
		cfg.Log = module + ".log"
	}
	if len(cfg.Log) > 0 {
		logger, err := os.OpenFile(cfg.Log,
			os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
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
			defer noInteractionAllowed()
			return
		}
		cfg.User = chkUsr(cfg.User)
		cfg.Svrs, _ = addSvr(cfg.Svrs, cfg.Pass)
		return
	}
	if !uiSilent {
		cfg.User = chkUsr(cfg.User)
		if !chkSvr(cfg.Svrs, cfg.Pass) {
			eztools.LogPrint(module + " cannot run without server configs!")
			return
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
			saveCfg()
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
	issueInfo := make(issueInfos)
	//if eztools.Debugging && eztools.Verbose > 0 {
	dispResultOutputFunc = eztools.LogPrint
	//} else {
	//op = eztools.ShowSthln
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
	if svr != nil {
		if len(paramA) > 0 {
			for _, v := range cats[svr.Type] {
				if paramA == v.n {
					uiSilent = true
					fun = v.f
					issueInfo = issueInfos{
						IssueinfoStrID:       paramI,
						IssueinfoStrKey:      paramK,
						IssueinfoStrHead:     paramHD,
						IssueinfoStrProj:     paramP,
						IssueinfoStrBranch:   paramB,
						IssueinfoStrState:    paramS,
						IssueinfoStrComments: paramC}
					eztools.Log("runtime params: server=" +
						svr.Name + ", action=" + v.n +
						", info array:")
					eztools.Log(issueInfo)
					break
				}
			}
			if fun == nil {
				eztools.LogPrint("\"" + paramA +

					"\" NOT recognized as a command")
			}
		}
	}
	if paramReverse {
		step = -1
	} else {
		step = 1
	}
	for ; ; svr = nil { // reset nil among loops
		if svr == nil {
			svr = chooseSvr(cats, cfg.Svrs)
			if svr == nil {
				break
			}
		}
		if len(svr.Proj) > 0 {
			eztools.ShowSthln("default project/ID prefix: " + svr.Proj)
		}
		if fun == nil {
			choices = makeActs2Choose(*svr, cats[svr.Type])
		}
		for ; ; fun = nil { // reset fun among loops
			if fun == nil {
				fun, issueInfo = chooseAct(svr.Type, choices, cats[svr.Type],
					issueInfos{
						IssueinfoStrID:       paramI,
						IssueinfoStrKey:      paramK,
						IssueinfoStrHead:     paramHD,
						IssueinfoStrProj:     paramP,
						IssueinfoStrBranch:   paramB,
						IssueinfoStrState:    paramS,
						IssueinfoStrComments: paramC})
				if fun == nil {
					break
				}
			}
			// auto changing ID is redundant for fun that do this, too.
			// TODO: restructure single input in inputIssueInfo4Act and loop in fun
			changingID, prefID, postID := parseTypicalJiraNum(svr, issueInfo[IssueinfoStrID])
			if changingID {
				issueInfo[IssueinfoStrID] = prefID + postID
			}
			authInfo, err := cfg2AuthInfo(*svr, cfg)
			if err != nil {
				eztools.LogErrFatal(err)
				break
			}
			issues, err := fun(svr, authInfo, issueInfo)
			if err != nil {
				eztools.LogErrFatal(err)
			}
			dispResults(issues)
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
	if <-upch {
		if eztools.Debugging {
			eztools.ShowStrln("waiting for update check to end...")
		}
		<-upch
	}
}

func dispResults(issues []issueInfos) {
	if issues == nil {
		dispResultOutputFunc("No results.")
	} else {
		var i int
		if step < 1 {
			i = len(issues) - 1
		} else {
			i = 0
		}
		for ; i >= 0 && i < len(issues); i += step {
			dispResultOutputFunc("Issue/Reviewer/Comment/File " +
				strconv.Itoa(i+1))
			for i1, v1 := range issues[i] {
				dispResultOutputFunc(i1 + "=" + v1)
			}
		}
	}
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

func chkUsr(user string) string {
	var unDef string
	if len(user) > 0 {
		return user
		//unDef = "[Enter=" + user + "]"
	}
	un := eztools.PromptStr("username" + unDef)
	if len(un) < 1 && len(unDef) > 0 {
		un = user
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

func chkSvr(svr []svrs, pass passwords) bool {
	if nil == svr || len(svr) < 1 {
		eztools.LogPrint("NO servers defined. Run with param -setsvrcfg to add some!")
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
				eztools.ShowSthln("server already exists")
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
	eztools.ShowStrln(cfgFile + " saved.")
	return true
}

func chkUpdate(eztoolscfg string, upch chan bool) {
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
			upch <- false
			if /*err == os.PathErr ||*/ err == eztools.ErrNoValidResults {
				eztools.ShowStrln("NO configuration for EZtools. Get one to auto update this app!")
			}
			eztools.LogErrPrint(err)
			return
		}
	}
	defer db.Close()
	eztools.AppUpgrade(db, module, ver, nil, upch)
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
type actionFunc func(*svrs, eztools.AuthInfo, issueInfos) ([]issueInfos, error)
type action2Func struct {
	n string
	f actionFunc
}

type cat2Act map[string][]action2Func

type postRESTs func([]interface{})

var postREST postRESTs

func setPostREST(fun postRESTs) {
	postREST = fun
}

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
		defer noInteractionAllowed()
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
		defer noInteractionAllowed()
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

// return nil for 404
func restSth(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (body interface{}, err error) {
	var errno int
	body, errno, err = eztools.RestGetOrPostWtMagic(method,
		url, authInfo, bodyReq, []byte(magic))
	if err != nil {
		if errno == 404 {
			return nil, nil
		}
		eztools.LogErrPrint( /*strconv.Itoa(errno), */ err)
		if body != nil {
			bodyBytes, ok := body.([]byte)
			if !ok {
				eztools.LogPrint("REST response type " +
					"not byte slice for error " +
					reflect.TypeOf(body).String())
				if eztools.Debugging && eztools.Verbose > 2 {
					eztools.ShowSthln(body)
				}
			} else {
				//eztools.ShowSthln(bodyBytes)
				eztools.LogPrint(string(bodyBytes))
			}
		}
		return
	}
	return
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

func restMapOrSth(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (body interface{},
	bodyMap map[string]interface{}, err error) {
	body, err = restSth(method, url, authInfo, bodyReq, magic)
	if err != nil || body == nil {
		return
	}
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		eztools.LogPrint("REST response type error for map " +
			reflect.TypeOf(body).String())
	} else {
		showRspBody(err, body)
	}
	return
}

func restMap(method, url string, authInfo eztools.AuthInfo,
	bodyReq io.Reader, magic string) (
	bodyMap map[string]interface{}, err error) {
	_, bodyMap, err = restMapOrSth(method, url, authInfo, bodyReq, magic)
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
		defer noInteractionAllowed()
		return ""
	}
	if choice := eztools.ChooseStrings(values); choice != eztools.InvalidID {
		return values[choice]
	}
	return ""
}

const (
	//common use

	/*// IssueinfoStrID ID for issueInfos
	IssueinfoStrID = iota
	// IssueinfoStrKey key for issueInfos
	IssueinfoStrKey
	// IssueinfoStrHead head/title for issueInfos
	IssueinfoStrHead
	// IssueinfoStrProj project for issueInfos
	IssueinfoStrProj
	// IssueinfoStrBranch branch for issueInfos
	IssueinfoStrBranch
	// IssueinfoStrState state for issueInfos
	IssueinfoStrState
	// IssueinfoStrExt extension/placeholder for issueInfos
	IssueinfoStrExt // placeholder for mergable of gerrit and comment of jira
	// IssueinfoStrMax number of issueInfos indexes
	IssueinfoStrMax

	// gerrit state

	// placeholder for ID

	// IssueinfoStrSubmittable submittable of issueInfos for gerrit
	IssueinfoStrSubmittable = iota - IssueinfoStrMax
	// IssueinfoStrVerified verified of issueInfos for gerrit
	IssueinfoStrVerified
	// IssueinfoStrOldPath binary of issueInfos for gerrit file list
	IssueinfoStrOldPath
	// IssueinfoStrCodereview codereview of issueInfos for gerrit
	IssueinfoStrCodereview = iota - IssueinfoStrMax - 1
	// IssueinfoStrBin binary of issueInfos for gerrit file list
	IssueinfoStrBin
	// IssueinfoStrScore configured score of issueInfos for gerrit
	IssueinfoStrScore = iota - IssueinfoStrMax - 2 // upper bound of scoreInfos
	// IssueinfoStrSubmitType submit type of issueInfos for gerrit
	IssueinfoStrSubmitType
	// IssueinfoStrMergeable mergable of issueInfos for gerrit
	IssueinfoStrMergeable

	// jira details

	// placeholder for ID

	// IssueinfoStrDesc description of issueInfos for Jira
	IssueinfoStrDesc = iota - 1 - IssueinfoStrMax*2
	// no id for summary, jira

	// IssueinfoStrDispname display name of issueInfos for Jira
	IssueinfoStrDispname = iota + 1 - IssueinfoStrMax*2
	// IssueinfoStrComment comment of issueInfos for Jira
	IssueinfoStrComment = iota + 2 - IssueinfoStrMax*2*/

	// IssueinfoStrID ID string for issueInfos
	IssueinfoStrID = "id"
	// IssueinfoStrSubmittable submittable string for issueInfos
	IssueinfoStrSubmittable = "submittable" // \
	// IssueinfoStrKey key string for issueInfos
	IssueinfoStrKey = "key" //
	// IssueinfoStrAssignee assignee string for issueInfos
	IssueinfoStrAssignee = "assignee" //
	// IssueinfoStrName name string for issueInfos
	IssueinfoStrName = "name" // /
	// IssueinfoStrHead subject string for issueInfos
	IssueinfoStrHead = "subject" // \
	// IssueinfoStrSummary summary string for issueInfos
	IssueinfoStrSummary = "summary" //
	// IssueinfoStrDesc description string for issueInfos
	IssueinfoStrDesc = "description" //
	// IssueinfoStrRevCur current revision string for issueInfos
	IssueinfoStrRevCur = "current_revision" //
	// IssueinfoStrVerified verified string for issueInfos
	IssueinfoStrVerified = "Verified" // /
	// IssueinfoStrProj project string for issueInfos
	IssueinfoStrProj = "project" // \
	// IssueinfoStrCodereview code review string for issueInfos
	IssueinfoStrCodereview = "Code-Review" // /
	// IssueinfoStrBranch branch string for issueInfos
	IssueinfoStrBranch = "branch" // \
	// IssueinfoStrDispname display name string for issueInfos
	IssueinfoStrDispname = "displayName" // /
	// for code-review, verified and manual-testing

	// IssueinfoStrSubmitType submit type string for issueInfos
	IssueinfoStrSubmitType = "submit_type" // \
	// IssueinfoStrApprovals approvals string for issueInfos
	IssueinfoStrApprovals = "approvals" //
	// IssueinfoStrState state string for issueInfos
	IssueinfoStrState = "status" // /
	// gerrit details

	// IssueinfoStrMergeable mergable string for issueInfos
	IssueinfoStrMergeable = "mergeable"
	// IssueinfoStrComments comment string for issueInfos
	IssueinfoStrComments = "comments"

	// gerrit file list

	// IssueinfoStrBin binary string for issueInfos
	IssueinfoStrBin = "binary"
	// IssueinfoStrBin old path/renamed from string for issueInfos
	IssueinfoStrOldPath = "old_path"

	// gerrit download list

	// IssueinfoStrCherry cherry pick string of download for issueInfos
	IssueinfoStrCherry = "Cherry Pick"

	// gerrit project config

	// IssueinfoStrLink is the JIRA link string in config of a project
	IssueinfoStrLink = "link"
	// IssueinfoStrMatch is the JIRA match pattern string in config of a project
	IssueinfoStrMatch = "match"
)

type issueInfos map[string]string

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

func chkNSetIssueInfo(v interface{}, issueInfo issueInfos, i string) bool {
	if v == nil {
		eztools.Log("nil got, not string")
		return false
	}
	str, ok := v.(string)
	if !ok {
		eztools.LogPrint(reflect.TypeOf(v).String() +
			" got instead of string")
		return false
	}
	issueInfo[i] = str
	return true
}

// check map type before looping it
func chkNLoopStringMap(m interface{},
	mustStr string, keyStr []string) ([]string, bool) {
	sub, ok := m.(map[string]interface{})
	if !ok {
		eztools.LogPrint(reflect.TypeOf(m).String() +
			" got instead of map[string]interface{}")
		return nil, false
	}
	return loopStringMap(sub, mustStr, keyStr, nil)
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
		//eztools.ShowStrln("looping " + i)
		if len(keyStr) > 0 {
			matched := false
			for j, key1 := range keyStr {
				if i == key1 {
					matched = true
					id, ok := v.(string)
					if !ok {
						eztools.LogPrint(
							reflect.TypeOf(v).String() +
								" got instead of string")
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

func custFld(jsonStr, fldKey, fldVal string) string {
	if len(fldKey) > 0 {
		if len(jsonStr) > 0 {
			jsonStr += `,
`
		}
		return jsonStr + `        "` +
			fldKey + `": "` + fldVal + `"`
	}
	return jsonStr
}

const typicalJiraSeparator = "-"

// return values
//	whether input is in exact x-0 or -0 format.
//		in case of -0, if previous project (x part) found, it is taken.
//		otherwise, false is returned.
//	the non digit part. this is saved as project.
//	the digit part
func parseTypicalJiraNum(svr *svrs, num string) (ret bool, nonDigit, digit string) {
	if len(num) < 1 {
		return false, "", ""
	}
	re := regexp.MustCompile(`^[^-,]+[-][\d]+$`)
	//eztools.ShowStrln("parsing " + num + " 2 typical JIRA")
	pref := re.FindStringSubmatch(num)
	if pref != nil {
		parts := strings.Split(pref[0], typicalJiraSeparator)
		if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 0 {
			saveProj(svr, parts[0])
			return true, parts[0] + typicalJiraSeparator, parts[1]
		}
	} else { // "-123", "A-1,B-2", "123", etc.
		if len(svr.Proj) > 0 {
			re = regexp.MustCompile(`^[-][\d]+$`)
			pref = re.FindStringSubmatch(num)
			if pref != nil {
				parts := strings.Split(pref[0], typicalJiraSeparator)
				// parts[0]=""
				if len(parts) == 2 && len(parts[1]) > 0 {
					if eztools.Debugging {
						eztools.ShowStrln("Auto changing to " +
							svr.Proj + typicalJiraSeparator + parts[1])
					}
					return true, svr.Proj + typicalJiraSeparator, parts[1]
				}
			}
		}
	}
	return false, "", ""
}

// loopIssues runs a function on all numbers between, inclusively,
// X-0 and X-1, or 0,1 from input in format of X-0,1 or 0,1
// If it is not a range, the function's return values are returned.
// Otherwise, no return values.
func loopIssues(svr *svrs, issueInfo issueInfos, fun func(issueInfos) (
	issueInfos, error)) (issueInfoOut []issueInfos, err error) {
	const separator = ","
	printID := func() {
		if err == nil {
			eztools.LogPrint("Done with " + issueInfo[IssueinfoStrID])
		}
	}
	//eztools.Log(strings.Count(issueInfo[IssueinfoStrID], separator))
	switch strings.Count(issueInfo[IssueinfoStrID], separator) {
	case 0: // single ID
		if ok, prefix, lowerBoundStr := parseTypicalJiraNum(svr,
			issueInfo[IssueinfoStrID]); ok {
			issueInfo[IssueinfoStrID] = prefix + lowerBoundStr
		}
		issueInfo, err := fun(issueInfo)
		printID()
		return []issueInfos{issueInfo}, err
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
				if ok, prefix, lowerBoundStr = parseTypicalJiraNum(svr, parts[0]); !ok {
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
	if ok, prefix, currentNo = parseTypicalJiraNum(svr, parts[0]); !ok {
		currentNo = parts[0]
	}
	i := 1
	/*eztools.ShowStrln(prefix + "_" + currentNo)
	eztools.ShowSthln(parts)*/
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
			if ok, prefixNew, currentNo = parseTypicalJiraNum(svr, parts[i]); !ok {
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
		defer noInteractionAllowed()
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

// return value: whether anything new is input
func cfmInputOrPromptStr(svr *svrs, inf issueInfos, ind, prompt string) bool {
	if uiSilent {
		return false
	}
	const linefeed = " (end with \\ to input multi lines)"
	var def, base string
	var smart bool // no smart affix available by default
	if len(inf[ind]) > 0 {
		var ok bool
		if ok, base, _ = parseTypicalJiraNum(svr, inf[ind]); ok {
			smart = true // there is a reference for smart affix
			//eztools.ShowStrln("not int previously")
		}
		def = "=" + inf[ind]
	}
	s := eztools.PromptStr(prompt + linefeed + def)
	if len(s) < 1 || s == inf[ind] {
		return false
	}
	if len(inf[ind]) < 1 {
		var (
			ok   bool
			sNum string
		)
		if ok, base, sNum = parseTypicalJiraNum(svr, s); ok {
			smart = true // there is a reference for smart affix
			s = sNum
			//eztools.ShowStrln("not int previously")
		}
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
	//if eztools.Debugging {
	eztools.ShowStrln("Auto changed to " + inf[ind])
	//}
	return true
}

func cfmInputOrPrompt(svr *svrs, inf issueInfos, ind string) bool {
	return cfmInputOrPromptStr(svr, inf, ind, ind)
}

func useInputOrPromptStr(inf issueInfos, ind, prompt string) {
	if len(inf[ind]) > 0 {
		return
	}
	if uiSilent {
		defer noInteractionAllowed()
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
		case "show details of a case",
			"list comments of a case",
			"list watchers of a case",
			"check whether watching a case",
			"watch a case",
			"unwatch a case":
			useInputOrPrompt(inf, IssueinfoStrID)
		case "close a case to resolved from any known statues":
			useInputOrPromptStr(inf, IssueinfoStrComments,
				"test step for closure")
		}
	case CategoryGerrit:
		switch action {
		case "show details of a submit",
			"show reviewers of a submit",
			"rebase a submit",
			"abandon a submit",
			"show current revision/commit of a submit",
			"add scores to a submit",
			"reject a case from any known statues",
			"revert a submit",
			"list files of a submit",
			"merge a submit":
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
			eztools.ShowStrln("Please input an ID that can make it " +
				"distinguished, such as commit, instead of Change " +
				"ID, which is reused among cherrypicks.")
			useInputOrPrompt(inf, IssueinfoStrID)
			useInputOrPrompt(inf, IssueinfoStrRevCur)
			useInputOrPrompt(inf, IssueinfoStrBranch)
		case "list links of a project":
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
			{"link a case to the other", jiraLink},
			{"reject a case from any known statues", jiraReject},
			{"close a case to resolved from any known statues", jiraClose},
			// the last two are to be hidden from choices,
			// if lack of configuration of Tst*
			{"close a case with default design as steps", jiraCloseDef},
			{"close a case with general requirement as steps", jiraCloseGen},
			{"list watchers of a case", jiraWatcherList},
			{"check whether watching a case", jiraWatcherCheck},
			{"watch a case", jiraWatcherAdd},
			{"unwatch a case", jiraWatcherDel}},
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
			{"list links of a project", gerritListPrj}}}
}
