package jd_cookie

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/astaxie/beego/httplib"
	"github.com/buger/jsonparser"
	"github.com/cdle/sillyGirl/core"
	"github.com/cdle/sillyGirl/develop/qinglong"
)

var jdWSCK = core.NewBucket("jdWSCK")

var ua2 = `http/3.12.1;jdmall;android;version/10.1.2;build/89743;screen/1440x3007;os/11;network/wifi;`

func initCheck() {
	go func() {
		for {
			data, _ := httplib.Get("https://hellodns.coding.net/p/sign/d/jsign/git/raw/master/sign").Bytes()
			uuid, _ := jsonparser.GetString(data, "uuid")
			if uuid != "" {
				jdWSCK.Set(uuid, string(data))
			}
			time.Sleep(time.Minute)
		}
	}()
	core.AddCommand("jd", []core.Function{
		{
			Rules: []string{`raw ^更新狗东账号`},
			Cron:  jdWSCK.Get("update", "55 * * * *"),
			Admin: true,
			Handle: func(s core.Sender) interface{} {
				if s.GetImType() == "fake" && !jd_cookie.GetBool("enable_auto_update", true) {
					return nil
				}
				var cks = map[string]qinglong.Env{}
				var wscks = map[string]qinglong.Env{}
				envs, _ := qinglong.GetEnvs("")
				for _, env := range envs {
					if env.Name == "JD_COOKIE" {
						cks[core.FetchCookieValue(env.Value, "pt_pin")] = env
					}
					if env.Name == "JD_WSCK" && env.Status == 0 {
						wscks[core.FetchCookieValue(env.Value, "pin")] = env
					}
				}
				for pin, env := range cks {
					if env.Status != 0 {
						continue
					}
					pt_key := core.FetchCookieValue(env.Value, "pt_key")
					ck := &JdCookie{
						PtPin: pin,
						PtKey: pt_key,
					}
					if ck.Available() {
						s.Reply(fmt.Sprintf("%s,JD_COOKIE有效。", ck.Nickname), core.E)
						delete(wscks, pin)
						continue
					}
					s.Reply(fmt.Sprintf("%s,JD_COOKIE已失效。", pin), core.E, core.N)
					Notify(pin, fmt.Sprintf("您的账号(%s)已过期，请及时登录。", pin))
					if err := qinglong.Config.Req(qinglong.PUT, qinglong.ENVS, "/disable", []byte(`["`+env.ID+`"]`)); err != nil {
						s.Reply(fmt.Sprintf("%s,JD_COOKIE禁用失败。%v", pin, err), core.E)
					} else {
						s.Reply(fmt.Sprintf("%s,JD_COOKIE已禁用。", pin), core.E)
					}
					wse, ok := wscks[pin]
					if !ok {
						continue
					}
					pt_key, err := getKey(wse.Value)
					if err != nil {
						s.Reply(fmt.Sprintf("%s,JD_WSCK转换失败。%v", pin, err), core.E, core.N)
						delete(wscks, pin)
						continue
					}
					if strings.Contains(pt_key, "fake") {
						s.Reply(fmt.Sprintf("%s,JD_WSCK已失效。", pin), core.E)
						if jdWSCK.GetBool("autoDisableWsck", false) {
							if err := qinglong.Config.Req(qinglong.PUT, qinglong.ENVS, "/disable", []byte(`["`+wse.ID+`"]`)); err != nil {
								s.Reply(fmt.Sprintf("%s,JD_WSCK禁用失败。%v", pin, err), core.E)
							} else {
								s.Reply(fmt.Sprintf("%s,JD_WSCK已禁用。", pin), core.E)
							}
						}
						delete(wscks, pin)
						continue
					}
					s.Reply(fmt.Sprintf("%s,JD_WSCK转换JD_COOKIE成功。", pin), core.E, core.N)
					if err := qinglong.Config.Req(qinglong.PUT, qinglong.ENVS, "/enable", []byte(`["`+env.ID+`"]`)); err != nil {
						s.Reply(fmt.Sprintf("%s,JD_COOKIE启用失败。%v", pin, err), core.E)
					} else {
						s.Reply(fmt.Sprintf("%s,JD_COOKIE已启用。", pin), core.E)
					}
					env.Status = 0
					env.Value = fmt.Sprintf("pt_key=%s;pt_pin=%s;", pt_key, pin)
					if err := qinglong.UdpEnv(env); err != nil {
						s.Reply(fmt.Sprintf("%s,JD_COOKIE更新失败。%v", pin, err), core.E)
					} else {
						s.Reply(fmt.Sprintf("%s,JD_COOKIE已更新。", pin), core.E)
					}
					delete(wscks, pin)
				}
				for pin, wse := range wscks {
					pt_key, err := getKey(wse.Value)
					if err != nil {
						s.Reply(fmt.Sprintf("%s,JD_WSCK转换失败。%v", pin, err), core.E, core.N)
						continue
					}
					if strings.Contains(pt_key, "fake") {
						s.Reply(fmt.Sprintf("%s,JD_WSCK已失效。", pin), core.E)
						if jdWSCK.GetBool("autoDisableWsck", false) {
							if err := qinglong.Config.Req(qinglong.PUT, qinglong.ENVS, "/disable", []byte(`["`+wse.ID+`"]`)); err != nil {
								s.Reply(fmt.Sprintf("%s,JD_WSCK禁用失败。%v", pin, err), core.E)
							} else {
								s.Reply(fmt.Sprintf("%s,JD_WSCK已禁用。", pin), core.E)
							}
						}
						continue
					}
					s.Reply(fmt.Sprintf("%s,JD_WSCK转换JD_COOKIE成功。", pin), core.E, core.N)
					value := fmt.Sprintf("pt_key=%s;pt_pin=%s;", pt_key, pin)
					if env, ok := cks[pin]; ok {
						env.Value = value
						if err := qinglong.Config.Req(qinglong.PUT, qinglong.ENVS, "/enable", []byte(`["`+env.ID+`"]`)); err != nil {
							s.Reply(fmt.Sprintf("%s,JD_COOKIE启用失败。%v", pin, err), core.E)
						} else {
							s.Reply(fmt.Sprintf("%s,JD_COOKIE已启用。", pin), core.E)
						}
						env.Status = 0
						if err := qinglong.UdpEnv(env); err != nil {
							s.Reply(fmt.Sprintf("%s,JD_COOKIE更新失败。%v", pin, err), core.E)
						} else {
							s.Reply(fmt.Sprintf("%s,JD_COOKIE已更新。", pin), core.E)
						}

					} else {
						if err := qinglong.AddEnv(qinglong.Env{
							Name:  "JD_COOKIE",
							Value: value,
						}); err != nil {
							s.Reply(fmt.Sprintf("%s,JD_COOKIE添加失败。%v", pin, err), core.E)
						} else {
							s.Reply(fmt.Sprintf("%s,JD_COOKIE已添加。", pin), core.E)
						}
					}
				}
				s.Reply("账号检查完成", core.E)
				return nil
			},
		},
	})
}

