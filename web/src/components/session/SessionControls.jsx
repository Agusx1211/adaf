import { useState, useCallback, useEffect, useRef } from 'react';
import { apiCall, apiBase } from '../../api/client.js';
import { useAppState, useDispatch, normalizePlans } from '../../state/store.js';
import { normalizeStatus, arrayOrEmpty, withAlpha } from '../../utils/format.js';
import { STATUS_RUNNING, statusColor } from '../../utils/colors.js';
import { persistProjectSelection } from '../../utils/projectLink.js';
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
        Start Loop
      </button>
      {showModal && <NewSessionModal onClose={function () { setShowModal(false); }} />}
    </>
  );
}

function NewSessionModal({ onClose }) {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var [loading, setLoading] = useState(false);
  var [loops, setLoops] = useState(null);
  var [selectedProject, setSelectedProject] = useState(state.currentProjectID || '');
  var [plans, setPlans] = useState(null);
  var [selectedPlanID, setSelectedPlanID] = useState('');
  var [planSearch, setPlanSearch] = useState('');
  var [selectedPriority, setSelectedPriority] = useState('normal');

  // Load loops on mount
  useState(function () {
    apiCall('/api/config/loops', 'GET', null, { allow404: true })
      .then(function (result) {
        setLoops(arrayOrEmpty(result).filter(function (l) { return l && l.name; }));
      })
      .catch(function () { setLoops([]); });
  });

  // Load plans when project changes
  useEffect(function () {
    setPlans(null);
    setSelectedPlanID('');
    var base = apiBase(selectedProject);
    apiCall(base + '/plans', 'GET', null, { allow404: true })
      .then(function (result) {
        setPlans(normalizePlans(result));
      })
      .catch(function () { setPlans([]); });
  }, [selectedProject]);

  var handleSubmit = useCallback(async function (e) {
    e.preventDefault();
    setLoading(true);
    var base = apiBase(selectedProject);
    var form = e.target;

    try {
      var loopName = form.loop_name?.value || '';
      var initialPrompt = form.initial_prompt?.value || '';
      if (!loopName) { showToast('Loop definition is required.', 'error'); setLoading(false); return; }

      var payload = { loop_name: loopName, loop: loopName };
      if (selectedPlanID) payload.plan_id = selectedPlanID;
      if (initialPrompt.trim()) payload.initial_prompt = initialPrompt.trim();
      if (selectedPriority) payload.priority = selectedPriority;

      var response = await apiCall(base + '/sessions/loop', 'POST', payload);
      var sessionID = Number(response && response.id);
      if (Number.isFinite(sessionID) && sessionID > 0) {
        if (selectedProject !== state.currentProjectID) {
          dispatch({ type: 'SET_PROJECT_ID', payload: selectedProject });
          dispatch({ type: 'RESET_PROJECT_STATE' });
          persistProjectSelection(selectedProject);
        }
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
  }, [selectedProject, selectedPlanID, selectedPriority, state.currentProjectID, dispatch, showToast, onClose]);

  var inputStyle = {
    width: '100%', padding: '6px 8px', background: 'var(--bg-3)', border: '1px solid var(--border)',
    borderRadius: 4, color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
  };

  var selectStyle = { ...inputStyle };

  var projects = arrayOrEmpty(state.projects);

  return (
    <Modal title="Start Loop" onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {projects.length > 1 && (
            <div>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Project</label>
              <select
                value={selectedProject}
                onChange={function (e) { setSelectedProject(e.target.value); }}
                style={selectStyle}
              >
                {projects.map(function (p) {
                  var id = p && p.id ? String(p.id) : '';
                  var label = p && p.name ? p.name : id || 'Unnamed';
                  if (p && p.is_default) label += ' (default)';
                  return <option key={id} value={id}>{label}</option>;
                })}
              </select>
            </div>
          )}
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
            <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Plan (optional)</label>
            <PlanPicker
              plans={plans}
              selectedPlanID={selectedPlanID}
              onSelect={function (id) { setSelectedPlanID(id); }}
              search={planSearch}
              onSearchChange={setPlanSearch}
              inputStyle={inputStyle}
            />
          </div>
          <div>
            <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Resource Priority</label>
            <select
              name="priority"
              value={selectedPriority}
              onChange={function (e) { setSelectedPriority(String(e.target.value || 'normal')); }}
              style={selectStyle}
            >
              <option value="quality">quality</option>
              <option value="normal">normal</option>
              <option value="cost">cost</option>
            </select>
          </div>
          <div>
            <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', display: 'block', marginBottom: 4 }}>Initial Prompt (optional)</label>
            <textarea
              name="initial_prompt"
              placeholder="General objective for all loop steps..."
              rows={3}
              style={{ ...inputStyle, resize: 'vertical', minHeight: 48 }}
            />
          </div>
        </div>

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
          }}>Start Loop</button>
        </div>
      </form>
    </Modal>
  );
}

