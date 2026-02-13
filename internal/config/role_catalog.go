package config

import (
	"fmt"
	"strings"
)

// PromptRule is one reusable instruction block that can be composed into roles.
type PromptRule struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

// RoleDefinition is a named role with a role-specific identity block and reusable rule IDs.
type RoleDefinition struct {
	Name         string   `json:"name"`
	Title        string   `json:"title,omitempty"`
	Description  string   `json:"description,omitempty"`
	Identity     string   `json:"identity,omitempty"`
	RuleIDs      []string `json:"rule_ids,omitempty"`
	CanWriteCode bool     `json:"can_write_code"`
}

// Built-in prompt rule IDs.
// Identity IDs are legacy compatibility IDs used for migration from older configs.
const (
	RuleManagerIdentity       = "manager_identity"
	RuleManagerCore           = "manager_core"
	RuleManagerAnti           = "manager_anti_patterns"
	RuleLeadDeveloperIdentity = "lead_developer_identity"
	RuleDeveloperIdentity     = "developer_identity"
	RuleSupervisorIdentity    = "supervisor_identity"

	RuleUIDesignerIdentity      = "ui_designer_identity"
	RuleQAIdentity              = "qa_identity"
	RuleBackendDesignerIdentity = "backend_designer_identity"
	RuleDocumentatorIdentity    = "documentator_identity"
	RuleReviewerIdentity        = "reviewer_identity"
	RuleScoutIdentity           = "scout_identity"
	RuleResearcherIdentity      = "researcher_identity"

	RuleCommunicationUpstream   = "communication_upstream_only"
	RuleCommunicationDownstream = "communication_downstream_only"
	RuleSupervisorCmds          = "supervisor_commands"
)

const (
	roleIdentityManager = "You are a MANAGER agent. You do NOT write code or run tests directly. " +
		"Your entire value comes from effective delegation and review."
	roleIdentityLeadDeveloper = "You are a LEAD DEVELOPER agent. Deliver high-quality code and coordinate implementation when delegation is available.\n\n" +
		"You are responsible for architecture coherence, technical quality, and delivery pacing."
	roleIdentityDeveloper  = "You are a DEVELOPER agent. Focus on implementation quality, test coverage, and clean execution of the assigned scope."
	roleIdentitySupervisor = "You are a SUPERVISOR agent. You review progress and provide guidance via notes. You do NOT write code."
	roleIdentityUIDesigner = "You are a UI DESIGNER role. Focus on user experience, interaction flows, visual hierarchy, responsive behavior, and accessibility.\n\n" +
		"Produce implementation-ready UI direction with explicit component states and edge cases."
	roleIdentityQA = "You are a QA role. Focus on verification, test design, regressions, failure modes, and release confidence.\n\n" +
		"Prioritize reproducible checks and clear pass/fail criteria."
	roleIdentityBackendDesigner = "You are a BACKEND DESIGNER role. Focus on API contracts, data modeling, reliability, performance, and operability.\n\n" +
		"Design for maintainability and explicit interface boundaries."
	roleIdentityDocumentator = "You are a DOCUMENTATOR role. Focus on clear technical writing, task handoff clarity, and accurate developer/user documentation.\n\n" +
		"Keep docs concise, actionable, and aligned with actual behavior."
	roleIdentityReviewer = "You are a REVIEWER role. Focus on correctness, regressions, risks, and missing tests.\n\n" +
		"Provide concrete findings and required changes with file-level precision."
	roleIdentityScout = "You are a SCOUT role. Focus on rapid investigation and evidence gathering.\n\n" +
		"Prefer read-only analysis and concise factual reporting."
	roleIdentityResearcher = "You are a RESEARCHER role. Focus on option analysis, tradeoffs, and recommendation quality.\n\n" +
		"Support conclusions with clear assumptions and constraints."
)

