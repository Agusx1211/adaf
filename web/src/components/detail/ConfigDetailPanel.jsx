import { useState, useEffect, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall } from '../../api/client.js';
import { useToast } from '../common/Toast.jsx';

var AGENTS_FALLBACK = ['claude', 'codex', 'gemini', 'vibe', 'opencode', 'generic'];
var SPEED_OPTIONS = ['', 'fast', 'medium', 'slow'];
var STYLE_PRESETS = ['', 'manager', 'parallel', 'scout', 'sequential'];
var ROLES = ['developer', 'lead-developer', 'manager', 'supervisor', 'ui-designer', 'qa', 'backend-designer'];

var inputStyle = {
  width: '100%', padding: '6px 10px', background: 'var(--bg-2)',
  border: '1px solid var(--border)', borderRadius: 4, color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
};

var selectStyle = { ...inputStyle };

var labelStyle = {
  fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
  color: 'var(--text-3)', display: 'block', marginBottom: 4,
  textTransform: 'uppercase', letterSpacing: '0.05em',
};

var sectionStyle = {
  padding: '12px 0', borderBottom: '1px solid var(--border)',
};

var btnStyle = {
  padding: '6px 14px', border: '1px solid var(--border)', background: 'var(--bg-3)',
  color: 'var(--text-1)', borderRadius: 4, cursor: 'pointer',
  fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
};

var btnPrimaryStyle = {
  ...btnStyle,
  border: '1px solid var(--accent)', background: 'var(--accent)',
  color: '#000', fontWeight: 600,
};

var btnDangerStyle = {
  ...btnStyle,
  border: '1px solid var(--red)40', background: 'transparent',
  color: 'var(--red)',
};

