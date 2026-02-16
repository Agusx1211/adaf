import { useState, useEffect, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall, apiBase } from '../../api/client.js';
import { timeAgo, cropText } from '../../utils/format.js';
import { useToast } from '../common/Toast.jsx';
import Modal from '../common/Modal.jsx';

export default function StandaloneConversationList() {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var base = apiBase(state.currentProjectID);
  var [instances, setInstances] = useState([]);
  var [loading, setLoading] = useState(true);
  var [showNewChat, setShowNewChat] = useState(false);
  var chatStatuses = state.standaloneChatStatuses || {};

  var loadInstances = useCallback(function () {
    if (!state.currentProjectID) return;
    apiCall(base + '/chat-instances', 'GET', null, { allow404: true })
      .then(function (list) {
        var items = (list || []).filter(function (i) { return i && i.id; });
        setInstances(items);
        // Auto-select first if none selected
        if (items.length > 0 && !state.standaloneChatID) {
          dispatch({ type: 'SET_STANDALONE_CHAT_ID', payload: items[0].id });
        }
      })
      .catch(function () {})
      .finally(function () { setLoading(false); });
  }, [base, state.currentProjectID]);

  useEffect(function () { loadInstances(); }, [loadInstances]);

  // Re-fetch when returning to standalone view (to pick up new messages/titles)
  useEffect(function () {
    if (state.leftView === 'standalone') loadInstances();
  }, [state.leftView]);

  function handleSelect(id) {
    dispatch({ type: 'SET_STANDALONE_CHAT_ID', payload: id });
  }

  function handleDelete(e, id) {
    e.stopPropagation();
    if (!window.confirm('Delete this chat?')) return;
    apiCall(base + '/chat-instances/' + encodeURIComponent(id), 'DELETE')
      .then(function () {
        setInstances(function (prev) { return prev.filter(function (i) { return i.id !== id; }); });
        if (state.standaloneChatID === id) {
          dispatch({ type: 'SET_STANDALONE_CHAT_ID', payload: '' });
        }
        showToast('Chat deleted', 'success');
      })
      .catch(function (err) {
        if (err.authRequired) return;
        showToast('Failed to delete: ' + (err.message || err), 'error');
      });
  }

  // Expose reload for external callers (e.g. after creating a chat from the chat view)
  useEffect(function () {
    window.__reloadChatInstances = loadInstances;
    return function () { delete window.__reloadChatInstances; };
  }, [loadInstances]);

  if (loading) {
    return (
      <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-3)' }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11 }}>Loading chats...</div>
      </div>
    );
  }

  if (instances.length === 0 && !showNewChat) {
    return (
      <div style={{ padding: 20, textAlign: 'center', color: 'var(--text-3)' }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, marginBottom: 8 }}>
          No Chats Yet
        </div>
        <div style={{ fontSize: 10, lineHeight: 1.5, opacity: 0.7, marginBottom: 12 }}>
          Start a new conversation with a profile and optional team.
        </div>
        <button
          onClick={function () { setShowNewChat(true); }}
          style={{
            padding: '6px 14px', border: '1px solid var(--accent)',
            background: 'var(--accent)15', color: 'var(--accent)',
            borderRadius: 4, cursor: 'pointer',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
          }}
        >+ New Chat</button>
      </div>
    );
  }

  return (
    <div style={{ padding: '4px 0' }}>
      {instances.map(function (inst) {
        var isSelected = state.standaloneChatID === inst.id;
        var title = inst.title || 'New Chat';
        if (title.length > 60) title = title.slice(0, 60) + '\u2026';
        var chatStatus = chatStatuses[inst.id]; // 'thinking' | 'responding' | undefined
        var statusColor = chatStatus === 'thinking' ? 'var(--accent)' : chatStatus === 'responding' ? 'var(--green)' : null;
        return (
          <div
            key={inst.id}
            onClick={function () { handleSelect(inst.id); }}
            style={{
              padding: '8px 14px', margin: '0 6px 2px 6px',
              borderRadius: 5, cursor: 'pointer',
              background: isSelected ? 'var(--bg-3)' : 'transparent',
              border: isSelected ? '1px solid var(--accent)40' : '1px solid transparent',
              transition: 'background 0.1s',
            }}
            onMouseEnter={function (e) { if (!isSelected) e.currentTarget.style.background = 'var(--bg-2)'; }}
            onMouseLeave={function (e) { if (!isSelected) e.currentTarget.style.background = isSelected ? 'var(--bg-3)' : 'transparent'; }}
          >
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              gap: 8,
            }}>
              <div style={{
                display: 'flex', alignItems: 'center', gap: 6,
                overflow: 'hidden', flex: 1, minWidth: 0,
              }}>
                {chatStatus && (
                  <span style={{
                    width: 6, height: 6, borderRadius: '50%',
                    background: statusColor,
                    animation: 'pulse 1.5s ease-in-out infinite',
                    flexShrink: 0,
                  }} />
                )}
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                  color: isSelected ? 'var(--text-0)' : 'var(--text-1)',
                  fontWeight: isSelected ? 600 : 400,
                  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                }}>{title}</span>
              </div>
              <button
                onClick={function (e) { handleDelete(e, inst.id); }}
                title="Delete chat"
                style={{
                  background: 'none', border: 'none', color: 'var(--text-3)',
                  cursor: 'pointer', fontSize: 12, padding: '0 2px',
                  opacity: 0.4, flexShrink: 0,
                }}
                onMouseEnter={function (e) { e.currentTarget.style.opacity = '1'; e.currentTarget.style.color = 'var(--red)'; }}
                onMouseLeave={function (e) { e.currentTarget.style.opacity = '0.4'; e.currentTarget.style.color = 'var(--text-3)'; }}
              >{'\u00D7'}</button>
            </div>
            <div style={{
              display: 'flex', alignItems: 'center', gap: 6,
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)',
              marginTop: 2,
              marginLeft: chatStatus ? 12 : 0,
            }}>
              {chatStatus ? (
                <span style={{
                  color: statusColor,
                  animation: 'pulse 1.5s ease-in-out infinite',
                  fontWeight: 500,
                }}>
                  {chatStatus === 'thinking' ? 'thinking\u2026' : 'responding\u2026'}
                </span>
              ) : (
                <>
                  <span style={{ opacity: 0.7 }}>{inst.profile}</span>
                  {inst.team && (
                    <>
                      <span style={{ opacity: 0.4 }}>{'\u00B7'}</span>
                      <span style={{ opacity: 0.7, color: 'var(--green)' }}>{inst.team}</span>
                    </>
                  )}
                  <span style={{ opacity: 0.4 }}>{'\u00B7'}</span>
                  <span style={{ opacity: 0.5 }}>{timeAgo(inst.updated_at)}</span>
                </>
              )}
            </div>
          </div>
        );
      })}

      {showNewChat && (
        <NewChatModal
          base={base}
          onCreated={function (inst) {
            setShowNewChat(false);
            setInstances(function (prev) { return [inst].concat(prev); });
            dispatch({ type: 'SET_STANDALONE_CHAT_ID', payload: inst.id });
          }}
          onClose={function () { setShowNewChat(false); }}
        />
      )}
    </div>
  );
}

