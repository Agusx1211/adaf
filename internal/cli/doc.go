package cli

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var docCmd = &cobra.Command{
	Use:     "doc",
	Aliases: []string{"docs", "document", "documents"},
	Short:   "Manage project documents",
	Long: `Create, list, show, and update project documentation tracked by adaf.

Documents can store API specs, architecture notes, onboarding guides, or any
text content relevant to the project. Content can be provided inline or read
from a file.

Examples:
  adaf doc list
  adaf doc create --title "API Spec" --file openapi.yaml
  adaf doc create --title "Architecture" --content "..."
  adaf doc show api-spec
  adaf doc update api-spec --file openapi-v2.yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var docListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all documents",
	RunE:    runDocList,
}

var docShowCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"get", "view", "display"},
	Short:   "Show a document",
	Args:    cobra.ExactArgs(1),
	RunE:    runDocShow,
}

var docCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new", "add"},
	Short:   "Create a new document",
	RunE:    runDocCreate,
}

var docUpdateCmd = &cobra.Command{
	Use:     "update <id>",
	Aliases: []string{"edit", "modify", "set"},
	Short:   "Update a document",
	Args:    cobra.ExactArgs(1),
	RunE:    runDocUpdate,
}

func init() {
	docListCmd.Flags().String("plan", "", "Filter for a plan context (shared + plan-scoped)")
	docListCmd.Flags().Bool("shared", false, "Show shared docs only")

	docCreateCmd.Flags().String("title", "", "Document title (required)")
	docCreateCmd.Flags().String("file", "", "Read content from file")
	docCreateCmd.Flags().String("content-file", "", "Read content from file (use '-' for stdin)")
	docCreateCmd.Flags().String("content", "", "Document content (inline)")
	docCreateCmd.Flags().String("id", "", "Custom document ID (optional, auto-generated if not set)")
	docCreateCmd.Flags().String("plan", "", "Plan scope for this doc (empty = shared)")
	_ = docCreateCmd.MarkFlagRequired("title")

	docUpdateCmd.Flags().String("title", "", "New title")
	docUpdateCmd.Flags().String("file", "", "Read new content from file")
	docUpdateCmd.Flags().String("content-file", "", "Read new content from file (use '-' for stdin)")
	docUpdateCmd.Flags().String("content", "", "New content (inline)")
	docUpdateCmd.Flags().String("plan", "", "Move doc to a plan scope (empty = shared)")

	docCmd.AddCommand(docListCmd)
	docCmd.AddCommand(docShowCmd)
	docCmd.AddCommand(docCreateCmd)
	docCmd.AddCommand(docUpdateCmd)
	rootCmd.AddCommand(docCmd)
}

func runDocList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	planFilter, _ := cmd.Flags().GetString("plan")
	sharedOnly, _ := cmd.Flags().GetBool("shared")
	var docs []store.Doc
	switch {
	case sharedOnly:
		docs, err = s.ListSharedDocs()
	case strings.TrimSpace(planFilter) != "":
		planFilter = strings.TrimSpace(planFilter)
		if err := validatePlanID(planFilter); err != nil {
			return err
		}
		plan, err := s.GetPlan(planFilter)
		if err != nil {
			return fmt.Errorf("loading plan %q: %w", planFilter, err)
		}
		if plan == nil {
			return fmt.Errorf("plan %q not found", planFilter)
		}
		docs, err = s.ListDocsForPlan(planFilter)
	default:
		project, _ := s.LoadProject()
		if project != nil && project.ActivePlanID != "" {
			docs, err = s.ListDocsForPlan(project.ActivePlanID)
		} else {
			docs, err = s.ListSharedDocs()
		}
	}
	if err != nil {
		return fmt.Errorf("listing docs: %w", err)
	}

	printHeader("Documents")

	if len(docs) == 0 {
		fmt.Printf("  %sNo documents found.%s\n\n", colorDim, colorReset)
		return nil
	}

	headers := []string{"ID", "PLAN", "TITLE", "UPDATED", "SIZE"}
	var rows [][]string
	for _, d := range docs {
		size := fmt.Sprintf("%d bytes", len(d.Content))
		if len(d.Content) > 1024 {
			size = fmt.Sprintf("%.1f KB", float64(len(d.Content))/1024)
		}
		scope := "shared"
		if d.PlanID != "" {
			scope = d.PlanID
		}
		rows = append(rows, []string{
			d.ID,
			scope,
			truncate(d.Title, 45),
			d.Updated.Format("2006-01-02 15:04"),
			size,
		})
	}
	printTable(headers, rows)

	fmt.Printf("\n  %sTotal: %d document(s)%s\n\n", colorDim, len(docs), colorReset)
	return nil
}

func runDocShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	doc, err := s.GetDoc(args[0])
	if err != nil {
		return fmt.Errorf("getting doc %q: %w", args[0], err)
	}

	printHeader(fmt.Sprintf("Document: %s", doc.Title))
	printField("ID", doc.ID)
	if doc.PlanID != "" {
		printField("Plan", doc.PlanID)
	} else {
		printField("Plan", "shared")
	}
	printField("Created", doc.Created.Format("2006-01-02 15:04:05"))
	printField("Updated", doc.Updated.Format("2006-01-02 15:04:05"))

	fmt.Println()
	fmt.Println(colorDim + "  " + strings.Repeat("-", 60) + colorReset)
	fmt.Println()

	// Print content with indentation
	for _, line := range strings.Split(doc.Content, "\n") {
		fmt.Printf("  %s\n", line)
	}

	fmt.Println()
	return nil
}

func runDocCreate(cmd *cobra.Command, args []string) error {
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

	if content == "" {
		return fmt.Errorf("provide content via --content/--content-file (or legacy --file)")
	}

	doc := &store.Doc{
		ID:      id,
		PlanID:  planID,
		Title:   title,
		Content: content,
	}

	if err := s.CreateDoc(doc); err != nil {
		return fmt.Errorf("creating doc: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sDocument created.%s\n", styleBoldGreen, colorReset)
	printField("ID", doc.ID)
	if doc.PlanID != "" {
		printField("Plan", doc.PlanID)
	} else {
		printField("Plan", "shared")
	}
	printField("Title", doc.Title)
	printField("Size", fmt.Sprintf("%d bytes", len(doc.Content)))
	fmt.Println()

	return nil
}

func runDocUpdate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	docID := args[0]

	doc, err := s.GetDoc(docID)
	if err != nil {
		return fmt.Errorf("getting doc %q: %w", docID, err)
	}

	changed := false

	if cmd.Flags().Changed("title") {
		title, _ := cmd.Flags().GetString("title")
		doc.Title = title
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
		doc.Content = resolvedContent
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
		doc.PlanID = planID
		changed = true
	}

	if !changed {
		return fmt.Errorf("no fields to update (use --title, --file, --content, or --plan)")
	}

	if err := s.UpdateDoc(doc); err != nil {
		return fmt.Errorf("updating doc: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sDocument %s updated.%s\n", styleBoldGreen, doc.ID, colorReset)
	printField("Title", doc.Title)
	if doc.PlanID != "" {
		printField("Plan", doc.PlanID)
	} else {
		printField("Plan", "shared")
	}
	printField("Size", fmt.Sprintf("%d bytes", len(doc.Content)))
	fmt.Println()

	return nil
}
