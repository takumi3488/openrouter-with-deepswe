package terminalbench

import "strings"

// DefaultEffort is stored for rows with no reasoning-effort label.
const DefaultEffort = "default"

var providerPunctuation = strings.NewReplacer(" ", "", "_", "", ".", "", "-", "")

// BuildIndex groups visible source rows by exact normalized provider and model.
// Source rows without a provider are never matchable.
func BuildIndex(rows []LeaderboardRow) map[string][]LeaderboardRow {
	index := make(map[string][]LeaderboardRow, len(rows))
	for _, row := range rows {
		key := identityKey(row.Provider, row.Model)
		if key == "" {
			continue
		}
		index[key] = append(index[key], row)
	}
	return index
}

// MatchModel returns every source row matching an OpenRouter model's provider
// and model identity. IDs are preferred over canonical slugs because the latter
// may have a deployment-date suffix. Matching is exact after punctuation and
// whitespace normalization; there is no fuzzy or cross-provider fallback.
func MatchModel(index map[string][]LeaderboardRow, id, canonicalSlug string) ([]LeaderboardRow, bool) {
	for _, candidate := range []string{id, canonicalSlug} {
		provider, model := splitModel(candidate)
		if provider == "" || model == "" {
			continue
		}
		if rows, ok := index[identityKey(provider, model)]; ok {
			return rows, true
		}
	}
	return nil, false
}

// EffortOrDefault keeps the source's empty-effort convention compatible with
// the non-null primary-key column used by both benchmark tables.
func EffortOrDefault(effort string) string {
	if strings.TrimSpace(effort) == "" {
		return DefaultEffort
	}
	return effort
}

func identityKey(provider, model string) string {
	provider = normalizeProvider(provider)
	model = normalizeModel(model)
	if provider == "" || model == "" {
		return ""
	}
	return provider + "\x00" + model
}

func splitModel(value string) (provider, model string) {
	value = strings.TrimSpace(value)
	if i := strings.IndexByte(value, '/'); i >= 0 {
		provider, model = value[:i], value[i+1:]
	} else {
		return "", ""
	}
	if i := strings.IndexByte(model, ':'); i >= 0 {
		model = model[:i]
	}
	return provider, model
}
func normalizeProvider(value string) string {
	return providerPunctuation.Replace(strings.ToLower(strings.TrimSpace(value)))
}
func normalizeModel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(value))
	separator := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			separator = false
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == ' ':
			separator = true
		default:
			return ""
		}
	}
	return strings.TrimPrefix(strings.Trim(b.String(), "-"), "claude-")
}
