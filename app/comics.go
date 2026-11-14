package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	vc "github.com/laincat/nsgo-pilot/internal/core"
	"github.com/laincat/veneracat-core/plugin"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// pick is the chosen source, else the first installed one of the kind.
func pick(a *App, chosen *core.Signal[string], want func(plugin.Record) bool) string {
	a.changed.Get()
	first := ""
	for _, rec := range a.Lib.Sources() {
		if !want(rec) {
			continue
		}
		if rec.Key == chosen.Get() {
			return rec.Key
		}
		if first == "" {
			first = rec.Key
		}
	}
	return first
}

// sourcePicker is a row of the installed sources of one kind, one chosen.
func sourcePicker(cx *core.Scope, a *App, chosen *core.Signal[string], want func(plugin.Record) bool) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		current := pick(a, chosen, want)
		var buttons []core.Node
		for _, rec := range a.Lib.Sources() {
			if !want(rec) {
				continue
			}
			v := ui.ButtonGhost
			if rec.Key == current {
				v = ui.ButtonSecondary
			}
			buttons = append(buttons, ui.Button{Text: rec.Name, Variant: v, OnPress: func() { chosen.Set(rec.Key) }})
		}
		if len(buttons) == 0 {
			return message(cx, func() string { return "先在「源」里安装一个源" })
		}
		return core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{Height: core.Pt(40)}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(0, 16)}, Children: buttons},
		}}
	})
}

func isComic(r plugin.Record) bool { return r.Type == plugin.TypeComic }

// searchTab searches one comic source and opens a result's details.
func searchTab(cx *core.Scope, a *App, push func(route)) core.Node {
	colors := ui.ColorsOf(cx)
	source := core.NewSignal(cx, "")
	keyword := core.NewSignal(cx, "")
	results := core.NewSignalFunc[[]vc.Comic](cx, nil, nil)
	status := core.NewSignal(cx, "")
	searched := ""

	search := func(string) {
		key, kw := pick(a, source, isComic), strings.TrimSpace(keyword.Get())
		if key == "" || kw == "" {
			return
		}
		status.Set("正在搜索…")
		load(cx, func(ctx context.Context) (vc.Page, error) {
			options, err := a.Core.DefaultOptions(ctx, key)
			if err != nil {
				options = nil
			}
			return a.Core.Search(ctx, key, kw, options, 1)
		}, func(p vc.Page, err error) {
			if err != nil {
				status.Set("搜索失败：" + err.Error())
				return
			}
			searched = key
			results.Set(p.Comics)
			status.Set(fmt.Sprintf("%d 条结果", len(p.Comics)))
		})
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8, Padding: core.Edges{Top: 12}}, Children: []core.Node{
		sourcePicker(cx, a, source, isComic),
		core.View{Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(0, 16)}, Children: []core.Node{
			ui.Input{Value: keyword.Get, OnChange: keyword.Set, OnSubmit: search, Placeholder: "搜索漫画", ReturnKeyType: core.ReturnKeySearch, Style: core.Style{FlexGrow: 1}},
			ui.Button{Text: "搜索", OnPress: func() { search("") }},
		}},
		message(cx, status.Get),
		core.List{
			Count:         func() int { return len(results.Get()) },
			Key:           func(i int) any { return results.Get()[i].ID },
			EstimatedSize: 104,
			Style:         core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)},
			Footer:        core.View{Style: core.Style{Height: core.Pt(barSpace)}},
			Row: func(cx *core.Scope, r core.ListRow) core.Node {
				return core.Dynamic(func(cx *core.Scope) core.Node {
					list, i := results.Get(), r.Index()
					if i >= len(list) {
						return core.View{}
					}
					c := list[i]
					key := searched
					return row(cx, func() { push(route{Kind: "detail", Key: key, ID: c.ID, Title: c.Title}) },
						cover(cx, func(ctx context.Context) ([]byte, string, error) {
							return a.Core.FetchThumbnail(ctx, key, c.Cover, c.ID)
						}, 60, 84),
						column(4,
							ui.Text(c.Title, 15, ui.Medium, colors.Text),
							ui.Text(c.Subtitle, 12, ui.Regular, colors.Muted),
							ui.Text(strings.Join(c.Tags, " · "), 12, ui.Regular, colors.Faint),
						),
					)
				})
			},
		},
	}}
}

