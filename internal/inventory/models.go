package inventory

import (
	"sort"
	"time"

	"vibecockpit/internal/provider"
)

func aggregateModels(sessions []provider.Session) []ModelUsage {
	type bucket struct {
		provider string
		count    int
		lastUsed time.Time
	}

	byModel := map[string]*bucket{}

	for _, s := range sessions {
		if s.Model == "" {
			continue
		}
		b, ok := byModel[s.Model]
		if !ok {
			b = &bucket{provider: s.Provider}
			byModel[s.Model] = b
		}
		b.count++
		if s.Modified.After(b.lastUsed) {
			b.lastUsed = s.Modified
			b.provider = s.Provider
		}
	}

	out := make([]ModelUsage, 0, len(byModel))
	for model, b := range byModel {
		out = append(out, ModelUsage{
			Model:        model,
			Provider:     b.provider,
			SessionCount: b.count,
			LastUsed:     b.lastUsed.Format(time.RFC3339),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].SessionCount > out[j].SessionCount
	})

	return out
}
