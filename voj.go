package vjudger

import (
    "encoding/base64"
    "html"
    "io/ioutil"
    "log"
    "net/http"
    "net/http/cookiejar"
    "net/url"
    // "os"
    "regexp"
    "strconv"
    "strings"
    "time"
    "encoding/json"
    "fmt"
)

type VJJudger struct {
    client   *http.Client
    token    string
    username string
    userpass string
}

type VJStatus struct {
    Data [][]interface{}
    Draw,RecordsFiltered,RecordsTotal int
}

const VJToken = "VJ"

var VJRes = map[string]int{
    "Wait":                  0,
    "Queue":                 0,
    "Queuing":               0,
    "Pending":               0,
    "Submitted":             0,
    "Compiling":             1,
    "Running":               1,
    "Judging":               1,
    "Judging...":            1,
    "ing":                   1,
    "Compile Error":         2,
    "Compilation Error":     2,
    "Accepted":              3,
    "Runtime Error":         4,
    "Floating Point Error":  4,
    "Crash":                 4,
    "Wrong Answer":          5,
    "Time Limit Exceed":     6,
    "Time Limit Exceeded":   6,
    "Memory Limit Exceed":   7,
    "Memory Limit Exceeded": 7,
    "Output Limit Exceed":   8,
    "Output Limit Exceeded": 8,
    "Presentation Error":    5,
    "Submit Failed":         10}

// only for CF
var VJLang = map[int]int{
    LanguageNA:   -1,
    LanguageC:    10,
    LanguageCPP:  42,
    LanguageJAVA: 36}

func (h *VJJudger) Init(_ UserInterface) error {
    jar, _ := cookiejar.New(nil)
    h.client = &http.Client{Jar: jar}
    h.token = VJToken
    h.username = "vsake"
    h.userpass = "JC945312"
    return nil
}

func (h *VJJudger) Match(token string) bool {
    if token == VJToken {
        return true
    }
    return false
}

func (h *VJJudger) Login(_ UserInterface) (err error) {
    for i := 0; i < 3; i++ {
        err = h.login()
        if err == nil {
            return nil
        }
    }

    return err
}

func (h *VJJudger) login() error {

    log.Println("vj login")

    resp, err := h.client.PostForm("http://acm.hust.edu.cn/vjudge/user/login.action", url.Values{
        "username": {h.username},
        "password": {h.userpass},
    })
    if err != nil {
        return BadInternet
    }
    defer resp.Body.Close()

    resp, err = h.client.Get("http://acm.hust.edu.cn/vjudge/user/checkLogInStatus.action")
	if err != nil {
		return BadInternet
	}
	b, _ := ioutil.ReadAll(resp.Body)
	if string(b) != "\"true\"" {
        return BadInternet
    }
    return nil
}

// FixCode sets a code id on the top of code
// Need to more than 50 chars
func (h *VJJudger) FixCode(sid string, code string) string {
    return "//" + sid + "\n//12345678901234567890123456789012345678901234567890\n" + code
}

func (h *VJJudger) Submit(u UserInterface) (err error) {
    for i := 1; i < 3; i++ {
        err = h.submit(u)
        if err != BadInternet || err == nil {
            break
        }
    }

    return
}

func (h *VJJudger) submit(u UserInterface) error {
    log.Println("vj submit")

    sd := h.FixCode(strconv.Itoa(u.GetSid()), u.GetCode())
    sd = strings.Replace(sd, "\r\n", "\n", -1)

    source := base64.StdEncoding.EncodeToString([]byte(sd))

    u.SetSubmitTime(time.Now())
    resp, err := h.client.PostForm("http://acm.hust.edu.cn/vjudge/problem/submit.action", url.Values{
        "language": {strconv.Itoa(VJLang[u.GetLang()])},
        "isOpen": {"0"},
        "source": {source},
        "id": {strconv.Itoa(u.GetVid())},
    })
    if err != nil {
        return BadInternet
    }
    defer resp.Body.Close()

    b, _ := ioutil.ReadAll(resp.Body)
    html := string(b)

    // log.Println(html)
    if strings.Index(html, "Virtual Judge is not a real online judge.") >= 0 {
        log.Println(NoSuchProblem)
        return NoSuchProblem
    }
    if strings.Index(html, "Source code should be longer than 50 characters!") >= 0 {
        log.Println(SubmitFailed)

        return SubmitFailed
    }

    if strings.Index(html, "504 Gateway Time-out") >= 0 {
        return BadInternet
    }

    log.Println("submit success")
    return nil
}

