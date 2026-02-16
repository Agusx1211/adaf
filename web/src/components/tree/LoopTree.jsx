import { useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus, formatElapsed, parseTimestamp } from '../../utils/format.js';
import { StopSessionButton } from '../session/SessionControls.jsx';

export default function LoopTree() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { sessions, spawns, loopRuns, selectedScope, expandedNodes } = state;

  var tree = useMemo(function () {
    // Build a set of session IDs claimed by each loop run via turn_ids
    var sessionToRun = {}; // session.id -> loopRun
    loopRuns.forEach(function (lr) {
      if (lr.turn_ids && lr.turn_ids.length) {
        lr.turn_ids.forEach(function (tid) {
          if (tid > 0) sessionToRun[tid] = lr;
        });
      }
    });

    // Group sessions into loop runs
    var runGroups = {}; // loopRun.id -> { loopRun, sessions }
    var unclaimedByName = {}; // loop_name -> sessions not matched by turn_ids
    var standaloneSessions = [];

    sessions.forEach(function (session) {
      // Try matching by turn_ids first
      var matchedRun = sessionToRun[session.id];
      if (matchedRun) {
        if (!runGroups[matchedRun.id]) {
          runGroups[matchedRun.id] = { loopRun: matchedRun, sessions: [] };
        }
        runGroups[matchedRun.id].sessions.push(session);
        return;
      }
      // Fallback: session has loop_name but no matching turn_ids
      if (session.loop_name) {
        if (!unclaimedByName[session.loop_name]) unclaimedByName[session.loop_name] = [];
        unclaimedByName[session.loop_name].push(session);
        return;
      }
      standaloneSessions.push(session);
    });

    // Try to match unclaimed sessions to loop runs by loop_name
    // Assign them to the most recent run of that name that doesn't already have turn_ids
    Object.keys(unclaimedByName).forEach(function (loopName) {
      var sessions = unclaimedByName[loopName];
      // Find runs for this loop name
      var matchingRuns = loopRuns.filter(function (lr) { return lr.loop_name === loopName; });
      if (matchingRuns.length === 1) {
        // Only one run, assign all unclaimed to it
        var run = matchingRuns[0];
        if (!runGroups[run.id]) {
          runGroups[run.id] = { loopRun: run, sessions: [] };
        }
        sessions.forEach(function (s) { runGroups[run.id].sessions.push(s); });
      } else if (matchingRuns.length > 1) {
        // Multiple runs, try time-based assignment
        sessions.forEach(function (s) {
          var sTime = parseTimestamp(s.started_at);
          var bestRun = null;
          var bestDiff = Infinity;
          matchingRuns.forEach(function (lr) {
            var rTime = parseTimestamp(lr.started_at);
            var diff = Math.abs(sTime - rTime);
            if (diff < bestDiff) { bestDiff = diff; bestRun = lr; }
          });
          if (bestRun) {
            if (!runGroups[bestRun.id]) {
              runGroups[bestRun.id] = { loopRun: bestRun, sessions: [] };
            }
            runGroups[bestRun.id].sessions.push(s);
          } else {
            standaloneSessions.push(s);
          }
        });
      } else {
        // No matching loop run at all - create a synthetic group
        var syntheticId = 'name-' + loopName;
        runGroups[syntheticId] = {
          loopRun: { id: 0, loop_name: loopName, status: 'completed', hex_id: '', cycle: 0, step_index: 0, started_at: sessions[0] ? sessions[0].started_at : '', steps: [] },
          sessions: sessions,
        };
      }
    });

    // Sort sessions within each group by id (most recent first)
    Object.keys(runGroups).forEach(function (key) {
      runGroups[key].sessions.sort(function (a, b) { return b.id - a.id; });
    });

    // Sort groups by most recent session start time
    var sortedRuns = Object.values(runGroups).sort(function (a, b) {
      var aTime = parseTimestamp(a.loopRun.started_at) || (a.sessions[0] ? parseTimestamp(a.sessions[0].started_at) : 0);
      var bTime = parseTimestamp(b.loopRun.started_at) || (b.sessions[0] ? parseTimestamp(b.sessions[0].started_at) : 0);
      return bTime - aTime;
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

    standaloneSessions.sort(function (a, b) { return b.id - a.id; });

    return { sortedRuns, standaloneSessions, childrenByParent, rootsBySession };
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
      {/* Loop run groups */}
      {tree.sortedRuns.map(function (group) {
        var lr = group.loopRun;
        var runKey = lr.id || lr.loop_name;
        var loopNodeID = 'looprun-' + runKey;
        var expanded = expandedNodes[loopNodeID] !== false; // expanded by default
        var isRunning = !!STATUS_RUNNING[normalizeStatus(lr.status)];
        var loopColor = isRunning ? 'var(--purple)' : 'var(--text-2)';
        var sColor = statusColor(lr.status);

        return (
          <div key={runKey}>
            {/* Loop run header */}
            <div
              onClick={function () { toggleNode(loopNodeID); }}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '8px 12px', cursor: 'pointer',
                background: 'transparent',
                borderBottom: '1px solid var(--bg-3)',
                transition: 'background 0.12s ease',
              }}
              onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
            >
              {/* Expand toggle */}
              <span style={{
                width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 10, color: loopColor,
                transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
              }}>{'\u25BE'}</span>

              {/* Status dot */}
              <span style={{
                width: 7, height: 7, borderRadius: '50%', background: sColor, flexShrink: 0,
                boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
                animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
              }} />

              {/* Loop icon */}
              {isRunning ? (
                <span style={{ color: loopColor, fontSize: 13, animation: 'spin 2s linear infinite', display: 'inline-block', flexShrink: 0 }}>{'\u21BB'}</span>
              ) : (
                <span style={{ color: loopColor, fontSize: 13, flexShrink: 0 }}>{'\u21BB'}</span>
              )}

              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 700, color: 'var(--text-0)' }}>
                    {lr.loop_name || 'loop'}
                  </span>
                  {lr.hex_id && (
                    <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
                      {lr.hex_id.slice(0, 8)}
                    </span>
                  )}
                  {isRunning && (
                    <span style={{
                      padding: '1px 5px', background: 'rgba(123,140,255,0.12)', border: '1px solid rgba(123,140,255,0.25)',
                      borderRadius: 3, fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--purple)', fontWeight: 600,
                    }}>RUNNING</span>
                  )}
                </div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 1 }}>
                  {group.sessions.length} turns
                  {lr.cycle > 0 ? ' \u00B7 cycle ' + (lr.cycle + 1) : ''}
                  {' \u00B7 '}
                  {formatElapsed(lr.started_at, lr.stopped_at)}
                </div>
              </div>
            </div>

            {/* Turns within this loop run */}
            {expanded && group.sessions.map(function (session) {
              return (
                <TurnNode
                  key={session.id}
                  session={session}
                  rootSpawns={tree.rootsBySession[session.id] || []}
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

      {/* Standalone sessions (no loop) */}
      {tree.standaloneSessions.length > 0 && (
        <div>
          {tree.sortedRuns.length > 0 && (
            <div style={{
              padding: '6px 12px', fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.08em',
              borderBottom: '1px solid var(--bg-3)', borderTop: '1px solid var(--bg-3)',
            }}>Standalone</div>
          )}
          {tree.standaloneSessions.map(function (session) {
            return (
              <TurnNode
                key={session.id}
                session={session}
                rootSpawns={tree.rootsBySession[session.id] || []}
                childrenByParent={tree.childrenByParent}
                selectedScope={selectedScope}
                expandedNodes={expandedNodes}
                onSelect={selectScope}
                onToggle={toggleNode}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}

function TurnNode({ session, rootSpawns, childrenByParent, selectedScope, expandedNodes, onSelect, onToggle }) {
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
          padding: '5px 12px 5px 28px', cursor: 'pointer',
          background: selected ? (agentI.color + '10') : 'transparent',
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
              width: 12, height: 12, display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 8, color: agentI.color, cursor: 'pointer',
              transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
            }}
          >{'\u25BE'}</span>
        ) : (
          <span style={{ width: 12 }} />
        )}

        {/* Status dot */}
        <span style={{
          width: 6, height: 6, borderRadius: '50%', background: sColor, flexShrink: 0,
          boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />

        {/* Agent icon */}
        <span style={{ color: agentI.color, fontSize: 10, flexShrink: 0 }}>{agentI.icon}</span>

        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{session.profile || 'unknown'}</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>#{session.id}</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
              {formatElapsed(session.started_at, session.ended_at)}
            </span>
          </div>
        </div>

        {isRunning && (
          <>
            <StopSessionButton sessionID={session.id} />
            <span style={{
              width: 8, height: 8, border: '1.5px solid ' + agentI.color, borderTopColor: 'transparent',
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
            depth={3}
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
          display: 'flex', alignItems: 'center', gap: 5,
          padding: '3px 12px', paddingLeft: 12 + depth * 14, cursor: 'pointer',
          background: selected ? (sColor + '10') : 'transparent',
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
              width: 10, height: 10, display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: 7, color: sColor, cursor: 'pointer',
              transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
            }}
          >{'\u25BE'}</span>
        ) : (
          <span style={{ width: 10, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <span style={{ width: 3, height: 3, borderRadius: '50%', background: sColor + '60' }} />
          </span>
        )}

        <span style={{
          width: 4, height: 4, borderRadius: '50%', background: sColor, flexShrink: 0,
          animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
        }} />

        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ fontSize: 10, fontWeight: 600, color: 'var(--text-0)' }}>{spawn.profile || 'spawn'}</span>
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 8, color: 'var(--text-3)' }}>#{spawn.id}</span>
            {spawn.role && <span style={{ fontSize: 8, color: 'var(--text-3)' }}>{spawn.role}</span>}
          </div>
        </div>

        {isRunning && (
          <span style={{
            width: 7, height: 7, border: '1.5px solid ' + sColor, borderTopColor: 'transparent',
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