func legacyIdentityRuleText(ruleID string) (string, bool) {
	switch normalizeRuleID(ruleID) {
	case RuleManagerIdentity:
		return roleIdentityManager, true
	case RuleLeadDeveloperIdentity:
		return roleIdentityLeadDeveloper, true
	case RuleDeveloperIdentity:
		return roleIdentityDeveloper, true
	case RuleSupervisorIdentity:
		return roleIdentitySupervisor, true
	case RuleUIDesignerIdentity:
		return roleIdentityUIDesigner, true
	case RuleQAIdentity:
		return roleIdentityQA, true
	case RuleBackendDesignerIdentity:
		return roleIdentityBackendDesigner, true
	case RuleDocumentatorIdentity:
		return roleIdentityDocumentator, true
	case RuleReviewerIdentity:
		return roleIdentityReviewer, true
	case RuleScoutIdentity:
		return roleIdentityScout, true
	case RuleResearcherIdentity:
		return roleIdentityResearcher, true
	default:
		return "", false
	}
}

var defaultPromptRules = []PromptRule{
	{
		ID: RuleManagerCore,
		Body: "## Core Principles\n\n" +
			"1. **Delegate aggressively.** Every piece of work — coding, investigation, testing, review — should be done by a sub-agent. Spawn early, spawn often, spawn in parallel.\n" +
			"2. **Prefer scouts over doing it yourself.** For reading files, checking git history, running tests, or inspecting the repo, spawn `--read-only` scouts. Your context window is expensive — save it for decisions. You can read a file directly when truly needed, but default to delegation.\n" +
			"3. **Maximize parallelism.** Spawn all independent tasks at once, then `wait-for-spawns`. Sequential spawning wastes time. If you have 3 tasks, spawn 3 agents simultaneously.\n" +
			"4. **Review every diff** with `adaf spawn-diff` before merging. When work needs corrections, prefer sending feedback via `spawn-message --interrupt` (if still running) or writing a precise corrective task rather than blindly rejecting and re-spawning.",
	},
	{
		ID: RuleManagerAnti,
		Body: "## Anti-Patterns (avoid these)\n\n" +
			"- Spawning one agent at a time with `--wait` — this burns tokens while you idle. Use `wait-for-spawns`\n" +
			"- Doing 3 sequential spawn-reject-respawn cycles for the same issue — give better instructions upfront, or use `spawn-message` mid-flight\n" +
			"- Writing or editing any file yourself\n\n" +
			"Use the Delegation section below for available profiles and commands.",
	},
	{
		ID: RuleCommunicationDownstream,
		Body: "## Communication Style: Downstream Only\n\n" +
			"Communicate primarily to child sessions. Keep direction concrete and executable.\n\n" +
			"- `adaf spawn-message --spawn-id N \"guidance\"` — Send async guidance to a child\n" +
			"- `adaf spawn-message --spawn-id N --interrupt \"new priority\"` — Interrupt child turn with updated direction\n" +
			"- `adaf spawn-reply --spawn-id N \"answer\"` — Answer a child's question\n" +
			"- `adaf spawn-status [--spawn-id N]` / `adaf spawn-watch --spawn-id N` — Monitor children",
	},
	{
		ID: RuleSupervisorCmds,
		Body: "## Supervisor Commands\n\n" +
			"- `adaf note add [--session <N>] --note \"guidance text\"` — Send a note to a running agent session\n" +
			"- `adaf note list [--session <N>]` — List supervisor notes",
	},
}

