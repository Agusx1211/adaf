import { useState, useCallback } from 'react';
import { apiCall, apiBase } from '../../api/client.js';
import { useAppState, useDispatch } from '../../state/store.js';
import { normalizeStatus, arrayOrEmpty } from '../../utils/format.js';
import { STATUS_RUNNING } from '../../utils/colors.js';
import Modal from '../common/Modal.jsx';
import { useToast } from '../common/Toast.jsx';

export function NewSessionButton() {
  var [showModal, setShowModal] = useState(false);

  return (
    <>
      <button
        onClick={function () { setShowModal(true); }}
        style={{
          padding: '4px 10px', border: '1px solid var(--accent)40',
          background: 'var(--accent)15', color: 'var(--accent)',
          borderRadius: 4, cursor: 'pointer',
          fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
        }}
      >
        New Session
      </button>
      {showModal && <NewSessionModal onClose={function () { setShowModal(false); }} />}
    </>
  );
}

function NewSessionModal({ onClose }) {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var [mode, setMode] = useState('standalone');
  var [loading, setLoading] = useState(false);
  var [profiles, setProfiles] = useState(null);
  var [loops, setLoops] = useState(null);

  // Load profiles and loops on mount
  useState(function () {
    var base = apiBase(state.currentProjectID);
    Promise.all([
      apiCall('/api/config/profiles', 'GET', null, { allow404: true }),
      apiCall('/api/config/loops', 'GET', null, { allow404: true }),
    ]).then(function (results) {
      setProfiles(arrayOrEmpty(results[0]).filter(function (p) { return p && p.name; }));
      setLoops(arrayOrEmpty(results[1]).filter(function (l) { return l && l.name; }));
    }).catch(function () {
      setProfiles([]);
      setLoops([]);
    });
  });

  var handleSubmit = useCallback(async function (e) {
    e.preventDefault();
    setLoading(true);
    var base = apiBase(state.currentProjectID);
    var form = e.target;

    try {
      var payload = {};
      var endpoint = '';

      if (mode === 'standalone') {
        var saProfile = form.sa_profile?.value || '';
        var saPlanId = form.sa_plan_id?.value || '';
        if (!saProfile) { showToast('Profile is required.', 'error'); setLoading(false); return; }
        endpoint = base + '/sessions/ask';
        payload = { profile: saProfile };
        if (saPlanId) payload.plan_id = saPlanId;
      } else if (mode === 'ask') {
        var profile = form.ask_profile?.value || '';
        var prompt = form.ask_prompt?.value || '';
        var planId = form.ask_plan_id?.value || '';
        if (!profile || !prompt) { showToast('Profile and prompt are required.', 'error'); setLoading(false); return; }
        endpoint = base + '/sessions/ask';
        payload = { profile: profile, prompt: prompt };
        if (planId) payload.plan_id = planId;
      } else if (mode === 'pm') {
        var pmProfile = form.pm_profile?.value || '';
        var pmPlanId = form.pm_plan_id?.value || '';
        if (!pmProfile) { showToast('Profile is required.', 'error'); setLoading(false); return; }
        endpoint = base + '/sessions/pm';
        payload = { profile: pmProfile };
        if (pmPlanId) payload.plan_id = pmPlanId;
      } else {
        var loopName = form.loop_name?.value || '';
        var loopPlanId = form.loop_plan_id?.value || '';
        if (!loopName) { showToast('Loop definition is required.', 'error'); setLoading(false); return; }
        endpoint = base + '/sessions/loop';
        payload = { loop_name: loopName, loop: loopName };
        if (loopPlanId) payload.plan_id = loopPlanId;
      }

      var response = await apiCall(endpoint, 'POST', payload);
      var sessionID = Number(response && response.id);
      if (Number.isFinite(sessionID) && sessionID > 0) {
        dispatch({ type: 'SET_SELECTED_SCOPE', payload: 'session-' + sessionID });
      }
      showToast('Session started.', 'success');
      onClose();
    } catch (err) {
      if (err && err.authRequired) { onClose(); return; }
      showToast('Failed to start session: ' + (err.message || err), 'error');
    } finally {
      setLoading(false);
    }
  }, [mode, state.currentProjectID, dispatch, showToast, onClose]);

  var inputStyle = {
    width: '100%', padding: '6px 8px', background: 'var(--bg-3)', border: '1px solid var(--border)',
    borderRadius: 4, color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
  };

  var selectStyle = { ...inputStyle };

  return (
    <Modal title="New Session" onClose={onClose}>
      <form onSubmit={handleSubmit}>
        {/* Mode tabs */}
        <div style={{ display: 'flex', gap: 4, marginBottom: 16 }}>
          {['standalone', 'ask', 'pm', 'loop'].map(function (m) {
            return (
              <button key={m} type="button"
                onClick={function () { setMode(m); }}
                style={{
                  flex: 1, padding: '6px', border: '1px solid ' + (mode === m ? 'var(--accent)' : 'var(--border)'),
                  background: mode === m ? 'var(--accent)15' : 'var(--bg-2)',
                  color: mode === m ? 'var(--accent)' : 'var(--text-2)',
                  borderRadius: 4, cursor: 'pointer',
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: mode === m ? 600 : 400,
                  textTransform: 'uppercase',
                }}
              >{m}</button>
            );
          })}
        </div>

        {mode === 'standalone' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', lineHeight: 1.4 }}>
              Agent runs once with full project context. No prompt needed.
            </div>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Profile</label>
              <select name="sa_profile" style={selectStyle}>
                <option value="">Select profile</option>
                {(profiles || []).map(function (p) {
                  var label = p.name + (p.agent ? ' (' + p.agent + ')' : '');
                  return <option key={p.name} value={p.name}>{label}</option>;
                })}
              </select>
            </div>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Plan ID (optional)</label>
              <input name="sa_plan_id" placeholder="plan-id" style={inputStyle} />
            </div>
          </div>
        )}

        {mode === 'ask' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Profile</label>
              <select name="ask_profile" style={selectStyle}>
                <option value="">Select profile</option>
                {(profiles || []).map(function (p) {
                  var label = p.name + (p.agent ? ' (' + p.agent + ')' : '');
                  return <option key={p.name} value={p.name}>{label}</option>;
                })}
              </select>
            </div>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Prompt</label>
              <textarea name="ask_prompt" placeholder="Describe what the agent should do" style={{ ...inputStyle, minHeight: 60, resize: 'vertical' }} />
            </div>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Plan ID (optional)</label>
              <input name="ask_plan_id" placeholder="plan-id" style={inputStyle} />
            </div>
          </div>
        )}

        {mode === 'pm' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Profile</label>
              <select name="pm_profile" style={selectStyle}>
                <option value="">Select profile</option>
                {(profiles || []).map(function (p) { return <option key={p.name} value={p.name}>{p.name}</option>; })}
              </select>
            </div>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Plan ID (optional)</label>
              <input name="pm_plan_id" placeholder="plan-id" style={inputStyle} />
            </div>
          </div>
        )}

        {mode === 'loop' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Loop Definition</label>
              <select name="loop_name" style={selectStyle}>
                <option value="">Select loop</option>
                {(loops || []).map(function (l) {
                  var steps = arrayOrEmpty(l.steps).length;
                  return <option key={l.name} value={l.name}>{l.name}{steps ? ' (' + steps + ' steps)' : ''}</option>;
                })}
              </select>
            </div>
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Plan ID (optional)</label>
              <input name="loop_plan_id" placeholder="plan-id" style={inputStyle} />
            </div>
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
          <button type="button" onClick={onClose} style={{
            padding: '6px 12px', border: '1px solid var(--border)', background: 'var(--bg-2)',
            color: 'var(--text-1)', borderRadius: 4, cursor: 'pointer',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          }}>Cancel</button>
          <button type="submit" disabled={loading} style={{
            padding: '6px 12px', border: '1px solid var(--accent)',
            background: 'var(--accent)', color: '#000',
            borderRadius: 4, cursor: loading ? 'wait' : 'pointer',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
            opacity: loading ? 0.6 : 1,
          }}>Start Session</button>
        </div>
      </form>
    </Modal>
  );
}

