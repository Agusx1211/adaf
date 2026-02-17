package profilescore

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	minSamplesForSignal         = 3
	maxRecentPerProfile         = 20
	defaultSpeedScore           = 50.0
	judgeReliabilitySmoothing   = 3.0
	minJudgeWeight              = 0.20
	scoreWeightQuality          = 0.50
	scoreWeightResidual         = 0.30
	scoreWeightDifficulty       = 0.20
	residualToPercentMultiplier = 12.5
	speedLogScale               = 25.0
)

type metricAccumulator struct {
	count           int
	totalQuality    float64
	totalDifficulty float64
	totalDuration   float64
	weightedScore   float64
	scoreWeight     float64
	weightedSpeed   float64
	speedWeight     float64
}

func (m *metricAccumulator) add(rec FeedbackRecord, eval recordEvaluation) {
	if m == nil {
		return
	}
	m.count++
	m.totalQuality += rec.Quality
	m.totalDifficulty += rec.Difficulty
	if eval.Weight > 0 {
		m.weightedScore += eval.Score * eval.Weight
		m.scoreWeight += eval.Weight
	}
	if rec.DurationSecs > 0 {
		m.totalDuration += float64(rec.DurationSecs)
		if eval.Weight > 0 {
			m.weightedSpeed += eval.SpeedScore * eval.Weight
			m.speedWeight += eval.Weight
		}
	}
}

func (m *metricAccumulator) toBreakdown(name string) BreakdownStats {
	if m == nil || m.count <= 0 {
		return BreakdownStats{Name: name, SpeedScore: defaultSpeedScore}
	}
	score := 0.0
	if m.scoreWeight > 0 {
		score = round2(m.weightedScore / m.scoreWeight)
	}
	speed := defaultSpeedScore
	if m.speedWeight > 0 {
		speed = round2(m.weightedSpeed / m.speedWeight)
	}
	return BreakdownStats{
		Name:            name,
		Count:           m.count,
		AvgQuality:      round2(m.totalQuality / float64(m.count)),
		AvgDifficulty:   round2(m.totalDifficulty / float64(m.count)),
		AvgDurationSecs: round2(m.totalDuration / float64(m.count)),
		Score:           score,
		SpeedScore:      speed,
	}
}

type scoringModel struct {
	qualityIntercept  float64
	qualitySlope      float64
	hasQualityModel   bool
	meanQuality       float64
	durationIntercept float64
	durationSlope     float64
	hasDurationModel  bool
	meanDurationSecs  float64
	judgeWeights      map[string]float64
}

type recordEvaluation struct {
	Score      float64
	SpeedScore float64
	Weight     float64
}

func buildScoringModel(records []FeedbackRecord) scoringModel {
	model := scoringModel{
		meanQuality:      (MinScore + MaxScore) / 2,
		judgeWeights:     make(map[string]float64),
		meanDurationSecs: 0,
	}

	difficulties := make([]float64, 0, len(records))
	qualities := make([]float64, 0, len(records))
	durationDifficulties := make([]float64, 0, len(records))
	logDurations := make([]float64, 0, len(records))
	totalDuration := 0.0
	durationCount := 0
	for _, rec := range records {
		if strings.TrimSpace(rec.ChildProfile) == "" {
			continue
		}
		difficulties = append(difficulties, rec.Difficulty)
		qualities = append(qualities, rec.Quality)
		if rec.DurationSecs > 0 {
			d := float64(rec.DurationSecs)
			durationDifficulties = append(durationDifficulties, rec.Difficulty)
			logDurations = append(logDurations, math.Log(d))
			totalDuration += d
			durationCount++
		}
	}
	if len(qualities) > 0 {
		model.meanQuality = sumFloat64(qualities) / float64(len(qualities))
	}
	if intercept, slope, ok := fitLinearModel(difficulties, qualities); ok {
		model.qualityIntercept = intercept
		model.qualitySlope = slope
		model.hasQualityModel = true
	}
	if intercept, slope, ok := fitLinearModel(durationDifficulties, logDurations); ok {
		model.durationIntercept = intercept
		model.durationSlope = slope
		model.hasDurationModel = true
	}
	if durationCount > 0 {
		model.meanDurationSecs = totalDuration / float64(durationCount)
	}

	model.judgeWeights = buildJudgeWeights(records, model)
	return model
}

func buildJudgeWeights(records []FeedbackRecord, model scoringModel) map[string]float64 {
	type bucketStats struct {
		sumQuality float64
		count      int
	}
	type judgeStats struct {
		sumErr float64
		count  int
	}

	buckets := make(map[string]bucketStats)
	for _, rec := range records {
		if strings.TrimSpace(rec.ChildProfile) == "" {
			continue
		}
		key := childBucketKey(rec)
		stats := buckets[key]
		stats.sumQuality += rec.Quality
		stats.count++
		buckets[key] = stats
	}

	judgeAgg := make(map[string]judgeStats)
	for _, rec := range records {
		if strings.TrimSpace(rec.ChildProfile) == "" {
			continue
		}
		judge := judgeKey(rec.ParentProfile, rec.ParentRole, rec.ParentPosition)
		if judge == "" {
			judge = "unknown"
		}
		stats := buckets[childBucketKey(rec)]
		consensus := model.expectedQuality(rec.Difficulty)
		if stats.count > 1 {
			consensus = (stats.sumQuality - rec.Quality) / float64(stats.count-1)
		}
		errNorm := clamp(math.Abs(rec.Quality-consensus)/MaxScore, 0, 1)
		agg := judgeAgg[judge]
		agg.sumErr += errNorm
		agg.count++
		judgeAgg[judge] = agg
	}

	out := make(map[string]float64, len(judgeAgg))
	for judge, agg := range judgeAgg {
		if agg.count <= 0 {
			continue
		}
		rawReliability := 1 - (agg.sumErr / float64(agg.count))
		rawReliability = clamp(rawReliability, 0, 1)
		shrink := float64(agg.count) / (float64(agg.count) + judgeReliabilitySmoothing)
		weight := 0.5 + (rawReliability-0.5)*shrink
		out[judge] = clamp(weight, minJudgeWeight, 1)
	}
	return out
}

