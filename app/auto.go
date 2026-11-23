package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func autopilot(ctx context.Context, cx *core.Scope, a *App, nav interface {
	Push(route)
	PopToRoot()
}) {
	if runtime.GOOS == "windows" {
		defer func() {
			a.close()
			cx.Post(cx.RequestQuit)
		}()
	}
	step := func(what string, next route, result <-chan error) bool {
		log.Printf("pilot: step %s", what)
		cx.Post(func() { nav.PopToRoot(); nav.Push(next) })
		timer := time.NewTimer(45 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			log.Printf("pilot: failed %s: timed out", what)
			return false
		case err := <-result:
			if err != nil {
				log.Printf("pilot: failed %s: %v", what, err)
				return false
			}
			log.Printf("pilot: %s step: ok", what)
			return true
		}
	}
	if !step("login", route{Kind: "login", Key: "probe", Title: "探针漫画"}, a.probe.login) {
		return
	}
	if !step("details", route{Kind: "detail", Key: "probe", ID: "1", Title: "探针漫画"}, a.probe.details) {
		return
	}
	if !step("reader", route{Kind: "reader", Key: "probe", ID: "1", Episode: "c1", Title: "第一话"}, a.probe.reader) {
		return
	}
	if !step("playback", route{Kind: "play", Key: "probe_video", ID: "fixture", Line: "l1", Episode: "e1", Title: "回环 HLS"}, a.probe.playback) {
		return
	}
	log.Print("pilot: complete")
	if os.Getenv("NSGO_PILOT_LIVE") != "" {
		live, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if found, ok := realComic(live, a, "Komiic", "海賊王"); ok {
			cx.Post(func() { nav.PopToRoot(); nav.Push(found) })
		}
	}
}

func reportProbe(result chan<- error, err error) {
	select {
	case result <- err:
	default:
	}
}

func verifyProbeMedia(ctx context.Context, a *App, master string) error {
	read := func(address, byteRange string, wantedStatus int) ([]byte, error) {
		headers := http.Header{}
		if byteRange != "" {
			headers.Set("Range", byteRange)
		}
		resp, err := a.Core.OpenVideoMedia(ctx, "probe_video", address, headers)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != wantedStatus {
			return nil, fmt.Errorf("media HTTP %d, want %d", resp.StatusCode, wantedStatus)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	}
	base, err := url.Parse(master)
	if err != nil {
		return err
	}
	masterBytes, err := read(master, "", http.StatusOK)
	if err != nil {
		return err
	}
	if !strings.Contains(string(masterBytes), "main.m3u8") {
		return fmt.Errorf("probe master has no media playlist")
	}
	playlist, err := read(base.JoinPath("..", "main.m3u8").String(), "", http.StatusOK)
	if err != nil {
		return err
	}
	if !strings.Contains(string(playlist), "stream.mp4") {
		return fmt.Errorf("probe playlist has no media segments")
	}
	segment, err := read(base.JoinPath("..", "stream.mp4").String(), "bytes=0-31", http.StatusPartialContent)
	if err != nil {
		return err
	}
	if len(segment) != 32 || !strings.Contains(string(segment), "ftyp") {
		return fmt.Errorf("probe range did not return the MP4 header")
	}
	return nil
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
