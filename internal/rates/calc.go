package rates

import (
	"fmt"
	"math"
	"regexp"
)

var monthRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

func ValidMonth(m string) error {
	if !monthRe.MatchString(m) {
		return fmt.Errorf("month は YYYY-MM 形式で指定してください: %q", m)
	}
	return nil
}

// ppm converts a percent value to units of 0.001% so integer math is exact.
func ppm(pct float64) int64 { return int64(math.Round(pct * 1000)) }

// 50銭以下切り捨て・50銭超切り上げ (給与天引きの端数処理)
func roundHalfDown(num, den int64) int64 {
	q, r := num/den, num%den
	if r*2 > den {
		q++
	}
	return q
}

// 四捨五入
func roundHalfUp(num, den int64) int64 {
	q, r := num/den, num%den
	if r*2 >= den {
		q++
	}
	return q
}

// GradeFor returns the health insurance grade for a 報酬月額.
func (d *Dataset) GradeFor(amount int) Grade {
	g := d.HealthGrades[0]
	for _, x := range d.HealthGrades {
		if amount >= x.Lower {
			g = x
		}
	}
	return g
}

// Share is one premium component, split between the employee and employer.
type Share struct {
	Name     string  `json:"name"`
	Rate     float64 `json:"rate_percent"`
	Base     int     `json:"base"`
	Total    int64   `json:"total"`
	Worker   int64   `json:"worker"`
	Employer int64   `json:"employer"`
}

// split halves a premium. worker: 50銭ルール, employer: total(四捨五入)-worker.
func split(name string, base int, ratePct float64) Share {
	num := int64(base) * ppm(ratePct)
	total := roundHalfUp(num, 100000)
	w := roundHalfDown(num, 200000)
	return Share{Name: name, Rate: ratePct, Base: base, Total: total, Worker: w, Employer: total - w}
}

type SocialInput struct {
	Month                string
	Prefecture           string
	MonthlyRemuneration  int // 報酬月額 (標準報酬月額を直接渡す場合は StandardRemuneration)
	StandardRemuneration int
	Age                  int // 0 = 未指定 (介護なし・年齢上限なし)
}

type SocialResult struct {
	Month               string   `json:"month"`
	Prefecture          string   `json:"prefecture"`
	HealthGrade         int      `json:"health_grade"`
	HealthStandard      int      `json:"health_standard"`
	PensionStandard     int      `json:"pension_standard"`
	CareApplies         bool     `json:"care_applies"`
	Items               []Share  `json:"items"`
	WorkerTotal         int64    `json:"worker_total"`
	EmployerTotal       int64    `json:"employer_total"`
	Notes               []string `json:"notes"`
	UnverifiedRateKinds []string `json:"unverified_rate_kinds,omitempty"`
}

