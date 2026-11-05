// Command server serves the REST API and MCP endpoint, or runs MCP over stdio.
//
//	server -addr :8080 [-token SECRET]   REST (/v1/...) + MCP over HTTP (/mcp)
//	server -stdio                        MCP over stdio (for Claude Desktop etc.)
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"insurance-rates/internal/api"
	"insurance-rates/internal/mcp"
	"insurance-rates/internal/rates"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	stdio := flag.Bool("stdio", false, "serve MCP over stdio instead of HTTP")
	token := flag.String("token", os.Getenv("RATES_API_TOKEN"), "Bearer token required for /mcp and /v1 (empty = none)")
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
	mux := http.NewServeMux()
	rest := auth(*token, (&api.Server{DS: ds}).Handler())
	mux.Handle("/v1/", rest)
	mux.Handle("/mcp", m.HTTPHandler(*token))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func auth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
