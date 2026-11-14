package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	vc "github.com/laincat/nsgo-pilot/internal/core"
	"github.com/laincat/veneracat-core/webcookie"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// loginPage logs a source in on its own page. After each navigation Core runs
// the source's checkStatus; once it agrees, Core reads the web view's cookies
// for the login page and the page it ended on, stores them in the source's
// partition, and runs onLoginSuccess.
func loginPage(cx *core.Scope, a *App, r route, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	decl := core.NewSignalFunc(cx, vc.WebviewLogin{}, nil)
	status := core.NewSignal(cx, "正在读取登录方式…")
	current := core.NewSignal(cx, "")
	finished := false

	load(cx, func(ctx context.Context) (vc.WebviewLogin, error) { return a.Core.AccountWebviewLogin(ctx, r.Key) }, func(d vc.WebviewLogin, err error) {
		switch {
		case err != nil:
			status.Set("读取失败：" + err.Error())
		case d.URL == "" || d.Handle == "":
			status.Set("这个源没有网页登录")
		default:
			decl.Set(d)
			status.Set("请在页面里登录")
		}
	})

	complete := func(d vc.WebviewLogin, pageURL string) {
		status.Set("源确认已登录，正在读取 Cookie…")
		load(cx, func(ctx context.Context) (string, error) {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			seen := map[string]bool{}
			httpOnly := 0
			for _, u := range distinct(d.URL, pageURL) {
				cookies, err := webcookie.Read(ctx, u, a.cookies)
				log.Printf("pilot: webcookie %s: %d cookies, %v", u, len(cookies), err)
				if err != nil {
					return "", err
				}
				a.Core.Jar().Set(vc.ComicPartition(r.Key), u, cookies)
				for _, c := range cookies {
					id := c.Name + ";" + c.Domain + ";" + c.Path
					if !seen[id] && c.HttpOnly {
						httpOnly++
					}
					seen[id] = true
				}
			}
			total := len(seen)
			if err := a.Core.WebviewLoginDone(ctx, r.Key, d.Handle); err != nil {
				return "", fmt.Errorf("Cookie 已保存 %d 个，但 onLoginSuccess 失败：%w", total, err)
			}
			return fmt.Sprintf("登录完成：Core 读到并保存了 %d 个 Cookie，其中 HttpOnly %d 个", total, httpOnly), nil
		}, func(done string, err error) {
			if err != nil {
				log.Printf("pilot: login %s failed: %v", r.Key, err)
				a.logged(err.Error())
				status.Set(err.Error())
				return
			}
			log.Printf("pilot: login %s: %s", r.Key, done)
			a.logged("ok")
			status.Set(done)
		})
	}
	navigated := func(nav core.WebNavigation) {
		current.Set(nav.URL)
		log.Printf("pilot: login page %q title %q loading %v", nav.URL, nav.Title, nav.Loading)
		d := decl.Get()
		if finished || d.Handle == "" || nav.Loading {
			return
		}
		load(cx, func(ctx context.Context) (bool, error) {
			return a.Core.WebviewStatus(ctx, r.Key, d.Handle, nav.URL, nav.Title)
		}, func(ok bool, err error) {
			if err != nil {
				status.Set("checkStatus 失败：" + err.Error())
				return
			}
			log.Printf("pilot: checkStatus %q: %v", nav.Title, ok)
			if ok && !finished {
				finished = true
				complete(d, nav.URL)
			}
		})
	}
	// inspect shows what Core reads for the current page, names only.
	inspect := func() {
		u := current.Get()
		if u == "" {
			return
		}
		load(cx, func(ctx context.Context) ([]*http.Cookie, error) {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			return webcookie.Read(ctx, u, a.cookies)
		}, func(cookies []*http.Cookie, err error) {
			if err != nil {
				status.Set("读取 Cookie 失败：" + err.Error())
				return
			}
			names := make([]string, 0, len(cookies))
			for _, c := range cookies {
				n := c.Name
				if c.HttpOnly {
					n += "(HttpOnly)"
				}
				names = append(names, n)
			}
			sort.Strings(names)
			status.Set(fmt.Sprintf("Core 为当前页读到 %d 个 Cookie：%s", len(cookies), strings.Join(names, "、")))
		})
	}

	return ui.NavPage(cx, base.NavPage{
		Title: "登录 " + r.Title,
		Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 16)}, Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{ui.Textf(status.Get, 13, ui.Regular, colors.Muted)}},
				ui.Button{Text: "读取 Cookie", Variant: ui.ButtonGhost, OnPress: inspect},
			}},
			core.Show{When: func() bool { return decl.Get().URL != "" }, Then: func(*core.Scope) core.Node {
				return core.WebView{
					Source:                  core.WebSource{Src: decl.Get().URL},
					InjectedJavaScript:      pageScript,
					OnMessage:               func(data string) { pageLoaded(data, navigated) },
					OnNavigationStateChange: navigated,
					OnLoad:                  navigated,
					Style:                   core.Style{FlexGrow: 1},
				}
			}},
		}}},
	}, ui.NavHeader{OnBack: back, BackLabel: "返回"})
}

// pageScript posts each loaded page's address and title. On iOS, WKWebView
// can still have no title when NSGo reports the load, and a source's
// checkStatus often decides by the title.
const pageScript = "window.nsgo.postMessage(JSON.stringify({pilotPage: location.href, title: document.title}))"

// pageLoaded hands what pageScript posted to navigated.
func pageLoaded(data string, navigated func(core.WebNavigation)) {
	var page struct {
		URL   string `json:"pilotPage"`
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(data), &page) == nil && page.URL != "" {
		navigated(core.WebNavigation{URL: page.URL, Title: page.Title})
	}
}

// logged reports a web view login's result to the autopilot, if it is
// running; a second report is dropped rather than waited on.
func (a *App) logged(result string) {
	select {
	case a.login <- result:
	default:
	}
}

// distinct returns the addresses, each once, without an empty one.
func distinct(addresses ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range addresses {
		u, err := url.Parse(a)
		if a == "" || err != nil || seen[u.Scheme+"://"+u.Host+u.Path] {
			continue
		}
		seen[u.Scheme+"://"+u.Host+u.Path] = true
		out = append(out, a)
	}
	return out
}
