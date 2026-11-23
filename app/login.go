package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	vc "github.com/laincat/veneracat-core/gohost"
	"github.com/laincat/veneracat-core/host"
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
	var latest core.WebNavigation
	var attempt context.CancelFunc
	var generation uint64
	finished := false
	reader := readBrowserCookies(cx)
	fixture := a.probe != nil && r.Key == "probe"
	if fixture {
		read := reader
		reader = func(ctx context.Context, raw string) ([]*http.Cookie, error) {
			rows, err := read(ctx, raw)
			return slices.DeleteFunc(rows, func(row *http.Cookie) bool { return row.Name != a.probe.cookieName }), err
		}
	}
	ctx := cx.Context()
	if a.loginActive {
		status.Set(errLoginAlreadyOpen.Error())
	} else {
		a.loginActive = true
		cx.Cleanup(func() { a.loginActive = false })
		load(cx, func(ctx context.Context) (vc.WebviewLogin, error) { return a.Core.AccountWebviewLogin(ctx, r.Key) }, func(d vc.WebviewLogin, err error) {
			switch {
			case err != nil:
				status.Set("读取失败：" + err.Error())
				a.reportLogin(r.Key, err)
			case d.URL == "" || d.Handle == "":
				status.Set("这个源没有网页登录")
			default:
				ready := func(err error) {
					if err != nil {
						status.Set("浏览器登录状态清理失败：" + err.Error())
						a.reportLogin(r.Key, err)
						return
					}
					decl.Set(d)
					status.Set("请在页面里登录")
				}
				if fixture {
					ready(nil)
				} else {
					cx.ClearWebsiteData(ready)
				}
			}
		})
	}

	navigated := func(nav core.WebNavigation) {
		if finished || (nav.URL == latest.URL && nav.Title == latest.Title && nav.Loading == latest.Loading) {
			return
		}
		latest = nav
		current.Set(nav.URL)
		generation++
		if attempt != nil {
			attempt()
		}
		d := decl.Get()
		if d.Handle == "" || nav.Loading || host.ValidateURL(nav.URL) != nil {
			return
		}
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		attempt = cancel
		ticket := generation
		go func() {
			defer cancel()
			ok, err := a.Core.WebviewStatus(checkCtx, r.Key, d.Handle, nav.URL, nav.Title)
			if !ok && err == nil {
				return
			}
			var result loginResult
			if err == nil {
				result, err = finishLogin(checkCtx, reader, []string{d.URL, nav.URL}, func(rows []*http.Cookie) error {
					return a.Core.Jar().Import(vc.ComicPartition(r.Key), rows)
				}, func(ctx context.Context) error { return a.Core.WebviewLoginDone(ctx, r.Key, d.Handle) }, 150*time.Millisecond)
			}
			cx.Post(func() {
				if ticket != generation || ctx.Err() != nil {
					return
				}
				finished = result.saved
				if err != nil {
					status.Set(err.Error())
					a.reportLogin(r.Key, err)
					return
				}
				message := fmt.Sprintf("登录完成：Core 读到并保存了 %d 个 Cookie，其中 HttpOnly %d 个", result.total, result.httpOnly)
				if fixture {
					cx.SetWebCookies([]core.WebCookie{{Name: a.probe.cookieName, Domain: "127.0.0.1", Path: "/", Expires: time.Now().Add(-time.Minute)}}, nil)
				}
				log.Printf("pilot: login %s: %s", r.Key, message)
				status.Set(message)
				a.reportLogin(r.Key, nil)
			})
		}()
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
			return reader(ctx, u)
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
					MessageOrigins:          []string{"*"},
					OnMessage:               func(data string) { pageLoaded(data, latest, navigated) },
					OnLoadStart:             navigated,
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

func pageLoaded(data string, latest core.WebNavigation, navigated func(core.WebNavigation)) {
	var page struct {
		URL   string `json:"pilotPage"`
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(data), &page) == nil && page.URL == latest.URL && !latest.Loading && page.Title != "" {
		latest.Title = page.Title
		navigated(latest)
	}
}

func (a *App) reportLogin(key string, err error) {
	if a.probe != nil && key == "probe" {
		reportProbe(a.probe.login, err)
	}
}