export function SessionMessageBar() {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var [draft, setDraft] = useState('');
  var [sending, setSending] = useState(false);

  var { sessions, loopRun, selectedScope } = state;

  // Find running session for scope
  var target = null;
  if (selectedScope && selectedScope.indexOf('session-') === 0) {
    var sessionID = parseInt(selectedScope.slice(8), 10);
    var session = sessions.find(function (s) { return s.id === sessionID; });
    if (session && STATUS_RUNNING[normalizeStatus(session.status)]) {
      var loopID = loopRun && loopRun.id;
      var loopStatus = normalizeStatus(loopRun && loopRun.status);
      if (loopID && STATUS_RUNNING[loopStatus] && session.loop_name) {
        target = { kind: 'loop', id: loopID, sessionID: session.id };
      } else {
        target = { kind: 'session', id: session.id, sessionID: session.id };
      }
    }
  }

  if (!target) return null;

  var handleSubmit = async function (e) {
    e.preventDefault();
    if (!draft.trim() || sending) return;
    setSending(true);

    var base = apiBase(state.currentProjectID);
    var path = target.kind === 'loop'
      ? base + '/loops/' + encodeURIComponent(String(target.id)) + '/message'
      : base + '/sessions/' + encodeURIComponent(String(target.id)) + '/message';

    try {
      await apiCall(path, 'POST', { message: draft, content: draft });
      setDraft('');
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to send message: ' + (err.message || err), 'error');
    } finally {
      setSending(false);
    }
  };

  var targetLabel = target.kind === 'loop'
    ? 'loop #' + target.id + ' (session #' + target.sessionID + ')'
    : 'session #' + target.id;

  return (
    <form onSubmit={handleSubmit} style={{
      display: 'flex', alignItems: 'center', gap: 8, padding: '6px 12px',
      borderTop: '1px solid var(--border)', background: 'var(--bg-2)',
    }}>
      <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', flexShrink: 0 }}>{targetLabel}</span>
      <input
        type="text"
        value={draft}
        onChange={function (e) { setDraft(e.target.value); }}
        placeholder="Send message to running session"
        style={{
          flex: 1, padding: '4px 8px', background: 'var(--bg-3)', border: '1px solid var(--border)',
          borderRadius: 4, color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
        }}
        autoComplete="off"
      />
      <button type="submit" disabled={sending || !draft.trim()} style={{
        padding: '4px 10px', border: '1px solid var(--accent)',
        background: 'var(--accent)', color: '#000', borderRadius: 4,
        cursor: sending ? 'wait' : 'pointer',
        fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 600,
        opacity: sending || !draft.trim() ? 0.5 : 1,
      }}>Send</button>
    </form>
  );
}

export function StopSessionButton({ sessionID }) {
  var state = useAppState();
  var showToast = useToast();

  var handleStop = async function () {
    if (!window.confirm('Stop session #' + sessionID + '?')) return;
    var base = apiBase(state.currentProjectID);
    try {
      await apiCall(base + '/sessions/' + encodeURIComponent(String(sessionID)) + '/stop', 'POST', {});
      showToast('Stop signal sent for session #' + sessionID + '.', 'success');
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to stop session: ' + (err.message || err), 'error');
    }
  };

  return (
    <button onClick={handleStop} title="Stop session" style={{
      padding: '2px 6px', border: '1px solid var(--red)40',
      background: 'transparent', color: 'var(--red)', borderRadius: 3,
      cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
    }}>{'\u25A0'}</button>
  );
}
