package profilescore

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreUpsertFeedbackCreatesAndUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile-feedback.json")
	s := NewAtPath(path)

	created, err := s.UpsertFeedback(FeedbackRecord{
		ProjectID:    "proj-a",
		SpawnID:      42,
		ChildProfile: "spark",
		ChildRole:    "scout",
		Difficulty:   4,
		Quality:      8,
	})
	if err != nil {
		t.Fatalf("UpsertFeedback(create) error = %v", err)
	}
	if created.ID == "" {
		t.Fatalf("created id is empty")
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("created_at is zero")
	}

	updated, err := s.UpsertFeedback(FeedbackRecord{
		ProjectID:    "proj-a",
		SpawnID:      42,
		ChildProfile: "spark",
		ChildRole:    "scout",
		Difficulty:   6,
		Quality:      9,
		Notes:        "improved after review",
	})
	if err != nil {
		t.Fatalf("UpsertFeedback(update) error = %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("updated id = %q, want %q", updated.ID, created.ID)
	}
	if updated.Quality != 9 {
		t.Fatalf("updated quality = %v, want 9", updated.Quality)
	}
	if updated.CreatedAt.IsZero() || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("updated created_at = %v, want %v", updated.CreatedAt, created.CreatedAt)
	}

	all, err := s.ListFeedback()
	if err != nil {
		t.Fatalf("ListFeedback() error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListFeedback() len = %d, want 1", len(all))
	}
	if all[0].Difficulty != 6 {
		t.Fatalf("difficulty = %v, want 6", all[0].Difficulty)
	}
}

func TestStoreUpsertFeedbackRejectsInvalidScores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile-feedback.json")
	s := NewAtPath(path)

	_, err := s.UpsertFeedback(FeedbackRecord{
		ProjectID:    "proj-a",
		SpawnID:      7,
		ChildProfile: "spark",
		Difficulty:   11,
		Quality:      8,
	})
	if err == nil {
		t.Fatalf("UpsertFeedback() error = nil, want validation error")
	}
}

