package profilescore

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	minSamplesForSignal = 3
	maxRecentPerProfile = 20
)

type metricAccumulator struct {
	count           int
	totalQuality    float64
	totalDifficulty float64
	totalDuration   float64
}

func (m *metricAccumulator) add(rec FeedbackRecord) {
	if m == nil {
		return
	}
	m.count++
	m.totalQuality += rec.Quality
	m.totalDifficulty += rec.Difficulty
	if rec.DurationSecs > 0 {
		m.totalDuration += float64(rec.DurationSecs)
	}
}

func (m *metricAccumulator) toBreakdown(name string) BreakdownStats {
	if m == nil || m.count <= 0 {
		return BreakdownStats{Name: name}
	}
	return BreakdownStats{
		Name:            name,
		Count:           m.count,
		AvgQuality:      round2(m.totalQuality / float64(m.count)),
		AvgDifficulty:   round2(m.totalDifficulty / float64(m.count)),
		AvgDurationSecs: round2(m.totalDuration / float64(m.count)),
	}
}

type summaryAccumulator struct {
	metrics metricAccumulator
	roles   map[string]*metricAccumulator
	parents map[string]*metricAccumulator
	trend   map[string]*metricAccumulator
	recent  []FeedbackRecord
}

func BuildDashboard(catalog []ProfileCatalogEntry, records []FeedbackRecord) Dashboard {
	profileToCost := make(map[string]string, len(catalog))
	order := make([]string, 0, len(catalog))
	for _, c := range catalog {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := profileToCost[key]; exists {
			continue
		}
		order = append(order, name)
		profileToCost[key] = strings.TrimSpace(c.Cost)
	}

	accByProfile := make(map[string]*summaryAccumulator)
	addAccumulator := func(profile string) *summaryAccumulator {
		key := strings.ToLower(strings.TrimSpace(profile))
		if key == "" {
			return nil
		}
		acc, ok := accByProfile[key]
		if !ok {
			acc = &summaryAccumulator{
				roles:   make(map[string]*metricAccumulator),
				parents: make(map[string]*metricAccumulator),
				trend:   make(map[string]*metricAccumulator),
				recent:  make([]FeedbackRecord, 0),
			}
			accByProfile[key] = acc
		}
		return acc
	}

	totalFeedback := 0
	for _, rec := range records {
		profile := strings.TrimSpace(rec.ChildProfile)
		if profile == "" {
			continue
		}
		acc := addAccumulator(profile)
		if acc == nil {
			continue
		}
		totalFeedback++
		acc.metrics.add(rec)

		role := strings.TrimSpace(rec.ChildRole)
		if role == "" {
			role = "(unspecified)"
		}
		parent := strings.TrimSpace(rec.ParentProfile)
		if parent == "" {
			parent = "(unknown)"
		}
		day := rec.CreatedAt.UTC().Format("2006-01-02")
		if rec.CreatedAt.IsZero() {
			day = time.Now().UTC().Format("2006-01-02")
		}

		if acc.roles[role] == nil {
			acc.roles[role] = &metricAccumulator{}
		}
		acc.roles[role].add(rec)

		if acc.parents[parent] == nil {
			acc.parents[parent] = &metricAccumulator{}
		}
		acc.parents[parent].add(rec)

		if acc.trend[day] == nil {
			acc.trend[day] = &metricAccumulator{}
		}
		acc.trend[day].add(rec)

		acc.recent = append(acc.recent, rec)
	}

	// Merge any profile names that only appear in feedback.
	for key, acc := range accByProfile {
		if acc == nil {
			continue
		}
		found := false
		for _, name := range order {
			if strings.EqualFold(name, key) {
				found = true
				break
			}
		}
		if !found {
			order = append(order, key)
		}
	}
	sort.Slice(order, func(i, j int) bool {
		return strings.ToLower(order[i]) < strings.ToLower(order[j])
	})

	summaries := make([]ProfileSummary, 0, len(order))
	for _, profileName := range order {
		key := strings.ToLower(strings.TrimSpace(profileName))
		acc := accByProfile[key]
		if acc == nil {
			acc = &summaryAccumulator{
				roles:   make(map[string]*metricAccumulator),
				parents: make(map[string]*metricAccumulator),
				trend:   make(map[string]*metricAccumulator),
				recent:  []FeedbackRecord{},
			}
		}
		metrics := acc.metrics.toBreakdown(profileName)
		summary := ProfileSummary{
			Profile:          profileName,
			Cost:             profileToCost[key],
			TotalFeedback:    metrics.Count,
			AvgQuality:       metrics.AvgQuality,
			AvgDifficulty:    metrics.AvgDifficulty,
			AvgDurationSecs:  metrics.AvgDurationSecs,
			RoleBreakdown:    mapToBreakdowns(acc.roles),
			ParentBreakdown:  mapToBreakdowns(acc.parents),
			Trend:            mapToTrend(acc.trend),
			HasEnoughSamples: metrics.Count >= minSamplesForSignal,
		}
		if len(acc.recent) > 0 {
			sort.Slice(acc.recent, func(i, j int) bool {
				return acc.recent[i].CreatedAt.After(acc.recent[j].CreatedAt)
			})
			limit := len(acc.recent)
			if limit > maxRecentPerProfile {
				limit = maxRecentPerProfile
			}
			summary.RecentFeedback = append([]FeedbackRecord(nil), acc.recent[:limit]...)
		}
		summary.Signals = buildRoleSignals(summary.RoleBreakdown)
		summaries = append(summaries, summary)
	}

	attachSpeedSignals(summaries)
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].TotalFeedback != summaries[j].TotalFeedback {
			return summaries[i].TotalFeedback > summaries[j].TotalFeedback
		}
		return strings.ToLower(summaries[i].Profile) < strings.ToLower(summaries[j].Profile)
	})

	return Dashboard{
		GeneratedAt:   time.Now().UTC(),
		TotalFeedback: totalFeedback,
		Profiles:      summaries,
	}
}