export default function ConfigDetailPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var showToast = useToast();
  var sel = state.configSelection;

  var [data, setData] = useState(null);
  var [profiles, setProfiles] = useState([]);
  var [agentsMeta, setAgentsMeta] = useState(null);
  var [saving, setSaving] = useState(false);

  // Fetch agents metadata once on mount.
  var fetchAgentsMeta = useCallback(function () {
    return apiCall('/api/config/agents', 'GET', null, { allow404: true })
      .then(function (res) { if (res) setAgentsMeta(res); })
      .catch(function () {});
  }, []);
  useEffect(function () { fetchAgentsMeta(); }, [fetchAgentsMeta]);

  // Load current item data
  var loadItem = useCallback(async function () {
    var profs = [];
    try { profs = (await apiCall('/api/config/profiles', 'GET', null, { allow404: true })) || []; } catch (_) {}
    setProfiles(profs);

    if (!sel) { setData(null); return; }

    if (sel.isNew) {
      if (sel.type === 'profile') setData({ name: '', agent: 'claude', model: '', reasoning_level: '', description: '', intelligence: 0, max_instances: 0, speed: '' });
      else if (sel.type === 'loop') setData({ name: '', steps: [emptyStep()] });
      else if (sel.type === 'standalone') setData({ name: '', profile: '', instructions: '', delegation: null });
      return;
    }

    try {
      if (sel.type === 'profile') {
        var allProfs = profs;
        var found = allProfs.find(function (p) { return p.name === sel.name; });
        setData(found ? { ...found } : null);
      } else if (sel.type === 'loop') {
        var allLoops = (await apiCall('/api/config/loops', 'GET', null, { allow404: true })) || [];
        var loop = allLoops.find(function (l) { return l.name === sel.name; });
        setData(loop ? deepCopy(loop) : null);
      } else if (sel.type === 'standalone') {
        var allSP = (await apiCall('/api/config/standalone-profiles', 'GET', null, { allow404: true })) || [];
        var sp = allSP.find(function (s) { return s.name === sel.name; });
        setData(sp ? deepCopy(sp) : null);
      }
    } catch (err) {
      if (!err.authRequired) console.error('Config load error:', err);
    }
  }, [sel && sel.type, sel && sel.name, sel && sel.isNew]);

  useEffect(function () { loadItem(); }, [loadItem]);

  if (!sel) {
    return (
      <div style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)' }}>
        <div style={{ textAlign: 'center' }}>
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 13, marginBottom: 8 }}>Select a config item</div>
          <div style={{ fontSize: 11, opacity: 0.6 }}>Choose a profile, loop, or standalone profile from the left panel</div>
        </div>
      </div>
    );
  }

  if (!data) {
    return <div style={{ padding: 20, color: 'var(--text-3)', textAlign: 'center' }}>Loading...</div>;
  }

  async function handleSave() {
    setSaving(true);
    try {
      if (sel.type === 'profile') {
        var pOut = { name: data.name, agent: data.agent };
        if (data.model) pOut.model = data.model;
        if (data.reasoning_level) pOut.reasoning_level = data.reasoning_level;
        if (data.description) pOut.description = data.description;
        if (data.intelligence) pOut.intelligence = Number(data.intelligence);
        if (data.max_instances) pOut.max_instances = Number(data.max_instances);
        if (data.speed) pOut.speed = data.speed;
        if (sel.isNew) {
          await apiCall('/api/config/profiles', 'POST', pOut);
        } else {
          await apiCall('/api/config/profiles/' + encodeURIComponent(data.name), 'PUT', pOut);
        }
      } else if (sel.type === 'loop') {
        var lOut = { name: data.name, steps: (data.steps || []).map(cleanStep) };
        if (sel.isNew) {
          await apiCall('/api/config/loops', 'POST', lOut);
        } else {
          await apiCall('/api/config/loops/' + encodeURIComponent(data.name), 'PUT', lOut);
        }
      } else if (sel.type === 'standalone') {
        var spOut = { name: data.name, profile: data.profile };
        if (data.instructions) spOut.instructions = data.instructions;
        if (data.delegation && data.delegation.profiles && data.delegation.profiles.length > 0) {
          spOut.delegation = cleanDelegation(data.delegation);
        }
        if (sel.isNew) {
          await apiCall('/api/config/standalone-profiles', 'POST', spOut);
        } else {
          await apiCall('/api/config/standalone-profiles/' + encodeURIComponent(data.name), 'PUT', spOut);
        }
      }
      showToast('Saved', 'success');
      if (sel.isNew) {
        dispatch({ type: 'SET_CONFIG_SELECTION', payload: { type: sel.type, name: data.name } });
      }
      if (window.__configReload) window.__configReload();
    } catch (err) {
      if (err.authRequired) return;
      showToast('Failed: ' + (err.message || err), 'error');
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!window.confirm('Delete "' + data.name + '"?')) return;
    try {
      var endpoint = sel.type === 'profile' ? '/api/config/profiles/' :
        sel.type === 'loop' ? '/api/config/loops/' :
        '/api/config/standalone-profiles/';
      await apiCall(endpoint + encodeURIComponent(data.name), 'DELETE');
      showToast('Deleted', 'success');
      dispatch({ type: 'SET_CONFIG_SELECTION', payload: null });
      if (window.__configReload) window.__configReload();
    } catch (err) {
      if (err.authRequired) return;
      showToast('Failed: ' + (err.message || err), 'error');
    }
  }

  function set(key, val) {
    setData(function (prev) { return { ...prev, [key]: val }; });
  }

  var typeLabel = sel.type === 'profile' ? 'Profile' : sel.type === 'loop' ? 'Loop' : 'Standalone Profile';

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      {/* Header */}
      <div style={{
        padding: '10px 20px', borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        background: 'var(--bg-1)', flexShrink: 0,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 9, padding: '2px 6px',
            borderRadius: 3, background: 'var(--accent)15', color: 'var(--accent)',
            textTransform: 'uppercase', fontWeight: 600,
          }}>{typeLabel}</span>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 600, color: 'var(--text-0)' }}>
            {sel.isNew ? 'New ' + typeLabel : data.name}
          </span>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          {!sel.isNew && <button onClick={handleDelete} style={btnDangerStyle}>Delete</button>}
          <button onClick={handleSave} disabled={saving} style={{ ...btnPrimaryStyle, opacity: saving ? 0.6 : 1 }}>
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      {/* Body */}
      <div style={{ flex: 1, overflow: 'auto', padding: '16px 20px' }}>
        {sel.type === 'profile' && <ProfileEditor data={data} set={set} setData={setData} isNew={sel.isNew} agentsMeta={agentsMeta} onRefreshAgents={fetchAgentsMeta} showToast={showToast} />}
        {sel.type === 'loop' && <LoopEditor data={data} setData={setData} profiles={profiles} isNew={sel.isNew} />}
        {sel.type === 'standalone' && <StandaloneEditor data={data} set={set} setData={setData} profiles={profiles} isNew={sel.isNew} />}
      </div>
    </div>
  );
}

// ── Profile Editor ──

