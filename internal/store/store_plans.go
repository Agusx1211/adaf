// store_plans.go contains plan management methods.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) LoadPlan() (*Plan, error) {
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	plan, err := s.ActivePlan()
	if err != nil {
		return nil, err
	}
	if plan != nil {
		return plan, nil
	}

	return &Plan{Status: "active", Updated: time.Now().UTC()}, nil
}

func (s *Store) SavePlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}

	project, err := s.LoadProject()
	if err != nil {
		return err
	}

	if plan.ID == "" {
		if project.ActivePlanID != "" {
			plan.ID = project.ActivePlanID
		} else {
			plan.ID = "default"
		}
	}

	existing, err := s.GetPlan(plan.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		if err := s.CreatePlan(plan); err != nil {
			return err
		}
		return s.SetActivePlan(plan.ID)
	}

	if plan.Created.IsZero() {
		plan.Created = existing.Created
	}
	return s.UpdatePlan(plan)
}

func (s *Store) ListPlans() ([]Plan, error) {
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.plansDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plans []Plan
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var plan Plan
		if err := s.readJSON(filepath.Join(s.plansDir(), e.Name()), &plan); err != nil {
			continue
		}
		if plan.ID == "" {
			plan.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		if plan.Status == "" {
			plan.Status = "active"
		}
		plans = append(plans, plan)
	}

	sort.Slice(plans, func(i, j int) bool { return plans[i].ID < plans[j].ID })
	return plans, nil
}

func (s *Store) GetPlan(id string) (*Plan, error) {
	if id == "" {
		return nil, nil
	}
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	var plan Plan
	if err := s.readJSON(s.planPath(id), &plan); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if plan.ID == "" {
		plan.ID = id
	}
	if plan.Status == "" {
		plan.Status = "active"
	}
	return &plan, nil
}

func (s *Store) CreatePlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}
	if plan.ID == "" {
		return fmt.Errorf("plan ID is required")
	}

	path := s.planPath(plan.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("plan %q already exists", plan.ID)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	now := time.Now().UTC()
	if plan.Status == "" {
		plan.Status = "active"
	}
	if plan.Created.IsZero() {
		plan.Created = now
	}
	plan.Updated = now
	filename := plan.ID + ".json"
	if err := s.writeJSON(path, plan); err != nil {
		return err
	}

	// Auto-commit the created plan
	s.AutoCommit([]string{"plans/" + filename}, fmt.Sprintf("adaf: create plan %s", plan.ID))
	return nil
}

func (s *Store) UpdatePlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}
	if plan.ID == "" {
		return fmt.Errorf("plan ID is required")
	}

	existing, err := s.GetPlan(plan.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("plan %q does not exist", plan.ID)
	}

	if plan.Status == "" {
		plan.Status = existing.Status
		if plan.Status == "" {
			plan.Status = "active"
		}
	}
	if plan.Created.IsZero() {
		plan.Created = existing.Created
	}
	if plan.Created.IsZero() {
		plan.Created = time.Now().UTC()
	}
	plan.Updated = time.Now().UTC()

	filename := plan.ID + ".json"
	if err := s.writeJSON(s.planPath(plan.ID), plan); err != nil {
		return err
	}

	// Auto-commit the updated plan
	s.AutoCommit([]string{"plans/" + filename}, fmt.Sprintf("adaf: update plan %s", plan.ID))
	return nil
}

func (s *Store) DeletePlan(id string) error {
	if id == "" {
		return fmt.Errorf("plan ID is required")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}

	plan, err := s.GetPlan(id)
	if err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	if plan.Status != "done" && plan.Status != "cancelled" {
		return fmt.Errorf("plan %q status is %q; only done/cancelled can be deleted", id, plan.Status)
	}

	path := s.planPath(id)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	project, err := s.LoadProject()
	if err == nil && project != nil && project.ActivePlanID == id {
		project.ActivePlanID = ""
		if saveErr := s.SaveProject(project); saveErr != nil {
			return saveErr
		}
	}

	return os.Remove(path)
}

func (s *Store) ActivePlan() (*Plan, error) {
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	project, err := s.LoadProject()
	if err != nil {
		return nil, err
	}
	if project.ActivePlanID == "" {
		return nil, nil
	}
	return s.GetPlan(project.ActivePlanID)
}

func (s *Store) SetActivePlan(id string) error {
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}

	project, err := s.LoadProject()
	if err != nil {
		return err
	}

	if id != "" {
		plan, err := s.GetPlan(id)
		if err != nil {
			return err
		}
		if plan == nil {
			return fmt.Errorf("plan %q not found", id)
		}
	}

	project.ActivePlanID = id
	return s.SaveProject(project)
}

func (s *Store) plansDir() string {
	return filepath.Join(s.root, "plans")
}

func (s *Store) planPath(id string) string {
	return filepath.Join(s.plansDir(), id+".json")
}

func (s *Store) ensurePlanStorage() error {
	if err := os.MkdirAll(s.plansDir(), 0755); err != nil {
		return err
	}
	return nil
}
