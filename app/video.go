package app

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/laincat/veneracat-core/plugin"
	"github.com/laincat/veneracat-core/video"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// PlayerComponent is the native component the iOS host registers for
// AetherEngine. Its props are url, format, headers (name to value) and title;
// it emits "state" with a string and "error" with a message.
const PlayerComponent = "aether.player"

func isVideo(r plugin.Record) bool {
	return r.Type == plugin.TypeVideoJS || r.Type == plugin.TypeKazumi || r.Type == plugin.TypeBuiltin
}

// videoTab searches one video source and opens a result's lines.
func videoTab(cx *core.Scope, a *App, push func(route)) core.Node {
	colors := ui.ColorsOf(cx)
	source := core.NewSignal(cx, "")
	keyword := core.NewSignal(cx, "")
	results := core.NewSignalFunc[[]video.Item](cx, nil, nil)
	status := core.NewSignal(cx, "")
	searched := ""
	search := func(string) {
		key, kw := pick(a, source, isVideo), strings.TrimSpace(keyword.Get())
		if key == "" || kw == "" {
			return
		}
		status.Set("正在搜索…")
		load(cx, func(ctx context.Context) (*video.Page, error) { return a.Core.VideoSearch(ctx, key, kw, "") }, func(p *video.Page, err error) {
			if err != nil {
				status.Set("搜索失败：" + err.Error())
				return
			}
			searched = key
			results.Set(p.Items)
			status.Set(fmt.Sprintf("%d 条结果", len(p.Items)))
		})
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8, Padding: core.Edges{Top: 12}}, Children: []core.Node{
		sourcePicker(cx, a, source, isVideo),
		core.View{Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(0, 16)}, Children: []core.Node{
			ui.Input{Value: keyword.Get, OnChange: keyword.Set, OnSubmit: search, Placeholder: "搜索视频", Style: core.Style{FlexGrow: 1}},
			ui.Button{Text: "搜索", OnPress: func() { search("") }},
		}},
		message(cx, status.Get),
		core.List{
			Count:         func() int { return len(results.Get()) },
			Key:           func(i int) any { return results.Get()[i].ID },
			EstimatedSize: 64,
			Style:         core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)},
			Row: func(cx *core.Scope, r core.ListRow) core.Node {
				return core.Dynamic(func(cx *core.Scope) core.Node {
					list, i := results.Get(), r.Index()
					if i >= len(list) {
						return core.View{}
					}
					it := list[i]
					key := searched
					return row(cx, func() { push(route{Kind: "video", Key: key, ID: it.ID, Title: it.Title}) },
						column(2, ui.Text(it.Title, 15, ui.Medium, colors.Text), ui.Text(it.Subtitle, 12, ui.Regular, colors.Muted)))
				})
			},
		},
	}}
}

type episodeRow struct {
	line, lineName string
	video.Episode
}

// videoDetailPage lists every line's episodes.
func videoDetailPage(cx *core.Scope, a *App, r route, push func(route), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	episodes := core.NewSignalFunc[[]episodeRow](cx, nil, nil)
	status := core.NewSignal(cx, "正在读取剧集…")
	load(cx, func(ctx context.Context) (*video.Details, error) { return a.Core.VideoDetails(ctx, r.Key, r.ID) }, func(d *video.Details, err error) {
		if err != nil {
			status.Set("读取失败：" + err.Error())
			return
		}
		var rows []episodeRow
		for _, l := range d.Lines {
			for _, e := range l.Episodes {
				rows = append(rows, episodeRow{line: l.ID, lineName: l.Name, Episode: e})
			}
		}
		episodes.Set(rows)
		status.Set(fmt.Sprintf("%d 条线路，%d 集", len(d.Lines), len(rows)))
	})
	return page(cx, func() string { return r.Title }, back,
		message(cx, status.Get),
		core.List{
			Count:         func() int { return len(episodes.Get()) },
			Key:           func(i int) any { e := episodes.Get()[i]; return e.line + "/" + e.ID },
			EstimatedSize: 48,
			Style:         core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)},
			Row: func(cx *core.Scope, lr core.ListRow) core.Node {
				return core.Dynamic(func(cx *core.Scope) core.Node {
					list, i := episodes.Get(), lr.Index()
					if i >= len(list) {
						return core.View{}
					}
					e := list[i]
					return row(cx, func() {
						push(route{Kind: "play", Key: r.Key, ID: r.ID, Line: e.line, Episode: e.ID, Title: r.Title + " " + e.Title})
					}, ui.Text(strings.TrimSpace(e.lineName+" "+e.Title), 15, ui.Regular, colors.Text))
				})
			},
		},
	)
}

// playPage asks Core where the episode plays and hands that to the host's
// player, AetherEngine on iOS.
func playPage(cx *core.Scope, a *App, r route, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	play := core.NewSignalFunc[*video.Play](cx, nil, nil)
	status := core.NewSignal(cx, "正在取播放地址…")
	load(cx, func(ctx context.Context) (*video.Play, error) {
		p, err := a.Core.VideoPlay(ctx, r.Key, r.ID, r.Line, r.Episode)
		if err == nil && a.probe != nil && r.Key == "probe_video" && runtime.GOOS != "ios" {
			err = verifyProbeMedia(ctx, a, p.URL)
		}
		return p, err
	}, func(p *video.Play, err error) {
		if a.probe != nil && r.Key == "probe_video" && (err != nil || runtime.GOOS != "ios") {
			if err == nil {
				log.Print("pilot: HLS playlists and media byte range fetched")
			}
			reportProbe(a.probe.playback, err)
		}
		if err == nil {
			log.Printf("pilot: play %s/%s: %s %s", r.Key, r.Episode, p.Format, p.URL)
		}
		switch {
		case err != nil:
			log.Printf("pilot: play %s/%s failed: %v", r.Key, r.Episode, err)
			status.Set("取播放地址失败：" + err.Error())
		case p.Encrypted:
			status.Set("这个媒体是加密的，播放器不能直接播放")
		case p.URL == "":
			status.Set("这一集要在网页里嗅探地址，试点没有接嗅探")
		default:
			play.Set(p)
			status.Set(string(p.Format) + " " + p.URL)
		}
	})
	// Below the header: the page has no list to inset its content.
	return page(cx, func() string { return r.Title }, back,
		core.View{Style: core.Style{PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea}}},
		core.Show{When: func() bool { return play.Get() != nil }, Then: func(*core.Scope) core.Node {
			if runtime.GOOS != "ios" {
				return message(cx, func() string {
					return "AetherEngine 只在 iOS 外壳里注册；这里只显示 Core 给出的地址。"
				})
			}
			p := play.Get()
			headers := map[string]any{}
			for name := range p.Header {
				headers[name] = p.Header.Get(name)
			}
			return core.Native{
				Component: PlayerComponent,
				Props:     map[string]any{"url": p.URL, "format": string(p.Format), "headers": headers, "title": r.Title},
				Style:     core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9},
				OnEvent: func(name string, data any) {
					log.Printf("pilot: player %s: %v", name, data)
					if a.probe != nil && r.Key == "probe_video" {
						switch name {
						case "progress":
							reportProbe(a.probe.playback, nil)
						case "error":
							reportProbe(a.probe.playback, fmt.Errorf("native player: %v", data))
						}
					}
					status.Set(fmt.Sprintf("播放器 %s：%v", name, data))
				},
			}
		}},
		core.View{Style: core.Style{Padding: core.All(16)}, Children: []core.Node{ui.Textf(status.Get, 13, ui.Regular, colors.Muted)}},
	)
}