func (d *Dataset) CalcSocial(in SocialInput) (*SocialResult, error) {
	if err := ValidMonth(in.Month); err != nil {
		return nil, err
	}
	hv, ok := d.HealthAt(in.Month)
	if !ok {
		return nil, fmt.Errorf("%s に適用できる健康保険料率データがありません", in.Month)
	}
	hr, ok := hv.Rates[in.Prefecture]
	if !ok {
		return nil, fmt.Errorf("都道府県が不明です: %q (例: 東京都)", in.Prefecture)
	}
	std := in.StandardRemuneration
	var g Grade
	if std > 0 {
		found := false
		for _, x := range d.HealthGrades {
			if x.Standard == std {
				g, found = x, true
			}
		}
		if !found {
			return nil, fmt.Errorf("標準報酬月額 %d は等級表にありません。報酬月額で指定してください", std)
		}
	} else if in.MonthlyRemuneration > 0 {
		g = d.GradeFor(in.MonthlyRemuneration)
	} else {
		return nil, fmt.Errorf("monthly_remuneration か standard_remuneration を指定してください")
	}
	res := &SocialResult{Month: in.Month, Prefecture: in.Prefecture, HealthGrade: g.Grade, HealthStandard: g.Standard}
	unv := map[string]bool{}
	mark := func(kind string, verified bool) {
		if !verified {
			unv[kind] = true
		}
	}
	mark("health", hv.Verified)

	age := in.Age
	if age < 75 { // 協会けんぽは75歳未満 (0=未指定を含む)
		care, hasCare := d.CareAt(in.Month)
		res.CareApplies = age >= 40 && age < 65 && hasCare
		if res.CareApplies {
			// 健保+介護を合算率で計算し、介護分は差分で表示する
			comb := split("health+care", g.Standard, hr+care.Rate)
			h := split("健康保険", g.Standard, hr)
			c := Share{Name: "介護保険", Rate: care.Rate, Base: g.Standard,
				Total: comb.Total - h.Total, Worker: comb.Worker - h.Worker, Employer: comb.Employer - h.Employer}
			res.Items = append(res.Items, h, c)
			mark("care", care.Verified)
		} else {
			res.Items = append(res.Items, split("健康保険", g.Standard, hr))
		}
		if cs, ok := d.ChildSupportAt(in.Month); ok {
			res.Items = append(res.Items, split("子ども・子育て支援金", g.Standard, cs.Rate))
			mark("child_support", cs.Verified)
		}
	} else {
		res.Notes = append(res.Notes, "75歳以上は後期高齢者医療のため健康保険料は0")
	}

	ps := g.Standard
	if ps < d.PensionRange.MinStandard {
		ps = d.PensionRange.MinStandard
	}
	if ps > d.PensionRange.MaxStandard {
		ps = d.PensionRange.MaxStandard
	}
	res.PensionStandard = ps
	if age >= 70 {
		res.Notes = append(res.Notes, "70歳以上は厚生年金保険料と子ども・子育て拠出金は0")
	} else if pr, ok := d.PensionAt(in.Month); ok {
		res.Items = append(res.Items, split("厚生年金", ps, pr.Rate))
		mark("pension", pr.Verified)
		if cc, ok := d.ChildContributionAt(in.Month); ok {
			total := roundHalfUp(int64(ps)*ppm(cc.Rate), 100000)
			res.Items = append(res.Items, Share{Name: "子ども・子育て拠出金", Rate: cc.Rate, Base: ps, Total: total, Employer: total})
			mark("child_contribution", cc.Verified)
		}
	}
	for _, s := range res.Items {
		res.WorkerTotal += s.Worker
		res.EmployerTotal += s.Employer
	}
	for k := range unv {
		res.UnverifiedRateKinds = append(res.UnverifiedRateKinds, k)
	}
	if !d.GradesVerified {
		res.UnverifiedRateKinds = append(res.UnverifiedRateKinds, "grade_table")
	}
	res.Notes = append(res.Notes,
		"協会けんぽ管掌のみ。本人=50銭以下切捨て/50銭超切上げ、事業主=総額(四捨五入)-本人。",
		"介護保険は40歳以上65歳未満(年齢到達月の厳密な判定は呼び出し側で)。賞与・随時改定・産休免除等は対象外。")
	return res, nil
}

type Rate1 struct {
	PerMille float64 `json:"per_mille"`
	Amount   int64   `json:"amount"`
}

type EmploymentResult struct {
	Month    string   `json:"month"`
	Industry string   `json:"industry"`
	Wage     int      `json:"wage"`
	Worker   Rate1    `json:"worker"`
	Employer Rate1    `json:"employer"`
	Verified bool     `json:"verified"`
	Notes    []string `json:"notes"`
}

var industries = map[string]string{"general": "一般の事業", "agri": "農林水産・清酒製造の事業", "construction": "建設の事業"}

func Industries() map[string]string { return industries }

func (d *Dataset) CalcEmployment(month, industry string, wage int) (*EmploymentResult, error) {
	if err := ValidMonth(month); err != nil {
		return nil, err
	}
	if industry == "" {
		industry = "general"
	}
	if _, ok := industries[industry]; !ok {
		return nil, fmt.Errorf("industry は general / agri / construction のいずれか: %q", industry)
	}
	if wage < 0 {
		return nil, fmt.Errorf("wage は0以上")
	}
	ev, ok := d.EmploymentAt(month)
	if !ok {
		return nil, fmt.Errorf("%s に適用できる雇用保険料率データがありません", month)
	}
	s := ev.PerMille[industry]
	// 1‰ = 0.1% = 100 ppm(0.001%)。 amount = wage * permille*100 / 100000
	amt := func(pm float64) int64 {
		return roundHalfDown(int64(wage)*int64(math.Round(pm*100)), 100000)
	}
	return &EmploymentResult{Month: month, Industry: industry, Wage: wage,
		Worker: Rate1{s.Worker, amt(s.Worker)}, Employer: Rate1{s.Employer, amt(s.Employer)}, Verified: ev.Verified,
		Notes: []string{"賃金総額(通勤手当等を含む)に乗じる。端数は50銭以下切捨て。適用は賃金の締切日基準。労災保険(事業主全額)は対象外。"}}, nil
}
