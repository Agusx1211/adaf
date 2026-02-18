package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var wikiCmd = &cobra.Command{
	Use:     "wiki",
	Aliases: []string{"knowledge"},
	Short:   "Manage shared wiki entries",
	Long: `Create, list, search, show, update, and delete project wiki entries.

Wiki is shared memory across sessions and agents. Keep entries concise, current,
and high-signal. If you discover stale information, update the wiki immediately.

Examples:
  adaf wiki list
  adaf wiki search "release process"
  adaf wiki create --title "Runbook" --content-file runbook.md
  adaf wiki show runbook
  adaf wiki update runbook --content "..."
  adaf wiki delete runbook`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var wikiListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List wiki entries",
	RunE:    runWikiList,
}

var wikiSearchCmd = &cobra.Command{
	Use:     "search <query>",
	Aliases: []string{"find", "grep"},
	Short:   "Fuzzy search wiki entries",
	Args:    cobra.MinimumNArgs(1),
	RunE:    runWikiSearch,
}

var wikiShowCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"get", "view", "display"},
	Short:   "Show a wiki entry",
	Args:    cobra.ExactArgs(1),
	RunE:    runWikiShow,
}

var wikiCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new", "add"},
	Short:   "Create a wiki entry",
	RunE:    runWikiCreate,
}

var wikiUpdateCmd = &cobra.Command{
	Use:     "update <id>",
	Aliases: []string{"edit", "modify", "set"},
	Short:   "Update a wiki entry",
	Args:    cobra.ExactArgs(1),
	RunE:    runWikiUpdate,
}

var wikiDeleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a wiki entry",
	Args:    cobra.ExactArgs(1),
	RunE:    runWikiDelete,
}

func init() {
	wikiListCmd.Flags().String("plan", "", "Filter for a plan context (shared + plan-scoped)")
	wikiListCmd.Flags().Bool("shared", false, "Show shared wiki only")

	wikiSearchCmd.Flags().String("plan", "", "Filter for a plan context (shared + plan-scoped)")
	wikiSearchCmd.Flags().Bool("shared", false, "Show shared wiki only")
	wikiSearchCmd.Flags().Int("limit", 20, "Maximum number of matches")

	wikiCreateCmd.Flags().String("title", "", "Wiki title (derived from --id if omitted)")
	wikiCreateCmd.Flags().String("file", "", "Read content from file")
	wikiCreateCmd.Flags().String("content-file", "", "Read content from file (use '-' for stdin)")
	wikiCreateCmd.Flags().String("content", "", "Wiki content (inline)")
	wikiCreateCmd.Flags().String("id", "", "Custom wiki ID (optional, auto-generated if not set)")
	wikiCreateCmd.Flags().String("plan", "", "Plan scope for this wiki entry (empty = shared)")
	wikiCreateCmd.Flags().String("by", "", "Actor identity for change history (defaults from agent context)")

	wikiUpdateCmd.Flags().String("title", "", "New title")
	wikiUpdateCmd.Flags().String("file", "", "Read new content from file")
	wikiUpdateCmd.Flags().String("content-file", "", "Read new content from file (use '-' for stdin)")
	wikiUpdateCmd.Flags().String("content", "", "New content (inline)")
	wikiUpdateCmd.Flags().String("plan", "", "Move entry to a plan scope (empty = shared)")
	wikiUpdateCmd.Flags().String("by", "", "Actor identity for change history (defaults from agent context)")

	wikiCmd.AddCommand(wikiListCmd)
	wikiCmd.AddCommand(wikiSearchCmd)
	wikiCmd.AddCommand(wikiShowCmd)
	wikiCmd.AddCommand(wikiCreateCmd)
	wikiCmd.AddCommand(wikiUpdateCmd)
	wikiCmd.AddCommand(wikiDeleteCmd)
	rootCmd.AddCommand(wikiCmd)
}

