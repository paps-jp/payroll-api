package rates

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// 国税庁「令和8年分 源泉徴収税額表」電算機計算の特例の計算例
func TestKouOfficialExamples(t *testing.T) {
	d := load(t)
	cases := []struct {
		a, dep, want int64
	}{
		{175000, 2, 210},   // 配偶者+親族1
		{446000, 8, 940},   // 配偶者+親族7
		{775200, 3, 59470}, // 配偶者+親族2
	}
	for _, c := range cases {
		r, err := d.CalcWithholding("2026-05", "ko", c.a, c.dep)
		if err != nil || r.Tax != c.want {
			t.Errorf("ko(%d,%d)=%+v err=%v want %d", c.a, c.dep, r, err, c.want)
		}
	}
}

func TestKouEdges(t *testing.T) {
	d := load(t)
	// 控除が課税給与を上回れば0
	if r, _ := d.CalcWithholding("2026-05", "ko", 100000, 0); r.Tax != 0 {
		t.Errorf("%+v", r)
	}
	// 高額帯でも落ちない
	if r, _ := d.CalcWithholding("2026-05", "ko", 5000000, 0); r.Tax <= 0 {
		t.Errorf("%+v", r)
	}
}

func TestOtsu(t *testing.T) {
	d := load(t)
	// 手計算: base=299,000 → Ⓐ71,708 Ⓑ19,254 Ⓒ→52,500 ×1.021→53,600
	if r, _ := d.CalcWithholding("2026-05", "otsu", 300000, 0); r.Tax != 53600 {
		t.Errorf("otsu 300000 = %d", r.Tax)
	}
	// 105,000未満: 3.063%
	if r, _ := d.CalcWithholding("2026-05", "otsu", 100000, 0); r.Tax != 3063 {
		t.Errorf("otsu 100000 = %d", r.Tax)
	}
	// 740,000超: 259,200 + 超過分×40.84%
	if r, _ := d.CalcWithholding("2026-05", "otsu", 800000, 0); r.Tax != 259200+60000*40840/100000 {
		t.Errorf("otsu 800000 = %d", r.Tax)
	}
	// 従たる給与の扶養控除: 1人1,610円、0未満にならない
	if r, _ := d.CalcWithholding("2026-05", "otsu", 100000, 100); r.Tax != 0 {
		t.Errorf("%+v", r)
	}
}

func TestWithholdingBeforeData(t *testing.T) {
	d := load(t)
	if _, err := d.CalcWithholding("2015-12", "ko", 300000, 0); err == nil {
		t.Error("expected error before 2016-01")
	}
}

// 国税庁『あらまし』『税額表』に載っている計算例・税額表の値を全年分再現できること(2016〜2025年分)。
func TestWithholdingHistoryExamples(t *testing.T) {
	b, err := os.ReadFile("testdata/withholding_examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Examples []struct {
			Year        int    `json:"year"`
			Column      string `json:"column"`
			Amount      int64  `json:"amount"`
			Dependents  int64  `json:"dependents"`
			ExpectedTax int64  `json:"expected_tax"`
		} `json:"examples"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Examples) < 200 {
		t.Fatalf("examples = %d", len(f.Examples))
	}
	d := load(t)
	for _, e := range f.Examples {
		r, err := d.CalcWithholding(fmt.Sprintf("%d-06", e.Year), e.Column, e.Amount, e.Dependents)
		if err != nil || r.Tax != e.ExpectedTax {
			t.Errorf("%d %s amount=%d dep=%d: got %+v err=%v want %d", e.Year, e.Column, e.Amount, e.Dependents, r, err, e.ExpectedTax)
		}
	}
}
