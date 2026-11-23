package app

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/laincat/veneracat-core/gohost/cookies"
)

func TestLoginWaitsThenImportsOneCombinedSet(t *testing.T) {
	jar, err := cookies.Open("")
	if err != nil {
		t.Fatal(err)
	}
	const start = "https://example.test/login"
	const end = "https://example.test/finish"
	reads, imports, callbacks := 0, 0, 0
	read := func(_ context.Context, address string) ([]*http.Cookie, error) {
		reads++
		if reads <= 2 {
			return nil, nil
		}
		if address == start {
			return []*http.Cookie{{Name: "sid", Value: "old"}}, nil
		}
		return []*http.Cookie{{Name: "sid", Value: "new", HttpOnly: true}, {Name: "domain", Value: "shared", Domain: ".example.test"}}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := finishLogin(ctx, read, []string{start, end}, func(rows []*http.Cookie) error {
		imports++
		return jar.Import("comic:one", rows)
	}, func(context.Context) error {
		callbacks++
		if got := jar.Header("comic:one", end); got != "sid=new; domain=shared" {
			t.Errorf("callback saw %q", got)
		}
		return nil
	}, time.Millisecond)
	if err != nil || result.total != 2 || result.httpOnly != 1 || reads != 4 || imports != 1 || callbacks != 1 {
		t.Fatalf("got %+v, %v, reads=%d imports=%d callbacks=%d", result, err, reads, imports, callbacks)
	}
	if jar.Header("comic:two", end) != "" || jar.Header("comic:one", "https://sub.example.test/") != "domain=shared" {
		t.Fatal("host-only or partition rules were lost")
	}
}

func TestLoginAbortsBeforePartialImport(t *testing.T) {
	for _, scenario := range []string{"read failed", "closed", "write failed", "callback failed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New(scenario)
			count, imports, calls := 0, 0, 0
			read := func(context.Context, string) ([]*http.Cookie, error) {
				count++
				if count == 2 && scenario == "read failed" {
					return nil, failure
				}
				if count == 2 && scenario == "closed" {
					cancel()
				}
				return []*http.Cookie{{Name: "sid", Value: "session", Domain: "example.test"}}, nil
			}
			result, err := finishLogin(ctx, read, []string{"https://example.test/a", "https://example.test/b"}, func([]*http.Cookie) error {
				imports++
				if scenario == "write failed" {
					return failure
				}
				return nil
			}, func(context.Context) error { calls++; return failure }, time.Millisecond)
			switch scenario {
			case "closed":
				if !errors.Is(err, context.Canceled) || imports != 0 || calls != 0 {
					t.Fatalf("closed: %+v %v %d %d", result, err, imports, calls)
				}
			case "read failed":
				if !errors.Is(err, failure) || imports != 0 || calls != 0 {
					t.Fatalf("partial: %+v %v %d %d", result, err, imports, calls)
				}
			case "write failed":
				if !errors.Is(err, failure) || imports != 1 || calls != 0 || result.saved {
					t.Fatalf("write: %+v %v %d %d", result, err, imports, calls)
				}
			case "callback failed":
				if !errors.Is(err, failure) || imports != 1 || calls != 1 || !result.saved {
					t.Fatalf("callback: %+v %v %d %d", result, err, imports, calls)
				}
			}
		})
	}
}
