package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/laincat/veneracat-core/plugin"
	"github.com/laincat/veneracat-core/subscription"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

var installKinds = []struct {
	label string
	typ   plugin.Type
}{
	{"漫画源", plugin.TypeComic},
	{"视频源", plugin.TypeVideoJS},
	{"Kazumi 规则", plugin.TypeKazumi},
}

// sourcesTab installs sources from the official index or a script's address
// and lists what is installed, with a browser login for comic sources.
func sourcesTab(cx *core.Scope, a *App, push func(route)) core.Node {
	colors := ui.ColorsOf(cx)
	address := core.NewSignal(cx, officialIndex)
	kind := core.NewSignal(cx, 0)
	status := core.NewSignal(cx, "")
	busy := core.NewSignal(cx, false)
	entries := core.NewSignalFunc[[]subscription.Row](cx, nil, nil)
	indexURL := ""

	run := func(what string, f func(ctx context.Context) (string, error)) {
		busy.Set(true)
		status.Set(what)
		load(cx, func(ctx context.Context) (string, error) {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			return f(ctx)
		}, func(done string, err error) {
			busy.Set(false)
			if err != nil {
				status.Set(err.Error())
				return
			}
			status.Set(done)
			a.changed.Set(a.changed.Get() + 1)
		})
	}
	readIndex := func() {
		u := strings.TrimSpace(address.Get())
		run("正在读取订阅…", func(ctx context.Context) (string, error) {
			_, rows, err := a.Lib.Index(ctx, u)
			if err != nil {
				return "", err
			}
			cx.Post(func() { indexURL = u; entries.Set(rows) })
			return fmt.Sprintf("订阅里有 %d 个源", len(rows)), nil
		})
	}
	installURL := func() {
		u := strings.TrimSpace(address.Get())
		typ := installKinds[kind.Get()].typ
		run("正在安装…", func(ctx context.Context) (string, error) {
			rec, err := a.Lib.InstallTypedURL(ctx, typ, u)
			if err != nil {
				return "", err
			}
			return "已安装 " + rec.Name, nil
		})
	}
	installEntry := func(e subscription.Entry) {
		run("正在安装 "+e.Name+"…", func(ctx context.Context) (string, error) {
			rec, err := a.Lib.InstallEntry(ctx, e, indexURL)
			if err != nil {
				return "", err
			}
			return "已安装 " + rec.Name, nil
		})
	}

	kindButtons := make([]core.Node, len(installKinds))
	for i, k := range installKinds {
		kindButtons[i] = core.Dynamic(func(*core.Scope) core.Node {
			v := ui.ButtonGhost
			if kind.Get() == i {
				v = ui.ButtonSecondary
			}
			return ui.Button{Text: k.label, Variant: v, OnPress: func() { kind.Set(i) }}
		})
	}
	disabled := func() bool { return busy.Get() }

	installed := func() []plugin.Record {
		a.changed.Get()
		return a.Lib.Sources()
	}
	return core.Scroll{Style: core.Style{FlexGrow: 1}, Children: []core.Node{core.View{Style: core.Style{Gap: 4, Padding: core.Edges{Bottom: barSpace}}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.All(16), Gap: 10}, Children: []core.Node{
			ui.Input{Value: address.Get, OnChange: address.Set, Placeholder: "订阅 index.json 或源文件的地址", NoAutoCorrect: true, AutoCapitalize: core.AutoCapitalizeNone},
			core.View{Style: core.Style{Direction: core.Row, Gap: 8, Wrap: core.Wrap}, Children: kindButtons},
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Button{Text: "读取订阅", Disabled: disabled, OnPress: readIndex},
				ui.Button{Text: "按地址安装", Disabled: disabled, OnPress: installURL},
			}},
		}},
		message(cx, status.Get),
		heading(cx, "已安装"),
		core.Dynamic(func(cx *core.Scope) core.Node {
			recs := installed()
			if len(recs) == 0 {
				return message(cx, func() string { return "还没有源" })
			}
			rows := make([]core.Node, 0, len(recs))
			for _, rec := range recs {
				var trailing core.Node = core.View{}
				if rec.Type == plugin.TypeComic {
					trailing = ui.Button{Text: "网页登录", Variant: ui.ButtonGhost, OnPress: func() {
						push(route{Kind: "login", Key: rec.Key, Title: rec.Name})
					}}
				}
				rows = append(rows, row(cx, nil,
					column(2,
						ui.Text(rec.Name, 15, ui.Medium, colors.Text),
						ui.Text(fmt.Sprintf("%s · %s · %s", rec.Key, rec.Type, rec.Version), 12, ui.Regular, colors.Muted),
					),
					trailing,
				))
			}
			return core.View{Children: rows}
		}),
		core.Show{When: func() bool { return len(entries.Get()) > 0 }, Then: func(cx *core.Scope) core.Node {
			rows := []core.Node{heading(cx, "订阅中的源")}
			for _, e := range entries.Get() {
				rows = append(rows, row(cx, nil,
					column(2,
						ui.Text(e.Name, 15, ui.Medium, colors.Text),
						ui.Text(fmt.Sprintf("%s · %s · %s", e.Key, e.Version, e.Status), 12, ui.Regular, colors.Muted),
					),
					ui.Button{Text: "安装", Variant: ui.ButtonGhost, Disabled: disabled, OnPress: func() { installEntry(e.Entry) }},
				))
			}
			return core.View{Style: core.Style{Gap: 4}, Children: rows}
		}},
	}}}}
}