func TestBuildDashboardIncludesBreakdownsAndSignals(t *testing.T) {
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	records := []FeedbackRecord{
		{
			ProjectID:     "proj-a",
			SpawnID:       1,
			ParentProfile: "lead-a",
			ChildProfile:  "spark",
			ChildRole:     "scout",
			Difficulty:    3,
			Quality:       9,
			DurationSecs:  50,
			CreatedAt:     now.Add(-48 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       2,
			ParentProfile: "lead-a",
			ChildProfile:  "spark",
			ChildRole:     "scout",
			Difficulty:    4,
			Quality:       8,
			DurationSecs:  60,
			CreatedAt:     now.Add(-24 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       3,
			ParentProfile: "lead-b",
			ChildProfile:  "spark",
			ChildRole:     "developer",
			Difficulty:    5,
			Quality:       7,
			DurationSecs:  55,
			CreatedAt:     now,
		},
		{
			ProjectID:     "proj-b",
			SpawnID:       11,
			ParentProfile: "lead-c",
			ChildProfile:  "devstral",
			ChildRole:     "scout",
			Difficulty:    7,
			Quality:       8,
			DurationSecs:  200,
			CreatedAt:     now.Add(-48 * time.Hour),
		},
		{
			ProjectID:     "proj-b",
			SpawnID:       12,
			ParentProfile: "lead-c",
			ChildProfile:  "devstral",
			ChildRole:     "scout",
			Difficulty:    7,
			Quality:       8,
			DurationSecs:  220,
			CreatedAt:     now.Add(-24 * time.Hour),
		},
		{
			ProjectID:     "proj-b",
			SpawnID:       13,
			ParentProfile: "lead-d",
			ChildProfile:  "devstral",
			ChildRole:     "scout",
			Difficulty:    8,
			Quality:       8,
			DurationSecs:  210,
			CreatedAt:     now,
		},
	}
	catalog := []ProfileCatalogEntry{
		{Name: "spark", Cost: "cheap"},
		{Name: "devstral", Cost: "expensive"},
		{Name: "idle", Cost: "free"},
	}

	dashboard := BuildDashboard(catalog, records)
	if dashboard.TotalFeedback != len(records) {
		t.Fatalf("TotalFeedback = %d, want %d", dashboard.TotalFeedback, len(records))
	}
	if len(dashboard.Profiles) != 3 {
		t.Fatalf("Profiles len = %d, want 3", len(dashboard.Profiles))
	}

	var spark, devstral, idle *ProfileSummary
	for i := range dashboard.Profiles {
		switch strings.ToLower(dashboard.Profiles[i].Profile) {
		case "spark":
			spark = &dashboard.Profiles[i]
		case "devstral":
			devstral = &dashboard.Profiles[i]
		case "idle":
			idle = &dashboard.Profiles[i]
		}
	}
	if spark == nil || devstral == nil || idle == nil {
		t.Fatalf("expected spark/devstral/idle summaries, got %+v", dashboard.Profiles)
	}

	if spark.TotalFeedback != 3 {
		t.Fatalf("spark feedback count = %d, want 3", spark.TotalFeedback)
	}
	if spark.Score <= 0 {
		t.Fatalf("spark score = %v, want > 0", spark.Score)
	}
	if spark.SpeedScore <= 0 {
		t.Fatalf("spark speed_score = %v, want > 0", spark.SpeedScore)
	}
	if spark.Cost != "cheap" {
		t.Fatalf("spark cost = %q, want %q", spark.Cost, "cheap")
	}
	if len(spark.RoleBreakdown) != 2 {
		t.Fatalf("spark role breakdown len = %d, want 2", len(spark.RoleBreakdown))
	}
	if len(spark.ParentBreakdown) != 2 {
		t.Fatalf("spark parent breakdown len = %d, want 2", len(spark.ParentBreakdown))
	}
	if len(spark.Trend) != 3 {
		t.Fatalf("spark trend len = %d, want 3", len(spark.Trend))
	}
	if !spark.HasEnoughSamples {
		t.Fatalf("spark HasEnoughSamples = false, want true")
	}
	if len(spark.RecentFeedback) != 3 {
		t.Fatalf("spark recent feedback len = %d, want 3", len(spark.RecentFeedback))
	}
	if !containsSignal(spark.Signals, "fast on average") {
		t.Fatalf("spark signals = %v, want fast signal", spark.Signals)
	}

	if devstral.TotalFeedback != 3 {
		t.Fatalf("devstral feedback count = %d, want 3", devstral.TotalFeedback)
	}
	if !containsSignal(devstral.Signals, "good at scout") {
		t.Fatalf("devstral signals = %v, want role-quality signal", devstral.Signals)
	}
	if !containsSignal(devstral.Signals, "slower on average") {
		t.Fatalf("devstral signals = %v, want slower signal", devstral.Signals)
	}

	if idle.TotalFeedback != 0 {
		t.Fatalf("idle feedback count = %d, want 0", idle.TotalFeedback)
	}
	if idle.SpeedScore != 50 {
		t.Fatalf("idle speed_score = %v, want 50", idle.SpeedScore)
	}
	if idle.HasEnoughSamples {
		t.Fatalf("idle HasEnoughSamples = true, want false")
	}
}

func TestBuildDashboardDifficultyAdjustedScoreRewardsHardTasks(t *testing.T) {
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	records := []FeedbackRecord{
		{
			ProjectID:     "proj-a",
			SpawnID:       1,
			ParentProfile: "lead-a",
			ChildProfile:  "easy-specialist",
			ChildRole:     "developer",
			Difficulty:    2,
			Quality:       9,
			DurationSecs:  40,
			CreatedAt:     now.Add(-3 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       2,
			ParentProfile: "lead-b",
			ChildProfile:  "easy-specialist",
			ChildRole:     "developer",
			Difficulty:    2,
			Quality:       9,
			DurationSecs:  45,
			CreatedAt:     now.Add(-2 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       3,
			ParentProfile: "lead-c",
			ChildProfile:  "easy-specialist",
			ChildRole:     "developer",
			Difficulty:    2,
			Quality:       9,
			DurationSecs:  42,
			CreatedAt:     now.Add(-time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       4,
			ParentProfile: "lead-a",
			ChildProfile:  "hard-specialist",
			ChildRole:     "developer",
			Difficulty:    9,
			Quality:       7,
			DurationSecs:  120,
			CreatedAt:     now.Add(-3 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       5,
			ParentProfile: "lead-b",
			ChildProfile:  "hard-specialist",
			ChildRole:     "developer",
			Difficulty:    9,
			Quality:       7,
			DurationSecs:  125,
			CreatedAt:     now.Add(-2 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       6,
			ParentProfile: "lead-c",
			ChildProfile:  "hard-specialist",
			ChildRole:     "developer",
			Difficulty:    9,
			Quality:       7,
			DurationSecs:  118,
			CreatedAt:     now.Add(-time.Hour),
		},
	}

	dashboard := BuildDashboard(nil, records)
	easy := findSummaryByName(t, dashboard, "easy-specialist")
	hard := findSummaryByName(t, dashboard, "hard-specialist")

	if hard.Score <= easy.Score {
		t.Fatalf("hard score = %.2f, easy score = %.2f, want hard > easy due to higher difficulty", hard.Score, easy.Score)
	}
	if hard.RoleBreakdown[0].Score <= easy.RoleBreakdown[0].Score {
		t.Fatalf("hard role score = %.2f, easy role score = %.2f, want hard > easy", hard.RoleBreakdown[0].Score, easy.RoleBreakdown[0].Score)
	}
}

func TestBuildJudgeWeightsDownweightsOutlierJudge(t *testing.T) {
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	records := make([]FeedbackRecord, 0, 24)
	spawnID := 1
	add := func(parent, child string, quality float64) {
		records = append(records, FeedbackRecord{
			ProjectID:     "proj-a",
			SpawnID:       spawnID,
			ParentProfile: parent,
			ChildProfile:  child,
			ChildRole:     "developer",
			Difficulty:    5,
			Quality:       quality,
			CreatedAt:     now,
		})
		spawnID++
	}
	for i := 0; i < 8; i++ {
		add("good-a", "alpha", 8)
		add("good-b", "alpha", 8)
		add("bad-z", "alpha", 0)
	}
	for i := 0; i < 8; i++ {
		add("good-a", "beta", 8)
		add("good-b", "beta", 8)
		add("bad-z", "beta", 0)
	}

	model := buildScoringModel(records)
	badWeight := model.judgeWeight("bad-z", "", "")
	goodWeight := model.judgeWeight("good-a", "", "")
	if badWeight >= goodWeight {
		t.Fatalf("bad judge weight = %.3f, good judge weight = %.3f, want bad < good", badWeight, goodWeight)
	}
	if badWeight > 0.46 {
		t.Fatalf("bad judge weight = %.3f, want <= 0.46", badWeight)
	}
}

func TestFeedbackForProfile(t *testing.T) {
	now := time.Now().UTC()
	records := []FeedbackRecord{
		{ProjectID: "p", SpawnID: 1, ChildProfile: "spark", Difficulty: 5, Quality: 8, CreatedAt: now.Add(-time.Hour)},
		{ProjectID: "p", SpawnID: 2, ChildProfile: "devstral", Difficulty: 5, Quality: 7, CreatedAt: now},
		{ProjectID: "p", SpawnID: 3, ChildProfile: "spark", Difficulty: 4, Quality: 9, CreatedAt: now},
	}
	got := FeedbackForProfile(records, "spark")
	if len(got) != 2 {
		t.Fatalf("FeedbackForProfile len = %d, want 2", len(got))
	}
	if got[0].SpawnID != 3 {
		t.Fatalf("first record spawn id = %d, want 3", got[0].SpawnID)
	}
}

func containsSignal(signals []string, wantPart string) bool {
	for _, s := range signals {
		if strings.Contains(strings.ToLower(s), strings.ToLower(wantPart)) {
			return true
		}
	}
	return false
}

func findSummaryByName(t *testing.T, dashboard Dashboard, profile string) ProfileSummary {
	t.Helper()
	for _, s := range dashboard.Profiles {
		if strings.EqualFold(s.Profile, profile) {
			return s
		}
	}
	t.Fatalf("profile %q not found in dashboard", profile)
	return ProfileSummary{}
}
