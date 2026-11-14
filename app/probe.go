package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"time"

	"github.com/laincat/veneracat-core/plugin"
)

// installProbe serves a login page and a chapter on loopback and installs a
// comic source and a video source for them, so the whole path runs without a
// site or an account. NSGO_PILOT_PROBE turns it on.
//
// The login page sets an HttpOnly cookie, which a page script cannot read: the
// source's onLoginSuccess sees it only if Core read it from the web view's
// store and put it in the source's partition.
func installProbe(ctx context.Context, a *App) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	base := "http://" + ln.Addr().String()
	page, err := probePage()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><meta name="viewport" content="width=device-width"><title>Sign in</title>
<p>Signing in…</p><script>setTimeout(function(){location.href="/done"},1200)</script>`)
	})
	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "probe-session", Path: "/", HttpOnly: true, Expires: time.Now().Add(24 * time.Hour)})
		fmt.Fprint(w, `<!doctype html><meta name="viewport" content="width=device-width"><title>Logged in</title><p>Logged in.</p>`)
	})
	mux.HandleFunc("/page.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(page)
	})
	go http.Serve(ln, mux)

	comic := `class Probe extends ComicSource {
    name = "探针漫画"; key = "probe"; version = "1.0.0"
    account = {
        loginWithWebview: {
            url: "` + base + `/login",
            checkStatus: (url, title) => title === "Logged in",
            onLoginSuccess: async () => {
                const cs = await Network.getCookies("` + base + `/")
                if (!cs.some(c => c.name === "sid")) throw new Error("the source's partition holds no sid cookie")
            },
        },
    }
    search = { load: async () => ({comics: [{id: "1", title: "探针漫画", subtitle: "本机回环", cover: "` + base + `/page.png"}], maxPage: 1}) }
    comic = {
        loadInfo: async (id) => ({title: "探针漫画", cover: "` + base + `/page.png", description: "回环服务器生成的页面", chapters: {"c1": "第一话"}}),
        loadEp: async (id, ep) => ({images: ["` + base + `/page.png?1", "` + base + `/page.png?2", "` + base + `/page.png?3"]}),
    }
}`
	// Apple's public example stream: the episode is real HLS, through Core's
	// video path, with no media of our own to host.
	videoSource := `class ProbeVideo extends VideoSource {
    name = "探针视频"; key = "probe_video"; version = "1.0.0"
    search = { load: async () => ({videos: [{id: "bipbop", title: "Apple HLS 示例"}], maxPage: 1}) }
    video = {
        loadInfo: async (id) => new VideoDetails({title: "Apple HLS 示例", lines: [{id: "l1", name: "线路一", episodes: [{id: "e1", title: "第1集"}]}]}),
        loadPlay: async () => new PlayInfo({url: "https://devstreaming-cdn.apple.com/videos/streaming/examples/img_bipbop_adv_example_fmp4/master.m3u8", format: "hls"}),
    }
}`
	for _, s := range []struct {
		typ  plugin.Type
		body string
	}{{plugin.TypeComic, comic}, {plugin.TypeVideoJS, videoSource}} {
		if _, err := a.Lib.InstallTypedText(ctx, s.typ, s.body, "probe"); err != nil {
			return err
		}
	}
	return nil
}

// probePage is a tall page with bands, drawn at run time, so a reader that
// cannot decode it cannot pass for one that can.
func probePage() ([]byte, error) {
	const width, height = 800, 1400
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		shade := uint8(0x90)
		if (y/140)%2 == 0 {
			shade = 0x30
		}
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 0x30, G: shade, B: 0xa0, A: 0xff})
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}