function ProfileEditor({ data, set, setData, isNew, agentsMeta, onRefreshAgents, showToast }) {
  var [detecting, setDetecting] = useState(false);
  var agentNames = agentsMeta ? agentsMeta.map(function (a) { return a.name; }) : AGENTS_FALLBACK;
  var currentAgentMeta = agentsMeta ? agentsMeta.find(function (a) { return a.name === data.agent; }) : null;
  var models = currentAgentMeta ? (currentAgentMeta.supported_models || []) : [];
  var reasoningLevels = currentAgentMeta ? (currentAgentMeta.reasoning_levels || []) : [];

  function handleAgentChange(newAgent) {
    setData(function (prev) { return { ...prev, agent: newAgent, model: '', reasoning_level: '' }; });
  }

  function handleRedetect() {
    setDetecting(true);
    apiCall('/api/config/agents/detect', 'POST')
      .then(function () {
        return onRefreshAgents();
      })
      .then(function () {
        showToast('Detection complete', 'success');
      })
      .catch(function (err) {
        if (!err.authRequired) showToast('Detection failed: ' + (err.message || err), 'error');
      })
      .finally(function () { setDetecting(false); });
  }

  return (
    <div style={{ maxWidth: 600, display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Field label="Name" value={data.name} onChange={function (v) { set('name', v); }} disabled={!isNew} placeholder="my-profile" />
      <div>
        <label style={labelStyle}>Agent</label>
        <select value={data.agent || ''} onChange={function (e) { handleAgentChange(e.target.value); }} style={selectStyle}>
          {agentNames.map(function (a) { return <option key={a} value={a}>{a}</option>; })}
        </select>
      </div>
      <div>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 4 }}>
          <label style={{ ...labelStyle, marginBottom: 0 }}>Model (optional)</label>
          <button onClick={handleRedetect} disabled={detecting} style={{ ...btnStyle, fontSize: 9, padding: '2px 8px', opacity: detecting ? 0.6 : 1 }}>
            {detecting ? 'Scanning...' : 'Re-detect models'}
          </button>
        </div>
        {models.length > 0 ? (
          <select value={data.model || ''} onChange={function (e) { set('model', e.target.value); }} style={selectStyle}>
            <option value="">Agent default{currentAgentMeta && currentAgentMeta.default_model ? ' (' + currentAgentMeta.default_model + ')' : ''}</option>
            {models.map(function (m) { return <option key={m} value={m}>{m}</option>; })}
          </select>
        ) : (
          <input
            type="text"
            value={data.model || ''}
            onChange={function (e) { set('model', e.target.value); }}
            placeholder="e.g. model-name"
            style={inputStyle}
          />
        )}
      </div>
      {reasoningLevels.length > 0 && (
        <div>
          <label style={labelStyle}>Reasoning Level</label>
          <select value={data.reasoning_level || ''} onChange={function (e) { set('reasoning_level', e.target.value); }} style={selectStyle}>
            <option value="">Default</option>
            {reasoningLevels.map(function (l) { return <option key={l.name} value={l.name}>{l.name}</option>; })}
          </select>
        </div>
      )}
      <div>
        <label style={labelStyle}>Speed</label>
        <select value={data.speed || ''} onChange={function (e) { set('speed', e.target.value); }} style={selectStyle}>
          <option value="">Not set</option>
          {SPEED_OPTIONS.filter(Boolean).map(function (s) { return <option key={s} value={s}>{s}</option>; })}
        </select>
      </div>
      <Field label="Intelligence (1-10, 0=unset)" value={data.intelligence || 0} type="number" onChange={function (v) { set('intelligence', Number(v)); }} />
      <Field label="Max Instances (0=unlimited)" value={data.max_instances || 0} type="number" onChange={function (v) { set('max_instances', Number(v)); }} />
      <div>
        <label style={labelStyle}>Description</label>
        <textarea value={data.description || ''} onChange={function (e) { set('description', e.target.value); }} style={{ ...inputStyle, minHeight: 80, resize: 'vertical' }} placeholder="Strengths, weaknesses, best use cases..." />
      </div>
    </div>
  );
}

// ── Loop Editor ──