func (m scoringModel) evaluate(rec FeedbackRecord) recordEvaluation {
	weight := m.judgeWeight(rec.ParentProfile, rec.ParentRole, rec.ParentPosition)
	if weight <= 0 {
		weight = 0.5
	}
	return recordEvaluation{
		Score:      m.score(rec),
		SpeedScore: m.speedScore(rec),
		Weight:     weight,
	}
}

func (m scoringModel) judgeWeight(parentProfile, parentRole, parentPosition string) float64 {
	if len(m.judgeWeights) == 0 {
		return 0.5
	}
	key := judgeKey(parentProfile, parentRole, parentPosition)
	if key == "" {
		return 0.5
	}
	weight, ok := m.judgeWeights[key]
	if !ok {
		return 0.5
	}
	return weight
}

func (m scoringModel) score(rec FeedbackRecord) float64 {
	qualityPct := clamp(rec.Quality*10, 0, 100)
	difficultyPct := clamp(rec.Difficulty*10, 0, 100)
	expected := m.expectedQuality(rec.Difficulty)
	residualScore := clamp(50+((rec.Quality-expected)*residualToPercentMultiplier), 0, 100)
	score := (scoreWeightQuality * qualityPct) +
		(scoreWeightResidual * residualScore) +
		(scoreWeightDifficulty * difficultyPct)
	return round2(clamp(score, 0, 100))
}

func (m scoringModel) speedScore(rec FeedbackRecord) float64 {
	if rec.DurationSecs <= 0 {
		return defaultSpeedScore
	}
	actual := float64(rec.DurationSecs)
	expected := m.expectedDuration(rec.Difficulty)
	if expected <= 0 {
		return defaultSpeedScore
	}
	score := 50 + speedLogScale*math.Log(expected/actual)
	return round2(clamp(score, 0, 100))
}

func (m scoringModel) expectedQuality(difficulty float64) float64 {
	if m.hasQualityModel {
		return clamp(m.qualityIntercept+(m.qualitySlope*difficulty), MinScore, MaxScore)
	}
	return clamp(m.meanQuality, MinScore, MaxScore)
}

func (m scoringModel) expectedDuration(difficulty float64) float64 {
	if m.hasDurationModel {
		return math.Max(1, math.Exp(m.durationIntercept+(m.durationSlope*difficulty)))
	}
	if m.meanDurationSecs > 0 {
		return m.meanDurationSecs
	}
	return 0
}

func judgeKey(parentProfile, parentRole, parentPosition string) string {
	if profile := strings.ToLower(strings.TrimSpace(parentProfile)); profile != "" {
		return "profile:" + profile
	}
	role := strings.ToLower(strings.TrimSpace(parentRole))
	position := strings.ToLower(strings.TrimSpace(parentPosition))
	if role == "" && position == "" {
		return ""
	}
	return "role:" + role + "|position:" + position
}

func childBucketKey(rec FeedbackRecord) string {
	profile := strings.ToLower(strings.TrimSpace(rec.ChildProfile))
	role := strings.ToLower(strings.TrimSpace(rec.ChildRole))
	if role == "" {
		role = "(unspecified)"
	}
	return profile + "|" + role
}

func fitLinearModel(xs, ys []float64) (float64, float64, bool) {
	if len(xs) == 0 || len(xs) != len(ys) {
		return 0, 0, false
	}
	meanX := sumFloat64(xs) / float64(len(xs))
	meanY := sumFloat64(ys) / float64(len(ys))
	var covariance float64
	var varianceX float64
	for i := range xs {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		covariance += dx * dy
		varianceX += dx * dx
	}
	if varianceX == 0 {
		return meanY, 0, true
	}
	slope := covariance / varianceX
	intercept := meanY - slope*meanX
	return intercept, slope, true
}

func sumFloat64(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
	model := buildScoringModel(records)
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
		eval := model.evaluate(rec)
		acc.metrics.add(rec, eval)

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
		acc.roles[role].add(rec, eval)

		if acc.parents[parent] == nil {
			acc.parents[parent] = &metricAccumulator{}
		}
		acc.parents[parent].add(rec, eval)

		if acc.trend[day] == nil {
			acc.trend[day] = &metricAccumulator{}
		}
		acc.trend[day].add(rec, eval)

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
			Score:            metrics.Score,
			SpeedScore:       metrics.SpeedScore,
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
