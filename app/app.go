// Package app is the pilot's interface: the shortest path through VeneraCat
// Core on NSGo, from installing a source to reading a chapter, with a browser
// login whose cookies Core reads and an episode the host's player plays.
package app

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	vc "github.com/laincat/veneracat-core/gohost"
	"github.com/laincat/veneracat-core/gohost/library"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// sourceLevel is the Venera source API level sources compare minAppVersion
// with, the one Nekophage reports.
const sourceLevel = "1.6.3"

// officialIndex is the official comic source index.
const officialIndex = "https://raw.githubusercontent.com/venera-app/venera-configs/main/index.json"

// App is one Core profile shared by its screens.
type App struct {
	Core        *vc.Core
	Lib         *library.Library
	changed     *core.Signal[int]
	loginActive bool
	probe       *probeSession
	closeOnce   sync.Once
}

func open(ctx context.Context, dataDir string, fixture bool) (*App, error) {
	a := &App{}
	if fixture {
		root, err := os.MkdirTemp("", "nsgo-pilot-probe-")
		if err != nil {
			return nil, err
		}
		dataDir = root
		a.probe = &probeSession{root: root, login: make(chan error, 1), details: make(chan error, 1), reader: make(chan error, 1), playback: make(chan error, 1)}
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	c, err := vc.Open(vc.Options{DataDir: filepath.Join(dataDir, "core"), Platform: platform, Locale: "zh_CN", AppVersion: sourceLevel})
	if err != nil {
		a.close()
		return nil, err
	}
	a.Core, a.Lib = c, library.New(c)
	startup, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	failed, err := a.Lib.Restore(startup)
	if err == nil && a.probe != nil {
		err = installProbe(startup, a)
	}
	if err == nil {
		err = startup.Err()
	}
	if err != nil {
		a.close()
		return nil, err
	}
	for key, err := range failed {
		log.Printf("source %s was not restored: %v", key, err)
	}
	context.AfterFunc(ctx, a.close)
	return a, nil
}

func (a *App) close() {
	a.closeOnce.Do(func() {
		if a.probe != nil && a.probe.server != nil {
			_ = a.probe.server.Close()
		}
		if a.Core != nil {
			_ = a.Core.Close()
		}
		if a.probe != nil {
			_ = os.RemoveAll(a.probe.root)
		}
	})
}

// Root opens Core and shows the app once it is ready.
func Root(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	app := core.NewSignalFunc[*App](cx, nil, nil)
	failure := core.NewSignal(cx, "")
	ctx, dataDir := cx.Context(), cx.DataDir()
	fixture := os.Getenv("NSGO_PILOT_PROBE") != "" || os.Getenv("NSGO_PILOT_AUTO") != ""
	go func() {
		a, err := open(ctx, dataDir, fixture)
		cx.Post(func() {
			if err != nil {
				failure.Set("Core 打开失败：" + err.Error())
				return
			}
			cx.OnQuit(func(context.Context) { a.close() })
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
	if a.probe != nil && os.Getenv("NSGO_PILOT_AUTO") != "" {
		go autopilot(cx.Context(), cx, a, nav)
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
