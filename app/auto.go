package app

import (
	"context"
	"log"
	"time"
)

// autopilot walks the whole path by itself when NSGO_PILOT_AUTO is set, for a
// simulator nobody taps: the tabs, the probe's browser login, its details and
// chapter, a real source from the official index searched and read, then the
// probe's episode in the host's player. Each page logs what it got.
func autopilot(cx interface{ Post(func()) }, a *App, nav interface {
	Push(route)
	PopToRoot()
}, tab interface{ Set(int) }) {
	step := func(wait time.Duration, what string, f func()) {
		time.Sleep(wait)
		log.Printf("pilot: step %s", what)
		cx.Post(f)
	}
	step(4*time.Second, "search tab", func() { tab.Set(1) })
	step(6*time.Second, "video tab", func() { tab.Set(2) })
	step(6*time.Second, "sources tab", func() { tab.Set(0) })
	step(6*time.Second, "login probe", func() { nav.Push(route{Kind: "login", Key: "probe", Title: "探针漫画"}) })
	// The page loads when it loads: go on when the login reports, not on a guess.
	select {
	case result := <-a.login:
		log.Printf("pilot: login step: %s", result)
	case <-time.After(45 * time.Second):
		log.Printf("pilot: login step: no result in 45s")
	}
	step(0, "details probe", func() {
		nav.PopToRoot()
		nav.Push(route{Kind: "detail", Key: "probe", ID: "1", Title: "探针漫画"})
	})
	step(6*time.Second, "reader probe", func() { nav.Push(route{Kind: "reader", Key: "probe", ID: "1", Episode: "c1", Title: "第一话"}) })

	time.Sleep(8 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if r, ok := realComic(ctx, a, "Komiic", "海賊王"); ok {
		step(0, "details Komiic", func() {
			nav.PopToRoot()
			nav.Push(route{Kind: "detail", Key: r.Key, ID: r.ID, Title: r.Title})
		})
		step(8*time.Second, "reader Komiic", func() { nav.Push(r) })
		time.Sleep(15 * time.Second)
	}
	step(0, "play probe_video", func() {
		nav.PopToRoot()
		nav.Push(route{Kind: "play", Key: "probe_video", ID: "bipbop", Line: "l1", Episode: "e1", Title: "Apple HLS 示例 第1集"})
	})
}

// realComic installs key from the official index, searches it and returns the
// first result's first chapter as a reader route.
func realComic(ctx context.Context, a *App, key, keyword string) (route, bool) {
	fail := func(what string, err error) (route, bool) {
		log.Printf("pilot: %s %s failed: %v", key, what, err)
		return route{}, false
	}
	if !a.Lib.IsInstalled(key) {
		_, rows, err := a.Lib.Index(ctx, officialIndex)
		if err != nil {
			return fail("index", err)
		}
		for _, row := range rows {
			if row.Key == key {
				if _, err := a.Lib.InstallEntry(ctx, row.Entry, officialIndex); err != nil {
					return fail("install", err)
				}
			}
		}
	}
	options, _ := a.Core.DefaultOptions(ctx, key)
	found, err := a.Core.Search(ctx, key, keyword, options, 1)
	if err != nil || len(found.Comics) == 0 {
		return fail("search", err)
	}
	c := found.Comics[0]
	log.Printf("pilot: search %s %q: %d results, first %q", key, keyword, len(found.Comics), c.Title)
	d, err := a.Core.Details(ctx, key, c.ID)
	if err != nil || len(d.Chapters) == 0 {
		return fail("details", err)
	}
	return route{Kind: "reader", Key: key, ID: c.ID, Episode: d.Chapters[0].ID, Title: c.Title + " " + d.Chapters[0].Title}, true
}
