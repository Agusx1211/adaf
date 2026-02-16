import { useState, useEffect, useRef, useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { normalizeStatus, formatNumber, formatElapsed } from '../../utils/format.js';
import { STATUS_RUNNING, statusColor } from '../../utils/colors.js';
import StatusDot from '../common/StatusDot.jsx';
import { StopSessionButton, SessionMessageBar } from '../session/SessionControls.jsx';
import { useUsageLimits } from '../../api/hooks.js';
import { persistProjectSelection } from '../../utils/projectLink.js';
import ProjectBrowser from './ProjectBrowser.jsx';

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
  var { sessions, spawns, projects, currentProjectID, wsConnected, termWSConnected, usage, loopRun, leftView, usageLimits } = state;
  var [showRunning, setShowRunning] = useState(false);
  var [showUsage, setShowUsage] = useState(false);
  var [showProjectBrowser, setShowProjectBrowser] = useState(false);
  var dropdownRef = useRef(null);
  var usageDropdownRef = useRef(null);

  useUsageLimits();

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

  // Close usage dropdown on outside click
  useEffect(function () {
    if (!showUsage) return;
    function handleClick(e) {
      if (usageDropdownRef.current && !usageDropdownRef.current.contains(e.target)) {
        setShowUsage(false);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return function () { document.removeEventListener('mousedown', handleClick); };
  }, [showUsage]);

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
    persistProjectSelection(nextID);
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
        <span
          onClick={function () { setShowProjectBrowser(true); }}
          style={{
            fontFamily: "'Outfit', sans-serif", fontSize: 12, color: 'var(--text-2)', fontWeight: 400,
            cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4,
          }}
          title="Browse projects"
        >
          {projectName}
          <span style={{ fontSize: 7, opacity: 0.6 }}>{'\u25BE'}</span>
        </span>
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
        {/* Usage limits dropdown */}
        <div ref={usageDropdownRef} style={{ position: 'relative' }}>
          <UsagePill
            usageLimits={usageLimits}
            showUsage={showUsage}
            onClick={function () { setShowUsage(!showUsage); }}
          />
          {showUsage && (
            <UsageDropdown
              usageLimits={usageLimits}
              usage={usage}
              onClose={function () { setShowUsage(false); }}
            />
          )}
        </div>

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
      {showProjectBrowser && (
        <ProjectBrowser onClose={function () { setShowProjectBrowser(false); }} />
      )}
    </div>
  );
}

function getLevelColor(level) {
  if (level === 'exhausted') return '#ef4444';
  if (level === 'critical') return '#ef4444';
  if (level === 'warning') return '#eab308';
  return '#4ade80';
}

function formatResetTime(resetsAt) {
  if (!resetsAt) return '';
  var reset = new Date(resetsAt);
  var now = new Date();
  var diff = reset - now;
  if (diff <= 0) return 'resets soon';
  var mins = Math.floor(diff / 60000);
  if (mins < 60) return 'resets in ' + mins + 'm';
  var hours = Math.floor(mins / 60);
  if (hours < 24) return 'resets in ' + hours + 'h';
  var days = Math.floor(hours / 24);
  return 'resets in ' + days + 'd';
}

function getHighestLevel(snapshots) {
  if (!snapshots || !snapshots.length) return 'normal';
  var levels = { normal: 0, warning: 1, critical: 2, exhausted: 3 };
  var highest = 'normal';
  for (var i = 0; i < snapshots.length; i++) {
    var s = snapshots[i];
    if (levels[s.level] > levels[highest]) highest = s.level;
  }
  return highest;
}

function UsagePill(props) {
  var usageLimits = props.usageLimits;
  var showUsage = props.showUsage;
  var onClick = props.onClick;

  var snapshots = (usageLimits && usageLimits.snapshots) || [];
  var highestLevel = getHighestLevel(snapshots);
  var color = getLevelColor(highestLevel);
  var hasLimits = snapshots.length > 0;

  return (
    <button
      onClick={onClick}
      style={{
        display: 'flex', alignItems: 'center', gap: 5,
        padding: '3px 8px',
        border: '1px solid ' + (showUsage ? 'var(--accent)' : hasLimits ? color + '40' : 'var(--border)'),
        background: showUsage ? 'var(--accent)15' : hasLimits ? color + '15' : 'var(--bg-2)',
        borderRadius: 4, cursor: 'pointer',
        fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
        color: hasLimits ? color : 'var(--text-3)',
        transition: 'all 0.15s ease',
      }}
    >
      <span style={{
        width: 6, height: 6, borderRadius: '50%', background: color, flexShrink: 0,
        animation: highestLevel !== 'normal' ? 'pulse 2s ease-in-out infinite' : 'none',
      }} />
      <span>Limits</span>
      <span style={{ fontSize: 7, opacity: 0.6 }}>{'\u25BE'}</span>
    </button>
  );
}

function formatFetchedAgo(timestamp) {
  if (!timestamp) return '';
  var fetched = new Date(timestamp);
  var now = new Date();
  var diff = now - fetched;
  if (diff < 0) return 'just now';
  var secs = Math.floor(diff / 1000);
  if (secs < 10) return 'just now';
  if (secs < 60) return secs + 's ago';
  var mins = Math.floor(secs / 60);
  if (mins < 60) return mins + 'm ago';
  var hours = Math.floor(mins / 60);
  return hours + 'h ago';
}

function UsageDropdown(props) {
  var usageLimits = props.usageLimits;
  var usage = props.usage || { input_tokens: 0, output_tokens: 0, cost_usd: 0 };

  var snapshots = (usageLimits && usageLimits.snapshots) || [];
  var errors = (usageLimits && usageLimits.errors) || [];

  return (
    <div style={{
      position: 'absolute', top: 'calc(100% + 6px)', right: 0,
      width: 340, maxHeight: 420,
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
        }}>Usage Limits</span>
      </div>

      <div style={{ flex: 1, overflow: 'auto', padding: '4px 0' }}>
        {snapshots.length === 0 && errors.length === 0 && (
          <div style={{
            padding: 20, textAlign: 'center', color: 'var(--text-3)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
          }}>
            No usage data available.<br/>
            Configure Claude or Codex to see limits.
          </div>
        )}

        {snapshots.map(function (snapshot) {
          var fetchedAgo = formatFetchedAgo(snapshot.timestamp);
          return (
            <div key={snapshot.provider} style={{
              padding: '8px 12px',
              borderBottom: '1px solid var(--bg-3)',
            }}>
              <div style={{
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
                color: 'var(--text-1)', marginBottom: 8,
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              }}>
                <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{
                    width: 6, height: 6, borderRadius: '50%',
                    background: getLevelColor(snapshot.level),
                  }} />
                  {snapshot.provider}
                </span>
                {fetchedAgo && (
                  <span style={{
                    fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
                    color: 'var(--text-3)', fontWeight: 400,
                  }}>{fetchedAgo}</span>
                )}
              </div>
              {snapshot.limits && snapshot.limits.map(function (limit) {
                var pct = Number(limit.utilization_pct) || 0;
                var filled = Math.min(Math.max(pct, 0), 100);
                var color = getLevelColor(limit.level || 'normal');
                var resetText = formatResetTime(limit.resets_at);
                return (
                  <div key={limit.name} style={{ marginBottom: 8 }}>
                    <div style={{
                      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                      marginBottom: 4,
                    }}>
                      <span style={{
                        fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                        color: 'var(--text-2)',
                      }}>{limit.name}</span>
                      <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span style={{
                          fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                          color: color, fontWeight: 600,
                        }}>{Math.round(pct)}%</span>
                        {resetText && (
                          <span style={{
                            fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
                            color: 'var(--text-3)',
                          }}>{resetText}</span>
                        )}
                      </span>
                    </div>
                    <div style={{
                      width: '100%', height: 6, background: 'var(--bg-3)', borderRadius: 3,
                      overflow: 'hidden',
                    }}>
                      <div style={{
                        width: filled + '%', height: '100%',
                        background: color, borderRadius: 3,
                        transition: 'width 0.3s ease',
                      }} />
                    </div>
                  </div>
                );
              })}
            </div>
          );
        })}

        {errors.length > 0 && (
          <div style={{ padding: '8px 12px' }}>
            {errors.map(function (err, i) {
              return (
                <div key={i} style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                  color: '#ef4444', marginBottom: 2,
                }}>{err}</div>
              );
            })}
          </div>
        )}
      </div>

      {usage && (usage.input_tokens || usage.output_tokens || usage.cost_usd) && (
        <div style={{
          padding: '8px 12px', borderTop: '1px solid var(--border)',
          display: 'flex', gap: 12,
          fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
          color: 'var(--text-3)',
        }}>
          <span>in={formatNumber(usage.input_tokens || 0)}</span>
          <span>out={formatNumber(usage.output_tokens || 0)}</span>
          <span style={{ color: '#4ade80' }}>${Number(usage.cost_usd || 0).toFixed(4)}</span>
        </div>
      )}
    </div>
  );
}
