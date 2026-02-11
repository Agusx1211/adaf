package cli

import (
	"fmt"
	"os"
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
	docCreateCmd.Flags().String("title", "", "Document title (required)")
	docCreateCmd.Flags().String("file", "", "Read content from file")
	docCreateCmd.Flags().String("content", "", "Document content (inline)")
	docCreateCmd.Flags().String("id", "", "Custom document ID (optional, auto-generated if not set)")
	_ = docCreateCmd.MarkFlagRequired("title")

	docUpdateCmd.Flags().String("title", "", "New title")
	docUpdateCmd.Flags().String("file", "", "Read new content from file")
	docUpdateCmd.Flags().String("content", "", "New content (inline)")

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

	docs, err := s.ListDocs()
	if err != nil {
		return fmt.Errorf("listing docs: %w", err)
	}

	printHeader("Documents")

	if len(docs) == 0 {
		fmt.Printf("  %sNo documents found.%s\n\n", colorDim, colorReset)
		return nil
	}

	headers := []string{"ID", "TITLE", "UPDATED", "SIZE"}
	var rows [][]string
	for _, d := range docs {
		size := fmt.Sprintf("%d bytes", len(d.Content))
		if len(d.Content) > 1024 {
			size = fmt.Sprintf("%.1f KB", float64(len(d.Content))/1024)
		}
		rows = append(rows, []string{
			d.ID,
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
	content, _ := cmd.Flags().GetString("content")
	id, _ := cmd.Flags().GetString("id")

	// Get content from file or flag
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}
		content = string(data)
	}

	if content == "" {
		return fmt.Errorf("provide content via --file or --content flag")
	}

	doc := &store.Doc{
		ID:      id,
		Title:   title,
		Content: content,
	}

	if err := s.CreateDoc(doc); err != nil {
		return fmt.Errorf("creating doc: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sDocument created.%s\n", styleBoldGreen, colorReset)
	printField("ID", doc.ID)
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

	if cmd.Flags().Changed("file") {
		file, _ := cmd.Flags().GetString("file")
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}
		doc.Content = string(data)
		changed = true
	} else if cmd.Flags().Changed("content") {
		content, _ := cmd.Flags().GetString("content")
		doc.Content = content
		changed = true
	}

	if !changed {
		return fmt.Errorf("no fields to update (use --title, --file, or --content)")
	}

	if err := s.UpdateDoc(doc); err != nil {
		return fmt.Errorf("updating doc: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sDocument %s updated.%s\n", styleBoldGreen, doc.ID, colorReset)
	printField("Title", doc.Title)
	printField("Size", fmt.Sprintf("%d bytes", len(doc.Content)))
	fmt.Println()

	return nil
}
