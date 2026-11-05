package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// 生成が通り、全ページに構造化データ・canonical があり、主要な数字が出ていること。
func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	idx, err := os.ReadFile("../../docs/index.html")
	if err != nil {
		t.Skip("docs/index.html not available (e.g. inside the Docker build context)")
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), idx, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generate(dir, time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	ld := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)
	must := map[string][]string{
		"kenpo":   {"9.85%", "10.55%", "40都道府県", "1.62%"},
		"koyo":    {"13.5/1,000", "5/1,000", "8.5/1,000"},
		"tedori":  {"249,590", "月給30万円"},
		"how-to":  {"253,022", "45,408", "4,970"},
		"sources": {"240件"},
	}
	for slug, wants := range must {
		b, err := os.ReadFile(filepath.Join(dir, slug, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, `rel="canonical" href="https://paps-jp.github.io/payroll-api/`+slug+`/"`) {
			t.Errorf("%s: canonical missing", slug)
		}
		for _, w := range wants {
			if !strings.Contains(s, w) {
				t.Errorf("%s: want %q", slug, w)
			}
		}
		blocks := ld.FindAllStringSubmatch(s, -1)
		if len(blocks) < 2 {
			t.Errorf("%s: structured data missing", slug)
		}
		for _, m := range blocks {
			var v map[string]any
			if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
				t.Errorf("%s: invalid JSON-LD: %v", slug, err)
			}
		}
	}
	sm, _ := os.ReadFile(filepath.Join(dir, "sitemap.xml"))
	if n := strings.Count(string(sm), "<loc>"); n != 6 {
		t.Errorf("sitemap urls = %d, want 6", n)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if !strings.Contains(string(out), "令和8年度(2026年度)の主な料率") || !strings.Contains(string(out), "WebApplication") {
		t.Error("index.html blocks not generated")
	}
}
