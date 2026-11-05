// Command server serves the REST API and MCP endpoint, or runs MCP over stdio.
//
//	server -addr :8080 [-rate 120] [-token SECRET]   REST (/v1/...) + MCP over HTTP (/mcp)
//	server -stdio                                    MCP over stdio (for Claude Desktop etc.)
//
// By default the API is open (no token). Abuse is limited per IP: requests over
// the per-minute limit get 429, and persistent offenders are blocked for a while.
// Setting -token / RATES_API_TOKEN makes a Bearer token mandatory again.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"insurance-rates/internal/api"
	"insurance-rates/internal/guard"
	"insurance-rates/internal/mcp"
	"insurance-rates/internal/rates"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	stdio := flag.Bool("stdio", false, "serve MCP over stdio instead of HTTP")
	token := flag.String("token", os.Getenv("RATES_API_TOKEN"), "optional Bearer token for /v1 and /mcp (empty = open API)")
	perMin := flag.Int("rate", guard.Default().PerMinute, "max requests per minute per IP (persistent offenders are blocked temporarily)")
	dataFile := flag.String("data", "", "optional rates.json to use instead of the embedded one")
	flag.Parse()

	var ds *rates.Dataset
	var err error
	if *dataFile != "" {
		b, rerr := os.ReadFile(*dataFile)
		if rerr != nil {
			log.Fatal(rerr)
		}
		ds, err = rates.Parse(b)
	} else {
		ds, err = rates.Load()
	}
	if err != nil {
		log.Fatal(err)
	}
	m := &mcp.Server{DS: ds}
	if *stdio {
		log.SetOutput(os.Stderr)
		if err := m.Stdio(context.Background()); err != nil {
			log.Fatal(err)
		}
		return
	}

	cfg := guard.Default()
	cfg.PerMinute = *perMin
	g := guard.New(cfg)
	apiH := auth(*token, (&api.Server{DS: ds}).Handler())

	mux := http.NewServeMux()
	mux.Handle("/v1/", g.Middleware(apiH))
	mux.Handle("/mcp", g.Middleware(m.HTTPHandler(*token)))
	mux.Handle("/public/", g.Middleware(http.StripPrefix("/public", apiH))) // 旧パスの互換
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	log.Printf("listening on %s (rate %d/min/IP, token required: %v)", *addr, cfg.PerMinute, *token != "")
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func auth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
