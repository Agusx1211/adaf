import { useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus, formatElapsed, parseTimestamp } from '../../utils/format.js';
import StatusDot from '../common/StatusDot.jsx';
import { StopSessionButton } from '../session/SessionControls.jsx';

export default function AgentTree({ onSelectScope }) {
  var state = useAppState();
  var dispatch = useDispatch();
  var { sessions, spawns, loopRuns, selectedScope, expandedNodes } = state;

  var tree = useMemo(function () {
    // Build store-turn-ID -> daemon-session-ID mapping so spawns
    // (which reference store turn IDs) can be keyed by daemon session IDs.
    var storeTurnToDaemonSession = {};
    (loopRuns || []).forEach(function (lr) {
      if (lr.daemon_session_id > 0 && lr.turn_ids) {
        lr.turn_ids.forEach(function (tid) {
          if (tid > 0) storeTurnToDaemonSession[tid] = lr.daemon_session_id;
        });
      }
    });

    var childrenByParent = {};
    var rootsBySession = {};

    spawns.forEach(function (spawn) {
      if (spawn.parent_spawn_id > 0) {
        if (!childrenByParent[spawn.parent_spawn_id]) childrenByParent[spawn.parent_spawn_id] = [];
        childrenByParent[spawn.parent_spawn_id].push(spawn);
      } else {
        var storeTurnID = spawn.parent_turn_id || 0;
        var sessionKey = storeTurnToDaemonSession[storeTurnID] || storeTurnID;
        if (!rootsBySession[sessionKey]) rootsBySession[sessionKey] = [];
        rootsBySession[sessionKey].push(spawn);
      }
    });

    return { childrenByParent, rootsBySession };
  }, [spawns, loopRuns]);

  function toggleNode(nodeID) {
    dispatch({ type: 'TOGGLE_NODE', payload: nodeID });
  }

  function selectScope(scope) {
    dispatch({ type: 'SET_SELECTED_SCOPE', payload: scope });
    if (onSelectScope) onSelectScope(scope);
  }

  if (!sessions.length && !spawns.length) {
    return <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>No sessions or spawns yet.</div>;
  }

  var sortedSessions = sessions.slice().sort(function (a, b) { return b.id - a.id; });

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: '4px 0' }}>
      {sortedSessions.map(function (session) {
        var sessionNodeID = 'session-' + session.id;
        var selected = selectedScope === sessionNodeID;
        var rootSpawns = tree.rootsBySession[session.id] || [];
        var expanded = !!expandedNodes[sessionNodeID];
        var status = normalizeStatus(session.status);
        var isRunning = !!STATUS_RUNNING[status];
        var sColor = statusColor(session.status);
        var agentI = agentInfo(session.agent);

        return (
          <div key={session.id}>
            <div
              onClick={function () { selectScope(sessionNodeID); }}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '8px 12px', cursor: 'pointer',
                background: selected ? (agentI.color + '12') : 'transparent',
                borderLeft: selected ? ('2px solid ' + agentI.color) : '2px solid transparent',
                transition: 'all 0.15s ease',
              }}
              onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
              onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
            >
              {/* Expand toggle */}
              {rootSpawns.length > 0 ? (
                <span
                  onClick={function (e) { e.stopPropagation(); toggleNode(sessionNodeID); }}
                  style={{
                    width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: 10, color: agentI.color, border: '1px solid ' + agentI.color + '40',
                    borderRadius: 3, cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace",
                    transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
                  }}
                >{'\u25BE'}</span>
              ) : (
                <span style={{ width: 16 }} />
              )}

              <span style={{
                width: 8, height: 8, borderRadius: '50%', background: sColor, flexShrink: 0,
                boxShadow: isRunning ? '0 0 8px ' + sColor : 'none',
                animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
              }} />

              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ color: agentI.color, fontSize: 12 }}>{agentI.icon}</span>
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-1)' }}>turn #{session.id}</span>
                  <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{session.profile || 'unknown'}</span>
                  <span style={{ fontSize: 10, color: 'var(--text-3)' }}>({session.agent || 'agent'})</span>
                </div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', marginTop: 1 }}>
                  {session.model || 'model n/a'} {'\u00B7'} {formatElapsed(session.started_at, session.ended_at)}
                </div>
              </div>

              {isRunning && (
                <>
                  <StopSessionButton sessionID={session.id} />
                  <span style={{
                    width: 10, height: 10, border: '2px solid ' + agentI.color, borderTopColor: 'transparent',
                    borderRadius: '50%', animation: 'spin 1s linear infinite', flexShrink: 0,
                  }} />
                </>
              )}

              {rootSpawns.length > 0 && (
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)',
                  padding: '1px 5px', background: 'var(--bg-3)', borderRadius: 3,
                }}>{rootSpawns.length} spawns</span>
              )}
            </div>

            {/* Spawn children */}
            {expanded && rootSpawns.map(function (spawn) {
              return (
                <SpawnNode
                  key={spawn.id}
                  spawn={spawn}
                  depth={1}
                  childrenByParent={tree.childrenByParent}
                  selectedScope={selectedScope}
                  expandedNodes={expandedNodes}
                  onSelect={selectScope}
                  onToggle={toggleNode}
                />
              );
            })}
          </div>
        );
      })}
    </div>
  );
}

