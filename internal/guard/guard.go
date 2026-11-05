// Package guard provides the open-API protections: CORS for browsers, a per-IP
// rate limit, and a temporary block for persistent offenders.
package guard

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	PerMinute   int           // 1分あたりの上限(IPごと)
	StrikeLimit int           // 上限超過がこの回数に達したらブロック
	StrikeSpan  time.Duration // 超過回数を数える期間
	BlockFor    time.Duration // ブロック時間
}

func Default() Config {
	return Config{PerMinute: 120, StrikeLimit: 5, StrikeSpan: 10 * time.Minute, BlockFor: 15 * time.Minute}
}

type entry struct {
	window     time.Time
	count      int
	strikes    int
	strikeFrom time.Time
	blockedTil time.Time
}

type Guard struct {
	cfg Config
	now func() time.Time
	mu  sync.Mutex
	m   map[string]*entry
}

func New(cfg Config) *Guard { return &Guard{cfg: cfg, now: time.Now, m: map[string]*entry{}} }

// Check returns (allowed, retryAfterSeconds).
func (g *Guard) Check(ip string) (bool, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if len(g.m) > 100000 { // メモリ保護: 古いものを掃除
		for k, e := range g.m {
			if now.Sub(e.window) > g.cfg.StrikeSpan && now.After(e.blockedTil) {
				delete(g.m, k)
			}
		}
	}
	e := g.m[ip]
	if e == nil {
		e = &entry{}
		g.m[ip] = e
	}
	if now.Before(e.blockedTil) {
		return false, int(e.blockedTil.Sub(now).Seconds()) + 1
	}
	if w := now.Truncate(time.Minute); w.After(e.window) {
		e.window, e.count = w, 0
	}
	e.count++
	if e.count <= g.cfg.PerMinute {
		return true, 0
	}
	if e.count == g.cfg.PerMinute+1 { // この分の最初の超過だけを1回と数える
		if now.Sub(e.strikeFrom) > g.cfg.StrikeSpan {
			e.strikes, e.strikeFrom = 0, now
		}
		e.strikes++
		if e.strikes >= g.cfg.StrikeLimit {
			e.blockedTil, e.strikes = now.Add(g.cfg.BlockFor), 0
			return false, int(g.cfg.BlockFor.Seconds())
		}
	}
	return false, 60 - now.Second()
}

// Middleware adds CORS (no credentials) and the rate limit.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id, Mcp-Protocol-Version")
		h.Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if ok, retry := g.Check(ClientIP(r)); !ok {
			h.Set("Retry-After", strconv.Itoa(retry))
			h.Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"アクセスが多すぎます。しばらく待ってからもう一度お試しください。"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP trusts X-Forwarded-For because the only way in is the Caddy front,
// which replaces any client-supplied value with the real peer address.
func ClientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
