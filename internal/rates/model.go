// Package rates holds the social/employment insurance rate tables and the
// premium calculation logic shared by the REST API, the MCP server and the
// revision checker.
package rates

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed rates.json
var embedded []byte

type Dataset struct {
	Note              string               `json:"note"`
	Health            []HealthVersion      `json:"health"`
	Care              []RateVersion        `json:"care"`
	Pension           []RateVersion        `json:"pension"`
	ChildSupport      []RateVersion        `json:"child_support"`
	ChildContribution []RateVersion        `json:"child_contribution"`
	Employment        []EmploymentVersion  `json:"employment"`
	Withholding       []WithholdingVersion `json:"withholding"`
	HealthGrades      []Grade              `json:"health_grades"`
	PensionRange      PensionRange         `json:"pension_grade_range"`
	GradesVerified    bool                 `json:"grades_verified"`
}

type HealthVersion struct {
	EffectiveFrom string             `json:"effective_from"`
	Rates         map[string]float64 `json:"rates"` // 都道府県 -> %
	Source        string             `json:"source"`
	Verified      bool               `json:"verified"`
}

type RateVersion struct {
	EffectiveFrom string  `json:"effective_from"`
	Rate          float64 `json:"rate"` // %
	Source        string  `json:"source"`
	Verified      bool    `json:"verified"`
	Note          string  `json:"note,omitempty"`
}

type Split struct {
	Worker   float64 `json:"worker"`
	Employer float64 `json:"employer"`
}

type EmploymentVersion struct {
	EffectiveFrom string           `json:"effective_from"`
	PerMille      map[string]Split `json:"per_mille"` // general / agri / construction
	Source        string           `json:"source"`
	Verified      bool             `json:"verified"`
}

type Grade struct {
	Grade    int `json:"grade"`
	Standard int `json:"standard"`
	Lower    int `json:"lower"` // この額以上
}

type PensionRange struct {
	MinStandard int `json:"min_standard"`
	MaxStandard int `json:"max_standard"`
}

// Load parses the embedded dataset.
func Load() (*Dataset, error) { return Parse(embedded) }

func Parse(b []byte) (*Dataset, error) {
	var d Dataset
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	if len(d.HealthGrades) == 0 {
		return nil, fmt.Errorf("health_grades is empty")
	}
	return &d, nil
}

// pick returns the index of the latest version whose effective_from <= month.
func pick(n int, eff func(int) string, month string) int {
	idx := -1
	for i := 0; i < n; i++ {
		if eff(i) <= month && (idx < 0 || eff(i) >= eff(idx)) {
			idx = i
		}
	}
	return idx
}

func (d *Dataset) HealthAt(month string) (*HealthVersion, bool) {
	i := pick(len(d.Health), func(i int) string { return d.Health[i].EffectiveFrom }, month)
	if i < 0 {
		return nil, false
	}
	return &d.Health[i], true
}

func rateAt(v []RateVersion, month string) (*RateVersion, bool) {
	i := pick(len(v), func(i int) string { return v[i].EffectiveFrom }, month)
	if i < 0 {
		return nil, false
	}
	return &v[i], true
}

func (d *Dataset) CareAt(m string) (*RateVersion, bool)    { return rateAt(d.Care, m) }
func (d *Dataset) PensionAt(m string) (*RateVersion, bool) { return rateAt(d.Pension, m) }
func (d *Dataset) ChildSupportAt(m string) (*RateVersion, bool) {
	return rateAt(d.ChildSupport, m)
}
func (d *Dataset) ChildContributionAt(m string) (*RateVersion, bool) {
	return rateAt(d.ChildContribution, m)
}
func (d *Dataset) EmploymentAt(month string) (*EmploymentVersion, bool) {
	i := pick(len(d.Employment), func(i int) string { return d.Employment[i].EffectiveFrom }, month)
	if i < 0 {
		return nil, false
	}
	return &d.Employment[i], true
}
