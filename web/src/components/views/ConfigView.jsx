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

export default function ConfigView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var [profiles, setProfiles] = useState([]);
  var [loops, setLoops] = useState([]);
  var [standaloneProfiles, setStandaloneProfiles] = useState([]);
  var [expanded, setExpanded] = useState({ profiles: true, loops: true, standalone: true });

  var configSelection = state.configSelection;

  var loadAll = useCallback(async function () {
    try {
      var results = await Promise.all([
        apiCall('/api/config/profiles', 'GET', null, { allow404: true }),
        apiCall('/api/config/loops', 'GET', null, { allow404: true }),
        apiCall('/api/config/standalone-profiles', 'GET', null, { allow404: true }),
      ]);
      setProfiles(results[0] || []);
      setLoops(results[1] || []);
      setStandaloneProfiles(results[2] || []);
    } catch (err) {
      if (!err.authRequired) console.error('Config load error:', err);
    }
  }, []);

  useEffect(function () { loadAll(); }, [loadAll]);

  // Expose reload function on window for detail panel to call
  useEffect(function () {
    window.__configReload = loadAll;
    window.__configProfiles = profiles;
    return function () { delete window.__configReload; delete window.__configProfiles; };
  }, [loadAll, profiles]);

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
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>{p.agent}{p.model ? '/' + p.model : ''}</span>
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
            </div>
          );
        })}
        {expanded.loops && loops.length === 0 && (
          <div style={{ padding: 12, color: 'var(--text-3)', fontSize: 11, textAlign: 'center' }}>No loops configured.</div>
        )}
      </div>

      {/* Standalone Profiles */}
      <div>
        <div style={sectionHeaderStyle} onClick={function () { toggle('standalone'); }}>
          <span style={titleStyle}>{expanded.standalone ? '\u25BE' : '\u25B8'} Standalone Profiles ({standaloneProfiles.length})</span>
          <button onClick={function (e) { e.stopPropagation(); handleNew('standalone'); }} style={btnNewStyle}>+ New</button>
        </div>
        {expanded.standalone && standaloneProfiles.map(function (sp) {
          var sel = isSelected('standalone', sp.name);
          return (
            <div key={sp.name} onClick={function () { select('standalone', sp.name); }}
              style={{ ...rowStyle, background: sel ? 'var(--bg-3)' : 'transparent', borderLeft: sel ? '2px solid var(--orange)' : '2px solid transparent' }}
              onMouseEnter={function (e) { if (!sel) e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { if (!sel) e.currentTarget.style.background = 'transparent'; }}
            >
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-0)' }}>{sp.name}</div>
                <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginTop: 2 }}>
                  profile: {sp.profile}
                  {sp.instructions ? ' \u00B7 has instructions' : ''}
                  {sp.delegation ? ' \u00B7 has delegation' : ''}
                </div>
              </div>
            </div>
          );
        })}
        {expanded.standalone && standaloneProfiles.length === 0 && (
          <div style={{ padding: 12, color: 'var(--text-3)', fontSize: 11, textAlign: 'center' }}>No standalone profiles configured.</div>
        )}
      </div>
    </div>
  );
}
