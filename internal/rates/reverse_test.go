package rates

import "testing"

func tokyo() PayrollInput {
	return PayrollInput{Month: "2026-05", Prefecture: "東京都", Age: 35, Dependents: 1, NonTaxableCommute: 10000, ResidentTax: 15000}
}

func netAt(t *testing.T, d *Dataset, in PayrollInput, g int64) int64 {
	t.Helper()
	in.Gross = g
	r, err := d.CalcPayroll(in)
	if err != nil {
		t.Fatal(err)
	}
	return r.NetPay
}

// 順方向の結果を逆算すると、元の額面以下で同じ(以上の)手取りになる額面が見つかる。
func TestReverseRoundTrip(t *testing.T) {
	d := load(t)
	in := tokyo()
	for g := int64(150000); g <= 1_500_000; g += 7919 { // 等級の境界をまたぐように素数刻み
		target := netAt(t, d, in, g)
		r, err := d.CalcGrossFromNet(in, target)
		if err != nil {
			t.Fatalf("gross %d net %d: %v", g, target, err)
		}
		if r.Gross > g {
			t.Errorf("gross %d: reverse returned larger gross %d", g, r.Gross)
		}
		if r.AchievedNet < target {
			t.Errorf("gross %d: achieved %d < target %d", g, r.AchievedNet, target)
		}
		if r.Payroll.Gross != r.Gross {
			t.Errorf("payroll gross mismatch")
		}
	}
}

// 返された額面は「届く最小」: 1円下げると目標に届かない(税の10円丸めの揺れは許容)。
func TestReverseIsMinimal(t *testing.T) {
	d := load(t)
	in := tokyo()
	for _, target := range []int64{150000, 200000, 253022, 300000, 400000, 600000, 1000000} {
		r, err := d.CalcGrossFromNet(in, target)
		if err != nil {
			t.Fatal(err)
		}
		if r.AchievedNet < target {
			t.Errorf("target %d achieved %d", target, r.AchievedNet)
		}
		if r.NetJustBelow >= target {
			t.Errorf("target %d: gross-1 already reaches target (%d)", target, r.NetJustBelow)
		}
	}
}

// 等級の境界では額面を1円上げても手取りが下がる。逆算はその手前(小さい方)の額面を返す。
func TestReverseAtGradeBoundary(t *testing.T) {
	d := load(t)
	in := tokyo()
	checked := 0
	for _, gr := range d.HealthGrades[1:] {
		b := int64(gr.Lower)
		before, after := netAt(t, d, in, b-1), netAt(t, d, in, b)
		if after >= before {
			continue
		}
		checked++
		r, err := d.CalcGrossFromNet(in, before)
		if err != nil {
			t.Fatal(err)
		}
		if r.Gross > b-1 || !r.Exact {
			t.Errorf("boundary %d: target %d -> gross %d exact=%v", b, before, r.Gross, r.Exact)
		}
	}
	if checked < 10 {
		t.Errorf("expected many boundaries with a take-home dip, got %d", checked)
	}
}

func TestReverseErrors(t *testing.T) {
	d := load(t)
	in := tokyo()
	if _, err := d.CalcGrossFromNet(in, 0); err == nil {
		t.Error("net=0 must be rejected")
	}
	bad := in
	bad.Prefecture = "どこか"
	if _, err := d.CalcGrossFromNet(bad, 250000); err == nil {
		t.Error("unknown prefecture must be rejected")
	}
	if _, err := d.CalcGrossFromNet(in, 400_000_000); err != nil {
		t.Logf("very large target: %v", err) // 上限内なら計算できる/できなくても落ちない
	}
}
