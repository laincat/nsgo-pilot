package app

import (
	kubai "github.com/nsgo-dev/nsgo-icons-kubai"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

var tabs = []struct {
	title string
	icon  func() *kubai.Symbol
}{
	{"源", kubai.Puzzle},
	{"搜索", kubai.Search},
	{"视频", kubai.Video},
}

// barSpace is what the pages leave below their content for the bar.
const barSpace core.Points = 96

// tabBar floats over the pages: Liquid Glass on iOS and macOS 26, NSGo's glass
// shader elsewhere, with Kubai's icons, the chosen one held in its moving pose.
func tabBar(cx *core.Scope, tab *core.Signal[int]) core.Node {
	colors := ui.ColorsOf(cx)
	items := make([]core.Node, len(tabs))
	for i, t := range tabs {
		chosen := func() bool { return tab.Get() == i }
		items[i] = core.View{
			OnPress: func(core.PressEvent) { tab.Set(i) },
			Style:   core.Style{AlignItems: core.AlignCenter, Gap: 2, Padding: core.Symmetric(6, 20), CornerRadius: 24},
			StyleFunc: func() core.Style {
				if chosen() {
					return core.Style{Background: colors.Hover()}
				}
				return core.Style{}
			},
			Children: []core.Node{
				core.Dynamic(func(cx *core.Scope) core.Node {
					color := colors.Muted()
					if chosen() {
						color = colors.Accent()
					}
					return kubai.Icon{Symbol: t.icon(), Size: 26, Color: color, Hovered: chosen, Label: t.title}
				}),
				ui.Textf(func() string { return t.title }, 11, ui.Medium, func() core.Color {
					if chosen() {
						return colors.Accent()
					}
					return colors.Muted()
				}),
			},
		}
	}
	return core.View{
		Style: core.Style{
			Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0),
			AlignItems: core.AlignCenter, Padding: core.Edges{Bottom: 14},
			MarginEnv: core.EnvEdges{Bottom: core.EnvSafeArea},
		},
		Children: []core.Node{core.View{
			Style: core.Style{
				Direction: core.Row, Gap: 4, Padding: core.All(6), CornerRadius: 32,
				GlassEffect:     core.GlassRegular,
				BackdropFilters: []core.Filter{core.Blur(24)},
				ShadowColor:     core.RGBA(0x000000, 0.18), ShadowRadius: 12, ShadowOffset: core.Point{Y: 4},
			},
			Children: items,
		}},
	}
}
