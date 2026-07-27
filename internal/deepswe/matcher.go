package deepswe

import "strings"

// DefaultEffort is stored in place of an empty reasoning_effort, since the
// column participates in a primary key and cannot be NULL.
const DefaultEffort = "default"

// orSlug extracts the model slug DeepSWE would use from an OpenRouter model
// ID: it drops the "author/" prefix and any ":variant" suffix, e.g.
// "deepseek/deepseek-r1:free" -> "deepseek-r1".
func orSlug(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	if i := strings.Index(id, ":"); i >= 0 {
		id = id[:i]
	}
	return id
}

// normalize makes a slug comparable across OpenRouter and DeepSWE naming:
// lowercase, and "." folded to "-" (e.g. "claude-3.5-sonnet" and
// "claude-3-5-sonnet" normalize to the same string).
func normalize(s string) string {
	s = strings.ToLower(s)
	return strings.ReplaceAll(s, ".", "-")
}

// BuildIndex groups leaderboard rows by their normalized model slug.
func BuildIndex(rows []LeaderboardRow) map[string][]LeaderboardRow {
	index := make(map[string][]LeaderboardRow, len(rows))
	for _, r := range rows {
		key := normalize(r.Model)
		index[key] = append(index[key], r)
	}
	return index
}

// MatchModel looks up leaderboard rows for an OpenRouter model, trying id
// first and canonicalSlug as a fallback. It returns false if neither
// matched.
func MatchModel(index map[string][]LeaderboardRow, id, canonicalSlug string) ([]LeaderboardRow, bool) {
	if rows, ok := index[normalize(orSlug(id))]; ok {
		return rows, true
	}
	if rows, ok := index[normalize(orSlug(canonicalSlug))]; ok {
		return rows, true
	}
	return nil, false
}

// EffortOrDefault returns effort, or DefaultEffort if effort is empty.
func EffortOrDefault(effort string) string {
	if effort == "" {
		return DefaultEffort
	}
	return effort
}
