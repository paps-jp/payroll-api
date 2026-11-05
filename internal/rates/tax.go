package rates

import (
	"fmt"
	"math"
)

// ---- data model (rates.json "withholding") ----

type WithholdingVersion struct {
	EffectiveFrom      string       `json:"effective_from"` // YYYY-MM (支払月)
	Source             string       `json:"source"`
	Verified           bool         `json:"verified"`
	Note               string       `json:"note,omitempty"`
	DependentDeduction int64        `json:"dependent_deduction"`
	SalaryDeduction    []SalaryTier `json:"salary_deduction"`
	BasicDeduction     []AmountTier `json:"basic_deduction"`
	KoBrackets         []Bracket    `json:"ko_brackets"`
	Otsu               OtsuTable    `json:"otsu"`
}

type SalaryTier struct {
	Upto        int64 `json:"upto,omitempty"` // 0 = 上限なし
	Amount      int64 `json:"amount,omitempty"`
	RatePercent int64 `json:"rate_percent,omitempty"`
	Add         int64 `json:"add,omitempty"`
}

type AmountTier struct {
	Upto   int64 `json:"upto,omitempty"`
	Amount int64 `json:"amount"`
}

type Bracket struct {
	Upto        int64   `json:"upto,omitempty"`
	RatePercent float64 `json:"rate_percent"`
	Sub         int64   `json:"sub"`
}

type OtsuTable struct {
	LowBelow       int64     `json:"low_below"`
	LowRatePercent float64   `json:"low_rate_percent"`
	TableMax       int64     `json:"table_max"`
	Steps          []Step    `json:"steps"`
	BaseBrackets   []Bracket `json:"base_brackets"`
	BaseBasic      int64     `json:"base_basic_deduction"`
	Mid            Over      `json:"mid"`
	High           Over      `json:"high"`
	SubDependent   int64     `json:"sub_dependent"`
}

type Step struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
	Step int64 `json:"step"`
}

type Over struct {
	From        int64   `json:"from"`
	Add         int64   `json:"add"`
	RatePercent float64 `json:"rate_percent"`
}

func (d *Dataset) WithholdingAt(month string) (*WithholdingVersion, bool) {
	i := pick(len(d.Withholding), func(i int) string { return d.Withholding[i].EffectiveFrom }, month)
	if i < 0 {
		return nil, false
	}
	return &d.Withholding[i], true
}

// ---- calculation ----

func ceilDiv(a, b int64) int64 { return (a + b - 1) / b }

func (w *WithholdingVersion) salaryDeduction(a int64) int64 {
	for _, t := range w.SalaryDeduction {
		if t.Upto == 0 || a <= t.Upto {
			if t.RatePercent == 0 {
				return t.Amount
			}
			return ceilDiv(a*t.RatePercent, 100) + t.Add // 1円未満切り上げ
		}
	}
	return 0
}

func (w *WithholdingVersion) basicDeduction(a int64) int64 {
	for _, t := range w.BasicDeduction {
		if t.Upto == 0 || a <= t.Upto {
			return t.Amount
		}
	}
	return 0
}

// bracketTax returns floor(b*rate - sub) in yen for the bracket containing b (b>0).
func bracketNum(brs []Bracket, b int64) (num int64) {
	for _, br := range brs {
		if br.Upto == 0 || b <= br.Upto {
			return b*ppm(br.RatePercent) - br.Sub*100000
		}
	}
	return 0
}

// kou computes 月額表甲欄 (電算機計算の特例). dependents = 源泉控除対象配偶者+源泉控除対象親族(+障害者等の加算分)。
func (w *WithholdingVersion) kou(a, dependents int64) (tax, taxable int64) {
	b := a - w.salaryDeduction(a) - w.basicDeduction(a) - w.DependentDeduction*dependents
	if b <= 0 {
		return 0, 0
	}
	return roundHalfUp(bracketNum(w.KoBrackets, b), 1000000) * 10, b // 10円未満四捨五入
}

func floorPositive(n, den int64) int64 {
	if n <= 0 {
		return 0
	}
	return n / den
}

