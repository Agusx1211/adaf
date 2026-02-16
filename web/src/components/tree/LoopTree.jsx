import { useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus, formatElapsed, parseTimestamp } from '../../utils/format.js';
import { StopSessionButton } from '../session/SessionControls.jsx';

export default function LoopTree() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { sessions, spawns, loopRuns, selectedScope, expandedNodes } = state;

  // Group sessions by loop run
  var tree = useMemo(function () {
    // Build lookup: loop hex_id -> loop run
    var loopByHex = {};
    loopRuns.forEach(function (lr) {
      if (lr.hex_id) loopByHex[lr.hex_id] = lr;
    });

    // Group sessions by their loop_run_hex_id (from turns data) or loop_name
    var loopGroups = {}; // loop_name -> { loopRun, sessions }
    var standaloneSessions = [];

    sessions.forEach(function (session) {
      if (session.loop_name) {
        if (!loopGroups[session.loop_name]) {
          // Find the matching loop run
          var matchingRun = loopRuns.find(function (lr) {
            return lr.loop_name === session.loop_name;
          });
          loopGroups[session.loop_name] = { loopRun: matchingRun || null, sessions: [] };
        }
        loopGroups[session.loop_name].sessions.push(session);
      } else {
        standaloneSessions.push(session);
      }
    });

    // Build spawn hierarchy
    var childrenByParent = {};
    var rootsBySession = {};
    spawns.forEach(function (spawn) {
      if (spawn.parent_spawn_id > 0) {
        if (!childrenByParent[spawn.parent_spawn_id]) childrenByParent[spawn.parent_spawn_id] = [];
        childrenByParent[spawn.parent_spawn_id].push(spawn);
      } else {
        var sessionKey = spawn.parent_turn_id || 0;
        if (!rootsBySession[sessionKey]) rootsBySession[sessionKey] = [];
        rootsBySession[sessionKey].push(spawn);
      }
    });

    // Sort loop groups by most recent session
    var sortedGroups = Object.entries(loopGroups).sort(function (a, b) {
      var aLatest = a[1].sessions.reduce(function (max, s) { return Math.max(max, parseTimestamp(s.started_at)); }, 0);
      var bLatest = b[1].sessions.reduce(function (max, s) { return Math.max(max, parseTimestamp(s.started_at)); }, 0);
      return bLatest - aLatest;
    });

    return { sortedGroups, standaloneSessions, childrenByParent, rootsBySession };
  }, [sessions, spawns, loopRuns]);

  function toggleNode(nodeID) {
    dispatch({ type: 'TOGGLE_NODE', payload: nodeID });
  }

  function selectScope(scope) {
    dispatch({ type: 'SET_SELECTED_SCOPE', payload: scope });
  }

  if (!sessions.length && !loopRuns.length) {
    return (
      <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>
        No loop runs yet. Click "Start Loop" to begin.
      </div>
    );
  }

  return (
    <div style={{ flex: 1, overflow: 'auto', padding: '4px 0' }}>
      {/* Loop groups */}
      {tree.sortedGroups.map(function (entry) {
        var loopName = entry[0];
        var group = entry[1];
        var loopNodeID = 'loop-' + loopName;
        var expanded = expandedNodes[loopNodeID] !== false; // expanded by default
        var loopRun = group.loopRun;
        var isRunning = loopRun && !!STATUS_RUNNING[normalizeStatus(loopRun.status)];
        var loopColor = isRunning ? 'var(--purple)' : 'var(--text-2)';
        var latestSession = group.sessions[0]; // already sorted desc by id

        return (
          <div key={loopName}>
            {/* Loop header */}
            <div
              onClick={function () { toggleNode(loopNodeID); }}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '10px 12px', cursor: 'pointer',
                background: 'transparent',
                borderBottom: '1px solid var(--bg-3)',
                transition: 'background 0.12s ease',
              }}
              onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
            >
              {/* Expand toggle */}
              <span style={{
                width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 10, color: loopColor,
                transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
              }}>{'\u25BE'}</span>

              {/* Loop icon */}
              {isRunning ? (
                <span style={{ color: loopColor, fontSize: 14, animation: 'spin 2s linear infinite', display: 'inline-block', flexShrink: 0 }}>{'\u21BB'}</span>
              ) : (
                <span style={{ color: loopColor, fontSize: 14, flexShrink: 0 }}>{'\u21BB'}</span>
              )}

              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, fontWeight: 700, color: 'var(--text-0)' }}>
                    {loopName}
                  </span>
                  {isRunning && (
                    <span style={{
                      padding: '1px 6px', background: 'rgba(123,140,255,0.12)', border: '1px solid rgba(123,140,255,0.25)',
                      borderRadius: 3, fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--purple)', fontWeight: 600,
                    }}>RUNNING</span>
                  )}
                  {loopRun && (
                    <span style={{
                      fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)',
                    }}>C{(loopRun.cycle || 0) + 1}</span>
                  )}
                </div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', marginTop: 1 }}>
                  {group.sessions.length} turns {loopRun ? ' \u00B7 ' + formatElapsed(loopRun.started_at) : ''}
                </div>
              </div>

              {/* Turn count badge */}
              <span style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)',
                padding: '2px 6px', background: 'var(--bg-3)', borderRadius: 3,
              }}>{group.sessions.length}</span>
            </div>

            {/* Sessions within this loop */}
            {expanded && group.sessions.map(function (session) {
              return (
                <SessionNode
                  key={session.id}
                  session={session}
                  rootSpawns={tree.rootsBySession[session.id] || []}
                  childrenByParent={tree.childrenByParent}
                  selectedScope={selectedScope}
                  expandedNodes={expandedNodes}
                  onSelect={selectScope}
                  onToggle={toggleNode}
                  depth={1}
                />
              );
            })}
          </div>
        );
      })}

      {/* Standalone sessions (no loop) */}
      {tree.standaloneSessions.length > 0 && (
        <div>
          {tree.sortedGroups.length > 0 && (
            <div style={{
              padding: '6px 12px', fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.08em',
              borderBottom: '1px solid var(--bg-3)', borderTop: '1px solid var(--bg-3)',
            }}>Standalone Sessions</div>
          )}
          {tree.standaloneSessions.map(function (session) {
            return (
              <SessionNode
                key={session.id}
                session={session}
                rootSpawns={tree.rootsBySession[session.id] || []}
                childrenByParent={tree.childrenByParent}
                selectedScope={selectedScope}
                expandedNodes={expandedNodes}
                onSelect={selectScope}
                onToggle={toggleNode}
                depth={0}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}

function SessionNode({ session, rootSpawns, childrenByParent, selectedScope, expandedNodes, onSelect, onToggle, depth }) {
  var sessionNodeID = 'session-' + session.id;
  var selected = selectedScope === sessionNodeID;
  var expanded = !!expandedNodes[sessionNodeID];
  var status = normalizeStatus(session.status);
  var isRunning = !!STATUS_RUNNING[status];
  var sColor = statusColor(session.status);
  var agentI = agentInfo(session.agent);

  return (
    <div>
      <div
        onClick={function () { onSelect(sessionNodeID); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '6px 12px', paddingLeft: 12 + depth * 16, cursor: 'pointer',
          background: selected ? (agentI.color + '12') : 'transparent',
          borderLeft: selected ? ('2px solid ' + agentI.color) : '2px solid transparent',
          transition: 'all 0.12s ease',
        }}
        onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      >
        {/* Expand toggle for spawns */}
        {rootSpawns.length > 0 ? (
          <span
            onClick={function (e) { e.stopPropagation(); onToggle(sessionNodeID); }}
            style={{
              width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 9, color: agentI.color, cursor: 'pointer',
              transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
            }}
          >{'\u25BE'}</span>
        ) : (
          <span style={{ width: 14 }} />
        )}

        {/* Status dot */}
        <span style={{
          width: 7, height: 7, borderRadius: '50%', background: sColor, flexShrink: 0,
          boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />

        {/* Agent icon */}
        <span style={{ color: agentI.color, fontSize: 11, flexShrink: 0 }}>{agentI.icon}</span>

        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{session.profile || 'unknown'}</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
              #{session.id}
            </span>
          </div>
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 1 }}>
            {session.model || ''} {'\u00B7'} {formatElapsed(session.started_at, session.ended_at)}
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
      </div>

      {/* Spawn children */}
      {expanded && rootSpawns.map(function (spawn) {
        return (
          <SpawnNode
            key={spawn.id}
            spawn={spawn}
            depth={depth + 2}
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

function SpawnNode({ spawn, depth, childrenByParent, selectedScope, expandedNodes, onSelect, onToggle }) {
  var nodeID = 'spawn-' + spawn.id;
  var selected = selectedScope === nodeID;
  var children = childrenByParent[spawn.id] || [];
  var expanded = !!expandedNodes[nodeID];
  var status = normalizeStatus(spawn.status);
  var isRunning = !!STATUS_RUNNING[status];
  var sColor = statusColor(spawn.status);

  return (
    <div>
      <div
        onClick={function () { onSelect(nodeID); }}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '4px 12px', paddingLeft: 12 + depth * 16, cursor: 'pointer',
          background: selected ? (sColor + '12') : 'transparent',
          borderLeft: selected ? ('2px solid ' + sColor) : '2px solid transparent',
          transition: 'all 0.12s ease',
        }}
        onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
        onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
      >
        {children.length > 0 ? (
          <span
            onClick={function (e) { e.stopPropagation(); onToggle(nodeID); }}
            style={{
              width: 12, height: 12, display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 8, color: sColor, cursor: 'pointer',
              transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
            }}
          >{'\u25BE'}</span>
        ) : (
          <span style={{ width: 12, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <span style={{ width: 3, height: 3, borderRadius: '50%', background: sColor + '60' }} />
          </span>
        )}

        <span style={{
          width: 5, height: 5, borderRadius: '50%', background: sColor, flexShrink: 0,
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />

        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>#{spawn.id}</span>
            <span style={{ fontSize: 10, fontWeight: 600, color: 'var(--text-0)' }}>{spawn.profile || 'spawn'}</span>
            {spawn.role && <span style={{ fontSize: 9, color: 'var(--text-3)' }}>as {spawn.role}</span>}
          </div>
          {spawn.task && (
            <div style={{ fontSize: 9, color: 'var(--text-2)', marginTop: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
              {spawn.task}
            </div>
          )}
        </div>

        {isRunning && (
          <span style={{
            width: 8, height: 8, border: '1.5px solid ' + sColor, borderTopColor: 'transparent',
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
    claude: { color: '#E8A838', icon: '\u26A1' },
    codex: { color: '#4AE68A', icon: '\u25C6' },
    gemini: { color: '#7B8CFF', icon: '\u2726' },
    vibe: { color: '#FF6B9D', icon: '\u25C8' },
    opencode: { color: '#5BCEFC', icon: '\u25C9' },
    generic: { color: '#9CA3AF', icon: '\u25CB' },
  };
  return AGENT_TYPES[agent] || AGENT_TYPES.generic;
}