function LoopEditor({ data, setData, profiles, isNew }) {
  function setName(val) {
    setData(function (prev) { return { ...prev, name: val }; });
  }

  function setStep(idx, key, val) {
    setData(function (prev) {
      var steps = prev.steps.map(function (s, i) {
        if (i !== idx) return s;
        return { ...s, [key]: val };
      });
      return { ...prev, steps: steps };
    });
  }

  function addStep() {
    setData(function (prev) { return { ...prev, steps: (prev.steps || []).concat([emptyStep()]) }; });
  }

  function removeStep(idx) {
    setData(function (prev) { return { ...prev, steps: prev.steps.filter(function (_, i) { return i !== idx; }) }; });
  }

  function setStepDelegation(idx, deleg) {
    setStep(idx, 'delegation', deleg);
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Field label="Loop Name" value={data.name} onChange={function (v) { setName(v); }} disabled={!isNew} placeholder="my-loop" />

      <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, fontWeight: 600, color: 'var(--text-1)' }}>
        Steps ({(data.steps || []).length})
      </div>

      {(data.steps || []).map(function (step, idx) {
        return (
          <div key={idx} style={{ padding: 16, border: '1px solid var(--border)', borderRadius: 6, background: 'var(--bg-1)' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
              <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--accent)' }}>
                Step {idx + 1}
              </span>
              {(data.steps || []).length > 1 && (
                <button onClick={function () { removeStep(idx); }} style={btnDangerStyle}>Remove Step</button>
              )}
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div>
                <label style={labelStyle}>Profile</label>
                <select value={step.profile || ''} onChange={function (e) { setStep(idx, 'profile', e.target.value); }} style={selectStyle}>
                  <option value="">Select profile</option>
                  {profiles.map(function (p) { return <option key={p.name} value={p.name}>{p.name} ({p.agent})</option>; })}
                </select>
              </div>
              <div style={{ display: 'flex', gap: 12 }}>
                <div style={{ flex: 1 }}>
                  <label style={labelStyle}>Role (optional)</label>
                  <select value={step.role || ''} onChange={function (e) { setStep(idx, 'role', e.target.value); }} style={selectStyle}>
                    <option value="">Default</option>
                    {ROLES.map(function (r) { return <option key={r} value={r}>{r}</option>; })}
                  </select>
                </div>
                <div style={{ width: 100 }}>
                  <label style={labelStyle}>Turns</label>
                  <input type="number" min="1" value={step.turns || 1} onChange={function (e) { setStep(idx, 'turns', parseInt(e.target.value, 10) || 1); }} style={inputStyle} />
                </div>
              </div>
              <div>
                <label style={labelStyle}>Instructions (optional)</label>
                <textarea value={step.instructions || ''} onChange={function (e) { setStep(idx, 'instructions', e.target.value); }} style={{ ...inputStyle, minHeight: 60, resize: 'vertical' }} placeholder="Step-specific instructions" />
              </div>
              <div style={{ display: 'flex', gap: 16 }}>
                <Checkbox label="can_stop" checked={!!step.can_stop} onChange={function (v) { setStep(idx, 'can_stop', v); }} />
                <Checkbox label="can_message" checked={!!step.can_message} onChange={function (v) { setStep(idx, 'can_message', v); }} />
                <Checkbox label="can_pushover" checked={!!step.can_pushover} onChange={function (v) { setStep(idx, 'can_pushover', v); }} />
              </div>

              {/* Delegation */}
              <DelegationEditor
                delegation={step.delegation}
                onChange={function (d) { setStepDelegation(idx, d); }}
                profiles={profiles}
                label={'Step ' + (idx + 1) + ' Sub-Agent Delegation'}
              />
            </div>
          </div>
        );
      })}

      <button onClick={addStep} style={{ ...btnStyle, alignSelf: 'flex-start' }}>+ Add Step</button>
    </div>
  );
}

// ── Standalone Editor ──

function StandaloneEditor({ data, set, setData, profiles, isNew }) {
  function setDelegation(deleg) {
    setData(function (prev) { return { ...prev, delegation: deleg }; });
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Field label="Name" value={data.name} onChange={function (v) { set('name', v); }} disabled={!isNew} placeholder="my-standalone" />
      <div>
        <label style={labelStyle}>Profile</label>
        <select value={data.profile || ''} onChange={function (e) { set('profile', e.target.value); }} style={selectStyle}>
          <option value="">Select profile</option>
          {profiles.map(function (p) { return <option key={p.name} value={p.name}>{p.name} ({p.agent})</option>; })}
        </select>
      </div>
      <div>
        <label style={labelStyle}>Instructions</label>
        <textarea value={data.instructions || ''} onChange={function (e) { set('instructions', e.target.value); }} style={{ ...inputStyle, minHeight: 120, resize: 'vertical' }} placeholder="Custom instructions for the standalone agent..." />
      </div>

      {/* Delegation */}
      <DelegationEditor
        delegation={data.delegation}
        onChange={setDelegation}
        profiles={profiles}
        label="Sub-Agent Delegation"
      />
    </div>
  );
}

