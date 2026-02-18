package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/profilescore"
)

func TestDelegationSection_IncludesSkillPointerWhenDelegationEnabled(t *testing.T) {
	got := delegationSection(&config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "worker"},
		},
	}, nil, nil, "")

	if !strings.Contains(got, "# Delegation") {
		t.Fatalf("expected delegation header\nprompt:\n%s", got)
	}

	if !strings.Contains(got, "adaf skill delegation") {
		t.Fatalf("expected pointer to adaf skill delegation\nprompt:\n%s", got)
	}

	if !strings.Contains(got, "Maximum concurrent sub-agents") {
		t.Fatalf("expected max parallel info\nprompt:\n%s", got)
	}
}

func TestDelegationSection_IncludesRoutingDiscipline(t *testing.T) {
	got := delegationSection(&config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "worker"},
		},
	}, nil, nil, "")

	if !strings.Contains(got, "## Routing Discipline") {
		t.Fatalf("expected routing discipline section\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "distribute work across available profiles") {
		t.Fatalf("expected balance guidance in routing discipline\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Match difficulty to cost") {
		t.Fatalf("expected cost-matching guidance in routing discipline\nprompt:\n%s", got)
	}
}

func TestDelegationSection_ResourcePriorityGuidance(t *testing.T) {
	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "worker"},
		},
	}

	costPrompt := delegationSection(deleg, nil, nil, config.ResourcePriorityCost)
	if !strings.Contains(costPrompt, "Current priority: **cost**") {
		t.Fatalf("expected cost priority section\nprompt:\n%s", costPrompt)
	}
	if !strings.Contains(costPrompt, "free`/`cheap` profiles") {
		t.Fatalf("expected cheap-first guidance for cost mode\nprompt:\n%s", costPrompt)
	}

	qualityPrompt := delegationSection(deleg, nil, nil, config.ResourcePriorityQuality)
	if !strings.Contains(qualityPrompt, "Current priority: **quality**") {
		t.Fatalf("expected quality priority section\nprompt:\n%s", qualityPrompt)
	}
	if !strings.Contains(qualityPrompt, "for review, QA, and scouting/research passes") {
		t.Fatalf("expected cheap-for-review/scouting guidance for quality mode\nprompt:\n%s", qualityPrompt)
	}
}

func TestDelegationSection_NoDelegation(t *testing.T) {
	got := delegationSection(nil, nil, nil, "")
	if got != "" {
		t.Fatalf("delegationSection(nil) = %q, want empty", got)
	}

	got = delegationSection(&config.DelegationConfig{}, nil, nil, "")
	if got != "" {
		t.Fatalf("delegationSection(empty) = %q, want empty", got)
	}
}

func TestDelegationSection_IncludesRoleDetails(t *testing.T) {
	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{
				Name:     "worker",
				Position: config.PositionWorker,
				Role:     config.RoleDeveloper,
			},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "worker", Agent: "codex"},
		},
	}

	got := delegationSection(deleg, globalCfg, nil, "")
	if !strings.Contains(got, "role=developer") {
		t.Fatalf("expected role annotation in delegation section\nprompt:\n%s", got)
	}
}

func TestReadOnlyPrompt_RequiresFinalMessageReport(t *testing.T) {
	got := ReadOnlyPrompt()

	if !strings.Contains(got, "Do NOT write reports into repository files") {
		t.Fatalf("expected read-only prompt to forbid writing reports to repo files\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "final assistant message") {
		t.Fatalf("expected read-only prompt to require final assistant message reporting\nprompt:\n%s", got)
	}
}

func TestRolePrompt_ComposesRulesFromCatalog(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Roles: []config.RoleDefinition{
			{
				Name:         "reviewer",
				Title:        "REVIEWER",
				Description:  "Read and assess changes.",
				CanWriteCode: false,
				RuleIDs:      []string{"r1", "r2"},
			},
		},
		PromptRules: []config.PromptRule{
			{ID: "r1", Body: "Rule one body."},
			{ID: "r2", Body: "Rule two body."},
		},
		DefaultRole: "reviewer",
	}
	got := RolePrompt(&config.Profile{Name: "p1"}, "reviewer", globalCfg)

	if !strings.Contains(got, "# Your Role: REVIEWER") {
		t.Fatalf("expected role title in prompt\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Rule one body.") {
		t.Fatalf("expected first rule body in prompt\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Rule two body.") {
		t.Fatalf("expected second rule body in prompt\nprompt:\n%s", got)
	}
}

func TestRolePrompt_DoesNotRenderDownstreamCommunicationRule(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Roles: []config.RoleDefinition{
			{
				Name:         "developer",
				Title:        "DEVELOPER",
				Description:  "Executes implementation.",
				CanWriteCode: true,
				RuleIDs:      []string{"dev_rule", config.RuleCommunicationDownstream},
			},
		},
		PromptRules: []config.PromptRule{
			{ID: "dev_rule", Body: "Developer identity."},
			{ID: config.RuleCommunicationDownstream, Body: "## Communication Style: Downstream Only\n\n- `adaf spawn-message --spawn-id 1 \"fix\"`"},
		},
		DefaultRole: "developer",
	}

	got := RolePrompt(&config.Profile{Name: "p1"}, "developer", globalCfg)
	if strings.Contains(got, "Communication Style: Downstream Only") {
		t.Fatalf("downstream communication should be emitted only in delegation context\nprompt:\n%s", got)
	}
	if strings.Contains(got, "adaf spawn-message") {
		t.Fatalf("spawn communication commands should come from delegation context, not role prompt\nprompt:\n%s", got)
	}
}

func TestRolePrompt_RendersRoleIdentityFromRoleDefinition(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Roles: []config.RoleDefinition{
			{
				Name:         "qa",
				Title:        "QA",
				Identity:     "You are a QA role focused on high-signal verification.",
				Description:  "Verification specialist.",
				CanWriteCode: true,
			},
		},
		PromptRules: []config.PromptRule{
			{ID: "shared_checks", Body: "Always include repro steps."},
		},
		DefaultRole: "qa",
	}

	got := RolePrompt(&config.Profile{Name: "p1"}, "qa", globalCfg)
	if !strings.Contains(got, "You are a QA role focused on high-signal verification.") {
		t.Fatalf("role identity should render from role definition\nprompt:\n%s", got)
	}
}