func runWikiList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	wiki, _, err := resolveWikiScope(cmd, s)
	if err != nil {
		return err
	}

	printHeader("Wiki")
	if len(wiki) == 0 {
		fmt.Printf("  %sNo wiki entries found.%s\n\n", colorDim, colorReset)
		return nil
	}

	printWikiTable(wiki)
	fmt.Printf("\n  %sTotal: %d wiki entr", colorDim, len(wiki))
	if len(wiki) == 1 {
		fmt.Printf("y%s\n\n", colorReset)
	} else {
		fmt.Printf("ies%s\n\n", colorReset)
	}
	return nil
}

func runWikiSearch(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		return fmt.Errorf("query is required")
	}

	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		return fmt.Errorf("--limit must be > 0")
	}

	results, err := s.SearchWiki(query, limit*5)
	if err != nil {
		return fmt.Errorf("searching wiki: %w", err)
	}

	_, scopeFilter, err := resolveWikiScope(cmd, s)
	if err != nil {
		return err
	}
	filtered := make([]store.WikiEntry, 0, len(results))
	for _, entry := range results {
		if scopeFilter(entry) {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	printHeader("Wiki Search")
	printField("Query", query)
	printField("Matches", fmt.Sprintf("%d", len(filtered)))
	fmt.Println()

	if len(filtered) == 0 {
		fmt.Printf("  %sNo wiki matches found.%s\n\n", colorDim, colorReset)
		return nil
	}

	printWikiTable(filtered)
	fmt.Println()
	return nil
}

func runWikiShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	entry, err := s.GetWikiEntry(args[0])
	if err != nil {
		return fmt.Errorf("getting wiki entry %q: %w", args[0], err)
	}

	printHeader(fmt.Sprintf("Wiki: %s", entry.Title))
	printField("ID", entry.ID)
	if entry.PlanID != "" {
		printField("Plan", entry.PlanID)
	} else {
		printField("Plan", "shared")
	}
	printField("Version", fmt.Sprintf("%d", entry.Version))
	printField("Created", entry.Created.Format("2006-01-02 15:04:05"))
	printField("Created By", fallbackWikiActor(entry.CreatedBy, "unknown"))
	printField("Updated", entry.Updated.Format("2006-01-02 15:04:05"))
	printField("Updated By", fallbackWikiActor(entry.UpdatedBy, fallbackWikiActor(entry.CreatedBy, "unknown")))

	if len(entry.History) > 0 {
		fmt.Println()
		fmt.Printf("  %sHistory:%s\n", colorBold, colorReset)
		history := entry.History
		if len(history) > 8 {
			history = history[len(history)-8:]
		}
		for _, change := range history {
			action := strings.TrimSpace(change.Action)
			if action == "" {
				action = "update"
			}
			fmt.Printf("  - v%d %s by %s at %s\n",
				change.Version,
				action,
				fallbackWikiActor(change.By, "unknown"),
				change.At.Format("2006-01-02 15:04:05"),
			)
		}
	}

	fmt.Println()
	fmt.Println(colorDim + "  " + strings.Repeat("-", 60) + colorReset)
	fmt.Println()

	for _, line := range strings.Split(entry.Content, "\n") {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println()
	return nil
}

func runWikiCreate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	title, _ := cmd.Flags().GetString("title")
	file, _ := cmd.Flags().GetString("file")
	contentFile, _ := cmd.Flags().GetString("content-file")
	content, _ := cmd.Flags().GetString("content")
	id, _ := cmd.Flags().GetString("id")
	planID, _ := cmd.Flags().GetString("plan")
	by, _ := cmd.Flags().GetString("by")

	// Derive title from --id when --title is omitted.
	title = strings.TrimSpace(title)
	if title == "" && strings.TrimSpace(id) != "" {
		title = wikiTitleFromID(strings.TrimSpace(id))
	}
	if title == "" {
		return fmt.Errorf("--title is required (or provide --id to auto-derive a title)")
	}

	planID = strings.TrimSpace(planID)
	if planID != "" {
		if err := validatePlanID(planID); err != nil {
			return err
		}
		plan, err := s.GetPlan(planID)
		if err != nil {
			return fmt.Errorf("loading plan %q: %w", planID, err)
		}
		if plan == nil {
			return fmt.Errorf("plan %q not found", planID)
		}
	}

	resolvedContentFile := strings.TrimSpace(contentFile)
	if resolvedContentFile == "" {
		resolvedContentFile = strings.TrimSpace(file)
	}
	if content == "-" && resolvedContentFile == "" {
		resolvedContentFile = "-"
	}
	content, err = resolveTextFlag(content, resolvedContentFile)
	if err != nil {
		return fmt.Errorf("resolving content: %w", err)
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("provide content via --content/--content-file (or legacy --file)")
	}

	actor := resolveWikiActor(by)
	entry := &store.WikiEntry{
		ID:        strings.TrimSpace(id),
		PlanID:    planID,
		Title:     strings.TrimSpace(title),
		Content:   content,
		CreatedBy: actor,
		UpdatedBy: actor,
	}
	if err := s.CreateWikiEntry(entry); err != nil {
		return fmt.Errorf("creating wiki entry: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sWiki entry created.%s\n", styleBoldGreen, colorReset)
	printField("ID", entry.ID)
	printField("Title", entry.Title)
	if entry.PlanID != "" {
		printField("Plan", entry.PlanID)
	} else {
		printField("Plan", "shared")
	}
	printField("By", fallbackWikiActor(entry.UpdatedBy, "unknown"))
	printField("Version", fmt.Sprintf("%d", entry.Version))
	printField("Size", fmt.Sprintf("%d bytes", len(entry.Content)))
	fmt.Println()
	return nil
}

func runWikiUpdate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	entryID := strings.TrimSpace(args[0])
	entry, err := s.GetWikiEntry(entryID)
	if err != nil {
		return fmt.Errorf("getting wiki entry %q: %w", entryID, err)
	}

	changed := false
	if cmd.Flags().Changed("title") {
		title, _ := cmd.Flags().GetString("title")
		entry.Title = strings.TrimSpace(title)
		changed = true
	}

	contentFileChanged := cmd.Flags().Changed("content-file")
	legacyFileChanged := cmd.Flags().Changed("file")
	contentChanged := cmd.Flags().Changed("content")
	if contentFileChanged || legacyFileChanged || contentChanged {
		content, _ := cmd.Flags().GetString("content")
		file, _ := cmd.Flags().GetString("file")
		contentFile, _ := cmd.Flags().GetString("content-file")
		resolvedContentFile := strings.TrimSpace(contentFile)
		if resolvedContentFile == "" && legacyFileChanged {
			resolvedContentFile = strings.TrimSpace(file)
		}
		if content == "-" && resolvedContentFile == "" {
			resolvedContentFile = "-"
		}
		resolvedContent, err := resolveTextFlag(content, resolvedContentFile)
		if err != nil {
			return fmt.Errorf("resolving content: %w", err)
		}
		entry.Content = resolvedContent
		changed = true
	}

	if cmd.Flags().Changed("plan") {
		planID, _ := cmd.Flags().GetString("plan")
		planID = strings.TrimSpace(planID)
		if planID != "" {
			if err := validatePlanID(planID); err != nil {
				return err
			}
			plan, err := s.GetPlan(planID)
			if err != nil {
				return fmt.Errorf("loading plan %q: %w", planID, err)
			}
			if plan == nil {
				return fmt.Errorf("plan %q not found", planID)
			}
		}
		entry.PlanID = planID
		changed = true
	}

	if !changed {
		return fmt.Errorf("no fields to update (use --title, --file, --content, or --plan)")
	}

	by, _ := cmd.Flags().GetString("by")
	entry.UpdatedBy = resolveWikiActor(by)
	if err := s.UpdateWikiEntry(entry); err != nil {
		return fmt.Errorf("updating wiki entry: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sWiki entry %s updated.%s\n", styleBoldGreen, entry.ID, colorReset)
	printField("Title", entry.Title)
	if entry.PlanID != "" {
		printField("Plan", entry.PlanID)
	} else {
		printField("Plan", "shared")
	}
	printField("By", fallbackWikiActor(entry.UpdatedBy, "unknown"))
	printField("Version", fmt.Sprintf("%d", entry.Version))
	printField("Size", fmt.Sprintf("%d bytes", len(entry.Content)))
	fmt.Println()
	return nil
}

func runWikiDelete(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	entryID := strings.TrimSpace(args[0])
	if entryID == "" {
		return fmt.Errorf("wiki entry id is required")
	}
	if err := s.DeleteWikiEntry(entryID); err != nil {
		return fmt.Errorf("deleting wiki entry: %w", err)
	}
	fmt.Println()
	fmt.Printf("  %sWiki entry %s deleted.%s\n\n", styleBoldGreen, entryID, colorReset)
	return nil
}

func resolveWikiScope(cmd *cobra.Command, s *store.Store) ([]store.WikiEntry, func(store.WikiEntry) bool, error) {
	planFilter, _ := cmd.Flags().GetString("plan")
	sharedOnly, _ := cmd.Flags().GetBool("shared")
	planFilter = strings.TrimSpace(planFilter)
	if sharedOnly && planFilter != "" {
		return nil, nil, fmt.Errorf("use either --shared or --plan, not both")
	}

	if planFilter != "" {
		if err := validatePlanID(planFilter); err != nil {
			return nil, nil, err
		}
		plan, err := s.GetPlan(planFilter)
		if err != nil {
			return nil, nil, fmt.Errorf("loading plan %q: %w", planFilter, err)
		}
		if plan == nil {
			return nil, nil, fmt.Errorf("plan %q not found", planFilter)
		}
		wiki, err := s.ListWikiForPlan(planFilter)
		return wiki, func(entry store.WikiEntry) bool {
			return entry.PlanID == "" || entry.PlanID == planFilter
		}, err
	}

	if sharedOnly {
		wiki, err := s.ListSharedWiki()
		return wiki, func(entry store.WikiEntry) bool { return entry.PlanID == "" }, err
	}

	project, _ := s.LoadProject()
	if project != nil && strings.TrimSpace(project.ActivePlanID) != "" {
		activePlanID := strings.TrimSpace(project.ActivePlanID)
		wiki, err := s.ListWikiForPlan(activePlanID)
		return wiki, func(entry store.WikiEntry) bool {
			return entry.PlanID == "" || entry.PlanID == activePlanID
		}, err
	}

	wiki, err := s.ListSharedWiki()
	return wiki, func(entry store.WikiEntry) bool { return entry.PlanID == "" }, err
}

func printWikiTable(entries []store.WikiEntry) {
	headers := []string{"ID", "PLAN", "TITLE", "UPDATED", "BY", "VER", "SIZE"}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		size := fmt.Sprintf("%d bytes", len(entry.Content))
		if len(entry.Content) > 1024 {
			size = fmt.Sprintf("%.1f KB", float64(len(entry.Content))/1024)
		}
		scope := "shared"
		if entry.PlanID != "" {
			scope = entry.PlanID
		}
		updated := "-"
		if !entry.Updated.IsZero() {
			updated = entry.Updated.Format("2006-01-02 15:04")
		}
		rows = append(rows, []string{
			entry.ID,
			scope,
			truncate(entry.Title, 36),
			updated,
			truncate(fallbackWikiActor(entry.UpdatedBy, fallbackWikiActor(entry.CreatedBy, "unknown")), 18),
			fmt.Sprintf("%d", entry.Version),
			size,
		})
	}
	printTable(headers, rows)
}

func resolveWikiActor(raw string) string {
	actor := strings.TrimSpace(raw)
	if actor != "" {
		return actor
	}

	if profile := strings.TrimSpace(os.Getenv("ADAF_PROFILE")); profile != "" {
		return profile
	}
	if role := strings.TrimSpace(os.Getenv("ADAF_ROLE")); role != "" {
		return role
	}
	if position := strings.TrimSpace(os.Getenv("ADAF_POSITION")); position != "" {
		return position
	}
	if os.Getenv("ADAF_AGENT") == "1" {
		return "agent"
	}
	return "human"
}

func fallbackWikiActor(actor, fallback string) string {
	value := strings.TrimSpace(actor)
	if value == "" {
		return fallback
	}
	return value
}

// wikiTitleFromID derives a human-readable title from a wiki ID slug
// (e.g. "testing-conventions" -> "Testing Conventions").
func wikiTitleFromID(id string) string {
	id = strings.ReplaceAll(id, "-", " ")
	id = strings.ReplaceAll(id, "_", " ")
	words := strings.Fields(id)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
