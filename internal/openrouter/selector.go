package openrouter

import (
	"math"
	"strconv"
	"time"
)

// IsTextToText reports whether m accepts and returns text (it may also
// accept/return other modalities, e.g. image input).
func IsTextToText(m Model) bool {
	return containsString(m.Architecture.InputModalities, "text") &&
		containsString(m.Architecture.OutputModalities, "text")
}

func containsString(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// IsRecentOrFavorite reports whether m was released within one month of now,
// or is present in favorites.
func IsRecentOrFavorite(m Model, now time.Time, favorites map[string]bool) bool {
	if favorites[m.ID] {
		return true
	}
	cutoff := now.AddDate(0, -1, 0)
	return time.Unix(m.Created, 0).After(cutoff)
}

// SelectCandidates filters models down to those that are text-to-text and
// either recently released or favorited.
func SelectCandidates(models []Model, now time.Time, favorites map[string]bool) []Model {
	var out []Model
	for _, m := range models {
		if IsTextToText(m) && IsRecentOrFavorite(m, now, favorites) {
			out = append(out, m)
		}
	}
	return out
}

// CheapestEndpoint returns the endpoint in eps with the lowest weighted
// price (weightInput*promptPrice + weightOutput*completionPrice), and true
// if at least one endpoint had valid pricing. An endpoint is skipped when
// its status is negative (degraded/deprecated), or its prices don't parse
// as non-negative numbers.
func CheapestEndpoint(eps []Endpoint, weightInput, weightOutput float64) (Endpoint, bool) {
	var best Endpoint
	bestScore := math.Inf(1)
	found := false

	for _, e := range eps {
		if e.Status < 0 {
			continue
		}
		prompt, completion, ok := validPricing(e.Pricing)
		if !ok {
			continue
		}
		score := weightInput*prompt + weightOutput*completion
		if !found || score < bestScore {
			best = e
			bestScore = score
			found = true
		}
	}
	return best, found
}

// validPricing parses p's prompt/completion prices, returning ok = false if
// either fails to parse as a non-negative number. OpenRouter uses "-1" as a
// sentinel for models with no fixed price (e.g. its meta "auto router",
// which bills at whatever model it dispatches to), which is not a usable
// price and must not be mistaken for a real (if unusual) one.
func validPricing(p Pricing) (prompt, completion float64, ok bool) {
	prompt, err := strconv.ParseFloat(p.Prompt, 64)
	if err != nil || math.IsNaN(prompt) || prompt < 0 {
		return 0, 0, false
	}
	completion, err = strconv.ParseFloat(p.Completion, 64)
	if err != nil || math.IsNaN(completion) || completion < 0 {
		return 0, 0, false
	}
	return prompt, completion, true
}
