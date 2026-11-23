package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/laincat/veneracat-core/host"
	"github.com/nsgo-dev/nsgo/core"
)

type cookieReader func(context.Context, string) ([]*http.Cookie, error)

func readBrowserCookies(cx *core.Scope) cookieReader {
	return func(ctx context.Context, rawURL string) ([]*http.Cookie, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		type result struct {
			rows []*http.Cookie
			err  error
		}
		answer := make(chan result, 1)
		cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			cx.WebCookies(core.WebCookieFilter{URL: rawURL}, func(rows []core.WebCookie, err error) {
				out := make([]*http.Cookie, 0, len(rows))
				for _, row := range rows {
					out = append(out, &http.Cookie{Name: row.Name, Value: row.Value, Domain: row.Domain, Path: row.Path, Expires: row.Expires, Secure: row.Secure, HttpOnly: row.HTTPOnly})
				}
				answer <- result{out, err}
			})
		})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-answer:
			return result.rows, result.err
		}
	}
}

type loginResult struct {
	total, httpOnly int
	saved           bool
}

func finishLogin(ctx context.Context, read cookieReader, addresses []string, importCookies func([]*http.Cookie) error, callback func(context.Context) error, retry time.Duration) (loginResult, error) {
	for {
		if err := ctx.Err(); err != nil {
			return loginResult{}, err
		}
		rows, err := collectLoginCookies(ctx, read, addresses)
		if err != nil {
			return loginResult{}, err
		}
		if len(rows) > 0 {
			if err = ctx.Err(); err != nil {
				return loginResult{}, err
			}
			if err = importCookies(rows); err != nil {
				return loginResult{}, fmt.Errorf("save login cookies: %w", err)
			}
			result := loginResult{total: len(rows), saved: true}
			for _, row := range rows {
				if row.HttpOnly {
					result.httpOnly++
				}
			}
			if err = callback(ctx); err != nil {
				return result, fmt.Errorf("Cookie 已保存 %d 个，但 onLoginSuccess 失败：%w", result.total, err)
			}
			return result, nil
		}
		timer := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return loginResult{}, fmt.Errorf("waiting for login cookies: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func collectLoginCookies(ctx context.Context, read cookieReader, addresses []string) ([]*http.Cookie, error) {
	type identity struct{ name, domain, path string }
	index := make(map[identity]int)
	visited := make(map[string]bool)
	var out []*http.Cookie
	for _, raw := range addresses {
		if visited[raw] {
			continue
		}
		visited[raw] = true
		if err := host.ValidateURL(raw); err != nil {
			return nil, err
		}
		u, _ := url.Parse(raw)
		rows, err := read(ctx, raw)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil || row.Name == "" || (!row.Expires.IsZero() && !row.Expires.After(time.Now())) {
				continue
			}
			cookie := *row
			cookie.Domain = strings.ToLower(cookie.Domain)
			if cookie.Domain == "" {
				cookie.Domain = strings.ToLower(u.Hostname())
			}
			if cookie.Path == "" {
				cookie.Path = "/"
			}
			key := identity{cookie.Name, strings.TrimPrefix(cookie.Domain, "."), cookie.Path}
			if i, duplicate := index[key]; duplicate {
				out[i] = &cookie
				continue
			}
			index[key] = len(out)
			out = append(out, &cookie)
		}
	}
	return out, nil
}

var errLoginAlreadyOpen = errors.New("another browser login is open")