// ── Delegation Editor (reusable) ──

function DelegationEditor({ delegation, onChange, profiles, label }) {
  var hasDeleg = delegation && delegation.profiles && delegation.profiles.length > 0;
  var [expanded, setExpanded] = useState(hasDeleg);

  function enable() {
    onChange({ profiles: [emptyDelegationProfile()], max_parallel: 4, style_preset: '', style: '' });
    setExpanded(true);
  }

  function disable() {
    onChange(null);
    setExpanded(false);
  }

  function setField(key, val) {
    onChange({ ...delegation, [key]: val });
  }

  function setProfile(idx, key, val) {
    var profs = (delegation.profiles || []).map(function (p, i) {
      if (i !== idx) return p;
      return { ...p, [key]: val };
    });
    onChange({ ...delegation, profiles: profs });
  }

  function addProfile() {
    onChange({ ...delegation, profiles: (delegation.profiles || []).concat([emptyDelegationProfile()]) });
  }

  function removeProfile(idx) {
    var profs = (delegation.profiles || []).filter(function (_, i) { return i !== idx; });
    onChange({ ...delegation, profiles: profs });
  }

  // Nested delegation for a sub-agent profile
  function setProfileDelegation(idx, childDeleg) {
    setProfile(idx, 'delegation', childDeleg);
  }

  return (
    <div style={{ border: '1px solid var(--border)', borderRadius: 6, overflow: 'hidden' }}>
      <div
        onClick={function () { if (delegation) setExpanded(!expanded); }}
        style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '8px 12px', background: 'var(--bg-2)', cursor: 'pointer',
        }}
      >
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: delegation ? 'var(--orange)' : 'var(--text-3)' }}>
          {delegation ? (expanded ? '\u25BE' : '\u25B8') : '\u25CB'} {label || 'Delegation'}
          {delegation && delegation.profiles ? ' (' + delegation.profiles.length + ' sub-agents)' : ''}
        </span>
        {!delegation ? (
          <button onClick={function (e) { e.stopPropagation(); enable(); }} style={{ ...btnStyle, fontSize: 10, padding: '2px 8px' }}>Enable</button>
        ) : (
          <button onClick={function (e) { e.stopPropagation(); disable(); }} style={{ ...btnDangerStyle, fontSize: 10, padding: '2px 8px' }}>Remove</button>
        )}
      </div>

      {delegation && expanded && (
        <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 12 }}>
          <div style={{ display: 'flex', gap: 12 }}>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>Max Parallel</label>
              <input type="number" min="1" value={delegation.max_parallel || 4} onChange={function (e) { setField('max_parallel', parseInt(e.target.value, 10) || 4); }} style={inputStyle} />
            </div>
            <div style={{ flex: 1 }}>
              <label style={labelStyle}>Style Preset</label>
              <select value={delegation.style_preset || ''} onChange={function (e) { setField('style_preset', e.target.value); }} style={selectStyle}>
                <option value="">None</option>
                {STYLE_PRESETS.filter(Boolean).map(function (s) { return <option key={s} value={s}>{s}</option>; })}
              </select>
            </div>
          </div>

          {!delegation.style_preset && (
            <div>
              <label style={labelStyle}>Custom Style (free-form)</label>
              <textarea value={delegation.style || ''} onChange={function (e) { setField('style', e.target.value); }} style={{ ...inputStyle, minHeight: 40, resize: 'vertical' }} placeholder="Custom delegation style guidance..." />
            </div>
          )}

          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--text-1)', marginTop: 4 }}>
            Sub-Agent Profiles
          </div>

          {(delegation.profiles || []).map(function (dp, idx) {
            return (
              <div key={idx} style={{ padding: 12, border: '1px solid var(--border)', borderRadius: 4, background: 'var(--bg-1)' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--orange)' }}>Sub-Agent {idx + 1}</span>
                  <button onClick={function () { removeProfile(idx); }} style={{ ...btnDangerStyle, fontSize: 9, padding: '1px 6px' }}>Remove</button>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <div>
                    <label style={labelStyle}>Profile</label>
                    <select value={dp.name || ''} onChange={function (e) { setProfile(idx, 'name', e.target.value); }} style={selectStyle}>
                      <option value="">Select profile</option>
                      {profiles.map(function (p) { return <option key={p.name} value={p.name}>{p.name} ({p.agent})</option>; })}
                    </select>
                  </div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <div style={{ flex: 1 }}>
                      <label style={labelStyle}>Role</label>
                      <select value={dp.role || ''} onChange={function (e) { setProfile(idx, 'role', e.target.value); }} style={selectStyle}>
                        <option value="">Default (developer)</option>
                        {ROLES.map(function (r) { return <option key={r} value={r}>{r}</option>; })}
                      </select>
                    </div>
                    <div style={{ width: 100 }}>
                      <label style={labelStyle}>Max Instances</label>
                      <input type="number" min="0" value={dp.max_instances || 0} onChange={function (e) { setProfile(idx, 'max_instances', parseInt(e.target.value, 10) || 0); }} style={inputStyle} />
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <div style={{ flex: 1 }}>
                      <label style={labelStyle}>Speed</label>
                      <select value={dp.speed || ''} onChange={function (e) { setProfile(idx, 'speed', e.target.value); }} style={selectStyle}>
                        <option value="">Not set</option>
                        {SPEED_OPTIONS.filter(Boolean).map(function (s) { return <option key={s} value={s}>{s}</option>; })}
                      </select>
                    </div>
                    <div style={{ flex: 1, display: 'flex', alignItems: 'flex-end', gap: 12, paddingBottom: 6 }}>
                      <Checkbox label="handoff" checked={!!dp.handoff} onChange={function (v) { setProfile(idx, 'handoff', v); }} />
                    </div>
                  </div>

                  {/* Nested delegation for this sub-agent */}
                  <DelegationEditor
                    delegation={dp.delegation}
                    onChange={function (d) { setProfileDelegation(idx, d); }}
                    profiles={profiles}
                    label={'Sub-Agent ' + (idx + 1) + ' Child Delegation'}
                  />
                </div>
              </div>
            );
          })}

          <button onClick={addProfile} style={{ ...btnStyle, fontSize: 10, alignSelf: 'flex-start' }}>+ Add Sub-Agent</button>
        </div>
      )}
    </div>
  );
}

