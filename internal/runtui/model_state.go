package runtui

import (
	"fmt"
	"strings"
	"time"
)

func (m *Model) applyLiveSpawnStatus(spawns []SpawnInfo) {
	changes := m.updateSpawnStatus(spawns)
	for _, sp := range changes {
		scope := m.spawnScope(sp.ID)
		m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("[spawn #%d] %s -> %s", sp.ID, sp.Profile, sp.Status)))
		m.addSimplifiedLine(scope, dimStyle.Render("spawn update"))
		m.bumpStats(scope, func(st *detailStats) { st.Spawns++ })
		if sp.Question != "" {
			m.addScopedLine(scope, dimStyle.Render("  Q: "+truncate(sp.Question, 180)))
		}
	}
}

func (m *Model) applySnapshotSpawnStatus(spawns []SpawnInfo) {
	_ = m.updateSpawnStatus(spawns)
}

func (m *Model) updateSpawnStatus(spawns []SpawnInfo) []SpawnInfo {
	m.spawns = spawns
	now := time.Now()
	nextSeen := make(map[int]time.Time, len(spawns))
	nextStatus := make(map[int]string, len(spawns))
	activeByParent := make(map[int]bool)
	seenByParent := make(map[int]bool)
	changes := make([]SpawnInfo, 0, len(spawns))

	for _, sp := range spawns {
		firstSeen, ok := m.spawnFirstSeen[sp.ID]
		if !ok {
			firstSeen = now
		}
		nextSeen[sp.ID] = firstSeen
		nextStatus[sp.ID] = sp.Status
		if sp.ParentTurnID > 0 {
			seenByParent[sp.ParentTurnID] = true
			if !isTerminalSpawnStatus(sp.Status) {
				activeByParent[sp.ParentTurnID] = true
			}
		}
		prev, hadPrev := m.spawnStatus[sp.ID]
		if !hadPrev || prev != sp.Status {
			changes = append(changes, sp)
		}
	}

	m.spawnFirstSeen = nextSeen
	m.spawnStatus = nextStatus
	for _, sid := range m.sessionOrder {
		s := m.sessions[sid]
		if s == nil || !isWaitingSessionStatus(s.Status) {
			continue
		}
		if !seenByParent[sid] || activeByParent[sid] {
			continue
		}
		s.Status = "completed"
		if s.Action == "" || strings.Contains(strings.ToLower(s.Action), "wait") {
			s.Action = "wait complete"
		}
		s.LastUpdate = now
	}
	return changes
}

func (m *Model) ensureSession(sessionID int) *sessionStatus {
	if sessionID <= 0 {
		return nil
	}
	if s, ok := m.sessions[sessionID]; ok {
		return s
	}
	now := time.Now()
	s := &sessionStatus{
		ID:         sessionID,
		Agent:      m.agentName,
		Profile:    m.loopStepProfile,
		Status:     "running",
		Action:     "starting",
		StartedAt:  now,
		LastUpdate: now,
	}
	m.sessions[sessionID] = s
	m.sessionOrder = append(m.sessionOrder, sessionID)
	return s
}

func (m *Model) setSessionAction(sessionID int, action string) {
	if sessionID <= 0 {
		return
	}
	s := m.ensureSession(sessionID)
	if s == nil {
		return
	}
	if action != "" {
		s.Action = action
	}
	s.LastUpdate = time.Now()
}

func isWaitingSessionStatus(status string) bool {
	switch status {
	case "waiting_for_spawns", "waiting":
		return true
	default:
		return false
	}
}

func isActiveSessionStatus(status string) bool {
	switch status {
	case "running", "awaiting_input":
		return true
	default:
		return isWaitingSessionStatus(status)
	}
}

func isTerminalSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled", "merged", "rejected":
		return true
	default:
		return false
	}
}

func (m *Model) finalizeWaitingSessions(exceptSessionID int) {
	now := time.Now()
	for _, sid := range m.sessionOrder {
		if sid == exceptSessionID {
			continue
		}
		s := m.sessions[sid]
		if s == nil || !isWaitingSessionStatus(s.Status) {
			continue
		}
		s.Status = "completed"
		if s.Action == "" || strings.Contains(strings.ToLower(s.Action), "wait") {
			s.Action = "wait complete"
		}
		if s.EndedAt.IsZero() {
			s.EndedAt = now
		}
		s.LastUpdate = now
	}
}
