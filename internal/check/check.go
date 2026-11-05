// Package check detects rate revisions by comparing the bundled dataset with
// the official publications (全国健康保険協会 / 厚生労働省).
package check

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"insurance-rates/internal/rates"
)

type Options struct {
	Now time.Time
	// StateHash is the previously stored fingerprint of the MHLW page ("" = first run).
	StateHash string
	// KyokaiURLFmt / MHLWURL are overridable for tests.
	KyokaiURLFmt string
	MHLWURL      string
	NTAURLFmt    string // %d = 西暦年
}

type Finding struct {
	Kind     string `json:"kind"`     // health | employment
	Severity string `json:"severity"` // info | warn
	Message  string `json:"message"`
	Hash     string `json:"hash,omitempty"`
}

const (
	defaultKyokai = "https://www.kyoukaikenpo.or.jp/about/business/insurance_rate/rate_prefectures/r%02d"
	defaultMHLW   = "https://www.mhlw.go.jp/stf/seisakunitsuite/bunya/0000108634.html"
)

var prefectures = []string{"北海道", "青森県", "岩手県", "宮城県", "秋田県", "山形県", "福島県", "茨城県", "栃木県", "群馬県", "埼玉県", "千葉県", "東京都", "神奈川県", "新潟県", "富山県", "石川県", "福井県", "山梨県", "長野県", "岐阜県", "静岡県", "愛知県", "三重県", "滋賀県", "京都府", "大阪府", "兵庫県", "奈良県", "和歌山県", "鳥取県", "島根県", "岡山県", "広島県", "山口県", "徳島県", "香川県", "愛媛県", "高知県", "福岡県", "佐賀県", "長崎県", "熊本県", "大分県", "宮崎県", "鹿児島県", "沖縄県"}