// ── Shared components ──

function Field({ label, value, onChange, disabled, placeholder, type }) {
  return (
    <div>
      <label style={labelStyle}>{label}</label>
      <input
        type={type || 'text'}
        value={value || ''}
        onChange={function (e) { onChange(e.target.value); }}
        disabled={disabled}
        placeholder={placeholder}
        style={{ ...inputStyle, opacity: disabled ? 0.5 : 1 }}
      />
    </div>
  );
}

function Checkbox({ label, checked, onChange }) {
  return (
    <label style={{
      fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
      color: 'var(--text-2)', display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer',
    }}>
      <input type="checkbox" checked={checked} onChange={function (e) { onChange(e.target.checked); }} />
      {label}
    </label>
  );
}

// ── Helpers ──

function emptyStep() {
  return { profile: '', role: '', turns: 1, instructions: '', can_stop: false, can_message: false, can_pushover: false, delegation: null };
}

function emptyDelegationProfile() {
  return { name: '', role: '', max_instances: 0, speed: '', handoff: false, delegation: null };
}

function cleanStep(s) {
  var out = { profile: s.profile };
  if (s.role) out.role = s.role;
  if (s.turns && s.turns > 0) out.turns = Number(s.turns);
  if (s.instructions) out.instructions = s.instructions;
  if (s.can_stop) out.can_stop = true;
  if (s.can_message) out.can_message = true;
  if (s.can_pushover) out.can_pushover = true;
  if (s.delegation && s.delegation.profiles && s.delegation.profiles.length > 0) {
    out.delegation = cleanDelegation(s.delegation);
  }
  return out;
}

function cleanDelegation(d) {
  if (!d) return null;
  var out = {};
  if (d.max_parallel) out.max_parallel = Number(d.max_parallel);
  if (d.style_preset) out.style_preset = d.style_preset;
  else if (d.style) out.style = d.style;
  out.profiles = (d.profiles || []).map(function (p) {
    var dp = { name: p.name };
    if (p.role) dp.role = p.role;
    if (p.max_instances) dp.max_instances = Number(p.max_instances);
    if (p.speed) dp.speed = p.speed;
    if (p.handoff) dp.handoff = true;
    if (p.delegation && p.delegation.profiles && p.delegation.profiles.length > 0) {
      dp.delegation = cleanDelegation(p.delegation);
    }
    return dp;
  });
  return out;
}

function deepCopy(obj) {
  return JSON.parse(JSON.stringify(obj));
}