function PlanPicker({ plans, selectedPlanID, onSelect, search, onSearchChange, inputStyle }) {
  var listRef = useRef(null);

  if (plans === null) {
    return <div style={{ ...inputStyle, color: 'var(--text-3)', border: 'none', padding: '6px 0' }}>Loading plans...</div>;
  }

  if (plans.length === 0) {
    return <div style={{ ...inputStyle, color: 'var(--text-3)', border: 'none', padding: '6px 0' }}>No plans found for this project.</div>;
  }

  var lowerSearch = (search || '').toLowerCase();
  var filtered = lowerSearch
    ? plans.filter(function (p) {
        return (p.id && p.id.toLowerCase().indexOf(lowerSearch) >= 0) ||
               (p.title && p.title.toLowerCase().indexOf(lowerSearch) >= 0);
      })
    : plans;

  function planStatusMarker(status) {
    var s = normalizeStatus(status);
    if (s === 'active') return '\u25C9';
    if (s === 'done' || s === 'complete' || s === 'completed') return '\u2713';
    if (s === 'cancelled' || s === 'canceled') return '\u2717';
    if (s === 'frozen') return '\u2744';
    return '\u25CB';
  }

  return (
    <div>
      <input
        type="text"
        value={search}
        onChange={function (e) { onSearchChange(e.target.value); }}
        placeholder="Search plans..."
        style={inputStyle}
        autoComplete="off"
      />
      {selectedPlanID && (
        <div style={{
          marginTop: 4, padding: '3px 8px', background: 'var(--accent)15',
          border: '1px solid var(--accent)40', borderRadius: 3,
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
        }}>
          <span style={{ color: 'var(--accent)' }}>Selected: <b>{selectedPlanID}</b></span>
          <button type="button" onClick={function () { onSelect(''); }} style={{
            background: 'none', border: 'none', color: 'var(--text-3)', cursor: 'pointer',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 10, padding: '0 2px',
          }}>{'\u2715'}</button>
        </div>
      )}
      <div ref={listRef} style={{
        marginTop: 4, maxHeight: 180, overflowY: 'auto', border: '1px solid var(--border)',
        borderRadius: 4, background: 'var(--bg-2)',
      }}>
        {filtered.length === 0 && (
          <div style={{ padding: '8px 10px', color: 'var(--text-3)', fontFamily: "'JetBrains Mono', monospace", fontSize: 10 }}>No matching plans.</div>
        )}
        {filtered.map(function (plan) {
          var isSelected = plan.id === selectedPlanID;
          var pStatus = normalizeStatus(plan.status);
          var pColor = statusColor(pStatus);

          return (
            <div key={plan.id} style={{ borderBottom: '1px solid var(--bg-3)' }}>
              <div
                style={{
                  padding: '6px 10px', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6,
                  background: isSelected ? 'var(--accent)12' : 'transparent',
                  transition: 'background 0.12s ease',
                }}
                onMouseEnter={function (e) { if (!isSelected) e.currentTarget.style.background = 'var(--bg-3)'; }}
                onMouseLeave={function (e) { if (!isSelected) e.currentTarget.style.background = 'transparent'; }}
                onClick={function () { onSelect(isSelected ? '' : plan.id); }}
              >
                <span style={{ color: pColor, fontSize: 11, flexShrink: 0 }}>{planStatusMarker(pStatus)}</span>
                <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', flexShrink: 0 }}>{plan.id}</span>
                <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-0)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{plan.title || plan.id}</span>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9, padding: '1px 4px',
                  background: withAlpha(pColor, 0.14), border: '1px solid ' + withAlpha(pColor, 0.28),
                  borderRadius: 3, color: pColor, flexShrink: 0,
                }}>{pStatus}</span>
              </div>
              {plan.description && (
                <div style={{ padding: '0 10px 6px 26px', fontSize: 10, color: 'var(--text-2)', lineHeight: 1.3 }}>
                  {plan.description}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
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
  var sessionID = 0;
  if (selectedScope && selectedScope.indexOf('session-main-') === 0) {
    sessionID = parseInt(selectedScope.slice(13), 10);
  } else if (selectedScope && selectedScope.indexOf('session-') === 0) {
    sessionID = parseInt(selectedScope.slice(8), 10);
  }
  if (sessionID > 0) {
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

export function StopLoopButton({ runID }) {
  var state = useAppState();
  var showToast = useToast();

  var handleStop = async function (e) {
    if (e && e.stopPropagation) e.stopPropagation();
    if (!window.confirm('Stop loop run #' + runID + '?')) return;
    var base = apiBase(state.currentProjectID);
    try {
      await apiCall(base + '/loops/' + encodeURIComponent(String(runID)) + '/stop', 'POST', {});
      showToast('Stop signal sent for loop run #' + runID + '.', 'success');
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to stop loop run: ' + (err.message || err), 'error');
    }
  };

  return (
    <button onClick={handleStop} title="Stop loop run" style={{
      padding: '2px 6px', border: '1px solid var(--red)40',
      background: 'transparent', color: 'var(--red)', borderRadius: 3,
      cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
    }}>{'\u25A0'}</button>
  );
}

export function WindDownLoopButton({ runID }) {
  var state = useAppState();
  var showToast = useToast();

  var handleWindDown = async function (e) {
    if (e && e.stopPropagation) e.stopPropagation();
    if (!window.confirm('Wind down loop run #' + runID + ' after the current turn?')) return;
    var base = apiBase(state.currentProjectID);
    try {
      await apiCall(base + '/loops/' + encodeURIComponent(String(runID)) + '/wind-down', 'POST', {});
      showToast('Wind-down requested for loop run #' + runID + '.', 'success');
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to wind down loop run: ' + (err.message || err), 'error');
    }
  };

  return (
    <button onClick={handleWindDown} title="Wind down loop run" style={{
      padding: '2px 6px', border: '1px solid var(--accent)40',
      background: 'transparent', color: 'var(--accent)', borderRadius: 3,
      cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
    }}>{'WD'}</button>
  );
}
