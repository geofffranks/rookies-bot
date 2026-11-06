package config

import (
	"fmt"
	"regexp"
	"strings"
)

// SeasonCycle is the chronological order of racing seasons. The year increments
// only on rollover from Winter back to New Year (handled by the caller, which
// takes the year from the matched championship rather than from rotation).
var SeasonCycle = []string{"New Year", "Spring", "Summer", "Fall", "Winter"}

// roleNames maps a season term to the Discord role name for that season. The
// names are deliberately inconsistent ("Rookie" vs "Rookies", "Springs"); they
// match the names configured in Discord exactly.
var roleNames = map[string]string{
	"New Year": "GT4 Rookie New",
	"Spring":   "GT4 Rookies Springs",
	"Summer":   "GT4 Rookies Summer",
	"Fall":     "GT4 Rookies Fall",
	"Winter":   "GT4 Rookies Winter",
}

// leadingYear matches an optional 4-digit year prefix (e.g. "2026 ") so a
// season string of either "Fall" or "2026 Fall" parses to the same term.
var leadingYear = regexp.MustCompile(`^\s*\d{4}\s+`)

// ParseSeasonTerm extracts the season term from a season string, tolerating an
// optional leading year and surrounding whitespace.
func ParseSeasonTerm(season string) (string, error) {
	s := strings.TrimSpace(leadingYear.ReplaceAllString(season, ""))
	for _, term := range SeasonCycle {
		if strings.EqualFold(s, term) {
			return term, nil
		}
	}
	return "", fmt.Errorf("could not determine season term from %q (expected one of: %s)", season, strings.Join(SeasonCycle, ", "))
}

// normalizeSeasonInput collapses an administrator-supplied season argument to
// a comparable form: leading years stripped, surrounding whitespace trimmed,
// and runs of internal whitespace reduced to single spaces.
func normalizeSeasonInput(s string) string {
	s = strings.TrimSpace(leadingYear.ReplaceAllString(s, ""))
	return strings.Join(strings.Fields(s), " ")
}

// ParseSeasonSelection resolves an administrator-supplied season argument into
// a canonical season term. Accepted forms are bare terms ("fall"), the season
// role names configured in Discord ("GT4 Rookies Fall", "GT4 Rookie New",
// "GT4 Rookies Springs"), and "GT4 Rookies <term>" combinations — all
// case-insensitively with flexible whitespace and an optional leading year.
// Empty or whitespace-only input returns "" with no error, meaning no explicit
// selection (the caller keeps automatic progression).
func ParseSeasonSelection(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", nil
	}
	norm := normalizeSeasonInput(input)
	for _, term := range SeasonCycle {
		if strings.EqualFold(norm, term) {
			return term, nil
		}
	}
	for term, alias := range roleNames {
		if strings.EqualFold(norm, alias) {
			return term, nil
		}
	}
	lower := strings.ToLower(norm)
	for _, prefix := range []string{"gt4 rookies ", "gt4 rookie "} {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		tail := strings.TrimPrefix(lower, prefix)
		for _, term := range SeasonCycle {
			if strings.EqualFold(tail, term) {
				return term, nil
			}
		}
	}
	return "", fmt.Errorf(
		"invalid season %q: expected a season term (New Year, Spring, Summer, Fall, Winter) or a season role name (e.g. \"GT4 Rookies Fall\")",
		input)
}

// NextTerm returns the season term that follows term in SeasonCycle, wrapping
// Winter back to New Year.
func NextTerm(term string) (string, error) {
	for i, t := range SeasonCycle {
		if t == term {
			return SeasonCycle[(i+1)%len(SeasonCycle)], nil
		}
	}
	return "", fmt.Errorf("unknown season term %q", term)
}

// RoleNameForTerm returns the Discord role name for the given season term.
func RoleNameForTerm(term string) (string, error) {
	name, ok := roleNames[term]
	if !ok {
		return "", fmt.Errorf("no role name configured for season term %q", term)
	}
	return name, nil
}
