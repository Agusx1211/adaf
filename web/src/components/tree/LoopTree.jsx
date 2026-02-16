import { useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { agentInfo, statusColor, STATUS_RUNNING } from '../../utils/colors.js';
import { normalizeStatus, formatElapsed, parseTimestamp } from '../../utils/format.js';
import { StopSessionButton } from '../session/SessionControls.jsx';

export default function LoopTree() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { sessions, loopRuns, selectedScope, expandedNodes } = state;

  var tree = useMemo(function () {
    var filteredRuns = loopRuns.filter(function (lr) {
      return lr.loop_name !== 'standalone-chat';
    });

    var sessionToRun = {};
    filteredRuns.forEach(function (lr) {
      if (lr.daemon_session_id > 0) {
        sessionToRun[lr.daemon_session_id] = lr;
      } else if (lr.turn_ids && lr.turn_ids.length) {
        lr.turn_ids.forEach(function (tid) {
          if (tid > 0) sessionToRun[tid] = lr;
        });
      }
    });

    var runGroups = {};
    var unclaimedByName = {};
    var standaloneSessions = [];

    sessions.forEach(function (session) {
      if (session.loop_name === 'standalone-chat') return;

      var matchedRun = sessionToRun[session.id];
      if (matchedRun) {
        if (!runGroups[matchedRun.id]) {
          runGroups[matchedRun.id] = { loopRun: matchedRun, sessions: [] };
        }
        runGroups[matchedRun.id].sessions.push(session);
        return;
      }

      if (session.loop_name) {
        if (!unclaimedByName[session.loop_name]) unclaimedByName[session.loop_name] = [];
        unclaimedByName[session.loop_name].push(session);
        return;
      }

      standaloneSessions.push(session);
    });

    Object.keys(unclaimedByName).forEach(function (loopName) {
      var loopSessions = unclaimedByName[loopName];
      var matchingRuns = filteredRuns.filter(function (lr) { return lr.loop_name === loopName; });

      if (matchingRuns.length === 1) {
        var run = matchingRuns[0];
        if (!runGroups[run.id]) {
          runGroups[run.id] = { loopRun: run, sessions: [] };
        }
        loopSessions.forEach(function (s) { runGroups[run.id].sessions.push(s); });
      } else if (matchingRuns.length > 1) {
        loopSessions.forEach(function (s) {
          var sTime = parseTimestamp(s.started_at);
          var bestRun = null;
          var bestDiff = Infinity;
          matchingRuns.forEach(function (lr) {
            var rTime = parseTimestamp(lr.started_at);
            var diff = Math.abs(sTime - rTime);
            if (diff < bestDiff) {
              bestDiff = diff;
              bestRun = lr;
            }
          });
          if (!bestRun) {
            standaloneSessions.push(s);
            return;
          }
          if (!runGroups[bestRun.id]) {
            runGroups[bestRun.id] = { loopRun: bestRun, sessions: [] };
          }
          runGroups[bestRun.id].sessions.push(s);
        });
      } else {
        var syntheticID = 'name-' + loopName;
        runGroups[syntheticID] = {
          loopRun: {
            id: 0,
            loop_name: loopName,
            status: 'completed',
            hex_id: '',
            cycle: 0,
            started_at: loopSessions[0] ? loopSessions[0].started_at : '',
          },
          sessions: loopSessions,
        };
      }
    });

    Object.keys(runGroups).forEach(function (key) {
      runGroups[key].sessions.sort(function (a, b) { return b.id - a.id; });
    });

    var sortedRuns = Object.values(runGroups).sort(function (a, b) {
      var aTime = parseTimestamp(a.loopRun.started_at) || (a.sessions[0] ? parseTimestamp(a.sessions[0].started_at) : 0);
      var bTime = parseTimestamp(b.loopRun.started_at) || (b.sessions[0] ? parseTimestamp(b.sessions[0].started_at) : 0);
      return bTime - aTime;
    });

    standaloneSessions.sort(function (a, b) { return b.id - a.id; });
    return { sortedRuns: sortedRuns, standaloneSessions: standaloneSessions };
  }, [sessions, loopRuns]);

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
      {tree.sortedRuns.map(function (group) {
        var lr = group.loopRun;
        var runKey = lr.id || lr.loop_name;
        var loopNodeID = 'looprun-' + runKey;
        var isRunning = !!STATUS_RUNNING[normalizeStatus(lr.status)];
        var expanded = loopNodeID in expandedNodes ? !!expandedNodes[loopNodeID] : isRunning;
        var loopColor = isRunning ? 'var(--purple)' : 'var(--text-2)';
        var sColor = statusColor(lr.status);

        return (
          <div key={runKey}>
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
              <span style={{
                width: 14, height: 14, display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 10, color: loopColor,
                transform: expanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 0.2s ease',
              }}>{'\u25BE'}</span>

              <span style={{
                width: 7, height: 7, borderRadius: '50%', background: sColor, flexShrink: 0,
                boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
                animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
              }} />

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

            {expanded && group.sessions.map(function (session) {
              return (
                <TurnNode
                  key={session.id}
                  session={session}
                  selectedScope={selectedScope}
                  onSelect={selectScope}
                />
              );
            })}
          </div>
        );
      })}

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
                selectedScope={selectedScope}
                onSelect={selectScope}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}

function TurnNode({ session, selectedScope, onSelect }) {
  var sessionNodeID = 'session-' + session.id;
  var mainScopeID = 'session-main-' + session.id;
  var selected = selectedScope === sessionNodeID || selectedScope === mainScopeID;
  var status = normalizeStatus(session.status);
  var isRunning = !!STATUS_RUNNING[status];
  var sColor = statusColor(session.status);
  var info = agentInfo(session.agent);

  return (
    <div
      onClick={function () { onSelect(sessionNodeID); }}
      style={{
        display: 'flex', alignItems: 'center', gap: 6,
        padding: '5px 12px 5px 28px', cursor: 'pointer',
        background: selected ? (info.color + '10') : 'transparent',
        borderLeft: selected ? ('2px solid ' + info.color) : '2px solid transparent',
        transition: 'all 0.12s ease',
      }}
      onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-3)'; }}
      onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
    >
      <span style={{
        width: 6, height: 6, borderRadius: '50%', background: sColor, flexShrink: 0,
        boxShadow: isRunning ? '0 0 6px ' + sColor : 'none',
        animation: isRunning ? 'pulse 2s ease-in-out infinite' : 'none',
      }} />

      <span style={{ color: info.color, fontSize: 10, flexShrink: 0 }}>{info.icon}</span>

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
            width: 8, height: 8, border: '1.5px solid ' + info.color, borderTopColor: 'transparent',
            borderRadius: '50%', animation: 'spin 1s linear infinite', flexShrink: 0,
          }} />
        </>
      )}
    </div>
  );
}
