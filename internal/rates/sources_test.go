package rates

import "testing"

func TestSplitSource(t *testing.T) {
	u, n := splitSource("https://www.mhlw.go.jp/file/x.pdf (平成28年度リーフレットの27年度欄)")
	if u != "https://www.mhlw.go.jp/file/x.pdf" || n != "平成28年度リーフレットの27年度欄" {
		t.Errorf("%q %q", u, n)
	}
	if u, n := splitSource("https://www.nenkin.go.jp/"); u != "https://www.nenkin.go.jp/" || n != "" {
		t.Errorf("%q %q", u, n)
	}
}

func TestPublisherOf(t *testing.T) {
	cases := map[string]string{
		"https://www.kyoukaikenpo.or.jp/about/x":                 "全国健康保険協会(協会けんぽ)",
		"https://www.nenkin.go.jp/service/a.html":                "日本年金機構",
		"https://www.mhlw.go.jp/content/001692566.pdf":           "厚生労働省",
		"https://jsite.mhlw.go.jp/tochigi-roudoukyoku/a.html":    "厚生労働省",
		"https://www.nta.go.jp/publication/pamph/gensen/a.pdf":   "国税庁",
		"https://warp.ndl.go.jp/20250513/www.nta.go.jp/gensen/x": "国立国会図書館 WARP(国税庁の保存版)",
	}
	for u, want := range cases {
		if got, _ := publisherOf(u); got != want {
			t.Errorf("%s: %q want %q", u, got, want)
		}
	}
}

func TestPayrollSources(t *testing.T) {
	d := load(t)
	in := tokyo()
	in.Gross = 320000
	r, err := d.CalcPayroll(in)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]Source{}
	for _, s := range r.Sources {
		keys[s.Key] = s
		if s.URL == "" || s.Publisher == "" || s.Title == "" {
			t.Errorf("incomplete source: %+v", s)
		}
	}
	for _, k := range []string{"health", "pension", "child_support", "child_contribution", "employment", "withholding"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("missing source %q", k)
		}
	}
	if _, ok := keys["care"]; ok {
		t.Error("35歳に介護保険の出典は不要")
	}
	if keys["health"].Publisher != "全国健康保険協会(協会けんぽ)" {
		t.Errorf("%+v", keys["health"])
	}
	// 45歳なら介護保険の出典が加わる
	in.Age = 45
	r, _ = d.CalcPayroll(in)
	found := false
	for _, s := range r.Sources {
		found = found || s.Key == "care"
	}
	if !found {
		t.Error("age 45 should cite care insurance")
	}
	// 2016年の月でも、その時点の版の出典になる
	in.Month, in.Age = "2016-04", 35
	r, _ = d.CalcPayroll(in)
	for _, s := range r.Sources {
		if s.EffectiveFrom > "2016-04" {
			t.Errorf("source newer than the month: %+v", s)
		}
	}
}
