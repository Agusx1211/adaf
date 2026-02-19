import { useState, useEffect, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall } from '../../api/client.js';
import { useToast } from '../common/Toast.jsx';

var sectionHeaderStyle = {
  display: 'flex', alignItems: 'center', justifyContent: 'space-between',
  padding: '8px 14px', borderBottom: '1px solid var(--border)',
  cursor: 'pointer', userSelect: 'none',
};

var titleStyle = {
  fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
  color: 'var(--text-1)',
};

var rowStyle = {
  display: 'flex', alignItems: 'center', gap: 8,
  padding: '8px 14px', borderBottom: '1px solid var(--bg-3)',
  cursor: 'pointer', transition: 'background 0.15s ease',
};

var btnNewStyle = {
  padding: '2px 8px', border: '1px solid var(--accent)40',
  background: 'var(--accent)15', color: 'var(--accent)',
  borderRadius: 3, cursor: 'pointer',
  fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
};

var btnCopyStyle = {
  padding: '2px 8px',
  border: '1px solid var(--border)',
  background: 'var(--bg-2)',
  color: 'var(--text-2)',
  borderRadius: 3,
  cursor: 'pointer',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  flexShrink: 0,
};

export default function ConfigView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var [profiles, setProfiles] = useState([]);
  var [loops, setLoops] = useState([]);
  var [teams, setTeams] = useState([]);
  var [skills, setSkills] = useState([]);
  var [roles, setRoles] = useState([]);
  var [expanded, setExpanded] = useState({ profiles: true, loops: true, teams: true, skills: false, roles: false });

  var configSelection = state.configSelection;

  var loadAll = useCallback(async function () {
    try {
      var results = await Promise.all([
        apiCall('/api/config/profiles', 'GET', null, { allow404: true }),
        apiCall('/api/config/loops', 'GET', null, { allow404: true }),
        apiCall('/api/config/teams', 'GET', null, { allow404: true }),
        apiCall('/api/config/skills', 'GET', null, { allow404: true }),
        apiCall('/api/config/roles', 'GET', null, { allow404: true }),
      ]);
      setProfiles(results[0] || []);
      setLoops(results[1] || []);
      setTeams(results[2] || []);
      setSkills(results[3] || []);
      setRoles(results[4] || []);
    } catch (err) {
      if (!err.authRequired) console.error('Config load error:', err);
    }
  }, []);

  useEffect(function () { loadAll(); }, [loadAll]);

  // Expose reload function on window for detail panel to call
  useEffect(function () {
    window.__configReload = loadAll;
    window.__configProfiles = profiles;
    window.__configTeams = teams;
    window.__configSkills = skills;
    return function () { delete window.__configReload; delete window.__configProfiles; delete window.__configTeams; delete window.__configSkills; };
  }, [loadAll, profiles, teams, skills]);

  function toggle(section) {
    setExpanded(function (prev) { return { ...prev, [section]: !prev[section] }; });
  }

  function select(type, name) {
    dispatch({ type: 'SET_CONFIG_SELECTION', payload: { type: type, name: name } });
  }

  function isSelected(type, name) {
    return configSelection && configSelection.type === type && configSelection.name === name;
  }

  function handleNew(type) {
    dispatch({ type: 'SET_CONFIG_SELECTION', payload: { type: type, name: null, isNew: true } });
  }

  function handleCopy(type, name) {
    dispatch({ type: 'SET_CONFIG_SELECTION', payload: { type: type, name: null, isNew: true, copyFrom: name } });
  }

  return (
    <div style={{ flex: 1, overflow: 'auto' }}>
      {/* Profiles */}
      <div>
        <div style={sectionHeaderStyle} onClick={function () { toggle('profiles'); }}>
          <span style={titleStyle}>{expanded.profiles ? '\u25BE' : '\u25B8'} Profiles ({profiles.length})</span>
          <button onClick={function (e) { e.stopPropagation(); handleNew('profile'); }} style={btnNewStyle}>+ New</button>
        </div>
        {expanded.profiles && profiles.map(function (p) {
          var sel = isSelected('profile', p.name);
          return (
            <div key={p.name} onClick={function () { select('profile', p.name); }}
              style={{ ...rowStyle, background: sel ? 'var(--bg-3)' : 'transparent', borderLeft: sel ? '2px solid var(--accent)' : '2px solid transparent' }}
              onMouseEnter={function (e) { if (!sel) e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { if (!sel) e.currentTarget.style.background = 'transparent'; }}
            >
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-0)', flex: 1 }}>{p.name}</span>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
                {p.agent}{p.model ? '/' + p.model : ''}{p.cost ? ' · ' + p.cost : ''}
              </span>
            </div>
          );
        })}
        {expanded.profiles && profiles.length === 0 && (
          <div style={{ padding: 12, color: 'var(--text-3)', fontSize: 11, textAlign: 'center' }}>No profiles configured.</div>
        )}
      </div>

      {/* Loops */}
      <div>
        <div style={sectionHeaderStyle} onClick={function () { toggle('loops'); }}>
          <span style={titleStyle}>{expanded.loops ? '\u25BE' : '\u25B8'} Loops ({loops.length})</span>
          <button onClick={function (e) { e.stopPropagation(); handleNew('loop'); }} style={btnNewStyle}>+ New</button>
        </div>
        {expanded.loops && loops.map(function (l) {
          var sel = isSelected('loop', l.name);
          var stepSummary = (l.steps || []).map(function (s) { return s.profile; }).join(' \u2192 ');
          return (
            <div key={l.name} onClick={function () { select('loop', l.name); }}
              style={{ ...rowStyle, background: sel ? 'var(--bg-3)' : 'transparent', borderLeft: sel ? '2px solid var(--purple)' : '2px solid transparent' }}
              onMouseEnter={function (e) { if (!sel) e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { if (!sel) e.currentTarget.style.background = 'transparent'; }}
            >
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{l.name}</div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {(l.steps || []).length} steps: {stepSummary || 'none'}
                </div>
              </div>
              <button
                type="button"
                aria-label={'Copy loop ' + l.name}
                onClick={function (e) { e.stopPropagation(); handleCopy('loop', l.name); }}
                style={btnCopyStyle}
              >
                Copy
              </button>
            </div>
          );
        })}
        {expanded.loops && loops.length === 0 && (
          <div style={{ padding: 12, color: 'var(--text-3)', fontSize: 11, textAlign: 'center' }}>No loops configured.</div>
        )}
      </div>

      {/* Teams */}
      <div>
        <div style={sectionHeaderStyle} onClick={function () { toggle('teams'); }}>
          <span style={titleStyle}>{expanded.teams ? '\u25BE' : '\u25B8'} Teams ({teams.length})</span>
          <button onClick={function (e) { e.stopPropagation(); handleNew('team'); }} style={{ ...btnNewStyle, border: '1px solid var(--green)40', background: 'var(--green)15', color: 'var(--green)' }}>+ New</button>
        </div>
        {expanded.teams && teams.map(function (t) {
          var sel = isSelected('team', t.name);
          var subCount = t.delegation && t.delegation.profiles ? t.delegation.profiles.length : 0;
          return (
            <div key={t.name} onClick={function () { select('team', t.name); }}
              style={{ ...rowStyle, background: sel ? 'var(--bg-3)' : 'transparent', borderLeft: sel ? '2px solid var(--green)' : '2px solid transparent' }}
              onMouseEnter={function (e) { if (!sel) e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { if (!sel) e.currentTarget.style.background = 'transparent'; }}
            >
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{t.name}</div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 2 }}>
                  {subCount} sub-agent{subCount !== 1 ? 's' : ''}
                  {t.description ? ' \u00B7 ' + t.description : ''}
                </div>
              </div>
              <button
                type="button"
                aria-label={'Copy team ' + t.name}
                onClick={function (e) { e.stopPropagation(); handleCopy('team', t.name); }}
                style={btnCopyStyle}
              >
                Copy
              </button>
            </div>
          );
        })}
        {expanded.teams && teams.length === 0 && (
          <div style={{ padding: 12, color: 'var(--text-3)', fontSize: 11, textAlign: 'center' }}>No teams configured.</div>
        )}
      </div>

      {/* Roles */}
      <div>
        <div style={sectionHeaderStyle} onClick={function () { toggle('roles'); }}>
          <span style={titleStyle}>{expanded.roles ? '\u25BE' : '\u25B8'} Roles ({roles.length})</span>
          <button onClick={function (e) { e.stopPropagation(); handleNew('role'); }} style={{ ...btnNewStyle, border: '1px solid var(--orange)40', background: 'var(--orange)15', color: 'var(--orange)' }}>+ New</button>
        </div>
        {expanded.roles && roles.map(function (rl) {
          var sel = isSelected('role', rl.name);
          var desc = rl.description || '';
          if (desc.length > 60) desc = desc.slice(0, 60) + '\u2026';
          return (
            <div key={rl.name} onClick={function () { select('role', rl.name); }}
              style={{ ...rowStyle, background: sel ? 'var(--bg-3)' : 'transparent', borderLeft: sel ? '2px solid var(--orange)' : '2px solid transparent' }}
              onMouseEnter={function (e) { if (!sel) e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { if (!sel) e.currentTarget.style.background = 'transparent'; }}
            >
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{rl.name}</span>
                  {!rl.can_write_code && (
                    <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 8, padding: '1px 4px', borderRadius: 3, background: 'var(--red)15', color: 'var(--red)' }}>read-only</span>
                  )}
                </div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {desc}
                </div>
              </div>
            </div>
          );
        })}
        {expanded.roles && roles.length === 0 && (
          <div style={{ padding: 12, color: 'var(--text-3)', fontSize: 11, textAlign: 'center' }}>No roles configured.</div>
        )}
      </div>

      {/* Skills */}
      <div>
        <div style={sectionHeaderStyle} onClick={function () { toggle('skills'); }}>
          <span style={titleStyle}>{expanded.skills ? '\u25BE' : '\u25B8'} Skills ({skills.length})</span>
          <button onClick={function (e) { e.stopPropagation(); handleNew('skill'); }} style={{ ...btnNewStyle, border: '1px solid var(--pink)40', background: 'var(--pink)15', color: 'var(--pink)' }}>+ New</button>
        </div>
        {expanded.skills && skills.map(function (sk) {
          var sel = isSelected('skill', sk.id);
          var summary = sk.short || '';
          if (summary.length > 60) summary = summary.slice(0, 60) + '\u2026';
          return (
            <div key={sk.id} onClick={function () { select('skill', sk.id); }}
              style={{ ...rowStyle, background: sel ? 'var(--bg-3)' : 'transparent', borderLeft: sel ? '2px solid var(--pink)' : '2px solid transparent' }}
              onMouseEnter={function (e) { if (!sel) e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { if (!sel) e.currentTarget.style.background = 'transparent'; }}
            >
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{sk.id}</div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 2, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {summary}
                </div>
              </div>
            </div>
          );
        })}
        {expanded.skills && skills.length === 0 && (
          <div style={{ padding: 12, color: 'var(--text-3)', fontSize: 11, textAlign: 'center' }}>No skills configured.</div>
        )}
      </div>

    </div>
  );
}
