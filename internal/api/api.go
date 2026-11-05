// Package api exposes the rate tables and calculators as a small REST API.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"insurance-rates/internal/rates"
)

type Server struct {
	DS *rates.Dataset
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/v1/rates", s.rates)
	mux.HandleFunc("/v1/revisions", s.revisions)
	mux.HandleFunc("/v1/grades", s.grades)
	mux.HandleFunc("/v1/calc/social", s.calcSocial)
	mux.HandleFunc("/v1/calc/employment", s.calcEmployment)
	mux.HandleFunc("/v1/calc/withholding", s.calcWithholding)
	mux.HandleFunc("/v1/calc/payroll", s.calcPayroll)
	return mux
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func bad(w http.ResponseWriter, err error) { write(w, 400, map[string]string{"error": err.Error()}) }

func atoi(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}

// GET /v1/rates?month=2026-04&prefecture=東京都
func (s *Server) rates(w http.ResponseWriter, r *http.Request) {
	out, err := Rates(s.DS, r.URL.Query().Get("month"), r.URL.Query().Get("prefecture"))
	if err != nil {
		bad(w, err)
		return
	}
	write(w, 200, out)
}

// Rates is shared with the MCP server.
func Rates(d *rates.Dataset, month, pref string) (map[string]any, error) {
	if err := rates.ValidMonth(month); err != nil {
		return nil, err
	}
	out := map[string]any{"month": month}
	if hv, ok := d.HealthAt(month); ok {
		h := map[string]any{"effective_from": hv.EffectiveFrom, "source": hv.Source, "verified": hv.Verified}
		if pref != "" {
			v, ok := hv.Rates[pref]
			if !ok {
				return nil, errPref(pref)
			}
			h["prefecture"], h["rate_percent"] = pref, v
		} else {
			h["rates_percent"] = hv.Rates
		}
		out["health"] = h
	}
	put := func(key string, v *rates.RateVersion, ok bool) {
		if ok {
			out[key] = v
		}
	}
	v, ok := d.CareAt(month)
	put("care", v, ok)
	v, ok = d.PensionAt(month)
	put("pension", v, ok)
	v, ok = d.ChildSupportAt(month)
	put("child_support", v, ok)
	v, ok = d.ChildContributionAt(month)
	put("child_contribution", v, ok)
	if e, ok := d.EmploymentAt(month); ok {
		out["employment"] = e
	}
	if w, ok := d.WithholdingAt(month); ok {
		out["withholding"] = w
	}
	return out, nil
}

type prefErr string

func (p prefErr) Error() string { return "都道府県が不明です: " + string(p) }
func errPref(p string) error    { return prefErr(p) }

// GET /v1/revisions — every known revision (effective month + source + verified flag)
func (s *Server) revisions(w http.ResponseWriter, r *http.Request) { write(w, 200, Revisions(s.DS)) }

type Revision struct {
	Kind          string `json:"kind"`
	EffectiveFrom string `json:"effective_from"`
	Source        string `json:"source"`
	Verified      bool   `json:"verified"`
}

func Revisions(d *rates.Dataset) []Revision {
	var out []Revision
	for _, v := range d.Health {
		out = append(out, Revision{"health", v.EffectiveFrom, v.Source, v.Verified})
	}
	add := func(kind string, vs []rates.RateVersion) {
		for _, v := range vs {
			out = append(out, Revision{kind, v.EffectiveFrom, v.Source, v.Verified})
		}
	}
	add("care", d.Care)
	add("pension", d.Pension)
	add("child_support", d.ChildSupport)
	add("child_contribution", d.ChildContribution)
	for _, v := range d.Employment {
		out = append(out, Revision{"employment", v.EffectiveFrom, v.Source, v.Verified})
	}
	for _, v := range d.Withholding {
		out = append(out, Revision{"withholding", v.EffectiveFrom, v.Source, v.Verified})
	}
	return out
}

func (s *Server) grades(w http.ResponseWriter, r *http.Request) {
	write(w, 200, map[string]any{"health_grades": s.DS.HealthGrades, "pension_range": s.DS.PensionRange, "verified": s.DS.GradesVerified})
}

// GET /v1/calc/social?month=&prefecture=&monthly_remuneration=|standard_remuneration=&age=
func (s *Server) calcSocial(w http.ResponseWriter, r *http.Request) {
	res, err := s.DS.CalcSocial(rates.SocialInput{
		Month: r.URL.Query().Get("month"), Prefecture: r.URL.Query().Get("prefecture"),
		MonthlyRemuneration: atoi(r, "monthly_remuneration"), StandardRemuneration: atoi(r, "standard_remuneration"),
		Age: atoi(r, "age"),
	})
	if err != nil {
		bad(w, err)
		return
	}
	write(w, 200, res)
}

// GET /v1/calc/withholding?month=&amount=&column=ko|otsu&dependents=
func (s *Server) calcWithholding(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.DS.CalcWithholding(q.Get("month"), q.Get("column"), int64(atoi(r, "amount")), int64(atoi(r, "dependents")))
	if err != nil {
		bad(w, err)
		return
	}
	write(w, 200, res)
}

// GET /v1/calc/payroll?month=&prefecture=&gross=&age=&non_taxable_commute=&industry=&column=&dependents=&resident_tax=
func (s *Server) calcPayroll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.DS.CalcPayroll(rates.PayrollInput{
		Month: q.Get("month"), Prefecture: q.Get("prefecture"), Age: atoi(r, "age"),
		Gross: int64(atoi(r, "gross")), NonTaxableCommute: int64(atoi(r, "non_taxable_commute")),
		Industry: q.Get("industry"), Column: q.Get("column"), Dependents: int64(atoi(r, "dependents")),
		ResidentTax: int64(atoi(r, "resident_tax")),
		SkipSocial:  q.Get("skip_social") == "true", SkipEmployment: q.Get("skip_employment") == "true",
	})
	if err != nil {
		bad(w, err)
		return
	}
	write(w, 200, res)
}

// GET /v1/calc/employment?month=&industry=&wage=
func (s *Server) calcEmployment(w http.ResponseWriter, r *http.Request) {
	res, err := s.DS.CalcEmployment(r.URL.Query().Get("month"), r.URL.Query().Get("industry"), atoi(r, "wage"))
	if err != nil {
		bad(w, err)
		return
	}
	write(w, 200, res)
}
