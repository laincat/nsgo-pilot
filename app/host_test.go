package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	vc "github.com/laincat/veneracat-core/gohost"
	"github.com/laincat/veneracat-core/subscription"
)

func localSource(key, imageURL string) string {
	return fmt.Sprintf("class Fixture extends ComicSource {name='fixture'; key=%q; version='1'; minAppVersion='1.6.3'; search={load:()=>({comics:[{id:'1',title:'fixture'}],maxPage:1})}; comic={loadEp:()=>({images:[%q]})}}", key, imageURL)
}

func TestSharedHostRestoresAnInstalledSourceAndReadsItsImage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	page, err := probePage()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page.png" {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(page)
			return
		}
		fmt.Fprint(w, localSource("saved", "http://"+r.Host+"/page.png"))
	}))
	defer server.Close()
	app, err := open(ctx, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Lib.InstallURL(ctx, server.URL+"/script.js"); err != nil {
		t.Fatal(err)
	}
	app.close()
	reopened, err := open(ctx, root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if reopened.Core.DataDir() != filepath.Join(root, "core") || reopened.Core.AppVersion() != sourceLevel {
		t.Fatal("profile or source API level changed")
	}
	results, err := reopened.Core.Search(ctx, "saved", "fixture", nil, 1)
	if err != nil || len(results.Comics) != 1 {
		t.Fatalf("search = %+v, %v", results, err)
	}
	images, err := reopened.Core.ChapterImages(ctx, "saved", "1", "")
	if err != nil || len(images) != 1 {
		t.Fatalf("chapter = %v, %v", images, err)
	}
	imageBytes, _, err := reopened.Core.FetchImage(ctx, "saved", images[0], "1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(bytes.NewReader(imageBytes)); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineFixtureDoesNotTouchTheNormalProfile(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := open(ctx, root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	if _, err := os.Stat(filepath.Join(root, "core")); !os.IsNotExist(err) {
		t.Fatalf("fixture opened the ordinary profile: %v", err)
	}
	found, err := a.Core.Search(ctx, "probe", "fixture", nil, 1)
	if err != nil || len(found.Comics) != 1 {
		t.Fatalf("probe search = %+v, %v", found, err)
	}
	images, err := a.Core.ChapterImages(ctx, "probe", "1", "c1")
	if err != nil || len(images) == 0 {
		t.Fatalf("probe images = %v, %v", images, err)
	}
	if _, _, err := a.Core.FetchImage(ctx, "probe", images[0], "1", "c1"); err == nil {
		t.Fatal("probe chapter accepted a request without its login")
	}
	browser := func(ctx context.Context, raw string) ([]*http.Cookie, error) {
		return []*http.Cookie{{Name: a.probe.cookieName, Value: "probe-session", Domain: "127.0.0.1", HttpOnly: true}}, nil
	}
	decl, err := a.Core.AccountWebviewLogin(ctx, "probe")
	if err != nil {
		t.Fatal(err)
	}
	result, err := finishLogin(ctx, browser, []string{decl.URL, a.probe.address + "/done"}, func(rows []*http.Cookie) error {
		return a.Core.Jar().Import(vc.ComicPartition("probe"), rows)
	}, func(ctx context.Context) error { return a.Core.WebviewLoginDone(ctx, "probe", decl.Handle) }, time.Millisecond)
	if err != nil || result.total != 1 || result.httpOnly != 1 {
		t.Fatalf("handoff = %+v, %v", result, err)
	}
	data, _, err := a.Core.FetchImage(ctx, "probe", images[0], "1", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	media, err := a.Core.VideoPlay(ctx, "probe_video", "fixture", "l1", "e1")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyProbeMedia(ctx, a, media.URL); err != nil {
		t.Fatal(err)
	}
	address, fixtureDir := a.probe.address, a.probe.root
	cancel()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	client := http.Client{Timeout: 100 * time.Millisecond}
	for {
		response, err := client.Get(address + "/done")
		if err == nil {
			_ = response.Body.Close()
		}
		_, statErr := os.Stat(fixtureDir)
		if err != nil && errors.Is(statErr, os.ErrNotExist) {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("fixture remained after close: HTTP %v; files %v", err, statErr)
		case <-time.After(time.Millisecond * 10):
		}
	}
	normal, err := open(context.Background(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer normal.close()
	if len(normal.Lib.Sources()) != 0 {
		t.Fatal("probe source leaked into the normal profile")
	}
}

func TestInstallationUsesTheSelectedIndexSnapshot(t *testing.T) {
	ctx := context.Background()
	app, err := open(ctx, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer app.close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, localSource(r.URL.Path[1:], "")) }))
	defer site.Close()
	previous := indexSnapshot{url: site.URL + "/first.json", rows: []subscription.Row{{Entry: subscription.Entry{Key: "first", URL: site.URL + "/first"}}}}
	selection := indexSelection{url: previous.url, entry: previous.rows[0].Entry}
	current := indexSnapshot{url: site.URL + "/second.json", rows: []subscription.Row{{Entry: subscription.Entry{Key: "second", URL: site.URL + "/second"}}}}
	for _, picked := range []indexSelection{{url: current.url, entry: current.rows[0].Entry}, selection} {
		rec, err := picked.install(ctx, app.Lib)
		if err != nil || rec.IndexURL != picked.url || rec.Key != picked.entry.Key {
			t.Fatalf("selected install = %+v, %v", rec, err)
		}
	}
}
