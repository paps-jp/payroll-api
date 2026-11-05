// Command sitegen builds the static, crawlable pages of the GitHub Pages site
// (docs/) from the same rate data and calculation engine the API uses, so every
// number on those pages is produced by tested code instead of typed by hand.
//
//	go run ./cmd/sitegen [-docs docs] [-date 2026-11-06]
//
// It writes: kenpo/ koyo/ tedori/ how-to/ sources/ (index.html each),
// sitemap.xml, and refreshes the marked blocks in docs/index.html.
package main

import (
	"flag"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"insurance-rates/internal/rates"
)

const (
	base     = "https://paps-jp.github.io/payroll-api/"
	apiBase  = "https://payroll-api.paps.jp"
	siteName = "payroll-api"
	refMonth = "2026-04" // 令和8年度の基準月
)

var (
	ds   *rates.Dataset
	date time.Time
)

func main() {
	docs := flag.String("docs", "docs", "output directory (GitHub Pages root)")
	dateStr := flag.String("date", time.Now().Format("2006-01-02"), "last-updated date (YYYY-MM-DD)")
	flag.Parse()
	d, err := time.Parse("2006-01-02", *dateStr)
	must(err)
	must(generate(*docs, d))
}

// generate writes every generated page under docs and refreshes docs/index.html.
func generate(docs string, d time.Time) error {
	date = d
	var err error
	if ds, err = rates.Load(); err != nil {
		return err
	}
	pages := []Page{kenpoPage(), koyoPage(), tedoriPage(), howtoPage(), sourcesPage()}
	for _, p := range pages {
		out := filepath.Join(docs, p.Slug, "index.html")
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, []byte(layout(p)), 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", out)
	}
	if err := os.WriteFile(filepath.Join(docs, "sitemap.xml"), []byte(sitemap(pages)), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote sitemap.xml")
	patchIndex(filepath.Join(docs, "index.html"))
	return nil
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// ---------- formatting helpers ----------

var esc = html.EscapeString

func yen(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
func pct(f float64) string { return num(f) + "%" }
func jpDate(t time.Time) string {
	return fmt.Sprintf("%d年%d月%d日", t.Year(), int(t.Month()), t.Day())
}
func jpYM(m string) string { // 2026-03 -> 2026年3月
	y, _ := strconv.Atoi(m[:4])
	mm, _ := strconv.Atoi(m[5:])
	return fmt.Sprintf("%d年%d月", y, mm)
}

// fy: 協会けんぽの年度名(3月分〜)。2026年3月分 = 令和8年度。
func fy(year int) string {
	switch {
	case year >= 2020:
		return fmt.Sprintf("令和%d年度", year-2018)
	case year == 2019:
		return "平成31(令和元)年度"
	default:
		return fmt.Sprintf("平成%d年度", year-1988)
	}
}

func signed(d float64) string {
	switch {
	case d > 0.0001:
		return `<span class="up">+` + fmt.Sprintf("%.2f", d) + `</span>`
	case d < -0.0001:
		return `<span class="down">` + fmt.Sprintf("%.2f", d) + `</span>`
	}
	return "±0"
}

func payroll(gross int64, dep int64, extra ...func(*rates.PayrollInput)) *rates.PayrollResult {
	in := rates.PayrollInput{Month: refMonth, Prefecture: "東京都", Age: 35, Gross: gross, Dependents: dep, Industry: "general", Column: "ko"}
	for _, f := range extra {
		f(&in)
	}
	r, err := ds.CalcPayroll(in)
	if err != nil {
		log.Fatal(err)
	}
	return r
}

func healthAt(m string) *rates.HealthVersion {
	v, ok := ds.HealthAt(m)
	if !ok {
		log.Fatalf("no health data for %s", m)
	}
	return v
}

// ---------- page model + layout ----------

type QA struct{ Q, A string }

type Page struct {
	Slug, Title, Desc, H1, Crumb string
	Answer                       string // 冒頭の結論(HTML)
	Body                         string
	FAQ                          []QA
	Dataset                      string // JSON-LD の Dataset 名(空なら出さない)
	DatasetDesc                  string
	Range                        string // temporalCoverage
}

func nav(prefix, cur string) string {
	items := [][2]string{{"", "電卓"}, {"kenpo/", "協会けんぽ料率"}, {"koyo/", "雇用保険料率"}, {"tedori/", "手取り早見表"}, {"how-to/", "計算方法"}, {"sources/", "出典"}}
	var b strings.Builder
	for _, it := range items {
		a := ""
		if it[0] == cur {
			a = ` aria-current="page"`
		}
		fmt.Fprintf(&b, `<a href="%s%s"%s>%s</a>`, prefix, it[0], a, it[1])
	}
	return b.String()
}

func footer(prefix string) string {
	return `<footer><div class="wrap">
  <div class="f-main"><span class="f-name">payroll-api</span><span>社会保険・雇用保険・源泉所得税の料率 API / MCP</span></div>
  <nav class="f-links" aria-label="関連リンク">
    <a href="` + prefix + `">手取り電卓</a><a href="` + prefix + `kenpo/">協会けんぽ料率</a><a href="` + prefix + `koyo/">雇用保険料率</a><a href="` + prefix + `tedori/">手取り早見表</a><a href="` + prefix + `how-to/">計算方法</a><a href="` + prefix + `sources/">出典と検証</a>
    <a href="https://paps-jp.github.io/">ぱっぷすラボ</a><a href="https://www.paps.jp">ぱっぷす ホームページ</a><a href="https://github.com/paps-jp/payroll-api">GitHub</a>
  </nav>
</div></footer>`
}

func ldJSON(v string) string { return `<script type="application/ld+json">` + v + `</script>` }

func jstr(s string) string { // JSON string literal
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "<", `<`, ">", `>`, "&", `&`)
	return `"` + r.Replace(s) + `"`
}

func plain(htmlStr string) string { // FAQ 回答などのタグを除去
	var b strings.Builder
	in := false
	for _, r := range htmlStr {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return html.UnescapeString(b.String())
}

func layout(p Page) string {
	url := base + p.Slug + "/"
	iso := date.Format("2006-01-02")
	pub := `{"@type":"Organization","name":"ぱっぷすラボ(PAPS-Lab)","url":"https://paps-jp.github.io/"}`
	var ld []string
	ld = append(ld, fmt.Sprintf(`{"@context":"https://schema.org","@type":"Article","headline":%s,"description":%s,"url":%s,"inLanguage":"ja","datePublished":"2026-11-06","dateModified":"%s","author":%s,"publisher":%s,"image":%s,"mainEntityOfPage":%s}`,
		jstr(p.H1), jstr(p.Desc), jstr(url), iso, pub, pub, jstr(base+"images/calculator-result.jpg"), jstr(url)))
	ld = append(ld, fmt.Sprintf(`{"@context":"https://schema.org","@type":"BreadcrumbList","itemListElement":[{"@type":"ListItem","position":1,"name":"payroll-api(手取り電卓)","item":%s},{"@type":"ListItem","position":2,"name":%s,"item":%s}]}`,
		jstr(base), jstr(p.Crumb), jstr(url)))
	if len(p.FAQ) > 0 {
		var q []string
		for _, f := range p.FAQ {
			q = append(q, fmt.Sprintf(`{"@type":"Question","name":%s,"acceptedAnswer":{"@type":"Answer","text":%s}}`, jstr(f.Q), jstr(plain(f.A))))
		}
		ld = append(ld, `{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[`+strings.Join(q, ",")+`]}`)
	}
	if p.Dataset != "" {
		ld = append(ld, fmt.Sprintf(`{"@context":"https://schema.org","@type":"Dataset","name":%s,"description":%s,"url":%s,"inLanguage":"ja","isAccessibleForFree":true,"temporalCoverage":%s,"creator":%s,"keywords":["社会保険料率","雇用保険料率","源泉所得税","給与計算","手取り"],"distribution":[{"@type":"DataDownload","encodingFormat":"application/json","contentUrl":%s}]}`,
			jstr(p.Dataset), jstr(p.DatasetDesc), jstr(url), jstr(p.Range), pub, jstr(apiBase+"/v1/rates?month="+refMonth)))
	}
	var lds strings.Builder
	for _, l := range ld {
		lds.WriteString(ldJSON(l) + "\n")
	}
	var faq strings.Builder
	if len(p.FAQ) > 0 {
		faq.WriteString("<h2 id=\"faq\">よくある質問</h2>\n")
		for _, f := range p.FAQ {
			fmt.Fprintf(&faq, "<details class=\"q\" open><summary>%s</summary><p>%s</p></details>\n", esc(f.Q), f.A)
		}
	}
	return `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + esc(p.Title) + `</title>
<meta name="description" content="` + esc(p.Desc) + `">
<link rel="canonical" href="` + url + `">
<meta property="og:type" content="article">
<meta property="og:title" content="` + esc(p.Title) + `">
<meta property="og:description" content="` + esc(p.Desc) + `">
<meta property="og:url" content="` + url + `">
<meta property="og:image" content="` + base + `images/calculator-result.jpg">
<meta property="og:site_name" content="payroll-api">
<meta name="twitter:card" content="summary_large_image">
<link rel="stylesheet" href="../assets/site.css">
` + lds.String() + `</head>
<body>
<div class="bar"><div class="wrap">
  <a class="logo" href="../"><b><svg class="i" viewBox="0 0 24 24"><rect x="5" y="3" width="14" height="18" rx="2.5"/><path d="M8.5 7.5h7"/><path d="M8.5 12h.01M12 12h.01M15.5 12h.01M8.5 16h.01M12 16h.01M15.5 16h.01"/></svg></b>payroll-api</a>
  <nav>` + nav("../", p.Slug+"/") + `</nav>
</div></div>
<main class="wrap">
<nav class="crumb" aria-label="パンくず"><a href="../">手取り電卓</a> › ` + esc(p.Crumb) + `</nav>
<h1>` + esc(p.H1) + `</h1>
<p class="meta">最終更新: ` + jpDate(date) + ` · 出典: 公式資料(協会けんぽ・日本年金機構・厚生労働省・国税庁) · <a href="../sources/">出典と検証方法</a></p>
<div class="answer">` + p.Answer + `</div>
<a class="cta" href="../">自分の条件で手取りを計算する →</a>
` + p.Body + `
` + faq.String() + `
<div class="note"><b>ご利用上の注意</b>掲載内容は公式資料をもとにした目安です。実際の給与計算では、最新の公式資料で確認してください。組合健保・共済は対象外で、賞与・年末調整も含みません。</div>
</main>
` + footer("../") + `
</body>
</html>
`
}

func sitemap(pages []Page) string {
	iso := date.Format("2006-01-02")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	add := func(path, pri string) {
		fmt.Fprintf(&b, "  <url><loc>%s%s</loc><lastmod>%s</lastmod><changefreq>monthly</changefreq><priority>%s</priority></url>\n", base, path, iso, pri)
	}
	add("", "1.0")
	for _, p := range pages {
		add(p.Slug+"/", "0.8")
	}
	b.WriteString("</urlset>\n")
	return b.String()
}

func related(prefix string, skip string) string {
	items := [][3]string{
		{"kenpo/", "協会けんぽ 都道府県別 保険料率", "令和8年度の47都道府県と2016年からの推移"},
		{"koyo/", "雇用保険料率の推移", "令和8年度は一般の事業で13.5/1,000"},
		{"tedori/", "手取り早見表", "月給・年収別の手取り額(東京都)"},
		{"how-to/", "手取りの計算方法", "額面から引かれる項目を、数字つきで解説"},
		{"sources/", "出典と検証方法", "公式資料の一覧と、確認のしかた"},
	}
	var b strings.Builder
	b.WriteString(`<h2>あわせて読みたい</h2><div class="links">`)
	for _, it := range items {
		if it[0] == skip {
			continue
		}
		fmt.Fprintf(&b, `<a href="%s%s">%s<small>%s</small></a>`, prefix, it[0], it[1], it[2])
	}
	b.WriteString(`<a href="` + prefix + `">手取り電卓<small>月給・年収から自動計算。手取り→額面の逆算も</small></a></div>`)
	return b.String()
}

func srcLink(raw string) string {
	f := strings.Fields(raw)
	if len(f) == 0 {
		return ""
	}
	return `<a href="` + esc(f[0]) + `" rel="noopener">` + esc(f[0]) + `</a>`
}

// ---------- 協会けんぽ ----------

func kenpoPage() Page {
	h7, h8 := healthAt("2025-03"), healthAt("2026-03")
	minP, maxP := "", ""
	up, down, same := 0, 0, 0
	var sameNames []string
	for _, p := range rates.Prefectures {
		if minP == "" || h8.Rates[p] < h8.Rates[minP] {
			minP = p
		}
		if maxP == "" || h8.Rates[p] > h8.Rates[maxP] {
			maxP = p
		}
		switch d := h8.Rates[p] - h7.Rates[p]; {
		case d > 0.0001:
			up++
		case d < -0.0001:
			down++
		default:
			same++
			sameNames = append(sameNames, strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(p, "県"), "府"), "都"))
		}
	}
	care8, _ := ds.CareAt("2026-03")
	sup, _ := ds.ChildSupportAt("2026-04")
	r := payroll(300000, 0, func(in *rates.PayrollInput) { in.Age = 30 })
	var health rates.Share
	for _, s := range r.Social.Items {
		if s.Name == "健康保険" {
			health = s
		}
	}

	var b strings.Builder
	b.WriteString(`<h2 id="list">令和8年度(2026年度) 都道府県別 健康保険料率一覧</h2>
<p>協会けんぽの健康保険料率は、都道府県ごとに毎年改定されます。下の表は令和7年度(2025年3月分〜)と令和8年度(2026年3月分〜)の料率と、労使折半した本人負担の率です。</p>
<div class="tw"><table><tr><th>都道府県</th><th>令和7年度</th><th>令和8年度</th><th>増減</th><th>本人負担(折半)</th></tr>`)
	for _, p := range rates.Prefectures {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td><b>%s</b></td><td>%s</td><td>%s</td></tr>", p, pct(h7.Rates[p]), pct(h8.Rates[p]), signed(h8.Rates[p]-h7.Rates[p]), pct(h8.Rates[p]/2))
	}
	fmt.Fprintf(&b, `</table></div>
<p>令和8年度は、47都道府県のうち<b>%d都道府県で引下げ、%d県(%s)は据置き</b>で、引上げはありませんでした。最も低いのは%s(%s)、最も高いのは%s(%s)です。</p>`,
		down, same, strings.Join(sameNames, "・"), minP, pct(h8.Rates[minP]), maxP, pct(h8.Rates[maxP]))

	b.WriteString(`<h2 id="care">介護保険料率・子ども・子育て支援金率</h2>
<p>介護保険料率は<b>全国一律</b>で、40歳以上65歳未満の被保険者だけが、健康保険料率に上乗せして負担します。令和8年度(2026年4月分〜)からは、子ども・子育て支援金(全国一律0.23%、労使折半)も加わりました。</p>
<div class="tw"><table><tr><th>年度</th><th>適用開始</th><th>介護保険料率</th></tr>`)
	for i := len(ds.Care) - 1; i >= 0; i-- {
		c := ds.Care[i]
		y, _ := strconv.Atoi(c.EffectiveFrom[:4])
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s分〜</td><td>%s</td></tr>", fy(y), jpYM(c.EffectiveFrom), pct(c.Rate))
	}
	fmt.Fprintf(&b, `</table></div>
<p>子ども・子育て支援金率は %s(%s分〜、労使折半)です。</p>`, pct(sup.Rate), jpYM(sup.EffectiveFrom))

	b.WriteString(`<h2 id="history">年度別の推移(平成28年度〜令和8年度)</h2>
<p>各年度とも3月分(4月納付分)から適用される料率です。横にスクロールできます。</p>
<div class="tw"><table><tr><th>都道府県</th>`)
	for _, v := range ds.Health {
		y, _ := strconv.Atoi(v.EffectiveFrom[:4])
		fmt.Fprintf(&b, "<th>%d<br><small>%s</small></th>", y, strings.TrimSuffix(fy(y), "年度"))
	}
	b.WriteString("</tr>")
	for _, p := range rates.Prefectures {
		fmt.Fprintf(&b, "<tr><td>%s</td>", p)
		for _, v := range ds.Health {
			fmt.Fprintf(&b, "<td>%s</td>", num(v.Rates[p]))
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table></div>\n<p>単位は%です。2016年(平成28年度)の値は、公式ページ1か所のみで確認した値です。</p>")

	fmt.Fprintf(&b, `<h2 id="example">計算例:東京都・標準報酬月額30万円</h2>
<p>東京都(%s)で標準報酬月額が30万円の場合、健康保険料の総額は 300,000円 × %s = <b>%s円</b>、本人負担(折半)は<b>%s円</b>です(端数は50銭以下切捨て・50銭超切上げ)。40〜64歳の方は、これに介護保険料(%s)がかかります。</p>`,
		pct(h8.Rates["東京都"]), pct(h8.Rates["東京都"]), yen(health.Total), yen(health.Worker), pct(care8.Rate))

	b.WriteString(`<h2 id="source">出典</h2><ul class="src-list">`)
	for i := len(ds.Health) - 1; i >= 0; i-- {
		v := ds.Health[i]
		y, _ := strconv.Atoi(v.EffectiveFrom[:4])
		tag := ""
		if !v.Verified {
			tag = ` <span class="tag">単独出典</span>`
		}
		fmt.Fprintf(&b, "<li>%s(%s分〜): %s%s</li>", fy(y), jpYM(v.EffectiveFrom), srcLink(v.Source), tag)
	}
	b.WriteString("</ul>")
	b.WriteString(related("../", "kenpo/"))

	tk := h8.Rates["東京都"]
	return Page{
		Slug: "kenpo", Crumb: "協会けんぽ 都道府県別 保険料率",
		Title: "協会けんぽ 令和8年度(2026年)都道府県別 健康保険料率一覧と推移【2016年〜】",
		Desc: fmt.Sprintf("協会けんぽの令和8年度(2026年3月分〜)の都道府県別 健康保険料率を一覧で掲載。東京都%s・大阪府%s・愛知県%s。介護保険料率%s、子ども・子育て支援金率%s。平成28年度からの推移つき。",
			pct(tk), pct(h8.Rates["大阪府"]), pct(h8.Rates["愛知県"]), pct(care8.Rate), pct(sup.Rate)),
		H1: "協会けんぽ 令和8年度(2026年) 都道府県別 健康保険料率一覧",
		Answer: fmt.Sprintf(`<b>令和8年度(2026年3月分〜)の協会けんぽの健康保険料率は、東京都 %s、大阪府 %s、愛知県 %s、北海道 %s</b> など、都道府県ごとに異なります。47都道府県で最も低いのは%s(%s)、最も高いのは%s(%s)で、%d都道府県が前年度から引下げ、%d県が据置きでした。介護保険料率(40〜64歳のみ)は全国一律%sです。`,
			pct(tk), pct(h8.Rates["大阪府"]), pct(h8.Rates["愛知県"]), pct(h8.Rates["北海道"]), minP, pct(h8.Rates[minP]), maxP, pct(h8.Rates[maxP]), down, same, pct(care8.Rate)),
		Body: b.String(),
		FAQ: []QA{
			{"令和8年度の東京都の協会けんぽの健康保険料率は?", fmt.Sprintf("令和8年度(2026年3月分〜)の東京都の健康保険料率は<b>%s</b>です(令和7年度は%s)。労使折半で、本人負担は%sです。", pct(tk), pct(h7.Rates["東京都"]), pct(tk/2))},
			{"健康保険料率が一番高い・低い都道府県はどこですか?", fmt.Sprintf("令和8年度は、最も高いのが%s(%s)、最も低いのが%s(%s)です。", maxP, pct(h8.Rates[maxP]), minP, pct(h8.Rates[minP]))},
			{"令和8年度は保険料率が上がりましたか?", fmt.Sprintf("上がった都道府県はありません。%d都道府県で引下げ、%d県(%s)は据置きでした。", down, same, strings.Join(sameNames, "・"))},
			{"介護保険料率はいくらですか?", fmt.Sprintf("令和8年度の介護保険料率は全国一律<b>%s</b>です(令和7年度は1.59%%)。40歳以上65歳未満の被保険者が、健康保険料率に上乗せして負担します。", pct(care8.Rate))},
			{"標準報酬月額30万円の場合、健康保険料はいくらですか?(東京都)", fmt.Sprintf("東京都の場合、総額は%s円、本人負担は<b>%s円</b>です(40歳未満・令和8年度)。", yen(health.Total), yen(health.Worker))},
		},
		Dataset: "協会けんぽ 都道府県別 健康保険料率(平成28年度〜令和8年度)", Range: "2016-03/2027-02",
		DatasetDesc: "全国健康保険協会(協会けんぽ)の都道府県単位保険料率と介護保険料率を、年度ごとに整理したデータ。",
	}
}

// ---------- 雇用保険 ----------

func koyoPage() Page {
	e8, _ := ds.EmploymentAt("2026-04")
	e7, _ := ds.EmploymentAt("2025-04")
	g8, g7 := e8.PerMille["general"], e7.PerMille["general"]
	rate := func(pm float64) string { return num(pm) + "/1,000(" + pct(pm/10) + ")" }

	table := func(key, title string) string {
		var b strings.Builder
		fmt.Fprintf(&b, `<h3>%s</h3><div class="tw"><table><tr><th>適用開始</th><th>労働者負担</th><th>事業主負担</th><th>合計</th></tr>`, title)
		for i := len(ds.Employment) - 1; i >= 0; i-- {
			e := ds.Employment[i]
			s := e.PerMille[key]
			cls := ""
			if e.EffectiveFrom == "2026-04" {
				cls = ` class="hl"`
			}
			fmt.Fprintf(&b, "<tr%s><td>%s〜</td><td>%s</td><td>%s</td><td>%s</td></tr>", cls, jpYM(e.EffectiveFrom), rate(s.Worker), rate(s.Employer), rate(s.Worker+s.Employer))
		}
		b.WriteString("</table></div>")
		return b.String()
	}

	r := payroll(300000, 0, func(in *rates.PayrollInput) { in.SkipSocial = true })
	var b strings.Builder
	b.WriteString(`<h2 id="rate">雇用保険料率の推移(2015年〜)</h2>
<p>雇用保険料率は、原則として毎年4月1日に改定されます(令和4年度は4月と10月の2回)。労働者負担の分は、毎月の給与から天引きされます。事業主負担には、事業主だけが負担する「雇用保険二事業」の分が含まれます。</p>`)
	b.WriteString(table("general", "一般の事業"))
	b.WriteString(table("agri", "農林水産・清酒製造の事業"))
	b.WriteString(table("construction", "建設の事業"))
	b.WriteString(`<p>園芸サービス、牛馬の育成、酪農、養鶏、養豚、内水面養殖および特定の船員を雇用する事業には、一般の事業の率が適用されます。</p>`)
	fmt.Fprintf(&b, `<h2 id="example">計算例:月給30万円(一般の事業)</h2>
<p>令和8年度(2026年4月〜)に月給(賃金総額)が30万円の場合、労働者負担は 300,000円 × %s = <b>%s円</b>、事業主負担は 300,000円 × %s = <b>%s円</b>です(労働者負担の端数は50銭以下切捨て)。</p>`,
		pct(g8.Worker/10), yen(r.Employment.Worker.Amount), pct(g8.Employer/10), yen(r.Employment.Employer.Amount))
	b.WriteString(`<h2 id="source">出典</h2><ul class="src-list">`)
	for i := len(ds.Employment) - 1; i >= 0; i-- {
		e := ds.Employment[i]
		fmt.Fprintf(&b, "<li>%s〜: %s</li>", jpYM(e.EffectiveFrom), srcLink(e.Source))
	}
	b.WriteString("</ul>")
	b.WriteString(related("../", "koyo/"))

	return Page{
		Slug: "koyo", Crumb: "雇用保険料率の推移",
		Title: "雇用保険料率の推移【令和8年度(2026年度)は一般の事業で13.5/1,000】",
		Desc: fmt.Sprintf("令和8年度(2026年4月〜)の雇用保険料率は、一般の事業で労働者負担%s、事業主負担%s、合計%s。農林水産・清酒製造、建設の事業と、2015年からの推移・計算例を掲載。",
			rate(g8.Worker), rate(g8.Employer), rate(g8.Worker+g8.Employer)),
		H1: "雇用保険料率の推移(令和8年度は一般の事業で13.5/1,000)",
		Answer: fmt.Sprintf(`<b>令和8年度(2026年4月1日〜2027年3月31日)の雇用保険料率は、一般の事業で、労働者負担 %s、事業主負担 %s、合計 %s</b> です。令和7年度(労働者%s・事業主%s)から、労使とも0.5/1,000ずつ引き下げられました。`,
			rate(g8.Worker), rate(g8.Employer), rate(g8.Worker+g8.Employer), rate(g7.Worker), rate(g7.Employer)),
		Body: b.String(),
		FAQ: []QA{
			{"令和8年度の雇用保険料率はいくらですか?", fmt.Sprintf("一般の事業の場合、労働者負担が%s、事業主負担が%s、合計が%sです。", rate(g8.Worker), rate(g8.Employer), rate(g8.Worker+g8.Employer))},
			{"令和7年度から何が変わりましたか?", fmt.Sprintf("一般の事業で、労働者負担が%s→%s、事業主負担が%s→%sに、それぞれ0.5/1,000引き下げられました。", rate(g7.Worker), rate(g8.Worker), rate(g7.Employer), rate(g8.Employer))},
			{"月給30万円の場合、雇用保険料はいくらですか?", fmt.Sprintf("令和8年度の一般の事業では、労働者負担は<b>%s円</b>、事業主負担は%s円です。", yen(r.Employment.Worker.Amount), yen(r.Employment.Employer.Amount))},
			{"建設業の雇用保険料率は?", fmt.Sprintf("令和8年度の建設の事業は、労働者負担%s、事業主負担%s、合計%sです。", rate(e8.PerMille["construction"].Worker), rate(e8.PerMille["construction"].Employer), rate(e8.PerMille["construction"].Worker+e8.PerMille["construction"].Employer))},
		},
		Dataset: "雇用保険料率の推移(2015年〜)", Range: "2015-04/2027-03",
		DatasetDesc: "厚生労働省が公表する雇用保険料率(一般・農林水産・清酒製造・建設の事業)の改定履歴。",
	}
}

// ---------- 手取り早見表 ----------

func tedoriPage() Page {
	row := func(g, dep int64) string {
		r := payroll(g, dep)
		return fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td><b>%s</b></td><td>%.1f%%</td></tr>",
			yen(g), yen(r.SocialWorker), yen(r.EmploymentWorker), yen(r.Withholding.Tax), yen(r.NetPay), float64(r.NetPay)/float64(g)*100)
	}
	head := `<div class="tw"><table><tr><th>月給(額面)</th><th>社会保険料</th><th>雇用保険料</th><th>所得税</th><th>手取り</th><th>手取り率</th></tr>`
	monthly := func(dep int64) string {
		var b strings.Builder
		b.WriteString(head)
		for g := int64(150000); g <= 600000; g += 10000 {
			b.WriteString(row(g, dep))
		}
		for g := int64(650000); g <= 1000000; g += 50000 {
			b.WriteString(row(g, dep))
		}
		b.WriteString("</table></div>")
		return b.String()
	}

	var yr strings.Builder
	yr.WriteString(`<div class="tw"><table><tr><th>年収</th><th>月給(年収÷12)</th><th>手取り月額</th><th>手取り年収</th><th>手取り率</th></tr>`)
	for y := int64(3000000); y <= 10000000; y += 500000 {
		g := (y + 6) / 12
		r := payroll(g, 0)
		fmt.Fprintf(&yr, "<tr><td>%s</td><td>%s</td><td>%s</td><td><b>%s</b></td><td>%.1f%%</td></tr>", yen(y), yen(g), yen(r.NetPay), yen(r.NetPay*12), float64(r.NetPay)/float64(g)*100)
	}
	yr.WriteString("</table></div>")

	r30 := payroll(300000, 0)
	r30b := payroll(300000, 0, func(in *rates.PayrollInput) { in.Age = 45 })
	g500 := int64(5000000+6) / 12
	r500 := payroll(g500, 0)
	r30d := payroll(300000, 1)

	var b strings.Builder
	b.WriteString(`<h2 id="assume">この表の前提</h2>
<ul>
<li>令和8年度(2026年4月〜)の料率、会社は<b>東京都</b>の協会けんぽ・一般の事業、<b>35歳</b>(介護保険なし)</li>
<li>給与は月額で、賞与なし。通勤手当はすべて課税扱いで入力(非課税の通勤手当があると、所得税は少し下がります)</li>
<li>所得税は源泉徴収(甲欄・扶養親族の数は表ごとに指定)</li>
<li><b>住民税は含めていません。</b>住民税は前年の所得をもとに市区町村が決めるため、通知額を別に差し引いてください(<a href="../">電卓</a>では住民税を入力できます)</li>
</ul>`)
	b.WriteString(`<h2 id="monthly0">月給別 手取り早見表(扶養なし・独身)</h2>` + monthly(0))
	b.WriteString(`<h2 id="monthly1">月給別 手取り早見表(扶養1人・配偶者あり)</h2><p>源泉控除対象配偶者または扶養親族が1人いる場合です。所得税が下がるため、手取りが少し増えます。</p>` + monthly(1))
	b.WriteString(`<h2 id="yearly">年収別 手取り早見表(扶養なし・ボーナスなし)</h2><p>年収を12か月で均等に割った月給で計算した目安です。賞与がある場合は、年収が同じでも手取りは少し変わります。</p>` + yr.String())
	b.WriteString(`<h2 id="reverse">手取りから額面を逆算したいとき</h2>
<p>「手取りを25万円にしたい」「手取り年収500万円には額面がいくら必要か」といった逆算は、<a href="../">電卓</a>の「手取りから額面を逆算」タブでできます。API(<code>/v1/calc/payroll/reverse</code>)からも使えます。</p>`)
	b.WriteString(related("../", "tedori/"))

	return Page{
		Slug: "tedori", Crumb: "手取り早見表",
		Title: "手取り早見表【令和8年度・東京都】月給15万〜100万円・年収300万〜1000万円",
		Desc:  fmt.Sprintf("月給別・年収別の手取り早見表(令和8年度・東京都・住民税を除く)。月給30万円の手取りは約%s円、年収500万円の手取りは約%s円。社会保険料・雇用保険料・所得税の内訳つき。", yen(r30.NetPay), yen(r500.NetPay*12)),
		H1:    "手取り早見表(令和8年度・東京都)月給別・年収別",
		Answer: fmt.Sprintf(`<b>月給30万円(東京都・35歳・扶養なし)の手取りは約%s円</b>(額面の%.1f%%)。内訳は 社会保険料%s円・雇用保険料%s円・所得税%s円です(令和8年度、住民税を除く)。<b>年収500万円(ボーナスなし)の手取り年収は約%s円</b>です。`,
			yen(r30.NetPay), float64(r30.NetPay)/3000, yen(r30.SocialWorker), yen(r30.EmploymentWorker), yen(r30.Withholding.Tax), yen(r500.NetPay*12)),
		Body: b.String(),
		FAQ: []QA{
			{"月給30万円の手取りはいくらですか?", fmt.Sprintf("東京都・35歳・扶養なし・令和8年度の場合、手取りは約<b>%s円</b>です(住民税を除く)。社会保険料%s円、雇用保険料%s円、所得税%s円が引かれます。扶養が1人いると、所得税が下がり手取りは約%s円になります。",
				yen(r30.NetPay), yen(r30.SocialWorker), yen(r30.EmploymentWorker), yen(r30.Withholding.Tax), yen(r30d.NetPay))},
			{"年収500万円の手取りはいくらですか?", fmt.Sprintf("ボーナスなし・東京都・35歳・扶養なし・令和8年度で、手取り年収は約<b>%s円</b>(月額約%s円)です。住民税は含まれていません。", yen(r500.NetPay*12), yen(r500.NetPay))},
			{"40歳を過ぎると手取りは減りますか?", fmt.Sprintf("40歳以上65歳未満は介護保険料がかかるため、手取りが減ります。月給30万円の場合、35歳の手取り%s円に対し、45歳では%s円(約%s円減)です。", yen(r30.NetPay), yen(r30b.NetPay), yen(r30.NetPay-r30b.NetPay))},
			{"この手取りに住民税は含まれていますか?", "含まれていません。住民税は前年の所得をもとに市区町村が決め、毎年6月ごろに通知されます。通知の月額を、表の手取りから差し引いてください。"},
		},
		Dataset: "月給別・年収別 手取り早見表(令和8年度・東京都)", Range: "2026-04/2027-03",
		DatasetDesc: "公式の料率と国税庁の源泉徴収税額の計算式から算出した、月給別・年収別の手取り額(東京都・協会けんぽ・一般の事業)。",
	}
}

// ---------- 計算方法 ----------

func howtoPage() Page {
	r := payroll(320000, 1, func(in *rates.PayrollInput) { in.NonTaxableCommute = 10000; in.ResidentTax = 15000 })
	items := map[string]rates.Share{}
	for _, it := range r.Social.Items {
		items[it.Name] = it
	}
	h, sp, pn := items["健康保険"], items["子ども・子育て支援金"], items["厚生年金"]
	rev, err := ds.CalcGrossFromNet(rates.PayrollInput{Month: refMonth, Prefecture: "東京都", Age: 35, Dependents: 1, Industry: "general", Column: "ko"}, 250000)
	if err != nil {
		log.Fatal(err)
	}
	w := r.Withholding

	var b strings.Builder
	b.WriteString(`<h2 id="items">額面から引かれる5つの項目</h2>
<p>給与の手取りは、<b>額面(総支給額)から、次の5つを引いた金額</b>です。</p>
<div class="tw"><table><tr><th class="txt">項目</th><th class="txt">何に使われるか</th><th class="txt">決まり方</th></tr>
<tr><td class="txt">健康保険料(・介護保険料)</td><td class="txt">医療費の自己負担を軽くする保険。40〜64歳は介護保険料も</td><td class="txt">標準報酬月額 × 都道府県ごとの率 ÷ 2</td></tr>
<tr><td class="txt">厚生年金保険料</td><td class="txt">将来の年金</td><td class="txt">標準報酬月額 × 18.3% ÷ 2</td></tr>
<tr><td class="txt">雇用保険料</td><td class="txt">失業・育児休業などの給付</td><td class="txt">賃金総額 × 労働者負担率</td></tr>
<tr><td class="txt">所得税(源泉徴収)</td><td class="txt">国に納める税金(年末調整で精算)</td><td class="txt">課税対象額を国税庁の算式に当てはめる</td></tr>
<tr><td class="txt">住民税</td><td class="txt">住んでいる自治体に納める税金</td><td class="txt">前年の所得をもとに市区町村が決定</td></tr></table></div>`)

	fmt.Fprintf(&b, `<h2 id="example">計算の例:月給32万円(東京都・35歳・扶養1人)</h2>
<p>令和8年度(%s)の料率で、非課税の通勤手当が1万円、住民税の通知額が月1万5千円の場合を、順番に計算します。</p>
<ol class="steps">
<li><b>額面を確認する</b>総支給額は <b>%s円</b>(通勤手当1万円を含む)。</li>
<li><b>社会保険料(本人分)を計算する</b>給与を「標準報酬月額」という区分に当てはめます。この例では等級%d・標準報酬月額 %s円です。<br>
健康保険: %s円 × %s ÷ 2 = <b>%s円</b><br>
子ども・子育て支援金: %s円 × %s ÷ 2 = <b>%s円</b><br>
厚生年金: %s円 × %s ÷ 2 = <b>%s円</b><br>
(40〜64歳は、さらに介護保険料がかかります)<br>
社会保険料の合計: <b>%s円</b></li>
<li><b>雇用保険料を計算する</b>%s円 × %s = <b>%s円</b>(一般の事業の労働者負担)。</li>
<li><b>所得税を計算する</b>課税の対象 = 額面 %s円 − 非課税の通勤手当 %s円 − 社会保険料 %s円 − 雇用保険料 %s円 = <b>%s円</b>。これを国税庁の「月額表(甲欄)」の算式に当てはめ、扶養1人として所得税は <b>%s円</b>。</li>
<li><b>住民税を引く</b>市区町村の通知額 <b>%s円</b>。</li>
<li><b>手取りを求める</b>%s円 − (%s円 + %s円 + %s円 + %s円) = <b>%s円</b>。</li>
</ol>`,
		"2026年4月〜", yen(r.Gross), r.Social.HealthGrade, yen(int64(r.Social.HealthStandard)),
		yen(int64(h.Base)), pct(h.Rate), yen(h.Worker),
		yen(int64(sp.Base)), pct(sp.Rate), yen(sp.Worker),
		yen(int64(pn.Base)), pct(pn.Rate), yen(pn.Worker),
		yen(r.SocialWorker),
		yen(r.Gross), pct(r.Employment.Worker.PerMille/10), yen(r.EmploymentWorker),
		yen(r.Gross), yen(10000), yen(r.SocialWorker), yen(r.EmploymentWorker), yen(r.TaxableBase), yen(w.Tax),
		yen(r.ResidentTax),
		yen(r.Gross), yen(r.SocialWorker), yen(r.EmploymentWorker), yen(w.Tax), yen(r.ResidentTax), yen(r.NetPay))

	fmt.Fprintf(&b, `<p>この例では、額面 %s円に対して手取りは <b>%s円</b>(額面の%.1f%%)です。会社は、これとは別に社会保険料・雇用保険料などで%s円を負担します。</p>`,
		yen(r.Gross), yen(r.NetPay), float64(r.NetPay)/float64(r.Gross)*100, yen(r.EmployerBurden))

	fmt.Fprintf(&b, `<h2 id="reverse">手取りから額面を逆算する方法</h2>
<p>反対に「手取りを○円にしたい」ときは、額面を少しずつ変えながら、手取りがその金額以上になる最小の額面を探します。社会保険料は標準報酬月額の等級ごとに段階的に変わるため、額面を1円上げただけで手取りが減る境目があります。</p>
<p>例えば、東京都・35歳・扶養1人・令和8年度で、<b>手取りを25万円にするために必要な額面は約%s円</b>です(住民税を除く)。<a href="../">電卓</a>の「手取りから額面を逆算」タブか、API(<code>/v1/calc/payroll/reverse</code>)で、条件を変えて調べられます。</p>`, yen(rev.Gross))
	b.WriteString(related("../", "how-to/"))

	return Page{
		Slug: "how-to", Crumb: "手取りの計算方法",
		Title: "手取りの計算方法|額面から引かれる5つの項目を、数字つきで解説【令和8年度】",
		Desc:  fmt.Sprintf("手取り=額面−社会保険料−雇用保険料−所得税−住民税。月給32万円(東京都・35歳・扶養1人)の例で、各項目の計算式と金額を順番に解説。手取りは%s円。手取りから額面の逆算方法も。", yen(r.NetPay)),
		H1:    "手取りの計算方法:額面から引かれる5つの項目(令和8年度)",
		Answer: fmt.Sprintf(`<b>手取り = 額面 − 社会保険料 − 雇用保険料 − 所得税 − 住民税</b>。月給32万円(東京都・35歳・扶養1人・令和8年度)なら、社会保険料%s円、雇用保険料%s円、所得税%s円、住民税(例)%s円を引いて、<b>手取りは%s円</b>です。`,
			yen(r.SocialWorker), yen(r.EmploymentWorker), yen(w.Tax), yen(r.ResidentTax), yen(r.NetPay)),
		Body: b.String(),
		FAQ: []QA{
			{"手取りの計算式を教えてください。", "手取り = 額面(総支給額) − 社会保険料(健康保険・介護保険・厚生年金など) − 雇用保険料 − 所得税 − 住民税 です。社会保険料は標準報酬月額に率をかけて労使折半、雇用保険料は賃金に率をかけ、所得税は国税庁の算式、住民税は自治体の通知額を使います。"},
			{"額面と手取りの違いは?", "額面は保険料や税金を引く前の総支給額、手取りは引いたあとに実際に振り込まれる金額です。一般に手取りは額面の約8割になります。"},
			{"手取りから額面を逆算できますか?", fmt.Sprintf("できます。希望の手取りになる最小の額面を探します。例:東京都・35歳・扶養1人・令和8年度で手取り25万円なら、額面は約%s円です(住民税を除く)。", yen(rev.Gross))},
			{"社会保険料はなぜ「標準報酬月額」で決まるのですか?", "毎月の給与は変動するため、給与を区分(等級)に当てはめた「標準報酬月額」を使って、保険料を一定の期間そろえて計算する仕組みだからです。等級が変わる境目では、保険料が段階的に増えます。"},
		},
	}
}

// ---------- 出典と検証 ----------

func sourcesPage() Page {
	var b strings.Builder
	b.WriteString(`<h2 id="how">データの作り方と検証</h2>
<ul>
<li><b>出典は公式資料のみ</b>: 協会けんぽ、日本年金機構、厚生労働省、国税庁の公表資料を使っています。個々の率に出典URLと「確認済みか」の印を付けています(API の <code>/v1/revisions</code> でも取得できます)。</li>
<li><b>複数の資料で突き合わせ</b>: 協会けんぽの料率は、隣り合う年度のページ(前年度の欄と当年度の欄)で相互に照合しています。令和8年度の雇用保険料率は、厚生労働省の公式PDFと全区分で一致を確認しています。</li>
<li><b>源泉所得税は240件の検算</b>: 国税庁の『源泉徴収のあらまし』『税額表』に載っている計算例・税額表の値240件を自動テストにかけ、平成28年分〜令和8年分のすべてで再現できることを確認しています。</li>
<li><b>毎週の自動チェック</b>: 協会けんぽ・厚生労働省・国税庁の公開状況を毎週確認し、データとの相違や、新しい年度の公開があれば検知します。</li>
<li><b>確認が弱い項目は明示</b>: 標準報酬月額の等級の境界額、平成28年度(2016年3月分)の健康保険料率、子ども・子育て拠出金率の2015年4月・2016年4月は、出典が1つ、または間接的なため、「確認中」として扱っています。</li>
</ul>`)

	b.WriteString(`<h2 id="list">出典一覧</h2><div class="tw"><table><tr><th class="txt">種別</th><th class="txt">適用開始</th><th class="txt">内容</th><th class="txt">出典</th></tr>`)
	row := func(kind, from, content, src string, verified bool) {
		tag := ""
		if !verified {
			tag = ` <span class="tag">確認中</span>`
		}
		fmt.Fprintf(&b, `<tr><td class="txt">%s</td><td class="txt">%s〜</td><td class="txt">%s%s</td><td class="txt">%s</td></tr>`, kind, jpYM(from), content, tag, srcLink(src))
	}
	for i := len(ds.Health) - 1; i >= 0; i-- {
		v := ds.Health[i]
		row("健康保険(協会けんぽ)", v.EffectiveFrom, "47都道府県(東京都 "+pct(v.Rates["東京都"])+")", v.Source, v.Verified)
	}
	for i := len(ds.Care) - 1; i >= 0; i-- {
		v := ds.Care[i]
		row("介護保険", v.EffectiveFrom, pct(v.Rate), v.Source, v.Verified)
	}
	for i := len(ds.Pension) - 1; i >= 0; i-- {
		v := ds.Pension[i]
		row("厚生年金", v.EffectiveFrom, pct(v.Rate), v.Source, v.Verified)
	}
	for i := len(ds.ChildSupport) - 1; i >= 0; i-- {
		v := ds.ChildSupport[i]
		row("子ども・子育て支援金", v.EffectiveFrom, pct(v.Rate), v.Source, v.Verified)
	}
	for i := len(ds.ChildContribution) - 1; i >= 0; i-- {
		v := ds.ChildContribution[i]
		row("子ども・子育て拠出金(会社のみ)", v.EffectiveFrom, pct(v.Rate), v.Source, v.Verified)
	}
	for i := len(ds.Employment) - 1; i >= 0; i-- {
		v := ds.Employment[i]
		g := v.PerMille["general"]
		row("雇用保険", v.EffectiveFrom, "一般の事業 労働者"+num(g.Worker)+"/1,000・事業主"+num(g.Employer)+"/1,000", v.Source, v.Verified)
	}
	for i := len(ds.Withholding) - 1; i >= 0; i-- {
		v := ds.Withholding[i]
		row("源泉所得税", v.EffectiveFrom, v.EffectiveFrom[:4]+"年分 月額表(甲欄・乙欄)", v.Source, v.Verified)
	}
	b.WriteString("</table></div>")

	b.WriteString(`<h2 id="scope">対象範囲と注意</h2>
<ul>
<li>協会けんぽ管掌の健康保険のみです。組合健保・共済組合は対象外です。</li>
<li>賞与、随時改定、産休・育休中の免除、労災保険、年末調整、日額表は未対応です。</li>
<li>令和2〜7年分の源泉所得税(乙欄)は、国税庁の算式が月額表(税額表)と約29万〜43.7万円付近の一部の行で最大数千円ずれます(算式のほうが大きい)。本サービスは算式どおりに計算します。</li>
<li>計算結果は目安です。法令や公式の発表が優先されます。</li>
</ul>
<h2 id="about">運営</h2>
<p>payroll-api は、<a href="https://paps-jp.github.io/">ぱっぷすラボ(PAPS-Lab)</a>が公開しているツールです。API・MCP は無料・登録不要で、ソースコードは <a href="https://github.com/paps-jp/payroll-api">GitHub</a> にあります。ぱっぷす全体については <a href="https://www.paps.jp">ぱっぷす ホームページ</a> をご覧ください。</p>
<h2 id="history">更新履歴</h2>
<ul>
<li>` + jpDate(date) + `: 協会けんぽ・雇用保険・源泉所得税の履歴(2016年〜)を整理し、解説ページを公開</li>
</ul>`)
	b.WriteString(related("../", "sources/"))

	return Page{
		Slug: "sources", Crumb: "出典と検証方法",
		Title:  "出典と検証方法|社会保険料率・雇用保険料率・源泉所得税のデータについて | payroll-api",
		Desc:   "payroll-api が使う社会保険料率・雇用保険料率・源泉所得税のデータの出典(協会けんぽ・日本年金機構・厚生労働省・国税庁)と、検証方法・対象範囲・注意点を公開しています。",
		H1:     "出典と検証方法(データの信頼性について)",
		Answer: `<b>すべての料率は、協会けんぽ・日本年金機構・厚生労働省・国税庁の公式資料</b>をもとにしています。源泉所得税は国税庁の計算例と税額表の値<b>240件</b>を自動テストで再現し、料率は毎週、公式ページと自動で照合しています。確認が弱い項目は「確認中」と表示します。`,
		Body:   b.String(),
	}
}

// ---------- index.html の差し込みブロック ----------

func patchIndex(path string) {
	b, err := os.ReadFile(path)
	must(err)
	s := string(b)
	replace := func(start, end, content string) {
		i, j := strings.Index(s, start), strings.Index(s, end)
		if i < 0 || j < 0 || j < i {
			log.Fatalf("markers %s ... %s not found in %s", start, end, path)
		}
		s = s[:i+len(start)] + "\n" + content + "\n" + s[j:]
	}

	h8 := healthAt("2026-03")
	care8, _ := ds.CareAt("2026-03")
	pn, _ := ds.PensionAt("2026-04")
	sup, _ := ds.ChildSupportAt("2026-04")
	e8, _ := ds.EmploymentAt("2026-04")
	g := e8.PerMille["general"]
	r30 := payroll(300000, 0)

	pub := `{"@type":"Organization","name":"ぱっぷすラボ(PAPS-Lab)","url":"https://paps-jp.github.io/"}`
	ld := fmt.Sprintf(`{"@context":"https://schema.org","@type":"WebApplication","name":"手取り計算(額面⇔手取り)| payroll-api","url":%s,"applicationCategory":"BusinessApplication","operatingSystem":"Web","inLanguage":"ja","isAccessibleForFree":true,"offers":{"@type":"Offer","price":"0","priceCurrency":"JPY"},"description":%s,"image":%s,"dateModified":"%s","publisher":%s}`,
		jstr(base), jstr("月給や年収から、社会保険料・雇用保険料・所得税・住民税を引いた手取りを自動計算。希望の手取りから必要な額面を逆算することもできます。2016年からの公式料率に対応。"),
		jstr(base+"images/calculator-result.jpg"), date.Format("2006-01-02"), pub)
	ld2 := fmt.Sprintf(`{"@context":"https://schema.org","@type":"WebSite","name":"payroll-api","url":%s,"inLanguage":"ja","publisher":%s}`, jstr(base), pub)
	replace("<!--LD:START-->", "<!--LD:END-->", ldJSON(ld)+"\n"+ldJSON(ld2))

	block := fmt.Sprintf(`<section id="rates-now">
  <div class="eyebrow">RATES</div>
  <h2 class="t">令和8年度(2026年度)の主な料率</h2>
  <p class="sub">電卓が使っている、東京都・一般の事業の料率です(最終更新: %s)。都道府県別・年度別の一覧は、各ページで確認できます。</p>
  <div class="tw"><table>
    <tr><th>項目</th><th>料率(総額)</th><th>本人負担</th><th>適用開始</th></tr>
    <tr><td><a href="kenpo/">健康保険(協会けんぽ・東京都)</a></td><td>%s</td><td>%s</td><td>%s</td></tr>
    <tr><td><a href="kenpo/#care">介護保険(40〜64歳)</a></td><td>%s</td><td>%s</td><td>%s</td></tr>
    <tr><td>厚生年金</td><td>%s</td><td>%s</td><td>%s</td></tr>
    <tr><td>子ども・子育て支援金</td><td>%s</td><td>%s</td><td>%s</td></tr>
    <tr><td><a href="koyo/">雇用保険(一般の事業)</a></td><td>%s</td><td>%s</td><td>%s</td></tr>
  </table></div>
  <p>例えば、<b>月給30万円(東京都・35歳・扶養なし)の手取りは約%s円</b>です(住民税を除く)。月給別・年収別の一覧は<a href="tedori/">手取り早見表</a>、計算の手順は<a href="how-to/">手取りの計算方法</a>をご覧ください。</p>
  <div class="grid">
    <a class="card" href="kenpo/" style="text-decoration:none;color:inherit"><h3>協会けんぽ 都道府県別 保険料率</h3><p>令和8年度の47都道府県と、2016年からの推移。</p></a>
    <a class="card" href="koyo/" style="text-decoration:none;color:inherit"><h3>雇用保険料率の推移</h3><p>一般・農林水産・建設の事業別。令和8年度の変更点も。</p></a>
    <a class="card" href="tedori/" style="text-decoration:none;color:inherit"><h3>手取り早見表</h3><p>月給15万〜100万円、年収300万〜1000万円の手取り。</p></a>
    <a class="card" href="how-to/" style="text-decoration:none;color:inherit"><h3>手取りの計算方法</h3><p>額面から引かれる5つの項目を、数字つきで解説。</p></a>
    <a class="card" href="sources/" style="text-decoration:none;color:inherit"><h3>出典と検証方法</h3><p>公式資料の一覧と、データの確認のしかた。</p></a>
  </div>
</section>`, jpDate(date),
		pct(h8.Rates["東京都"]), pct(h8.Rates["東京都"]/2), jpYM("2026-03"),
		pct(care8.Rate), pct(care8.Rate/2), jpYM("2026-03"),
		pct(pn.Rate), pct(pn.Rate/2), jpYM("2017-09"),
		pct(sup.Rate), pct(sup.Rate/2), jpYM("2026-04"),
		num(g.Worker+g.Employer)+"/1,000("+pct((g.Worker+g.Employer)/10)+")", num(g.Worker)+"/1,000("+pct(g.Worker/10)+")", jpYM("2026-04"),
		yen(r30.NetPay))
	replace("<!--SEO:START-->", "<!--SEO:END-->", block)
	must(os.WriteFile(path, []byte(s), 0o644))
	fmt.Println("patched", path)
}
