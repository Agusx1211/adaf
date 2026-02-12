package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/store"
)

// --- adaf note ---

var noteCmd = &cobra.Command{
	Use:     "note",
	Aliases: []string{"notes"},
	Short:   "Supervisor notes for running agent turns",
}

// --- adaf note add ---

var noteAddCmd = &cobra.Command{
	Use:     "add",
	Aliases: []string{"create", "new"},
	Short:   "Add a supervisor note to a turn",
	RunE:    runNoteAdd,
}

// --- adaf note list ---

var noteListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List supervisor notes",
	RunE:    runNoteList,
}

func init() {
	noteAddCmd.Flags().Int("session", 0, "Target turn ID (optional in agent context)")
	noteAddCmd.Flags().String("note", "", "Note text (required)")
	noteAddCmd.Flags().String("author", "supervisor", "Author name")

	noteListCmd.Flags().Int("session", 0, "Filter by turn ID (0 = all)")

	noteCmd.AddCommand(noteAddCmd)
	noteCmd.AddCommand(noteListCmd)
	rootCmd.AddCommand(noteCmd)
}

func runNoteAdd(cmd *cobra.Command, args []string) error {
	turnFlag, _ := cmd.Flags().GetInt("session")
	noteText, _ := cmd.Flags().GetString("note")
	author, _ := cmd.Flags().GetString("author")

	turnID, err := resolveRequiredTurnID(turnFlag)
	if err != nil {
		return err
	}
	if noteText == "" {
		return fmt.Errorf("--note is required")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	note := &store.SupervisorNote{
		TurnID: turnID,
		Author: author,
		Note:   noteText,
	}
	if err := s.CreateNote(note); err != nil {
		return fmt.Errorf("creating note: %w", err)
	}

	fmt.Printf("Note #%d added to turn %d\n", note.ID, turnID)
	return nil
}

func runNoteList(cmd *cobra.Command, args []string) error {
	turnID, _ := cmd.Flags().GetInt("session")

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	var notes []store.SupervisorNote
	if turnID > 0 {
		notes, err = s.NotesByTurn(turnID)
	} else {
		notes, err = s.ListNotes()
	}
	if err != nil {
		return err
	}

	if len(notes) == 0 {
		fmt.Println("No notes found.")
		return nil
	}

	printHeader("Supervisor Notes")
	for _, n := range notes {
		fmt.Printf("  %s#%d%s [turn=%d] %s%s%s (%s)\n",
			styleBoldCyan, n.ID, colorReset,
			n.TurnID,
			colorBold, n.Author, colorReset,
			n.CreatedAt.Format(time.RFC3339))
		fmt.Printf("    %s\n\n", n.Note)
	}
	return nil
}