var defaultRoleDefinitions = []RoleDefinition{
	{
		Name:         RoleManager,
		Title:        "MANAGER",
		Description:  "Planning/delegation focused; no direct coding.",
		Identity:     roleIdentityManager,
		CanWriteCode: false,
		RuleIDs:      []string{RuleManagerCore, RuleManagerAnti, RuleCommunicationDownstream},
	},
	{
		Name:         RoleLeadDeveloper,
		Title:        "LEAD DEVELOPER",
		Description:  "Lead coder; can also orchestrate work.",
		Identity:     roleIdentityLeadDeveloper,
		CanWriteCode: true,
		RuleIDs:      []string{RuleCommunicationDownstream},
	},
	{
		Name:         RoleDeveloper,
		Title:        "DEVELOPER",
		Description:  "Execution-focused developer.",
		Identity:     roleIdentityDeveloper,
		CanWriteCode: true,
		RuleIDs:      nil,
	},
	{
		Name:         RoleSupervisor,
		Title:        "SUPERVISOR",
		Description:  "Review and guidance role; no direct coding.",
		Identity:     roleIdentitySupervisor,
		CanWriteCode: false,
		RuleIDs:      []string{RuleCommunicationDownstream, RuleSupervisorCmds},
	},
	{
		Name:         RoleUIDesigner,
		Title:        "UI DESIGNER",
		Description:  "UI/UX-focused designer role.",
		Identity:     roleIdentityUIDesigner,
		CanWriteCode: true,
		RuleIDs:      nil,
	},
	{
		Name:         RoleQA,
		Title:        "QA",
		Description:  "Quality assurance and verification role.",
		Identity:     roleIdentityQA,
		CanWriteCode: true,
		RuleIDs:      nil,
	},
	{
		Name:         RoleBackendDesigner,
		Title:        "BACKEND DESIGNER",
		Description:  "Backend architecture and API design role.",
		Identity:     roleIdentityBackendDesigner,
		CanWriteCode: true,
		RuleIDs:      nil,
	},
	{
		Name:         RoleDocumentator,
		Title:        "DOCUMENTATOR",
		Description:  "Documentation-focused role.",
		Identity:     roleIdentityDocumentator,
		CanWriteCode: true,
		RuleIDs:      nil,
	},
	{
		Name:         RoleReviewer,
		Title:        "REVIEWER",
		Description:  "Diff/code reviewer role.",
		Identity:     roleIdentityReviewer,
		CanWriteCode: false,
		RuleIDs:      []string{RuleCommunicationDownstream},
	},
	{
		Name:         RoleScout,
		Title:        "SCOUT",
		Description:  "Fast read-only investigation role.",
		Identity:     roleIdentityScout,
		CanWriteCode: false,
		RuleIDs:      nil,
	},
	{
		Name:         RoleResearcher,
		Title:        "RESEARCHER",
		Description:  "Deep research and options analysis role.",
		Identity:     roleIdentityResearcher,
		CanWriteCode: false,
		RuleIDs:      nil,
	},
}

func normalizeRoleName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeRuleID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func clonePromptRules(in []PromptRule) []PromptRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]PromptRule, 0, len(in))
	for _, rule := range in {
		id := normalizeRuleID(rule.ID)
		if id == "" {
			continue
		}
		out = append(out, PromptRule{
			ID:   id,
			Body: rule.Body,
		})
	}
	return out
}

func cloneRoleDefinitions(in []RoleDefinition) []RoleDefinition {
	if len(in) == 0 {
		return nil
	}
	out := make([]RoleDefinition, 0, len(in))
	for _, role := range in {
		name := normalizeRoleName(role.Name)
		if name == "" {
			continue
		}
		seen := make(map[string]struct{}, len(role.RuleIDs))
		ruleIDs := make([]string, 0, len(role.RuleIDs))
		for _, rid := range role.RuleIDs {
			key := normalizeRuleID(rid)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ruleIDs = append(ruleIDs, key)
		}
		out = append(out, RoleDefinition{
			Name:         name,
			Title:        strings.TrimSpace(role.Title),
			Description:  strings.TrimSpace(role.Description),
			Identity:     strings.TrimSpace(role.Identity),
			RuleIDs:      ruleIDs,
			CanWriteCode: role.CanWriteCode,
		})
	}
	return out
}

// DefaultPromptRules returns the built-in prompt rules.
func DefaultPromptRules() []PromptRule {
	return clonePromptRules(defaultPromptRules)
}

// DefaultRoleDefinitions returns the built-in role definitions.
func DefaultRoleDefinitions() []RoleDefinition {
	return cloneRoleDefinitions(defaultRoleDefinitions)
}

func hasRoleName(roles []RoleDefinition, name string) bool {
	name = normalizeRoleName(name)
	for _, role := range roles {
		if normalizeRoleName(role.Name) == name {
			return true
		}
	}
	return false
}