func FeedbackForProfile(records []FeedbackRecord, profile string) []FeedbackRecord {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return nil
	}
	out := make([]FeedbackRecord, 0)
	for _, rec := range records {
		if strings.EqualFold(strings.TrimSpace(rec.ChildProfile), profile) {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func mapToBreakdowns(values map[string]*metricAccumulator) []BreakdownStats {
	if len(values) == 0 {
		return []BreakdownStats{}
	}
	out := make([]BreakdownStats, 0, len(values))
	for key, metric := range values {
		out = append(out, metric.toBreakdown(key))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func mapToTrend(values map[string]*metricAccumulator) []TrendPoint {
	if len(values) == 0 {
		return []TrendPoint{}
	}
	out := make([]TrendPoint, 0, len(values))
	for day, metric := range values {
		breakdown := metric.toBreakdown(day)
		out = append(out, TrendPoint{
			Date:            day,
			Count:           breakdown.Count,
			AvgQuality:      breakdown.AvgQuality,
			AvgDifficulty:   breakdown.AvgDifficulty,
			AvgDurationSecs: breakdown.AvgDurationSecs,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date < out[j].Date
	})
	return out
}

func buildRoleSignals(roles []BreakdownStats) []string {
	signals := make([]string, 0, 2)
	for _, role := range roles {
		if role.Count < minSamplesForSignal {
			continue
		}
		if role.AvgQuality >= 7.5 {
			signals = append(signals, fmt.Sprintf("good at %s (quality %.1f/10 over %d)", role.Name, role.AvgQuality, role.Count))
		}
		if len(signals) >= 2 {
			break
		}
	}
	return signals
}

func attachSpeedSignals(summaries []ProfileSummary) {
	type candidate struct {
		index    int
		duration float64
	}
	candidates := make([]candidate, 0, len(summaries))
	for i := range summaries {
		if summaries[i].TotalFeedback < minSamplesForSignal {
			continue
		}
		if summaries[i].AvgDurationSecs <= 0 {
			continue
		}
		candidates = append(candidates, candidate{index: i, duration: summaries[i].AvgDurationSecs})
	}
	if len(candidates) < 2 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].duration < candidates[j].duration
	})

	topCut := len(candidates) / 3
	if topCut < 1 {
		topCut = 1
	}
	for i, c := range candidates {
		switch {
		case i < topCut:
			summaries[c.index].Signals = append(summaries[c.index].Signals, fmt.Sprintf("fast on average (%.0fs)", summaries[c.index].AvgDurationSecs))
		case i >= len(candidates)-topCut:
			summaries[c.index].Signals = append(summaries[c.index].Signals, fmt.Sprintf("slower on average (%.0fs)", summaries[c.index].AvgDurationSecs))
		}
	}
}

func round2(v float64) float64 {
	if v == 0 {
		return 0
	}
	return float64(int(v*100+0.5)) / 100
}
