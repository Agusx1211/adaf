import { useState, useEffect, useRef, useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { normalizeStatus, formatNumber, formatElapsed } from '../../utils/format.js';
import { STATUS_RUNNING, statusColor } from '../../utils/colors.js';
import StatusDot from '../common/StatusDot.jsx';
import { StopSessionButton, SessionMessageBar } from '../session/SessionControls.jsx';

var NAV_ITEMS = [
  { id: 'loops', label: 'Loops' },
  { id: 'standalone', label: 'Standalone' },
  { id: 'issues', label: 'Issues' },
  { id: 'docs', label: 'Docs' },
  { id: 'plan', label: 'Plan' },
  { id: 'logs', label: 'Logs' },
  { id: 'config', label: 'Config' },
];

export default function TopBar() {
  var state = useAppState();
  var dispatch = useDispatch();
  var { sessions, spawns, projects, currentProjectID, wsConnected, termWSConnected, usage, loopRun, leftView } = state;
  var [showRunning, setShowRunning] = useState(false);
  var dropdownRef = useRef(null);

  var counts = useMemo(function () {
    var running = 0;
    var total = sessions.length + spawns.length;
    sessions.forEach(function (s) { if (STATUS_RUNNING[normalizeStatus(s.status)]) running++; });
    spawns.forEach(function (s) { if (STATUS_RUNNING[normalizeStatus(s.status)]) running++; });
    return { running, total };
  }, [sessions, spawns]);

  var runningSessions = useMemo(function () {
    return sessions.filter(function (s) { return !!STATUS_RUNNING[normalizeStatus(s.status)]; });
  }, [sessions]);

  // Close dropdown on outside click
  useEffect(function () {
    if (!showRunning) return;
    function handleClick(e) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target)) {
        setShowRunning(false);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return function () { document.removeEventListener('mousedown', handleClick); };
  }, [showRunning]);

  var projectName = useMemo(function () {
    if (!currentProjectID && projects.length) {
      var def = projects.find(function (p) { return p && p.is_default; }) || projects[0];
      return def && def.name ? def.name : 'project';
    }
    var p = projects.find(function (p) { return p && String(p.id || '') === currentProjectID; });
    return p && p.name ? p.name : (state.projectMeta && state.projectMeta.name ? state.projectMeta.name : 'project');
  }, [projects, currentProjectID, state.projectMeta]);

  var wsOnline = wsConnected || termWSConnected;
  var u = usage || { input_tokens: 0, output_tokens: 0, cost_usd: 0, num_turns: 0 };

  function switchProject(e) {
    var nextID = e.target.value || '';
    dispatch({ type: 'SET_PROJECT_ID', payload: nextID });
    dispatch({ type: 'RESET_PROJECT_STATE' });
    try { localStorage.setItem('adaf_project_id', nextID); } catch (_) {}
  }

  function setView(view) {
    dispatch({ type: 'SET_LEFT_VIEW', payload: view });
  }

  return (
    <div style={{
      display: 'flex', alignItems: 'center',
      padding: '0 16px', height: 42, background: 'var(--bg-1)',
      borderBottom: '1px solid var(--border)', flexShrink: 0,
      gap: 12,
    }}>
      {/* Brand + Project */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexShrink: 0 }}>
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontWeight: 800, fontSize: 15,
          background: 'linear-gradient(135deg, var(--accent), #FFD700)',
          WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent',
          letterSpacing: '0.08em',
        }}>ADAF</span>
        <span style={{ width: 1, height: 16, background: 'var(--border)' }} />
        <span style={{ fontFamily: "'Outfit', sans-serif", fontSize: 12, color: 'var(--text-2)', fontWeight: 400 }}>
          {projectName}
        </span>
        {projects.length > 1 && (
          <select
            value={currentProjectID}
            onChange={switchProject}
            style={{
              background: 'var(--bg-2)', border: '1px solid var(--border)', borderRadius: 3,
              color: 'var(--text-1)', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
              padding: '2px 4px', cursor: 'pointer',
            }}
          >
            {projects.map(function (p) {
              var id = p && p.id ? String(p.id) : '';
              var label = p && p.name ? p.name : id || 'Unnamed';
              if (p && p.is_default) label += ' (default)';
              return <option key={id} value={id}>{label}</option>;
            })}
          </select>
        )}
      </div>

      {/* Navigation */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 2,
        flex: 1, justifyContent: 'center',
      }}>
        {NAV_ITEMS.map(function (item) {
          var active = item.id === leftView;
          return (
            <button
              key={item.id}
              onClick={function () { setView(item.id); }}
              style={{
                padding: '4px 10px', border: 'none',
                background: active ? 'var(--accent)15' : 'transparent',
                color: active ? 'var(--accent)' : 'var(--text-3)',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                fontWeight: active ? 600 : 400,
                cursor: 'pointer', borderRadius: 4,
                transition: 'all 0.15s ease',
              }}
            >{item.label}</button>
          );
        })}
      </div>

      {/* Right stats */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexShrink: 0 }}>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'flex', gap: 8 }}>
          <span>in={formatNumber(u.input_tokens || 0)}</span>
          <span>out={formatNumber(u.output_tokens || 0)}</span>
          <span style={{ color: 'var(--green)' }}>${Number(u.cost_usd || 0).toFixed(4)}</span>
        </span>

        {/* Running sessions dropdown */}
        <div ref={dropdownRef} style={{ position: 'relative' }}>
          <button
            onClick={function () { setShowRunning(!showRunning); }}
            style={{
              display: 'flex', alignItems: 'center', gap: 5,
              padding: '3px 8px',
              border: '1px solid ' + (showRunning ? 'var(--accent)' : counts.running > 0 ? 'rgba(74,230,138,0.25)' : 'var(--border)'),
              background: showRunning ? 'var(--accent)15' : counts.running > 0 ? 'rgba(74,230,138,0.08)' : 'var(--bg-2)',
              borderRadius: 4, cursor: 'pointer',
              fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
              color: counts.running > 0 ? 'var(--green)' : 'var(--text-2)',
              transition: 'all 0.15s ease',
            }}
          >
            {counts.running > 0 && (
              <span style={{
                width: 5, height: 5, borderRadius: '50%',
                background: 'var(--green)',
                animation: 'pulse 2s ease-in-out infinite',
                flexShrink: 0,
              }} />
            )}
            <span>{counts.running > 0 ? counts.running + ' running' : '0 running'}</span>
            <span style={{ fontSize: 7, opacity: 0.6 }}>{'\u25BE'}</span>
          </button>

          {showRunning && (
            <div style={{
              position: 'absolute', top: 'calc(100% + 6px)', right: 0,
              width: 420, maxHeight: 360,
              background: 'var(--bg-1)', border: '1px solid var(--border)',
              borderRadius: 6, boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
              zIndex: 1000, display: 'flex', flexDirection: 'column',
              overflow: 'hidden', animation: 'slideIn 0.12s ease-out',
            }}>
              <div style={{
                padding: '8px 12px', borderBottom: '1px solid var(--border)',
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              }}>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
                  color: 'var(--text-1)',
                }}>Running Sessions</span>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                  padding: '1px 6px', borderRadius: 3,
                  background: runningSessions.length > 0 ? 'var(--green)20' : 'var(--bg-3)',
                  color: runningSessions.length > 0 ? 'var(--green)' : 'var(--text-3)',
                }}>{runningSessions.length}</span>
              </div>

              <div style={{ flex: 1, overflow: 'auto', maxHeight: 260 }}>
                {runningSessions.length === 0 ? (
                  <div style={{
                    padding: 20, textAlign: 'center', color: 'var(--text-3)',
                    fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                  }}>No running sessions</div>
                ) : (
                  runningSessions.map(function (session) {
                    var sColor = statusColor(session.status);
                    return (
                      <div key={session.id} style={{
                        display: 'flex', alignItems: 'center', gap: 8,
                        padding: '6px 12px', borderBottom: '1px solid var(--bg-3)',
                      }}
                      onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
                      onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
                      >
                        <span style={{
                          width: 5, height: 5, borderRadius: '50%', background: sColor, flexShrink: 0,
                          boxShadow: '0 0 6px ' + sColor,
                          animation: 'pulse 2s ease-in-out infinite',
                        }} />
                        <span style={{
                          fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', flexShrink: 0,
                        }}>#{session.id}</span>
                        <span style={{
                          fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
                          color: 'var(--text-0)', flex: 1, overflow: 'hidden',
                          textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                        }}>{session.profile || 'unknown'}</span>
                        <span style={{
                          fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', flexShrink: 0,
                        }}>{session.agent || 'agent'}</span>
                        <span style={{
                          fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', flexShrink: 0,
                        }}>{formatElapsed(session.started_at, session.ended_at)}</span>
                        <StopSessionButton sessionID={session.id} />
                      </div>
                    );
                  })
                )}
              </div>

              <SessionMessageBar />
            </div>
          )}
        </div>

        {loopRun && normalizeStatus(loopRun.status) === 'running' && (
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--purple)', display: 'flex', alignItems: 'center', gap: 4 }}>
            <span style={{ animation: 'spin 2s linear infinite', display: 'inline-block' }}>{'\u21BB'}</span>
            {loopRun.loop_name || 'loop'}
          </span>
        )}

        <span style={{
          display: 'flex', alignItems: 'center', gap: 4, padding: '2px 8px',
          background: wsOnline ? 'rgba(74,230,138,0.1)' : 'var(--bg-3)',
          border: '1px solid ' + (wsOnline ? 'rgba(74,230,138,0.25)' : 'var(--border)'),
          borderRadius: 4,
        }}>
          <span style={{
            width: 5, height: 5, borderRadius: '50%',
            background: wsOnline ? 'var(--green)' : 'var(--text-3)',
            animation: wsOnline ? 'pulse 2s ease-in-out infinite' : 'none',
          }} />
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: wsOnline ? 'var(--green)' : 'var(--text-3)' }}>
            {wsOnline ? 'live' : 'offline'}
          </span>
        </span>
      </div>
    </div>
  );
}