// EnsureDefaultRoleCatalog guarantees that role/rule catalogs exist and are valid.
// Returns true when it mutates cfg.
func EnsureDefaultRoleCatalog(cfg *GlobalConfig) bool {
	if cfg == nil {
		return false
	}
	changed := false
	legacyRuleBodies := make(map[string]string, len(cfg.PromptRules))
	for _, rule := range cfg.PromptRules {
		id := normalizeRuleID(rule.ID)
		if id == "" {
			continue
		}
		legacyRuleBodies[id] = strings.TrimSpace(rule.Body)
	}

	if len(cfg.PromptRules) == 0 {
		cfg.PromptRules = DefaultPromptRules()
		changed = true
	} else {
		seen := make(map[string]struct{}, len(cfg.PromptRules))
		normalized := make([]PromptRule, 0, len(cfg.PromptRules))
		for _, rule := range cfg.PromptRules {
			id := normalizeRuleID(rule.ID)
			if id == "" {
				changed = true
				continue
			}
			if id == RuleCommunicationUpstream {
				changed = true
				continue
			}
			if _, isIdentity := legacyIdentityRuleText(id); isIdentity {
				changed = true
				continue
			}
			if _, ok := seen[id]; ok {
				changed = true
				continue
			}
			seen[id] = struct{}{}
			if id != rule.ID {
				changed = true
			}
			normalized = append(normalized, PromptRule{
				ID:   id,
				Body: rule.Body,
			})
		}
		if len(normalized) == 0 {
			cfg.PromptRules = DefaultPromptRules()
			changed = true
		} else {
			cfg.PromptRules = normalized
		}
	}

	if len(cfg.Roles) == 0 {
		cfg.Roles = DefaultRoleDefinitions()
		changed = true
	} else {
		seen := make(map[string]struct{}, len(cfg.Roles))
		normalized := make([]RoleDefinition, 0, len(cfg.Roles))
		for _, role := range cfg.Roles {
			name := normalizeRoleName(role.Name)
			if name == "" {
				changed = true
				continue
			}
			if _, ok := seen[name]; ok {
				changed = true
				continue
			}
			seen[name] = struct{}{}
			roleIdentity := strings.TrimSpace(role.Identity)
			ruleIDs := make([]string, 0, len(role.RuleIDs))
			seenRules := make(map[string]struct{}, len(role.RuleIDs))
			for _, ruleID := range role.RuleIDs {
				id := normalizeRuleID(ruleID)
				if id == "" {
					changed = true
					continue
				}
				if id == RuleCommunicationUpstream {
					changed = true
					continue
				}
				if fallbackIdentity, isIdentity := legacyIdentityRuleText(id); isIdentity {
					changed = true
					if roleIdentity == "" {
						if body := legacyRuleBodies[id]; body != "" {
							roleIdentity = body
						} else {
							roleIdentity = fallbackIdentity
						}
					}
					continue
				}
				if _, ok := seenRules[id]; ok {
					changed = true
					continue
				}
				seenRules[id] = struct{}{}
				ruleIDs = append(ruleIDs, id)
				if id != ruleID {
					changed = true
				}
			}
			norm := RoleDefinition{
				Name:         name,
				Title:        strings.TrimSpace(role.Title),
				Description:  strings.TrimSpace(role.Description),
				Identity:     roleIdentity,
				RuleIDs:      ruleIDs,
				CanWriteCode: role.CanWriteCode,
			}
			if norm.Name != role.Name || norm.Title != role.Title || norm.Description != role.Description || norm.Identity != strings.TrimSpace(role.Identity) {
				changed = true
			}
			normalized = append(normalized, norm)
		}
		if len(normalized) == 0 {
			cfg.Roles = DefaultRoleDefinitions()
			changed = true
		} else {
			cfg.Roles = normalized
		}
	}

	defaultRole := normalizeRoleName(cfg.DefaultRole)
	if defaultRole == "" || !hasRoleName(cfg.Roles, defaultRole) {
		switch {
		case hasRoleName(cfg.Roles, RoleDeveloper):
			defaultRole = RoleDeveloper
		case len(cfg.Roles) > 0:
			defaultRole = cfg.Roles[0].Name
		default:
			defaultRole = RoleDeveloper
		}
		if cfg.DefaultRole != defaultRole {
			changed = true
		}
	}
	cfg.DefaultRole = defaultRole

	return changed
}