func TestDelegationSection_IncludesRoutingScoresSpeedAndCostTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := profilescore.Default()
	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	records := []profilescore.FeedbackRecord{
		{
			ProjectID:     "proj-a",
			SpawnID:       1,
			ParentProfile: "lead-a",
			ChildProfile:  "worker",
			ChildRole:     "developer",
			Difficulty:    8,
			Quality:       7,
			DurationSecs:  110,
			CreatedAt:     now.Add(-2 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       2,
			ParentProfile: "lead-b",
			ChildProfile:  "worker",
			ChildRole:     "developer",
			Difficulty:    8,
			Quality:       7,
			DurationSecs:  100,
			CreatedAt:     now.Add(-time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       3,
			ParentProfile: "lead-a",
			ChildProfile:  "worker",
			ChildRole:     "researcher",
			Difficulty:    3,
			Quality:       9,
			DurationSecs:  40,
			CreatedAt:     now,
		},
	}
	for _, rec := range records {
		if _, err := store.UpsertFeedback(rec); err != nil {
			t.Fatalf("UpsertFeedback(%d): %v", rec.SpawnID, err)
		}
	}

	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{
				Name: "worker",
				Role: config.RoleDeveloper,
			},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "worker", Agent: "codex", Cost: "cheap"},
		},
	}

	got := delegationSection(deleg, globalCfg, nil, "")
	if !strings.Contains(got, "cost=cheap") {
		t.Fatalf("expected profile cost in available profiles section\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "## Routing Scoreboard") {
		t.Fatalf("expected routing scoreboard section\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "### Profile Baseline") {
		t.Fatalf("expected profile baseline table heading\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| Profile | Cost | Speed |") {
		t.Fatalf("expected profile baseline table header\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| worker | cheap |") {
		t.Fatalf("expected profile baseline row with cost\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "### Score by Role") {
		t.Fatalf("expected score by role table heading\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| Profile | developer |") {
		t.Fatalf("expected role matrix table header with available roles\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| worker |") {
		t.Fatalf("expected role matrix row for worker profile\nprompt:\n%s", got)
	}
	if strings.Contains(got, "| Profile | Cost | Role | Score | Speed | Feedback |") {
		t.Fatalf("legacy single-table scoreboard should not be present\nprompt:\n%s", got)
	}
}

func TestDelegationSection_RoutingScoreboardIncludesAllProfilesWithAvailabilityAndSparseDataMarkers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := profilescore.Default()
	now := time.Date(2026, 2, 17, 11, 0, 0, 0, time.UTC)
	records := []profilescore.FeedbackRecord{
		{
			ProjectID:     "proj-a",
			SpawnID:       1,
			ParentProfile: "lead-a",
			ChildProfile:  "codex 5.3",
			ChildRole:     "qa",
			Difficulty:    7,
			Quality:       8,
			DurationSecs:  70,
			CreatedAt:     now.Add(-3 * time.Hour),
		},
		{
			ProjectID:     "proj-a",
			SpawnID:       2,
			ParentProfile: "lead-a",
			ChildProfile:  "codex 5.3 spark",
			ChildRole:     "backend-designer",
			Difficulty:    7,
			Quality:       7,
			DurationSecs:  50,
			CreatedAt:     now.Add(-2 * time.Hour),
		},
	}
	for _, rec := range records {
		if _, err := store.UpsertFeedback(rec); err != nil {
			t.Fatalf("UpsertFeedback(%d): %v", rec.SpawnID, err)
		}
	}

	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "opus 4.6", Roles: []string{"developer", "ui-designer", "researcher"}},
			{Name: "codex 5.3", Roles: []string{"developer", "backend-designer", "ui-designer", "qa", "researcher"}},
			{Name: "codex 5.3 spark", Roles: []string{"developer", "backend-designer", "researcher"}},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "opus 4.6", Agent: "claude", Cost: "expensive"},
			{Name: "codex 5.3", Agent: "codex", Cost: "expensive"},
			{Name: "codex 5.3 spark", Agent: "codex", Cost: "normal"},
		},
	}

	got := delegationSection(deleg, globalCfg, nil, "")
	if !strings.Contains(got, "| opus 4.6 | expensive | ... |") {
		t.Fatalf("expected baseline row for profile without feedback to show sparse marker\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| codex 5.3 | expensive |") {
		t.Fatalf("expected baseline row for codex 5.3 profile\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| codex 5.3 spark | normal |") {
		t.Fatalf("expected baseline row for codex 5.3 spark profile\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| Profile | backend-designer | developer | qa | researcher | ui-designer |") {
		t.Fatalf("expected role matrix to include all available delegation roles\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| opus 4.6 | -- | ... | -- | ... | ... |") {
		t.Fatalf("expected opus role matrix row with availability marker and sparse marker\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| codex 5.3 | ... | ... |") {
		t.Fatalf("expected codex 5.3 role matrix row to include sparse markers\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "| codex 5.3 spark |") {
		t.Fatalf("expected codex 5.3 spark role matrix row\nprompt:\n%s", got)
	}
}
