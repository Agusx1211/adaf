package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

func runPlanCreate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id, _ := cmd.Flags().GetString("id")
	title, _ := cmd.Flags().GetString("title")
	description, _ := cmd.Flags().GetString("description")
	filePath, _ := cmd.Flags().GetString("file")
	id = strings.TrimSpace(id)

	if err := validatePlanID(id); err != nil {
		return err
	}

	plan := &store.Plan{
		ID:          id,
		Title:       strings.TrimSpace(title),
		Description: description,
		Status:      "active",
	}

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading plan file %s: %w", filePath, err)
		}
		if err := json.Unmarshal(data, plan); err != nil {
			return fmt.Errorf("parsing plan JSON: %w", err)
		}
		plan.ID = id
		if strings.TrimSpace(title) != "" {
			plan.Title = strings.TrimSpace(title)
		}
		if description != "" {
			plan.Description = description
		}
		if plan.Status == "" {
			plan.Status = "active"
		}
	}

	if strings.TrimSpace(plan.Title) == "" {
		return fmt.Errorf("plan title is required (use --title or include title in --file)")
	}

	if err := s.CreatePlan(plan); err != nil {
		return fmt.Errorf("creating plan: %w", err)
	}

	project, _ := s.LoadProject()
	if project != nil && project.ActivePlanID == "" {
		_ = s.SetActivePlan(plan.ID)
	}

	fmt.Println()
	fmt.Printf("  %sPlan %s created.%s\n", styleBoldGreen, plan.ID, colorReset)
	printField("Title", plan.Title)
	printField("Status", plan.Status)
	printField("Phases", fmt.Sprintf("%d", len(plan.Phases)))
	fmt.Println()
	return nil
}

func runPlanSet(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	idOverride, _ := cmd.Flags().GetString("id")
	idOverride = strings.TrimSpace(idOverride)
	if idOverride != "" {
		if err := validatePlanID(idOverride); err != nil {
			return err
		}
	}

	var data []byte
	if len(args) > 0 {
		data, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading plan file %s: %w", args[0], err)
		}
	} else {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	}

	var plan store.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("parsing plan JSON: %w", err)
	}

	project, _ := s.LoadProject()
	active, _ := s.ActivePlan()

	targetID := idOverride
	if targetID == "" {
		targetID = strings.TrimSpace(plan.ID)
	}
	if targetID == "" && active != nil {
		targetID = active.ID
	}
	if targetID == "" && project != nil && project.ActivePlanID != "" {
		targetID = project.ActivePlanID
	}
	if targetID == "" {
		targetID = "default"
	}
	if err := validatePlanID(targetID); err != nil {
		return err
	}
	plan.ID = targetID

	existing, err := s.GetPlan(targetID)
	if err != nil {
		return fmt.Errorf("loading plan %q: %w", targetID, err)
	}
	if existing == nil {
		if plan.Status == "" {
			plan.Status = "active"
		}
		if err := s.CreatePlan(&plan); err != nil {
			return fmt.Errorf("creating plan: %w", err)
		}
	} else {
		if plan.Created.IsZero() {
			plan.Created = existing.Created
		}
		if err := s.UpdatePlan(&plan); err != nil {
			return fmt.Errorf("updating plan: %w", err)
		}
	}

	if project != nil && project.ActivePlanID == "" {
		_ = s.SetActivePlan(targetID)
	}

	fmt.Println()
	fmt.Printf("  %sPlan %s saved.%s\n", styleBoldGreen, targetID, colorReset)
	printField("Title", plan.Title)
	printField("Phases", fmt.Sprintf("%d", len(plan.Phases)))
	fmt.Println()
	return nil
}