function SpawnNode({ spawn, depth, childrenByParent, selectedScope, expandedNodes, onSelect, onToggle }) {
  var nodeID = 'spawn-' + spawn.id;
  var selected = selectedScope === nodeID;
  var children = childrenByParent[spawn.id] || [];
  var expanded = !!expandedNodes[nodeID];
  var status = normalizeStatus(spawn.status);
  var isRunning = !!STATUS_RUNNING[status];
  var sColor = statusColor(spawn.status);
  var hasPendingQuestion = status === 'awaiting_input' && !!spawn.question;

  return (
    <div style={{ marginLeft: depth * 18 }}>
      <div
        onClick={function () { onSelect(nodeID); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '6px 12px', cursor: 'pointer',
          background: selected ? (sColor + '12') : 'transparent',
          borderLeft: selected ? ('2px solid ' + sColor) : '2px solid transparent',
          transition: 'all 0.15s ease',
        }}
        onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      >
        {children.length > 0 ? (
          <span
            onClick={function (e) { e.stopPropagation(); onToggle(nodeID); }}
            style={{
              width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 9, color: sColor, border: '1px solid ' + sColor + '40',
              borderRadius: 3, cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace",
              transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
            }}
          >{'\u25BE'}</span>
        ) : (
          <span style={{ width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <span style={{ width: 4, height: 4, borderRadius: '50%', background: sColor + '60' }} />
          </span>
        )}

        <span style={{
          width: 6, height: 6, borderRadius: '50%', background: sColor, flexShrink: 0,
          boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />

        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>#{spawn.id}</span>
            <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{spawn.profile || 'spawn'}</span>
            {spawn.role && <span style={{ fontSize: 10, color: 'var(--text-3)' }}>as {spawn.role}</span>}
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
              {formatElapsed(spawn.started_at, spawn.completed_at)}
            </span>
          </div>
          {spawn.task && (
            <div style={{ fontSize: 10, color: 'var(--text-2)', marginTop: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
              {spawn.task}
            </div>
          )}
          {spawn.branch && (
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--purple)', marginTop: 1 }}>
              {spawn.branch}
            </div>
          )}
          {hasPendingQuestion && (
            <div style={{
              marginTop: 4, padding: '4px 8px', background: 'rgba(232,168,56,0.08)',
              border: '1px solid rgba(232,168,56,0.2)', borderRadius: 4,
              display: 'flex', alignItems: 'center', gap: 6,
            }}>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--orange)', animation: 'pulse 1.5s ease-in-out infinite' }} />
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--orange)' }}>AWAITING RESPONSE</span>
            </div>
          )}
        </div>

        {isRunning && (
          <span style={{
            width: 10, height: 10, border: '2px solid ' + sColor, borderTopColor: 'transparent',
            borderRadius: '50%', animation: 'spin 1s linear infinite', flexShrink: 0,
          }} />
        )}
      </div>

      {expanded && children.map(function (child) {
        return (
          <SpawnNode
            key={child.id}
            spawn={child}
            depth={depth + 1}
            childrenByParent={childrenByParent}
            selectedScope={selectedScope}
            expandedNodes={expandedNodes}
            onSelect={onSelect}
            onToggle={onToggle}
          />
        );
      })}
    </div>
  );
}

function agentInfo(agent) {
  var AGENT_TYPES = {
    claude: { color: '#E8A838', icon: '\u26A1', bg: 'rgba(232,168,56,0.08)' },
    codex: { color: '#4AE68A', icon: '\u25C6', bg: 'rgba(74,230,138,0.08)' },
    gemini: { color: '#7B8CFF', icon: '\u2726', bg: 'rgba(123,140,255,0.08)' },
    vibe: { color: '#FF6B9D', icon: '\u25C8', bg: 'rgba(255,107,157,0.08)' },
    opencode: { color: '#5BCEFC', icon: '\u25C9', bg: 'rgba(91,206,252,0.08)' },
    generic: { color: '#9CA3AF', icon: '\u25CB', bg: 'rgba(156,163,175,0.08)' },
  };
  return AGENT_TYPES[agent] || AGENT_TYPES.generic;
}
