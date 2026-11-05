package rates

import (
	"fmt"
	"sort"
)

// ReverseResult is the answer to "what gross pay gives this take-home?".
type ReverseResult struct {
	TargetNet    int64          `json:"target_net"`
	Gross        int64          `json:"gross"`          // 手取りが希望額以上になる最小の額面
	AchievedNet  int64          `json:"achieved_net"`   // その額面での手取り
	Exact        bool           `json:"exact"`          // 手取りがちょうど希望額になるか
	NetJustBelow int64          `json:"net_just_below"` // 額面を1円下げたときの手取り
	Payroll      *PayrollResult `json:"payroll"`
	Notes        []string       `json:"notes"`
}

// CalcGrossFromNet finds the smallest gross whose take-home is >= targetNet.
//
// Take-home is not strictly monotonic in gross: social insurance steps up with
// the 標準報酬月額 grade, so a 1-yen raise can lower the take-home. The search
// therefore evaluates a coarse grid plus both sides of every grade boundary,
// locates the first sample that reaches the target, and refines inside the
// preceding interval (where take-home is monotonic apart from 10-yen tax
// rounding jitter).
func (d *Dataset) CalcGrossFromNet(in PayrollInput, targetNet int64) (*ReverseResult, error) {
	if targetNet <= 0 || targetNet > 100_000_000 {
		return nil, fmt.Errorf("net は1〜1億の整数")
	}
	net := func(g int64) (int64, *PayrollResult, error) {
		x := in
		x.Gross = g
		r, err := d.CalcPayroll(x)
		if err != nil {
			return 0, nil, err
		}
		return r.NetPay, r, nil
	}

	lo := targetNet
	if in.NonTaxableCommute > lo {
		lo = in.NonTaxableCommute
	}
	hi := targetNet*3 + in.ResidentTax*2 + 100_000
	if hi > 1_000_000_000 {
		hi = 1_000_000_000
	}

	// サンプル点: 粗いグリッド + 等級の境界の前後
	pts := map[int64]bool{lo: true, hi: true}
	step := (hi - lo) / 4000 // 大きな金額でも評価回数を一定に保つ(境界は下で別に必ず入れる)
	if step < 500 {
		step = 500
	}
	for g := lo; g < hi; g += step {
		pts[g] = true
	}
	for _, gr := range d.HealthGrades {
		for _, g := range []int64{int64(gr.Lower) - 1, int64(gr.Lower)} {
			if g > lo && g < hi {
				pts[g] = true
			}
		}
	}
	xs := make([]int64, 0, len(pts))
	for g := range pts {
		xs = append(xs, g)
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })

	first := -1
	for i, g := range xs {
		n, _, err := net(g)
		if err != nil {
			return nil, err
		}
		if n >= targetNet {
			first = i
			break
		}
	}
	if first < 0 {
		return nil, fmt.Errorf("この条件では手取り %d 円に届く額面が見つかりません", targetNet)
	}

	// (前のサンプル, first] の中で、手取りが初めて目標に届く額面を二分探索
	l, h := lo-1, xs[first]
	if first > 0 {
		l = xs[first-1]
	}
	for h-l > 1 {
		m := (l + h) / 2
		n, _, _ := net(m)
		if n >= targetNet {
			h = m
		} else {
			l = m
		}
	}
	g := h
	// 税の10円丸めによる揺れを吸収: 直前の数十円も見直す
	for back := int64(1); back <= 30 && g-back >= lo; back++ {
		if n, _, _ := net(g - back); n >= targetNet {
			g -= back
			back = 0
		}
	}

	n, pr, err := net(g)
	if err != nil {
		return nil, err
	}
	below := int64(0)
	if g-1 >= 1 {
		if bn, _, e := net(g - 1); e == nil {
			below = bn
		}
	}
	res := &ReverseResult{TargetNet: targetNet, Gross: g, AchievedNet: n, Exact: n == targetNet, NetJustBelow: below, Payroll: pr}
	if !res.Exact {
		res.Notes = append(res.Notes, "手取りがちょうど希望額になる額面はありません(標準報酬月額の等級や税額の段階があるため)。希望額以上になる最小の額面を返しています。")
	}
	res.Notes = append(res.Notes, "社会保険料・雇用保険料・所得税・住民税を差し引いた手取りから逆算した、月額の目安です(賞与・年末調整なし)。")
	return res, nil
}