// detailPage shows a comic's details and its chapters.
func detailPage(cx *core.Scope, a *App, r route, push func(route), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	details := core.NewSignalFunc(cx, vc.Details{}, nil)
	status := core.NewSignal(cx, "正在读取详情…")
	load(cx, func(ctx context.Context) (vc.Details, error) { return a.Core.Details(ctx, r.Key, r.ID) }, func(d vc.Details, err error) {
		if err != nil {
			log.Printf("pilot: details %s/%s failed: %v", r.Key, r.ID, err)
			status.Set("详情读取失败：" + err.Error())
			return
		}
		log.Printf("pilot: details %s/%s: %q, %d chapters", r.Key, r.ID, d.Title, len(d.Chapters))
		details.Set(d)
		status.Set("")
	})
	title := func() string {
		if t := details.Get().Title; t != "" {
			return t
		}
		return r.Title
	}
	return page(cx, title, back,
		message(cx, status.Get),
		core.List{
			Count:         func() int { return len(details.Get().Chapters) },
			Key:           func(i int) any { return details.Get().Chapters[i].ID },
			EstimatedSize: 48,
			Style:         core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)},
			Header: core.Dynamic(func(cx *core.Scope) core.Node {
				d := details.Get()
				if d.Title == "" {
					return core.View{}
				}
				return core.View{Style: core.Style{Direction: core.Row, Gap: 16, Padding: core.All(16)}, Children: []core.Node{
					cover(cx, func(ctx context.Context) ([]byte, string, error) {
						return a.Core.FetchThumbnail(ctx, r.Key, d.Cover, r.ID)
					}, 120, 168),
					column(6,
						ui.Text(d.Title, 18, ui.Semibold, colors.Text),
						ui.Text(d.Subtitle, 13, ui.Regular, colors.Muted),
						ui.Text(d.Description, 13, ui.Regular, colors.Muted),
						ui.Text(fmt.Sprintf("%d 话", len(d.Chapters)), 12, ui.Regular, colors.Faint),
					),
				}}
			}),
			Row: func(cx *core.Scope, lr core.ListRow) core.Node {
				return core.Dynamic(func(cx *core.Scope) core.Node {
					chapters, i := details.Get().Chapters, lr.Index()
					if i >= len(chapters) {
						return core.View{}
					}
					ch := chapters[i]
					label := ch.Title
					if ch.Group != "" {
						label = ch.Group + " · " + label
					}
					return row(cx, func() { push(route{Kind: "reader", Key: r.Key, ID: r.ID, Episode: ch.ID, Title: ch.Title}) },
						ui.Text(label, 15, ui.Regular, colors.Text))
				})
			},
		},
	)
}

// readerPage shows a chapter's pages in one column, each fetched through Core
// as it comes near the screen and sized by its own pixels.
func readerPage(cx *core.Scope, a *App, r route, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	pages := core.NewSignalFunc[[]string](cx, nil, nil)
	status := core.NewSignal(cx, "正在读取章节…")
	load(cx, func(ctx context.Context) ([]string, error) { return a.Core.ChapterImages(ctx, r.Key, r.ID, r.Episode) }, func(p []string, err error) {
		if err != nil {
			log.Printf("pilot: chapter %s/%s failed: %v", r.Key, r.Episode, err)
			status.Set("章节读取失败：" + err.Error())
			return
		}
		log.Printf("pilot: chapter %s/%s: %d pages", r.Key, r.Episode, len(p))
		pages.Set(p)
		status.Set("")
	})
	return page(cx, func() string { return r.Title }, back,
		message(cx, status.Get),
		core.List{
			Count:         func() int { return len(pages.Get()) },
			Key:           func(i int) any { return pages.Get()[i] },
			EstimatedSize: 900,
			Style:         core.Style{FlexGrow: 1, FlexBasis: core.Pt(0)},
			Row: func(cx *core.Scope, lr core.ListRow) core.Node {
				return core.Dynamic(func(cx *core.Scope) core.Node {
					list, index := pages.Get(), lr.Index()
					if index >= len(list) {
						return core.View{}
					}
					address := list[index]
					pic := core.NewSignalFunc(cx, picture{}, nil)
					failure := core.NewSignal(cx, "")
					load(cx, func(ctx context.Context) (picture, error) {
						return fetchPicture(ctx, func(ctx context.Context) ([]byte, string, error) {
							return a.Core.FetchImage(ctx, r.Key, address, r.ID, r.Episode)
						})
					}, func(p picture, err error) {
						if err != nil {
							log.Printf("pilot: page %d failed: %v", index+1, err)
							failure.Set(fmt.Sprintf("第 %d 页加载失败：%v", index+1, err))
							return
						}
						log.Printf("pilot: page %d decoded %vx%v", index+1, p.size.Width, p.size.Height)
						pic.Set(p)
					})
					return core.Show{
						When: func() bool { return pic.Get().data != nil },
						Then: func(*core.Scope) core.Node {
							p := pic.Get()
							return core.Image{Source: core.ImageSource{Data: p.data, Size: p.size}, Style: core.Style{Width: core.Pct(100)}, Label: fmt.Sprintf("第 %d 页", index+1)}
						},
						Else: func(*core.Scope) core.Node {
							return core.View{Style: core.Style{Height: core.Pt(600), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, Children: []core.Node{
								ui.Textf(func() string {
									if f := failure.Get(); f != "" {
										return f
									}
									return fmt.Sprintf("第 %d 页", index+1)
								}, 13, ui.Regular, colors.Muted),
							}}
						},
					}
				})
			},
		},
	)
}
