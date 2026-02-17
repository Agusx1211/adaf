package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/profilescore"
	"github.com/agusx1211/adaf/internal/store"
)

var spawnFeedbackCmd = &cobra.Command{
	Use:     "spawn-feedback",
	Aliases: []string{"spawn_feedback", "spawnfeedback"},
	Short:   "Record numeric performance feedback for a completed spawn",
	Long: `Record parent feedback for a completed spawn.

Feedback is stored globally across all projects and is used to compare worker
profiles by role, parent ratings, duration, and trend over time.`,
	RunE: runSpawnFeedback,
}

func init() {
	spawnFeedbackCmd.Flags().Int("spawn-id", 0, "Spawn ID to score (required)")
	spawnFeedbackCmd.Flags().Float64("difficulty", -1, "Task difficulty score from 0 to 10 (required)")
	spawnFeedbackCmd.Flags().Float64("quality", -1, "Worker quality score from 0 to 10 (required)")
	spawnFeedbackCmd.Flags().String("notes", "", "Optional feedback notes")
	spawnFeedbackCmd.Flags().String("parent-role", "", "Optional parent role override (defaults to ADAF_ROLE)")
	spawnFeedbackCmd.Flags().String("parent-position", "", "Optional parent position override (defaults to ADAF_POSITION)")
	rootCmd.AddCommand(spawnFeedbackCmd)
}

func runSpawnFeedback(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	difficulty, _ := cmd.Flags().GetFloat64("difficulty")
	quality, _ := cmd.Flags().GetFloat64("quality")
	notes, _ := cmd.Flags().GetString("notes")
	parentRole, _ := cmd.Flags().GetString("parent-role")
	parentPosition, _ := cmd.Flags().GetString("parent-position")

	if spawnID <= 0 {
		return fmt.Errorf("--spawn-id is required")
	}
	if difficulty < profilescore.MinScore || difficulty > profilescore.MaxScore {
		return fmt.Errorf("--difficulty must be between %.0f and %.0f", profilescore.MinScore, profilescore.MaxScore)
	}
	if quality < profilescore.MinScore || quality > profilescore.MaxScore {
		return fmt.Errorf("--quality must be between %.0f and %.0f", profilescore.MinScore, profilescore.MaxScore)
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	rec, err := s.GetSpawn(spawnID)
	if err != nil {
		return fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}
	if !store.IsTerminalSpawnStatus(rec.Status) {
		return fmt.Errorf("spawn %d is %s; feedback can only be recorded after completion", spawnID, rec.Status)
	}

	parentRole = strings.ToLower(strings.TrimSpace(parentRole))
	if parentRole == "" {
		parentRole = strings.ToLower(strings.TrimSpace(os.Getenv("ADAF_ROLE")))
	}
	parentPosition = strings.ToLower(strings.TrimSpace(parentPosition))
	if parentPosition == "" {
		parentPosition = strings.ToLower(strings.TrimSpace(os.Getenv("ADAF_POSITION")))
	}

	durationSecs := 0
	if !rec.StartedAt.IsZero() && !rec.CompletedAt.IsZero() && rec.CompletedAt.After(rec.StartedAt) {
		durationSecs = int(rec.CompletedAt.Sub(rec.StartedAt).Seconds())
	}

	projectName := ""
	if project, loadErr := s.LoadProject(); loadErr == nil && project != nil {
		projectName = strings.TrimSpace(project.Name)
	}

	feedback := profilescore.FeedbackRecord{
		ProjectID:      strings.TrimSpace(s.ProjectID()),
		ProjectName:    projectName,
		SpawnID:        rec.ID,
		ParentTurnID:   rec.ParentTurnID,
		ChildTurnID:    rec.ChildTurnID,
		ParentProfile:  rec.ParentProfile,
		ParentRole:     parentRole,
		ParentPosition: parentPosition,
		ChildProfile:   rec.ChildProfile,
		ChildRole:      rec.ChildRole,
		ChildPosition:  rec.ChildPosition,
		ChildStatus:    rec.Status,
		ExitCode:       rec.ExitCode,
		Task:           rec.Task,
		DurationSecs:   durationSecs,
		Difficulty:     difficulty,
		Quality:        quality,
		Notes:          strings.TrimSpace(notes),
		CreatedAt:      time.Now().UTC(),
	}

	saved, err := profilescore.Default().UpsertFeedback(feedback)
	if err != nil {
		return fmt.Errorf("saving feedback: %w", err)
	}

	fmt.Printf("Recorded feedback for spawn #%d (%s)\n", rec.ID, rec.ChildProfile)
	fmt.Printf("  quality=%.1f difficulty=%.1f duration=%ds\n", saved.Quality, saved.Difficulty, saved.DurationSecs)
	return nil
}