export function NewChatModal({ base, onCreated, onClose }) {
  var [profiles, setProfiles] = useState([]);
  var [teams, setTeams] = useState([]);
  var [skills, setSkills] = useState([]);
  var [recentCombos, setRecentCombos] = useState([]);
  var [selectedProfile, setSelectedProfile] = useState('');
  var [selectedTeam, setSelectedTeam] = useState('');
  var [selectedSkills, setSelectedSkills] = useState([]);
  var [showSkills, setShowSkills] = useState(false);
  var [creating, setCreating] = useState(false);
  var showToast = useToast();

  useEffect(function () {
    Promise.all([
      apiCall('/api/config/profiles', 'GET', null, { allow404: true }),
      apiCall('/api/config/teams', 'GET', null, { allow404: true }),
      apiCall('/api/config/recent-combinations', 'GET', null, { allow404: true }),
      apiCall('/api/config/skills', 'GET', null, { allow404: true }),
    ]).then(function (results) {
      var profs = (results[0] || []).filter(function (p) { return p && p.name; });
      var ts = (results[1] || []).filter(function (t) { return t && t.name; });
      var combos = (results[2] || []).filter(function (c) { return c && c.profile; });
      var sks = (results[3] || []).filter(function (s) { return s && s.id; });
      setProfiles(profs);
      setTeams(ts);
      setRecentCombos(combos);
      setSkills(sks);
      if (profs.length > 0) setSelectedProfile(profs[0].name);
    }).catch(function () {});
  }, []);

  function handleCreate(e) {
    e.preventDefault();
    if (creating || !selectedProfile) return;

    var body = { profile: selectedProfile };
    if (selectedTeam) body.team = selectedTeam;
    if (selectedSkills.length > 0) body.skills = selectedSkills;

    setCreating(true);
    apiCall(base + '/chat-instances', 'POST', body)
      .then(function (inst) {
        onCreated(inst);
      })
      .catch(function (err) {
        if (err.authRequired) return;
        showToast('Failed to create chat: ' + (err.message || err), 'error');
      })
      .finally(function () { setCreating(false); });
  }

  function handleQuickPick(combo) {
    setSelectedProfile(combo.profile);
    setSelectedTeam(combo.team || '');
  }

  function toggleSkill(id) {
    setSelectedSkills(function (prev) {
      if (prev.indexOf(id) >= 0) {
        return prev.filter(function (s) { return s !== id; });
      }
      return prev.concat([id]);
    });
  }

  var selectStyle = {
    width: '100%', padding: '8px 10px', background: 'var(--bg-2)',
    border: '1px solid var(--border)', borderRadius: 4, color: 'var(--text-0)',
    fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
  };

  var labelStyle = {
    fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
    color: 'var(--text-3)', display: 'block', marginBottom: 6,
    textTransform: 'uppercase', letterSpacing: '0.05em',
  };

  var canCreate = !!selectedProfile;

  return (
    <Modal title="New Chat" onClose={onClose}>
      <form onSubmit={handleCreate}>
        {/* Recent Combinations */}
        {recentCombos.length > 0 && (
          <div style={{ marginBottom: 16 }}>
            <label style={labelStyle}>Recent</label>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {recentCombos.slice(0, 6).map(function (combo, idx) {
                var isActive = selectedProfile === combo.profile && selectedTeam === (combo.team || '');
                return (
                  <button
                    key={idx}
                    type="button"
                    onClick={function () { handleQuickPick(combo); }}
                    style={{
                      padding: '4px 10px', borderRadius: 12,
                      border: isActive ? '1px solid var(--accent)' : '1px solid var(--border)',
                      background: isActive ? 'var(--accent)15' : 'var(--bg-2)',
                      color: isActive ? 'var(--accent)' : 'var(--text-1)',
                      cursor: 'pointer',
                      fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                    }}
                  >
                    {combo.profile}{combo.team ? ' + ' + combo.team : ''}
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/* Profile dropdown */}
        <div style={{ marginBottom: 12 }}>
          <label style={labelStyle}>Profile</label>
          {profiles.length === 0 ? (
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-3)', padding: 8 }}>
              No profiles configured. Create one in the Config tab first.
            </div>
          ) : (
            <select value={selectedProfile} onChange={function (e) { setSelectedProfile(e.target.value); }} style={selectStyle}>
              <option value="">Select profile</option>
              {profiles.map(function (p) {
                return <option key={p.name} value={p.name}>{p.name} ({p.agent})</option>;
              })}
            </select>
          )}
        </div>

        {/* Team dropdown (optional) */}
        {selectedProfile && (
          <div style={{ marginBottom: 12 }}>
            <label style={labelStyle}>Team (optional)</label>
            <select value={selectedTeam} onChange={function (e) { setSelectedTeam(e.target.value); }} style={selectStyle}>
              <option value="">No team</option>
              {teams.map(function (t) {
                var subCount = t.delegation && t.delegation.profiles ? t.delegation.profiles.length : 0;
                return <option key={t.name} value={t.name}>{t.name} ({subCount} sub-agents)</option>;
              })}
            </select>
          </div>
        )}

        {/* Skills (optional, collapsible) */}
        {selectedProfile && skills.length > 0 && (
          <div style={{ marginBottom: 12 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
              <label style={{ ...labelStyle, marginBottom: 0 }}>
                Skills ({selectedSkills.length} selected)
              </label>
              <button type="button" onClick={function () { setShowSkills(!showSkills); }} style={{
                padding: '2px 8px', border: '1px solid var(--border)', background: 'var(--bg-2)',
                color: 'var(--text-2)', borderRadius: 3, cursor: 'pointer',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              }}>
                {showSkills ? 'Hide' : 'Show'}
              </button>
            </div>
            {selectedSkills.length > 0 && !showSkills && (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                {selectedSkills.map(function (id) {
                  return (
                    <span key={id} style={{
                      padding: '2px 6px', borderRadius: 3, fontSize: 9,
                      fontFamily: "'JetBrains Mono', monospace",
                      background: 'var(--pink)15', color: 'var(--pink)',
                      border: '1px solid var(--pink)30',
                    }}>{id}</span>
                  );
                })}
              </div>
            )}
            {showSkills && (
              <div style={{
                border: '1px solid var(--border)', borderRadius: 4, background: 'var(--bg-2)',
                padding: 8, maxHeight: 160, overflow: 'auto',
              }}>
                {skills.map(function (sk) {
                  var isChecked = selectedSkills.indexOf(sk.id) >= 0;
                  return (
                    <label key={sk.id} style={{
                      display: 'flex', alignItems: 'center', gap: 6, padding: '3px 0',
                      cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                      color: isChecked ? 'var(--text-0)' : 'var(--text-2)',
                    }}>
                      <input type="checkbox" checked={isChecked} onChange={function () { toggleSkill(sk.id); }} />
                      <span style={{ fontWeight: isChecked ? 600 : 400 }}>{sk.id}</span>
                    </label>
                  );
                })}
              </div>
            )}
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button type="button" onClick={onClose} style={{
            padding: '6px 12px', border: '1px solid var(--border)', background: 'var(--bg-2)',
            color: 'var(--text-1)', borderRadius: 4, cursor: 'pointer',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          }}>Cancel</button>
          <button type="submit" disabled={!canCreate || creating} style={{
            padding: '6px 12px', border: '1px solid var(--accent)',
            background: 'var(--accent)', color: '#000',
            borderRadius: 4, cursor: !canCreate || creating ? 'not-allowed' : 'pointer',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
            opacity: !canCreate || creating ? 0.6 : 1,
          }}>Create</button>
        </div>
      </form>
    </Modal>
  );
}
