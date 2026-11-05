# insurance-rates

社会保険料率(協会けんぽ)・雇用保険料率の **参照 / 保険料計算 / 改定検知** を行う Go 製の REST API + MCP サーバー。

- データの正本: [`internal/rates/rates.json`](internal/rates/rates.json) (適用開始月ごとの版管理 + 出典URL + `verified` フラグ)
- 外部依存なし (標準ライブラリのみ)。MCP は JSON-RPC を自前実装 (stdio / HTTP POST)

## 起動

```bash
go run ./cmd/server -addr 127.0.0.1:8080 [-rate 120] [-token SECRET]   # REST /v1/*  と  MCP /mcp
go run ./cmd/server -stdio                                   # MCP (stdio)
```

Claude Desktop 等 (stdio):

```json
{ "mcpServers": { "insurance-rates": { "command": "C:\\path\\to\\server.exe", "args": ["-stdio"] } } }
```

## 公開API (トークン不要)

`https://payroll-api.paps.jp` は**トークン不要・登録不要**で使えます。電卓ページ ([GitHub Pages](https://paps-jp.github.io/payroll-api/)) もこのAPIを呼んでいます。

- CORS は全オリジンに許可 (認証情報は扱わない読み取り専用)。入力値は保存しません。
- 濫用対策: **IPごとに毎分120回まで**。超えた分は `429` (`Retry-After` 付き)。10分間に5回超過を繰り返すと**15分間ブロック**。
- MCP の `check_for_revisions` は公式サイトを見に行くため、結果を1時間キャッシュします。
- トークン必須に戻したい場合は `RATES_API_TOKEN` (または `-token`) を設定。`/v1/*` と `/mcp` が Bearer 必須になります。
- 制限値は `-rate` で変更、ロジックは `internal/guard`。クライアントIPは Caddy が付ける `X-Forwarded-For` を信頼します。

## REST

| パス | 内容 |
|---|---|
| `GET /v1/meta` | 入力フォーム用の情報 (都道府県一覧・対応開始月・事業区分) |
| `GET /v1/rates?month=2026-04&prefecture=東京都` | 指定月の適用料率 (prefecture 省略で全47都道府県) |
| `GET /v1/revisions` | 登録済みの全改定 (種別・適用開始月・出典・verified) |
| `GET /v1/grades` | 標準報酬月額の等級表 |
| `GET /v1/calc/social?month=&prefecture=&monthly_remuneration=\|standard_remuneration=&age=` | 健保・介護・厚年・子ども子育て支援金/拠出金 |
| `GET /v1/calc/employment?month=&wage=&industry=general\|agri\|construction` | 雇用保険 |
| `GET /v1/calc/withholding?month=&amount=&column=ko\|otsu&dependents=` | 源泉所得税(復興特別所得税込み)。amount=社会保険料等控除後の給与額 |
| `GET /v1/calc/payroll?month=&prefecture=&gross=&age=&non_taxable_commute=&dependents=&column=&resident_tax=` | 社保+雇用保険+源泉+住民税の控除と手取り、会社負担 |

## MCP tools

`get_rates` / `list_revisions` / `get_standard_remuneration_grade` / `calculate_social_insurance` /
`calculate_employment_insurance` / `calculate_withholding_tax` / `calculate_payroll_deductions` / `check_for_revisions`

## 改定検知

```bash
go run ./cmd/ratecheck [-state .ratecheck-state] [-webhook https://hooks.slack.com/...]
```

- 協会けんぽの `rate_prefectures/rNN` (今年度・来年度) を取得し、47都道府県と介護保険料率を保有データと突き合わせ
- 厚労省「雇用保険料率について」ページの指紋変化と、次年度の記載有無を検知
- 警告があれば終了コード 1 (+ webhook 通知)。GitHub Actions: [.github/workflows/ratecheck.yml](.github/workflows/ratecheck.yml)

## 改定時の更新手順

1. 公式の公表値を確認 (`list_revisions` の `source`)
2. `rates.json` の該当配列に **新しい版を追記** (過去版は消さない。`effective_from` は対象月 `YYYY-MM`)
3. `verified: true` にして `go test ./...`

## 源泉所得税

- 令和8年分の国税庁「電算機計算の特例」(甲欄) と乙欄の電算機計算を実装 ([出典PDF](https://www.nta.go.jp/publication/pamph/gensen/aramashi2026/pdf/16.pdf))。税額表の値とは差が出ることがあり、年末調整で精算される。
- 甲欄は国税庁の計算例3件をテストで再現。税率表は `rates.json` の `withholding` に版管理 (次年分は新しい版を追記)。
- 住民税は自治体の通知額を `resident_tax` で渡す (計算しない)。賞与・年末調整・日額表は未対応。

## 収録期間 (2016年〜)

| 種別 | 収録 | 出典 |
|---|---|---|
| 健康保険・介護保険 (協会けんぽ) | 2016-03〜(平成28年度〜) | 協会けんぽ各年度ページ。平成28年度分のみ単独出典(`verified:false`) |
| 厚生年金 | 2015-09〜 (17.828 → 2016-09 18.182 → 2017-09 18.3) | 日本年金機構 |
| 子ども・子育て拠出金 | 2015-04〜 | 機構。2015-04/2016-04 は間接的な出典(`verified:false`) |
| 雇用保険 | 2015-04〜 (期中改定を含む) | 厚労省/労働局リーフレット |
| 源泉所得税 | 2016年分〜 (年ごとに版) | 国税庁『源泉徴収のあらまし』(2016〜2018年分は国会図書館WARPの保存版) |

- 2016年より前の月を指定するとエラー(データなし)になります。
- 源泉所得税は『あらまし』『税額表』の計算例・税額表の値240件を `internal/rates/testdata/withholding_examples.json` に置き、全年分を `go test` で再現しています。
- **乙欄の注意**: 令和2〜7年分の乙欄は、国税庁の算式が月額表(税額表)と約29万〜43.7万円付近の一部の行でずれます(算式の方が大きく、最大数千円)。当APIは算式どおり。令和8年分は全行一致。

## 前提・注意

- 協会けんぽ管掌のみ。組合健保・共済は対象外。賞与、随時改定、産休・育休免除、労災保険は未対応。
- 端数処理: 本人 = 50銭以下切捨て / 50銭超切上げ、事業主 = 総額(四捨五入) − 本人。
- 要確認 (`verified:false`): 子ども・子育て拠出金率 0.36%、等級表の境界額 (仕様書どおり公式で再確認を)。
- 厚生年金の等級は健保の標準報酬を 88,000〜650,000 円にクリップして算出。
