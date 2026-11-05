// Package mcp is a minimal Model Context Protocol server (JSON-RPC 2.0) exposing
// the rate tables and calculators as tools. It supports stdio (newline
// delimited JSON) and HTTP POST (single JSON response per request).
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"insurance-rates/internal/api"
	"insurance-rates/internal/check"
	"insurance-rates/internal/rates"
)

type Server struct {
	DS *rates.Dataset
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }

var tools = []tool{
	{"get_rates", "指定月に適用される社会保険料率・雇用保険料率を返す(協会けんぽ管掌)。prefecture省略で全47都道府県。",
		obj(map[string]any{"month": str("対象月 YYYY-MM"), "prefecture": str("都道府県名 例: 東京都")}, "month")},
	{"list_revisions", "データに登録されている全ての料率改定(種別・適用開始月・出典・検証済みフラグ)を返す。", obj(map[string]any{})},
	{"get_standard_remuneration_grade", "報酬月額から健康保険の等級・標準報酬月額を返す。",
		obj(map[string]any{"monthly_remuneration": num("報酬月額(円)")}, "monthly_remuneration")},
	{"calculate_social_insurance", "健康保険・介護保険・厚生年金・子ども・子育て支援金/拠出金の本人負担と事業主負担(月額)を計算する。",
		obj(map[string]any{"month": str("対象月 YYYY-MM"), "prefecture": str("都道府県名"),
			"monthly_remuneration": num("報酬月額(円)。standard_remunerationと排他"), "standard_remuneration": num("標準報酬月額(円)"),
			"age": num("年齢。40-64で介護保険、70以上で厚生年金なし、75以上で健保なし")}, "month", "prefecture")},
	{"calculate_employment_insurance", "雇用保険料の労働者負担・事業主負担を計算する。",
		obj(map[string]any{"month": str("対象月 YYYY-MM"), "wage": num("賃金総額(円)"),
			"industry": map[string]any{"type": "string", "enum": []string{"general", "agri", "construction"}, "description": "事業の種類(省略=general)"}}, "month", "wage")},
	{"calculate_withholding_tax", "月額給与の源泉所得税(復興特別所得税込み)を計算する。甲欄は国税庁の電算機計算の特例、乙欄は電算機計算。amountは社会保険料等控除後の給与額。",
		obj(map[string]any{"month": str("支払月 YYYY-MM"), "amount": num("社会保険料等控除後の給与等の金額(円)"),
			"column":     map[string]any{"type": "string", "enum": []string{"ko", "otsu"}, "description": "甲欄/乙欄(省略=ko)"},
			"dependents": num("甲欄: 源泉控除対象配偶者+親族の数(+加算分) / 乙欄: 従たる給与の扶養親族等の数")}, "month", "amount")},
	{"calculate_payroll_deductions", "総支給額から社会保険・雇用保険・源泉所得税・住民税を控除した手取りと会社負担を一括計算する。住民税は通知額を入力。",
		obj(map[string]any{"month": str("支払月 YYYY-MM"), "prefecture": str("都道府県名"), "gross": num("総支給額(円)"),
			"age": num("年齢"), "non_taxable_commute": num("非課税の通勤手当(円)"),
			"industry": map[string]any{"type": "string", "enum": []string{"general", "agri", "construction"}},
			"column":   map[string]any{"type": "string", "enum": []string{"ko", "otsu"}}, "dependents": num("扶養親族等の数"),
			"resident_tax": num("住民税月額(円)")}, "month", "prefecture", "gross")},
	{"check_for_revisions", "協会けんぽ・厚労省の公式ページを確認し、保有データとの差異や未登録の改定がないか検知する(ネットワークアクセスあり)。", obj(map[string]any{})},
}

func (s *Server) call(ctx context.Context, name string, a map[string]any) (any, error) {
	gs := func(k string) string { v, _ := a[k].(string); return v }
	gi := func(k string) int { v, _ := a[k].(float64); return int(v) }
	switch name {
	case "get_rates":
		return api.Rates(s.DS, gs("month"), gs("prefecture"))
	case "list_revisions":
		return api.Revisions(s.DS), nil
	case "get_standard_remuneration_grade":
		if gi("monthly_remuneration") <= 0 {
			return nil, fmt.Errorf("monthly_remuneration は正の整数")
		}
		return s.DS.GradeFor(gi("monthly_remuneration")), nil
	case "calculate_social_insurance":
		return s.DS.CalcSocial(rates.SocialInput{Month: gs("month"), Prefecture: gs("prefecture"),
			MonthlyRemuneration: gi("monthly_remuneration"), StandardRemuneration: gi("standard_remuneration"), Age: gi("age")})
	case "calculate_employment_insurance":
		return s.DS.CalcEmployment(gs("month"), gs("industry"), gi("wage"))
	case "calculate_withholding_tax":
		return s.DS.CalcWithholding(gs("month"), gs("column"), int64(gi("amount")), int64(gi("dependents")))
	case "calculate_payroll_deductions":
		return s.DS.CalcPayroll(rates.PayrollInput{Month: gs("month"), Prefecture: gs("prefecture"), Age: gi("age"),
			Gross: int64(gi("gross")), NonTaxableCommute: int64(gi("non_taxable_commute")), Industry: gs("industry"),
			Column: gs("column"), Dependents: int64(gi("dependents")), ResidentTax: int64(gi("resident_tax"))})
	case "check_for_revisions":
		c, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		return check.Run(c, s.DS, check.Options{Now: time.Now()}), nil
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

// Handle processes one JSON-RPC message. It returns nil for notifications.
func (s *Server) Handle(ctx context.Context, raw []byte) *response {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}}
	}
	if len(req.ID) == 0 { // notification
		return nil
	}
	ok := func(v any) *response { return &response{JSONRPC: "2.0", ID: req.ID, Result: v} }
	fail := func(code int, msg string) *response {
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{code, msg}}
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-03-26"
		}
		return ok(map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "insurance-rates", "version": "0.1.0"},
			"instructions":    "社会保険料率・雇用保険料率の参照と保険料計算。unverified_rate_kinds が返った場合は出典で再確認を促すこと。",
		})
	case "ping":
		return ok(map[string]any{})
	case "tools/list":
		return ok(map[string]any{"tools": tools})
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(-32602, "invalid params")
		}
		out, err := s.call(ctx, p.Name, p.Arguments)
		if err != nil {
			return ok(map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": err.Error()}}})
		}
		b, _ := json.MarshalIndent(out, "", " ")
		return ok(map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}})
	}
	return fail(-32601, "method not found: "+req.Method)
}

// ServeStdio reads newline-delimited JSON-RPC from r and writes to w.
func (s *Server) ServeStdio(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if resp := s.Handle(ctx, line); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// HTTPHandler serves MCP over plain HTTP POST. Set token to require a Bearer token.
func (s *Server) HTTPHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		resp := s.Handle(r.Context(), body)
		if resp == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(resp)
	})
}

// Stdio is a convenience wrapper over os.Stdin/os.Stdout.
func (s *Server) Stdio(ctx context.Context) error { return s.ServeStdio(ctx, os.Stdin, os.Stdout) }
