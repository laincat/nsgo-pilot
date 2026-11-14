package app

import (
	"bytes"
	"context"
	"image"
	_ "image/gif"  // pages and covers come in these formats
	_ "image/jpeg" //
	_ "image/png"  //

	_ "golang.org/x/image/webp" //

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// fetches bounds the pictures loading at once: a chapter asks for every page
// near the screen together.
var fetches = make(chan struct{}, 6)

// load runs f off the engine and hands its result to done on it, unless the
// page that asked has gone.
func load[T any](cx *core.Scope, f func(ctx context.Context) (T, error), done func(T, error)) {
	ctx := cx.Context()
	go func() {
		v, err := f(ctx)
		if ctx.Err() != nil {
			return
		}
		cx.Post(func() { done(v, err) })
	}()
}

// fetchPicture loads one picture through Core, which applies the source's
// headers, cookies and image callbacks, and reads its size.
func fetchPicture(ctx context.Context, get func(context.Context) ([]byte, string, error)) (picture, error) {
	select {
	case fetches <- struct{}{}:
	case <-ctx.Done():
		return picture{}, ctx.Err()
	}
	defer func() { <-fetches }()
	data, _, err := get(ctx)
	if err != nil {
		return picture{}, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return picture{}, err
	}
	return picture{data: data, size: core.Size{Width: core.Points(cfg.Width), Height: core.Points(cfg.Height)}}, nil
}

type picture struct {
	data []byte
	size core.Size
}

// cover is a fixed box that fills with a picture once it loads.
func cover(cx *core.Scope, get func(context.Context) ([]byte, string, error), w, h core.Points) core.Node {
	colors := ui.ColorsOf(cx)
	pic := core.NewSignalFunc(cx, picture{}, nil)
	load(cx, func(ctx context.Context) (picture, error) { return fetchPicture(ctx, get) }, func(p picture, err error) {
		if err == nil {
			pic.Set(p)
		}
	})
	return core.View{
		Style:     core.Style{Width: core.Pt(w), Height: core.Pt(h), CornerRadius: 8, Overflow: core.Hidden},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} },
		Children: []core.Node{core.Show{
			When: func() bool { return pic.Get().data != nil },
			Then: func(*core.Scope) core.Node {
				return core.Image{
					Source:     core.ImageSource{Data: pic.Get().data, Size: core.Size{Width: w, Height: h}},
					ResizeMode: core.Cover,
					Style:      core.Style{Width: core.Pt(w), Height: core.Pt(h)},
				}
			},
		}},
	}
}

// page is a pushed page with a back button.
func page(cx *core.Scope, title func() string, back func(), children ...core.Node) base.NavPage {
	return ui.NavPage(cx, base.NavPage{
		TitleFunc: title,
		Children:  []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: children}},
	}, ui.NavHeader{OnBack: back, BackLabel: "返回"})
}

// row is a pressable line of a list.
func row(cx *core.Scope, onPress func(), children ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	var press func(core.PressEvent)
	if onPress != nil {
		press = func(core.PressEvent) { onPress() }
	}
	return core.View{
		OnPress:   press,
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(10, 16), Borders: core.Edges{Bottom: 1}},
		StyleFunc: func() core.Style { return core.Style{BorderColor: colors.Border()} },
		Children:  children,
	}
}

// column stacks children that may shrink.
func column(gap core.Points, children ...core.Node) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: gap}, Children: children}
}

// message is a line of status or error text, hidden while empty.
func message(cx *core.Scope, text func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{
		When: func() bool { return text() != "" },
		Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Padding: core.Symmetric(8, 16)}, Children: []core.Node{ui.Textf(text, 13, ui.Regular, colors.Muted)}}
		},
	}
}

// heading names a section of a page.
func heading(cx *core.Scope, title string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Padding: core.Edges{Top: 12, Left: 16, Right: 16, Bottom: 4}}, Children: []core.Node{
		ui.Text(title, 13, ui.Semibold, colors.Muted),
	}}
}
