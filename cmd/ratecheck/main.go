// Command ratecheck compares the bundled rates with the official publications.
// Exit code 1 = a revision/mismatch needs attention (suitable for cron / GitHub Actions).
//
//	ratecheck [-state .ratecheck-state] [-webhook URL]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"insurance-rates/internal/check"
	"insurance-rates/internal/rates"
)

func main() {
	state := flag.String("state", ".ratecheck-state", "file storing the last MHLW page fingerprint")
	webhook := flag.String("webhook", os.Getenv("RATECHECK_WEBHOOK"), "Slack-compatible webhook URL (optional)")
	flag.Parse()

	ds, err := rates.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	prev, _ := os.ReadFile(*state)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fs := check.Run(ctx, ds, check.Options{Now: time.Now(), StateHash: strings.TrimSpace(string(prev))})

	var lines []string
	for _, f := range fs {
		fmt.Printf("[%s] %-10s %s\n", f.Severity, f.Kind, f.Message)
		if f.Severity == "warn" {
			lines = append(lines, "• "+f.Message)
		}
		// 指紋は警告を確認済みにするまで更新しない(変更が続けて通知される)ため、初回のみ保存
		if f.Hash != "" && len(prev) == 0 {
			_ = os.WriteFile(*state, []byte(f.Hash), 0o644)
		}
	}
	if len(lines) > 0 {
		if *webhook != "" {
			b, _ := json.Marshal(map[string]string{"text": "社会保険・雇用保険の料率改定チェック:\n" + strings.Join(lines, "\n")})
			if resp, err := http.Post(*webhook, "application/json", bytes.NewReader(b)); err == nil {
				resp.Body.Close()
			}
		}
		os.Exit(1)
	}
}