func (h *VJJudger) GetStatus(u UserInterface) error {

    log.Println("fetch status")

    endTime := time.Now().Add(MAX_WaitTime * time.Second)

    for true {
        if time.Now().After(endTime) {
            return BadStatus
        }
        resp, err := h.client.PostForm("http://acm.hust.edu.cn/vjudge/problem/fetchStatus.action", url.Values{
            "start": {"0"},
            "length": {"100"},
            "orderBy": {"run_id"},
            "un": {h.username},
            "draw": {"3"},
            "probNum": {""},
        })
        if err != nil {
            return BadInternet
        }
        defer resp.Body.Close()

        b, _ := ioutil.ReadAll(resp.Body)
        var AllStatus VJStatus
        json.Unmarshal([]byte(string(b)), &AllStatus)

        for i := 0; i < len(AllStatus.Data); i++ {
            status := AllStatus.Data[i]

            rid := strconv.Itoa(int(status[0].(float64))) //remote server run id

            //although it uses more time to get id, but it should work fine:)
            if h.GetCodeID(rid) == strconv.Itoa(u.GetSid()) {
                u.SetResult(VJRes[status[3].(string)])
                Time, Mem := 0, 0
                if u.GetResult() > JudgeRJ {
                    if u.GetResult() == JudgeCE {
                        CE, err := h.GetCEInfo(rid)
                        if err != nil {
                            log.Println(err)
                        }
                        u.SetErrorInfo(CE)
                    } else if u.GetResult() == JudgeAC {
                        Time = int(status[5].(float64))
                        Mem = int(status[4].(float64))
                    }
                    u.SetResource(Time, Mem, int(status[7].(float64)))
                    return nil
                }
            }
        }
        time.Sleep(1 * time.Second)
    }
    return nil
}

func (h *VJJudger) GetCodeID(rid string) string {
    resp, err := h.client.Get("http://acm.hust.edu.cn/vjudge/problem/viewSource.action?id=" + rid)
    if err != nil {
        return ""
    }

    b, _ := ioutil.ReadAll(resp.Body)

    pre := `(?s)<pre.*?>(.*?)</pre>`
    re := regexp.MustCompile(pre)
    match := re.FindStringSubmatch(string(b))
    if len(match) <=1 {
        return ""
    }
    code := html.UnescapeString(match[1])
    split := strings.Split(code, "\n")
    return strings.TrimPrefix(split[0], "//")
}

func (h *VJJudger) GetCEInfo(rid string) (string, error) {
    resp, err := h.client.Get("http://acm.hust.edu.cn/vjudge/problem/fetchSubmissionInfo.action?id=" + rid)
    if err != nil {
        return "", BadInternet
    }

    b, _ := ioutil.ReadAll(resp.Body)
    pre := `(?s)"<pre>(.*?)"`
    re := regexp.MustCompile(pre)
    match := re.FindStringSubmatch(string(b))
    if len(match) <=1 {
        return "", BadInternet
    }
    return html.UnescapeString(match[1]), nil
}

func (h *VJJudger) Run(u UserInterface) error {
    for _, apply := range []func(UserInterface) error{h.Init, h.Login, h.Submit, h.GetStatus} {
        if err := apply(u); err != nil {
            log.Println(err)
            return err
        }
    }
    return nil
}