type AutoGenerated struct {
	ClientVersion string `json:"clientVersion"`
	Client        string `json:"client"`
	Sv            string `json:"sv"`
	St            string `json:"st"`
	UUID          string `json:"uuid"`
	Sign          string `json:"sign"`
	FunctionID    string `json:"functionId"`
}

func getSign() *AutoGenerated {
	var a *AutoGenerated
	jdWSCK.Foreach(func(_, v []byte) error {
		t := &AutoGenerated{}
		if json.Unmarshal(v, t) == nil {
			if a == nil || t.St > a.St {
				a = t
			}
		}
		return nil
	})
	if a != nil {
		a.FunctionID = "genToken"
	}
	return a
}

func getKey(WSCK string) (string, error) {
	v := url.Values{}
	s := getSign()
	if s == nil {
		return "", errors.New("获取sign失败")
	}
	v.Add("functionId", s.FunctionID)
	v.Add("clientVersion", s.ClientVersion)
	v.Add("client", s.Client)
	v.Add("uuid", s.UUID)
	v.Add("st", s.St)
	v.Add("sign", s.Sign)
	v.Add("sv", s.Sv)
	req := httplib.Post(`https://api.m.jd.com/client.action?` + v.Encode())
	req.Header("cookie", WSCK)
	req.Header("User-Agent", ua2)
	req.Header("content-type", `application/x-www-form-urlencoded; charset=UTF-8`)
	req.Header("charset", `UTF-8`)
	req.Header("accept-encoding", `br,gzip,deflate`)
	req.Body(`body=%7B%22action%22%3A%22to%22%2C%22to%22%3A%22https%253A%252F%252Fplogin.m.jd.com%252Fcgi-bin%252Fm%252Fthirdapp_auth_page%253Ftoken%253DAAEAIEijIw6wxF2s3bNKF0bmGsI8xfw6hkQT6Ui2QVP7z1Xg%2526client_type%253Dandroid%2526appid%253D879%2526appup_type%253D1%22%7D&`)
	data, err := req.Bytes()
	if err != nil {
		return "", err
	}
	tokenKey, _ := jsonparser.GetString(data, "tokenKey")
	pt_key, err := appjmp(tokenKey)
	if err != nil {
		return "", err
	}
	return pt_key, nil
}

func appjmp(tokenKey string) (string, error) {
	v := url.Values{}
	v.Add("tokenKey", tokenKey)
	v.Add("to", ``)
	v.Add("client_type", "android")
	v.Add("appid", "879")
	v.Add("appup_type", "1")
	req := httplib.Get(`https://un.m.jd.com/cgi-bin/app/appjmp?` + v.Encode())
	req.Header("User-Agent", ua2)
	req.Header("accept", `text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3`)
	req.SetCheckRedirect(func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	})
	rsp, err := req.Response()
	if err != nil {
		return "", err
	}
	cookies := strings.Join(rsp.Header.Values("Set-Cookie"), " ")
	pt_key := core.FetchCookieValue(cookies, "pt_key")
	return pt_key, nil
}

func Notify(pt_pin string, content string) {
	if jd_cookie.GetBool("disable_notify") {
		return
	}
	qqGroup := jd_cookie.GetInt("qqGroup")
	wxGroup := jd_cookie.GetInt("wxGroup")
	mode := jd_cookie.Get("notify_mode", "private")
	for _, tp := range []string{
		"qq", "tg", "wx",
	} {
		core.Bucket("pin" + strings.ToUpper(tp)).Foreach(func(k, v []byte) error {
			if string(k) == pt_pin && pt_pin != "" {
				if mode != "group" {
					if push, ok := core.Pushs[tp]; ok {
						push(string(v), content, qqGroup, "")
					}
				} else {
					if push, ok := core.GroupPushs[tp]; ok {
						if tp == "qq" {
							push(qqGroup, string(v), content, "")
						}
						if tp == "wx" {
							push(wxGroup, string(v), content, "")
						}
					}
				}
				time.Sleep(time.Second)
			}
			return nil
		})
	}
}
