// Package app is the pilot's interface: the shortest path through VeneraCat
// Core on NSGo, from installing a source to reading a chapter, with a browser
// login whose cookies Core reads and an episode the host's player plays.
package app

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	vc "github.com/laincat/nsgo-pilot/internal/core"
	"github.com/laincat/nsgo-pilot/internal/library"
	"github.com/laincat/veneracat-core/webcookie"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// sourceLevel is the Venera source API level sources compare minAppVersion
// with, the one Nekophage reports.
const sourceLevel = "1.6.3"

// officialIndex is the official comic source index.
const officialIndex = "https://raw.githubusercontent.com/venera-app/venera-configs/main/index.json"

// App is one Core profile and what the screens share.
type App struct {
	Core    *vc.Core
	Lib     *library.Library
	cookies webcookie.Options
	// changed bumps when the installed sources change.
	changed *core.Signal[int]
	// login carries a web view login's result to the autopilot: what it read,
	// or why it could not.
	login chan string
}

// open runs off the engine's goroutine: restoring the sources runs their
// scripts.
func open(dataDir, cacheDir string) (*App, error) {
	c, err := vc.Open(vc.Options{DataDir: filepath.Join(dataDir, "core"), Locale: "zh_CN", AppVersion: sourceLevel})
	if err != nil {
		return nil, err
	}
	lib := library.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	failed, err := lib.Restore(ctx)
	if err != nil {
		c.Close()
		return nil, err
	}
	for key, err := range failed {
		log.Printf("source %s was not restored: %v", key, err)
	}
	// NSGo's WebView2 views keep their profile in the app's cache folder.
	a := &App{Core: c, Lib: lib, cookies: webcookie.Options{WebView2Folder: filepath.Join(cacheDir, "WebView2")}, login: make(chan string, 1)}
	if os.Getenv("NSGO_PILOT_PROBE") != "" {
		if err := installProbe(ctx, a); err != nil {
			log.Printf("probe sources: %v", err)
		}
	}
	return a, nil
}

// Root opens Core and shows the app once it is ready.
func Root(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	app := core.NewSignalFunc[*App](cx, nil, nil)
	failure := core.NewSignal(cx, "")
	dataDir, cacheDir := cx.DataDir(), cx.CacheDir()
	go func() {
		a, err := open(dataDir, cacheDir)
		cx.Post(func() {
			if err != nil {
				failure.Set("Core 打开失败：" + err.Error())
				return
			}
			app.Set(a)
		})
	}()
	return core.View{
		Style:     core.Style{FlexGrow: 1, FlexShrink: 1},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Background()} },
		Children: []core.Node{core.Show{
			When: func() bool { return app.Get() != nil },
			Then: func(cx *core.Scope) core.Node { return shell(cx, app.Get()) },
			Else: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 12}, Children: []core.Node{
					core.Show{When: func() bool { return failure.Get() == "" }, Then: func(*core.Scope) core.Node { return ui.Spinner{Label: "正在打开 Core"} }},
					ui.Textf(failure.Get, 14, ui.Regular, colors.Muted),
				}}
			},
		}},
	}
}

// route is one page of the stack; the zero route is the tabs.
type route struct {
	Kind, Key, ID, Line, Episode, Title string
}

func shell(cx *core.Scope, a *App) core.Node {
	a.changed = core.NewSignal(cx, 0)
	nav := base.NewNav(cx, route{})
	tab := core.NewSignal(cx, 0)
	if os.Getenv("NSGO_PILOT_AUTO") != "" {
		go autopilot(cx, a, nav, tab)
	}
	return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
		back := func() { nav.Pop(1) }
		switch r.Kind {
		case "detail":
			return detailPage(cx, a, r, nav.Push, back)
		case "reader":
			return readerPage(cx, a, r, back)
		case "login":
			return loginPage(cx, a, r, back)
		case "video":
			return videoDetailPage(cx, a, r, nav.Push, back)
		case "play":
			return playPage(cx, a, r, back)
		}
		return homePage(cx, a, tab, nav.Push)
	})
}

func homePage(cx *core.Scope, a *App, tab *core.Signal[int], push func(route)) base.NavPage {
	panes := []func(*core.Scope, *App, func(route)) core.Node{sourcesTab, searchTab, videoTab}
	children := make([]core.Node, 0, len(panes)+1)
	for i, pane := range panes {
		// Every tab stays mounted, so switching keeps a search and its scroll.
		children = append(children, core.View{
			Style: core.Style{FlexGrow: 1, FlexShrink: 1},
			StyleFunc: func() core.Style {
				if tab.Get() != i {
					return core.Style{Display: core.DisplayNone}
				}
				return core.Style{}
			},
			Children: []core.Node{pane(cx, a, push)},
		})
	}
	children = append(children, tabBar(cx, tab))
	return ui.NavPage(cx, base.NavPage{
		TitleFunc: func() string { return tabs[tab.Get()].title },
		Children:  []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: children}},
	}, ui.NavHeader{})
}