// FindRoleDefinition returns a pointer to a role definition by name.
func (c *GlobalConfig) FindRoleDefinition(name string) *RoleDefinition {
	if c == nil {
		return nil
	}
	EnsureDefaultRoleCatalog(c)
	key := normalizeRoleName(name)
	for i := range c.Roles {
		if normalizeRoleName(c.Roles[i].Name) == key {
			return &c.Roles[i]
		}
	}
	return nil
}

// FindPromptRule returns a pointer to a prompt rule by ID.
func (c *GlobalConfig) FindPromptRule(id string) *PromptRule {
	if c == nil {
		return nil
	}
	EnsureDefaultRoleCatalog(c)
	key := normalizeRuleID(id)
	for i := range c.PromptRules {
		if normalizeRuleID(c.PromptRules[i].ID) == key {
			return &c.PromptRules[i]
		}
	}
	return nil
}

// AddRoleDefinition adds a role definition if the name is unique.
func (c *GlobalConfig) AddRoleDefinition(role RoleDefinition) error {
	if c == nil {
		return fmt.Errorf("global config is nil")
	}
	EnsureDefaultRoleCatalog(c)
	name := normalizeRoleName(role.Name)
	if name == "" {
		return fmt.Errorf("role name cannot be empty")
	}
	if c.FindRoleDefinition(name) != nil {
		return fmt.Errorf("role already exists: %s", name)
	}
	role.Name = name
	role.Title = strings.TrimSpace(role.Title)
	role.Description = strings.TrimSpace(role.Description)
	role.Identity = strings.TrimSpace(role.Identity)
	seen := make(map[string]struct{}, len(role.RuleIDs))
	normRuleIDs := make([]string, 0, len(role.RuleIDs))
	for _, rid := range role.RuleIDs {
		key := normalizeRuleID(rid)
		if key == "" {
			continue
		}
		if key == RuleCommunicationUpstream {
			continue
		}
		if fallbackIdentity, isIdentity := legacyIdentityRuleText(key); isIdentity {
			if role.Identity == "" {
				role.Identity = fallbackIdentity
			}
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normRuleIDs = append(normRuleIDs, key)
	}
	role.RuleIDs = normRuleIDs
	c.Roles = append(c.Roles, role)
	return nil
}

// RemoveRoleDefinition removes a role definition by name.
func (c *GlobalConfig) RemoveRoleDefinition(name string) {
	if c == nil {
		return
	}
	EnsureDefaultRoleCatalog(c)
	key := normalizeRoleName(name)
	out := c.Roles[:0]
	for _, role := range c.Roles {
		if normalizeRoleName(role.Name) != key {
			out = append(out, role)
		}
	}
	c.Roles = out
	EnsureDefaultRoleCatalog(c)
}

// AddPromptRule adds a prompt rule if the ID is unique.
func (c *GlobalConfig) AddPromptRule(rule PromptRule) error {
	if c == nil {
		return fmt.Errorf("global config is nil")
	}
	EnsureDefaultRoleCatalog(c)
	id := normalizeRuleID(rule.ID)
	if id == "" {
		return fmt.Errorf("rule id cannot be empty")
	}
	if c.FindPromptRule(id) != nil {
		return fmt.Errorf("rule already exists: %s", id)
	}
	c.PromptRules = append(c.PromptRules, PromptRule{
		ID:   id,
		Body: rule.Body,
	})
	return nil
}

// RemovePromptRule removes a prompt rule by ID and unlinks it from all roles.
func (c *GlobalConfig) RemovePromptRule(id string) {
	if c == nil {
		return
	}
	EnsureDefaultRoleCatalog(c)
	key := normalizeRuleID(id)

	out := c.PromptRules[:0]
	for _, rule := range c.PromptRules {
		if normalizeRuleID(rule.ID) != key {
			out = append(out, rule)
		}
	}
	c.PromptRules = out

	for i := range c.Roles {
		ruleIDs := c.Roles[i].RuleIDs[:0]
		for _, rid := range c.Roles[i].RuleIDs {
			if normalizeRuleID(rid) != key {
				ruleIDs = append(ruleIDs, rid)
			}
		}
		c.Roles[i].RuleIDs = ruleIDs
	}
	EnsureDefaultRoleCatalog(c)
}