// round to 100円: 50円未満切捨て, 50円以上100円未満は100円に切上げ
func round100(v int64) int64 { return roundHalfUp(v, 100) * 100 }

func (o *OtsuTable) baseAmount(a int64) int64 {
	if a == o.TableMax {
		return a
	}
	for _, s := range o.Steps {
		if a >= s.From && a <= s.To {
			return s.From + (a-s.From)/s.Step*s.Step
		}
	}
	return a
}

func (w *WithholdingVersion) otsuTaxAt(x int64) int64 { // 課税給与所得金額に対する税額(円未満切捨て)
	b := x - w.salaryDeduction(x) - w.Otsu.BaseBasic
	if b <= 0 {
		return 0
	}
	return floorPositive(bracketNum(w.Otsu.BaseBrackets, b), 100000)
}

// otsu computes 月額表乙欄. subDependents = 従たる給与の扶養控除等申告書による扶養親族等の数。
func (w *WithholdingVersion) otsu(a, subDependents int64) int64 {
	o := w.Otsu
	var tax int64
	switch {
	case a < o.LowBelow:
		tax = a * ppm(o.LowRatePercent) / 100000
	case a <= o.TableMax:
		base := o.baseAmount(a)
		ta := w.otsuTaxAt(base * 5 / 2)
		tb := w.otsuTaxAt(base * 3 / 2)
		c := round100(ta - tb)
		tax = round100(c * 1021 / 1000)
	case a < o.High.From:
		tax = o.Mid.Add + (a-o.Mid.From)*ppm(o.Mid.RatePercent)/100000
	default:
		tax = o.High.Add + (a-o.High.From)*ppm(o.High.RatePercent)/100000
	}
	tax -= o.SubDependent * subDependents
	if tax < 0 {
		tax = 0
	}
	return tax
}

type WithholdingResult struct {
	Month         string   `json:"month"`
	Column        string   `json:"column"`
	TaxableBase   int64    `json:"amount_after_social_insurance"`
	Dependents    int64    `json:"dependents"`
	TaxableIncome int64    `json:"taxable_salary_income,omitempty"`
	Tax           int64    `json:"tax"`
	Verified      bool     `json:"verified"`
	Source        string   `json:"source"`
	Notes         []string `json:"notes"`
}

// CalcWithholding computes 源泉所得税+復興特別所得税 on a monthly salary.
// amount = 社会保険料等控除後の給与等の金額; column = "ko" | "otsu".
func (d *Dataset) CalcWithholding(month, column string, amount, dependents int64) (*WithholdingResult, error) {
	if err := ValidMonth(month); err != nil {
		return nil, err
	}
	if amount < 0 || dependents < 0 {
		return nil, fmt.Errorf("amount / dependents は0以上")
	}
	w, ok := d.WithholdingAt(month)
	if !ok {
		return nil, fmt.Errorf("%s に適用できる源泉徴収税額データがありません", month)
	}
	r := &WithholdingResult{Month: month, Column: column, TaxableBase: amount, Dependents: dependents,
		Verified: w.Verified, Source: w.Source, Notes: []string{w.Note}}
	switch column {
	case "ko", "":
		r.Column = "ko"
		r.Tax, r.TaxableIncome = w.kou(amount, dependents)
		r.Notes = append(r.Notes, "甲欄: 扶養控除等申告書提出済み。dependents=源泉控除対象配偶者+源泉控除対象親族(本人の障害者・寡婦・ひとり親・勤労学生は各1人加算、障害のある扶養親族は1人につきさらに1人加算)。")
	case "otsu":
		r.Tax = w.otsu(amount, dependents)
		r.Notes = append(r.Notes, "乙欄: 扶養控除等申告書未提出(副業先など)。dependents=従たる給与の扶養控除等申告書の扶養親族等の数(1人につき1,610円控除)。")
	default:
		return nil, fmt.Errorf("column は ko / otsu: %q", column)
	}
	if amount > math.MaxInt32 {
		return nil, fmt.Errorf("amount が大きすぎます")
	}
	return r, nil
}
