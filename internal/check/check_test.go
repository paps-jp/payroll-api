package check

import "testing"

func TestParsePrefectureRates(t *testing.T) {
	text := "令和8年度 北海道 １０．３１％ ↓ １０．２８％ 青森県 ９．８５％ → ９．８５％ 東京都 ９．９１％ ↓ ９．８５％ 京都府 １０．０３％ ↓ ９．８９％"
	got := ParsePrefectureRates(text)
	want := map[string]float64{"北海道": 10.28, "青森県": 9.85, "東京都": 9.85, "京都府": 9.89}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v want %v", k, got[k], v)
		}
	}
}
