package rates

import "fmt"

type PayrollInput struct {
	Month             string
	Prefecture        string
	Age               int
	Gross             int64 // 総支給額 (通勤手当を含む)
	NonTaxableCommute int64 // 非課税の通勤手当 (所得税の課税対象から除く)
	Industry          string
	Column            string // ko / otsu
	Dependents        int64
	ResidentTax       int64 // 住民税の月額(自治体の特別徴収通知額をそのまま入力)
	SkipSocial        bool  // 社会保険未加入
	SkipEmployment    bool  // 雇用保険未加入
}

type PayrollResult struct {
	Month            string             `json:"month"`
	Gross            int64              `json:"gross"`
	Social           *SocialResult      `json:"social,omitempty"`
	Employment       *EmploymentResult  `json:"employment,omitempty"`
	SocialWorker     int64              `json:"social_insurance_worker"`
	EmploymentWorker int64              `json:"employment_insurance_worker"`
	TaxableBase      int64              `json:"amount_after_social_insurance"`
	Withholding      *WithholdingResult `json:"withholding"`
	ResidentTax      int64              `json:"resident_tax"`
	TotalDeductions  int64              `json:"total_deductions"`
	NetPay           int64              `json:"net_pay"`
	EmployerBurden   int64              `json:"employer_insurance_burden"`
	Notes            []string           `json:"notes"`
}

// CalcPayroll combines social insurance, employment insurance and income tax
// withholding into a single monthly deduction / net-pay estimate.
func (d *Dataset) CalcPayroll(in PayrollInput) (*PayrollResult, error) {
	if in.Gross <= 0 || in.Gross > 1_000_000_000 {
		return nil, fmt.Errorf("gross は1〜10億の整数")
	}
	if in.Age < 0 || in.Age > 120 || in.Dependents < 0 || in.Dependents > 50 || in.ResidentTax < 0 || in.ResidentTax > in.Gross {
		return nil, fmt.Errorf("age / dependents / resident_tax の値が範囲外です")
	}
	if in.NonTaxableCommute < 0 || in.NonTaxableCommute > in.Gross {
		return nil, fmt.Errorf("non_taxable_commute は 0〜gross")
	}
	res := &PayrollResult{Month: in.Month, Gross: in.Gross, ResidentTax: in.ResidentTax}
	if !in.SkipSocial {
		s, err := d.CalcSocial(SocialInput{Month: in.Month, Prefecture: in.Prefecture,
			MonthlyRemuneration: int(in.Gross), Age: in.Age})
		if err != nil {
			return nil, err
		}
		res.Social, res.SocialWorker = s, s.WorkerTotal
		res.EmployerBurden += s.EmployerTotal
	}
	if !in.SkipEmployment {
		e, err := d.CalcEmployment(in.Month, in.Industry, int(in.Gross))
		if err != nil {
			return nil, err
		}
		res.Employment, res.EmploymentWorker = e, e.Worker.Amount
		res.EmployerBurden += e.Employer.Amount
	}
	res.TaxableBase = in.Gross - in.NonTaxableCommute - res.SocialWorker - res.EmploymentWorker
	if res.TaxableBase < 0 {
		res.TaxableBase = 0
	}
	w, err := d.CalcWithholding(in.Month, in.Column, res.TaxableBase, in.Dependents)
	if err != nil {
		return nil, err
	}
	res.Withholding = w
	res.TotalDeductions = res.SocialWorker + res.EmploymentWorker + w.Tax + in.ResidentTax
	res.NetPay = in.Gross - res.TotalDeductions
	res.Notes = []string{
		"住民税は自治体が通知する確定額を resident_tax に入力(計算はしない)。",
		"賞与・年末調整・各種手当の課税区分・労災保険は対象外。源泉所得税は電算機計算の特例による算出で、税額表の値と異なる場合がある。",
	}
	return res, nil
}
