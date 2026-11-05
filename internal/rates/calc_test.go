package rates

import "testing"

func load(t *testing.T) *Dataset {
	t.Helper()
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDatasetShape(t *testing.T) {
	d := load(t)
	if len(d.HealthGrades) != 50 {
		t.Fatalf("health grades = %d", len(d.HealthGrades))
	}
	if hv, _ := d.HealthAt("2026-03"); len(hv.Rates) != 47 {
		t.Fatalf("prefectures = %d", len(hv.Rates))
	}
}

func TestGradeBoundaries(t *testing.T) {
	d := load(t)
	cases := map[int]int{0: 58000, 62999: 58000, 63000: 68000, 92999: 88000, 93000: 98000,
		1354999: 1330000, 1355000: 1390000}
	for in, want := range cases {
		if got := d.GradeFor(in).Standard; got != want {
			t.Errorf("GradeFor(%d)=%d want %d", in, got, want)
		}
	}
}

func TestSocialTokyo300k(t *testing.T) {
	d := load(t)
	r, err := d.CalcSocial(SocialInput{Month: "2026-04", Prefecture: "東京都", StandardRemuneration: 300000, Age: 30})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Share{}
	for _, s := range r.Items {
		got[s.Name] = s
	}
	if s := got["健康保険"]; s.Total != 29550 || s.Worker != 14775 {
		t.Errorf("health %+v", s)
	}
	if s := got["厚生年金"]; s.Total != 54900 || s.Worker != 27450 {
		t.Errorf("pension %+v", s)
	}
	if s := got["子ども・子育て支援金"]; s.Total != 690 || s.Worker != 345 {
		t.Errorf("support %+v", s)
	}
	if s := got["子ども・子育て拠出金"]; s.Employer != 1080 {
		t.Errorf("contribution %+v", s)
	}
}

func TestSocialRounding(t *testing.T) {
	d := load(t)
	// 98,000 x 9.85% = 9,653 -> 折半 4,826.5 -> 本人4,826 / 事業主4,827
	r, _ := d.CalcSocial(SocialInput{Month: "2026-04", Prefecture: "東京都", StandardRemuneration: 98000})
	s := r.Items[0]
	if s.Total != 9653 || s.Worker != 4826 || s.Employer != 4827 {
		t.Errorf("%+v", s)
	}
}

func TestCareAndAge(t *testing.T) {
	d := load(t)
	r, _ := d.CalcSocial(SocialInput{Month: "2026-04", Prefecture: "東京都", StandardRemuneration: 300000, Age: 45})
	if !r.CareApplies || r.Items[1].Name != "介護保険" || r.Items[1].Total != 4860 {
		t.Errorf("%+v", r.Items)
	}
	r, _ = d.CalcSocial(SocialInput{Month: "2026-04", Prefecture: "東京都", StandardRemuneration: 300000, Age: 70})
	for _, s := range r.Items {
		if s.Name == "厚生年金" {
			t.Error("pension should be 0 at 70")
		}
	}
}

func TestPensionClamp(t *testing.T) {
	d := load(t)
	r, _ := d.CalcSocial(SocialInput{Month: "2026-04", Prefecture: "東京都", MonthlyRemuneration: 2000000})
	if r.HealthStandard != 1390000 || r.PensionStandard != 650000 {
		t.Errorf("%+v", r)
	}
	r, _ = d.CalcSocial(SocialInput{Month: "2026-04", Prefecture: "東京都", MonthlyRemuneration: 60000})
	if r.HealthStandard != 58000 || r.PensionStandard != 88000 {
		t.Errorf("%+v", r)
	}
}

func TestEmployment(t *testing.T) {
	d := load(t)
	r, err := d.CalcEmployment("2026-04", "general", 300000)
	if err != nil || r.Worker.Amount != 1500 || r.Employer.Amount != 2550 {
		t.Errorf("%+v %v", r, err)
	}
	r, _ = d.CalcEmployment("2026-03", "general", 300000) // 改定前 5.5‰
	if r.Worker.Amount != 1650 {
		t.Errorf("%+v", r)
	}
	// 50銭ルール: 10,001 x 0.5% = 50.005 -> 50
	r, _ = d.CalcEmployment("2026-04", "general", 10001)
	if r.Worker.Amount != 50 {
		t.Errorf("%+v", r)
	}
}

func TestVersionSelection(t *testing.T) {
	d := load(t)
	if v, _ := d.CareAt("2026-02"); v.Rate != 1.59 {
		t.Errorf("care 2026-02 = %v", v.Rate)
	}
	if v, _ := d.CareAt("2026-03"); v.Rate != 1.62 {
		t.Errorf("care 2026-03 = %v", v.Rate)
	}
	if _, ok := d.HealthAt("2016-02"); ok {
		t.Error("no health data before 2016-03 expected")
	}
	if v, _ := d.HealthAt("2026-02"); v.EffectiveFrom != "2025-03" || v.Rates["東京都"] != 9.91 {
		t.Errorf("2026-02 should use the 2025-03 version: %+v", v.EffectiveFrom)
	}
}
