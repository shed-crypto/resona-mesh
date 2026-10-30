package catalog

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// TrackRecommendation pairs a catalog track with its calculated similarity score.
type TrackRecommendation struct {
	Track Track   `json:"track"`
	Score float64 `json:"score"`
}

// GetRecommendations returns the top similar tracks for a seed track from the local catalog.
func (c *Catalog) GetRecommendations(ctx context.Context, contentID string, limit int) ([]TrackRecommendation, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	seed, err := c.GetTrack(ctx, contentID)
	if err != nil {
		return nil, fmt.Errorf("recommendations: failed to fetch seed track: %w", err)
	}

	// Fetch up to 1000 candidates from local catalog
	candidates, _, err := c.ListTracks(ctx, Query{
		Limit:          1000,
		IncludeBlocked: false,
	})
	if err != nil {
		return nil, fmt.Errorf("recommendations: failed to list candidates: %w", err)
	}

	seedTags := extractNormalizedTags(seed)

	var scored []TrackRecommendation
	for _, cand := range candidates {
		if cand.ContentID == seed.ContentID || cand.Blocked {
			continue
		}

		candTags := extractNormalizedTags(&cand)
		jaccard := computeJaccardSimilarity(seedTags, candTags)
		bpmScore := computeTempoProximity(seed.BPM, cand.BPM)

		composite := 0.75*jaccard + 0.25*bpmScore

		scored = append(scored, TrackRecommendation{
			Track: cand,
			Score: math.Round(composite*1000) / 1000,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if math.Abs(scored[i].Score-scored[j].Score) > 0.0001 {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Track.CreatedAt > scored[j].Track.CreatedAt
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}

	return scored, nil
}

func extractNormalizedTags(t *Track) map[string]struct{} {
	m := make(map[string]struct{})
	add := func(s string) {
		trimmed := strings.ToLower(strings.TrimSpace(s))
		if trimmed != "" {
			m[trimmed] = struct{}{}
		}
	}

	add(t.Genre)
	add(t.Mood)
	add(t.Artist)
	for _, tag := range t.Tags {
		add(tag)
	}
	return m
}

func computeJaccardSimilarity(setA, setB map[string]struct{}) float64 {
	if len(setA) == 0 && len(setB) == 0 {
		return 0.0
	}

	intersection := 0
	for k := range setA {
		if _, exists := setB[k]; exists {
			intersection++
		}
	}

	union := len(setA)
	for k := range setB {
		if _, exists := setA[k]; !exists {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func computeTempoProximity(bpma, bpmb int) float64 {
	if bpma <= 0 || bpmb <= 0 {
		return 0.5 // Neutral baseline when tempo is unknown
	}

	diffNominal := math.Abs(float64(bpma - bpmb))
	diffDouble := math.Abs(float64(bpma*2 - bpmb))
	diffHalf := math.Abs(float64(bpma - bpmb*2))

	minDiff := math.Min(diffNominal, math.Min(diffDouble, diffHalf))
	if minDiff >= 60.0 {
		return 0.0
	}
	return 1.0 - (minDiff / 60.0)
}
