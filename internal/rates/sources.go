package rates

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Source is one official publication that a calculation relied on, shaped for
// rendering as a citation ("参考文献").
type Source struct {
	Key           string `json:"key"`       // health / care / pension / child_support / child_contribution / employment / withholding
	Item          string `json:"item"`      // 表示名 (健康保険 など)
	Title         string `json:"title"`     // 何の率・どの時点か
	Publisher     string `json:"publisher"` // 発行元 (協会けんぽ など)
	URL           string `json:"url"`
	Host          string `json:"host"`
	EffectiveFrom string `json:"effective_from"` // 適用開始月 YYYY-MM
	Verified      bool   `json:"verified"`
	Note          string `json:"note,omitempty"`
}

// splitSource separates the URL from a trailing remark, e.g.
// "https://example/x.pdf (平成28年度リーフレットの27年度欄)".
func splitSource(s string) (u, note string) {
	s = strings.TrimSpace(s)
	f := strings.Fields(s)
	if len(f) == 0 || !strings.HasPrefix(f[0], "http") {
		return "", s
	}
	u = f[0]
	note = strings.TrimSpace(strings.TrimPrefix(s, f[0]))
	note = strings.Trim(note, "()（） ")
	return u, note
}

func publisherOf(rawURL string) (name, host string) {
	pu, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	host = strings.TrimPrefix(pu.Hostname(), "www.")
	switch {
	case strings.HasSuffix(host, "kyoukaikenpo.or.jp"):
		name = "全国健康保険協会(協会けんぽ)"
	case strings.HasSuffix(host, "nenkin.go.jp"):
		name = "日本年金機構"
	case strings.HasSuffix(host, "mhlw.go.jp"):
		name = "厚生労働省"
	case host == "warp.ndl.go.jp":
		name = "国立国会図書館 WARP(国税庁の保存版)"
	case strings.HasSuffix(host, "nta.go.jp"):
		name = "国税庁"
	default:
		name = host
	}
	return name, host
}

func jpMonth(m string) string { // "2026-03" -> "2026年3月"
	if len(m) != 7 {
		return m
	}
	mm, _ := strconv.Atoi(m[5:])
	return fmt.Sprintf("%s年%d月", m[:4], mm)
}

func trim(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func mkSource(key, item, title, rawURL, eff string, verified bool) Source {
	u, note := splitSource(rawURL)
	pub, host := publisherOf(u)
	return Source{Key: key, Item: item, Title: title, Publisher: pub, URL: u, Host: host, EffectiveFrom: eff, Verified: verified, Note: note}
}

// sourcesFor lists the publications behind the rates used in a payroll result.
func (d *Dataset) sourcesFor(in PayrollInput, month string, social *SocialResult, emp *EmploymentResult) []Source {
	var out []Source
	has := func(name string) bool {
		if social == nil {
			return false
		}
		for _, it := range social.Items {
			if it.Name == name {
				return true
			}
		}
		return false
	}
	if social != nil {
		if hv, ok := d.HealthAt(month); ok && has("健康保険") {
			out = append(out, mkSource("health", "健康保険",
				fmt.Sprintf("協会けんぽ 健康保険料率 %s %s%%(%s適用)", in.Prefecture, trim(hv.Rates[in.Prefecture]), jpMonth(hv.EffectiveFrom)),
				hv.Source, hv.EffectiveFrom, hv.Verified))
		}
		if cv, ok := d.CareAt(month); ok && has("介護保険") {
			out = append(out, mkSource("care", "介護保険", fmt.Sprintf("介護保険料率 %s%%(%s適用)", trim(cv.Rate), jpMonth(cv.EffectiveFrom)),
				cv.Source, cv.EffectiveFrom, cv.Verified))
		}
		if sv, ok := d.ChildSupportAt(month); ok && has("子ども・子育て支援金") {
			out = append(out, mkSource("child_support", "子ども・子育て支援金", fmt.Sprintf("子ども・子育て支援金率 %s%%(%s適用)", trim(sv.Rate), jpMonth(sv.EffectiveFrom)),
				sv.Source, sv.EffectiveFrom, sv.Verified))
		}
		if pv, ok := d.PensionAt(month); ok && has("厚生年金") {
			out = append(out, mkSource("pension", "厚生年金", fmt.Sprintf("厚生年金保険料率 %s%%(%s適用)", trim(pv.Rate), jpMonth(pv.EffectiveFrom)),
				pv.Source, pv.EffectiveFrom, pv.Verified))
		}
		if cc, ok := d.ChildContributionAt(month); ok && has("子ども・子育て拠出金") {
			out = append(out, mkSource("child_contribution", "子ども・子育て拠出金", fmt.Sprintf("子ども・子育て拠出金率 %s%%・会社のみ負担(%s適用)", trim(cc.Rate), jpMonth(cc.EffectiveFrom)),
				cc.Source, cc.EffectiveFrom, cc.Verified))
		}
	}
	if emp != nil {
		if ev, ok := d.EmploymentAt(month); ok {
			sp := ev.PerMille[emp.Industry]
			out = append(out, mkSource("employment", "雇用保険",
				fmt.Sprintf("雇用保険料率(%s) 労働者 %s‰・事業主 %s‰(%s適用)", Industries()[emp.Industry], trim(sp.Worker), trim(sp.Employer), jpMonth(ev.EffectiveFrom)),
				ev.Source, ev.EffectiveFrom, ev.Verified))
		}
	}
	if wv, ok := d.WithholdingAt(month); ok {
		col := "甲欄"
		if in.Column == "otsu" {
			col = "乙欄"
		}
		out = append(out, mkSource("withholding", "所得税",
			fmt.Sprintf("給与所得の源泉徴収税額(%s年分・月額表%s・電算機計算の特例)", wv.EffectiveFrom[:4], col),
			wv.Source, wv.EffectiveFrom, wv.Verified))
	}
	return out
}