var (
	tagRe   = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]+>`)
	spaceRe = regexp.MustCompile(`\s+`)
	client  = &http.Client{Timeout: 30 * time.Second}
	careRe  = regexp.MustCompile(`介護保険料率[^0-9]{0,40}(\d\.\d{2})\s*%`)
)

func fetch(ctx context.Context, url string) (text string, status int, err error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "insurance-rates-checker/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	t := html.UnescapeString(tagRe.ReplaceAllString(string(b), " "))
	return strings.Join(strings.Fields(t), " "), resp.StatusCode, nil
}

func fiscalYear(now time.Time) int {
	if now.Month() < 3 { // 健保の年度は3月分から
		return now.Year() - 1
	}
	return now.Year()
}

// ParsePrefectureRates extracts "都道府県 ... 9.85" pairs from stripped page text.
// The official table lists "前年度率 ↓ 当年度率" per row in full-width digits, so the
// LAST rate in the row is the one for the fiscal year.
func ParsePrefectureRates(text string) map[string]float64 {
	text = normalizeWidth(text)
	out := map[string]float64{}
	for _, p := range prefectures {
		re := regexp.MustCompile(regexp.QuoteMeta(p) + `\s*(\d{1,2}\.\d{2})%(?:\s*\S\s*(\d{1,2}\.\d{2})%)?`)
		if m := re.FindStringSubmatch(text); m != nil {
			s := m[1]
			if m[2] != "" {
				s = m[2]
			}
			out[p], _ = strconv.ParseFloat(s, 64)
		}
	}
	return out
}

var widthRepl = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4",
	"５", "5", "６", "6", "７", "7", "８", "8", "９", "9", "．", ".", "％", "%")

func normalizeWidth(s string) string { return widthRepl.Replace(s) }

func Run(ctx context.Context, ds *rates.Dataset, o Options) []Finding {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.KyokaiURLFmt == "" {
		o.KyokaiURLFmt = defaultKyokai
	}
	if o.NTAURLFmt == "" {
		o.NTAURLFmt = "https://www.nta.go.jp/publication/pamph/gensen/aramashi%d/pdf/16.pdf"
	}
	if o.MHLWURL == "" {
		o.MHLWURL = defaultMHLW
	}
	var fs []Finding
	fy := fiscalYear(o.Now)

	// 1) 協会けんぽ: 今年度と来年度の都道府県別料率を公式ページと突き合わせる
	for _, y := range []int{fy, fy + 1} {
		url := fmt.Sprintf(o.KyokaiURLFmt, y-2018)
		month := fmt.Sprintf("%d-03", y)
		text, status, err := fetch(ctx, url)
		switch {
		case err != nil:
			fs = append(fs, Finding{"health", "warn", fmt.Sprintf("%d年度: 公式ページを取得できません: %v", y, err), ""})
			continue
		case status == http.StatusNotFound:
			fs = append(fs, Finding{"health", "info", fmt.Sprintf("%d年度(令和%d年度)の料率ページは未公開 (%s)", y, y-2018, url), ""})
			continue
		case status != http.StatusOK:
			fs = append(fs, Finding{"health", "warn", fmt.Sprintf("%s が HTTP %d", url, status), ""})
			continue
		}
		official := ParsePrefectureRates(text)
		if len(official) < 47 {
			fs = append(fs, Finding{"health", "warn", fmt.Sprintf("%d年度: 公式ページの解析に失敗 (%d/47件)。レイアウト変更の可能性: %s", y, len(official), url), ""})
			continue
		}
		hv, ok := ds.HealthAt(month)
		if !ok {
			fs = append(fs, Finding{"health", "warn", fmt.Sprintf("%d年度の料率が公開されていますが、データに未登録です: %s", y, url), ""})
			continue
		}
		var diffs []string
		for _, p := range prefectures {
			if hv.Rates[p] != official[p] {
				diffs = append(diffs, fmt.Sprintf("%s: 保有%.2f%% / 公式%.2f%%", p, hv.Rates[p], official[p]))
			}
		}
		if len(diffs) > 0 {
			fs = append(fs, Finding{"health", "warn", fmt.Sprintf("%d年度: 保有データと公式で %d 件相違 (適用版 %s): %s", y, len(diffs), hv.EffectiveFrom, strings.Join(diffs, ", ")), ""})
		} else {
			fs = append(fs, Finding{"health", "info", fmt.Sprintf("%d年度: 47都道府県すべて公式と一致 (%s)", y, hv.EffectiveFrom), ""})
		}
		if m := careRe.FindStringSubmatch(normalizeWidth(text)); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			if c, ok := ds.CareAt(month); ok && c.Rate != v {
				fs = append(fs, Finding{"health", "warn", fmt.Sprintf("%d年度: 介護保険料率 保有%.2f%% / 公式%.2f%%", y, c.Rate, v), ""})
			}
		}
	}

	// 2) 雇用保険: 厚労省ページの指紋変化と、次年度の記載有無を監視
	text, status, err := fetch(ctx, o.MHLWURL)
	if err != nil || status != http.StatusOK {
		fs = append(fs, Finding{"employment", "warn", fmt.Sprintf("厚労省ページを取得できません (status=%d err=%v): %s", status, err, o.MHLWURL), ""})
		return append(fs, checkWithholding(ctx, ds, o)...)
	}
	sum := sha256.Sum256([]byte(text))
	h := hex.EncodeToString(sum[:])
	switch {
	case o.StateHash == "":
		fs = append(fs, Finding{"employment", "info", "厚労省ページの指紋を初回記録しました", h})
	case o.StateHash != h:
		fs = append(fs, Finding{"employment", "warn", "厚労省の雇用保険料率ページの内容が変わりました。改定の有無を確認してください: " + o.MHLWURL, h})
	default:
		fs = append(fs, Finding{"employment", "info", "厚労省ページに変更なし", h})
	}
	nextFY := fy + 1
	if strings.Contains(text, fmt.Sprintf("令和%d年度", nextFY-2018)) {
		if ev, ok := ds.EmploymentAt(fmt.Sprintf("%d-04", nextFY)); !ok || ev.EffectiveFrom < fmt.Sprintf("%d-04", nextFY) {
			fs = append(fs, Finding{"employment", "warn", fmt.Sprintf("厚労省ページに令和%d年度の記載がありますが、%d-04 以降の雇用保険料率がデータにありません", nextFY-2018, nextFY), h})
		}
	}

	return append(fs, checkWithholding(ctx, ds, o)...)
}

// headPDF returns 200 only when the URL really serves a PDF. nta.go.jp answers
// missing pages with a redirect to an HTML error page that itself returns 200.
func headPDF(ctx context.Context, url string) (int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "insurance-rates-checker/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && !strings.Contains(resp.Header.Get("Content-Type"), "pdf") {
		return http.StatusNotFound, nil
	}
	return resp.StatusCode, nil
}

// 源泉所得税: 国税庁の「電算機計算の特例」PDFが来年分として公開されたか
func checkWithholding(ctx context.Context, ds *rates.Dataset, o Options) []Finding {
	var fs []Finding
	for _, y := range []int{o.Now.Year(), o.Now.Year() + 1} {
		url := fmt.Sprintf(o.NTAURLFmt, y)
		st, err := headPDF(ctx, url)
		if err != nil || st != http.StatusOK {
			fs = append(fs, Finding{"withholding", "info", fmt.Sprintf("令和%d年分の電算機計算の特例ページは未公開/取得不可 (status=%d)", y-2018, st), ""})
			continue
		}
		if wv, ok := ds.WithholdingAt(fmt.Sprintf("%d-12", y)); !ok || wv.EffectiveFrom < fmt.Sprintf("%d-01", y) {
			fs = append(fs, Finding{"withholding", "warn", fmt.Sprintf("国税庁が令和%d年分の源泉徴収税額表を公開していますが、%d-01 以降の源泉徴収データがありません: %s", y-2018, y, url), ""})
		} else {
			fs = append(fs, Finding{"withholding", "info", fmt.Sprintf("令和%d年分の源泉徴収データは登録済み (%s)", y-2018, wv.EffectiveFrom), ""})
		}
	}
	return fs
}

// HasWarnings reports whether any finding needs attention.
func HasWarnings(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == "warn" {
			return true
		}
	}
	return false
}
