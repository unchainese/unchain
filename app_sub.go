package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
)

//vless://6fe57e3f-e618-4873-ba96-a76ad1112ccd@felix-us.xxx.cn:443?
//encryption=none&
//security=tls&
//sni=felix-us.xxx.cn&
//allowInsecure=1&
//type=ws&
//hostSni=felix-us.xxx.cn
//&path=%2Fws-vless%3Fed%3D2560
//#felix-us.xxx.cn

// vless://6fe57e3f-e618-4873-ba96-a76ad1112ccd@aws.xxx.cn:80?encryption=none&security=none&sni=s5cf.xxx.cn&allowInsecure=1
// &type=ws
// &hostSni=aws.xxx.cn&path=%2Fws-vless%3Fed%3D2560#local-clone
type vlessSub struct {
	remark       string
	addrWithPort string //eg node.cloudflare.cn:443 or node.cloudflare.cn:80
	UID          string
	path         string //eg /ws-vless?ed=2560
}

func (s vlessSub) vlessURL(hostSni string, isTLS bool) string {

	u := url.Values{
		"encryption":    {"none"},
		"allowInsecure": {"1"},
		"type":          {"ws"},
		"path":          {s.path},
	}
	if hostSni != "" {
		u["host"] = []string{hostSni}
		u["sni"] = []string{hostSni}
	}

	if !isTLS {
		u["security"] = []string{"none"}
		u.Del("sni")
	} else {
		u["security"] = []string{"tls"}
	}
	//&security=none&allowInsecure=1&type=ws&path=#n-cn1.unchainese.com%3A80
	return fmt.Sprintf("vless://%s@%s?%s#%s", s.UID, s.addrWithPort, u.Encode(), s.remark)
}

func (app *App) Sub(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	if app.IsUserNotAllowed(uid) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	subURLs := app.vlessUrls(uid)

	//json response hello world
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	lines := []string{
		app.cfg.GitHash,
		app.cfg.BuildTime,
		"VLESS Subscription URL:",
	}
	lines = append(lines, subURLs...)
	w.Write([]byte(strings.Join(lines, "\n\n")))
}

func (app *App) vlessUrls(uid string) []string {
	var subURLs []string
	for _, subAddr := range app.cfg.SubHostWithPort() {
		sub := vlessSub{
			remark:       subAddr,
			addrWithPort: subAddr,
			UID:          uid,
			path:         "/wsv/" + uid + "?ed=2560",
		}
		isTLS := strings.HasSuffix(subAddr, ":443")
		subURL := sub.vlessURL("", isTLS)
		subURLs = append(subURLs, subURL)
	}
	return subURLs
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
