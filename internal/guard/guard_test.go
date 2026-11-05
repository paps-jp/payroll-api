package guard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitAndBlock(t *testing.T) {
	g := New(Config{PerMinute: 3, StrikeLimit: 2, StrikeSpan: 10 * time.Minute, BlockFor: 15 * time.Minute})
	clock := time.Date(2026, 11, 6, 10, 0, 5, 0, time.UTC)
	g.now = func() time.Time { return clock }

	for i := 0; i < 3; i++ {
		if ok, _ := g.Check("1.1.1.1"); !ok {
			t.Fatalf("request %d should pass", i)
		}
	}
	if ok, _ := g.Check("1.1.1.1"); ok {
		t.Fatal("4th request in the minute must be limited")
	}
	if ok, _ := g.Check("2.2.2.2"); !ok {
		t.Fatal("other IPs are unaffected")
	}
	clock = clock.Add(time.Minute) // 次の分: また使える
	if ok, _ := g.Check("1.1.1.1"); !ok {
		t.Fatal("new minute should reset")
	}
	for i := 0; i < 3; i++ { // 2回目の超過 → ブロック
		g.Check("1.1.1.1")
	}
	if ok, retry := g.Check("1.1.1.1"); ok || retry < 600 {
		t.Fatalf("should be blocked for ~15min, retry=%d ok=%v", retry, ok)
	}
	clock = clock.Add(16 * time.Minute)
	if ok, _ := g.Check("1.1.1.1"); !ok {
		t.Fatal("block should expire")
	}
}

func TestMiddlewareCORS(t *testing.T) {
	g := New(Config{PerMinute: 1, StrikeLimit: 5, StrikeSpan: time.Hour, BlockFor: time.Hour})
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	do := func(method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/x", nil)
		r.Header.Set("X-Forwarded-For", "9.9.9.9, 10.0.0.1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := do("OPTIONS"); w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("preflight: %d %v", w.Code, w.Header())
	}
	if w := do("GET"); w.Code != 200 {
		t.Errorf("first: %d", w.Code)
	}
	if w := do("GET"); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Errorf("second: %d %v", w.Code, w.Header())
	}
}
