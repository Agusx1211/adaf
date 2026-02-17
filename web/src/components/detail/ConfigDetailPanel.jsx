import { useState, useEffect, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall } from '../../api/client.js';
import { useToast } from '../common/Toast.jsx';

var AGENTS_FALLBACK = ['claude', 'codex', 'gemini', 'vibe', 'opencode', 'generic'];
var SPEED_OPTIONS = ['', 'fast', 'medium', 'slow'];
var STYLE_PRESETS = ['', 'manager', 'parallel', 'scout', 'sequential'];
var DEFAULT_ROLE_NAMES = ['developer', 'ui-designer', 'qa', 'backend-designer', 'documentator', 'reviewer', 'scout', 'researcher'];
var LOOP_STEP_POSITIONS = ['lead', 'manager', 'supervisor'];
var TEAM_SUBAGENT_PREVIEW_TASK = 'Preview task: implement the delegated sub-task and report clear results back to the parent agent.';

var inputStyle = {
  width: '100%', padding: '6px 10px', background: 'var(--bg-2)',
  border: '1px solid var(--border)', borderRadius: 4, color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
};

var selectStyle = { ...inputStyle };

var labelStyle = {
  fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
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
  var [teams, setTeams] = useState([]);
  var [skills, setSkills] = useState([]);
  var [roleDefs, setRoleDefs] = useState([]);
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

    var teamsList = [];
    try { teamsList = (await apiCall('/api/config/teams', 'GET', null, { allow404: true })) || []; } catch (_) {}
    setTeams(teamsList);

    var skillsList = [];
    try { skillsList = (await apiCall('/api/config/skills', 'GET', null, { allow404: true })) || []; } catch (_) {}
    setSkills(skillsList);

    var rolesList = [];
    try { rolesList = (await apiCall('/api/config/roles', 'GET', null, { allow404: true })) || []; } catch (_) {}
    setRoleDefs(rolesList);

    if (!sel) { setData(null); return; }

    if (sel.isNew) {
      if (sel.type === 'profile') setData({ name: '', agent: 'claude', model: '', reasoning_level: '', description: '', intelligence: 0, max_instances: 0, speed: '' });
      else if (sel.type === 'loop') setData({ name: '', steps: [emptyStep()] });
      else if (sel.type === 'team') setData({ name: '', description: '', delegation: null });
      else if (sel.type === 'skill') setData({ id: '', short: '', long: '' });
      else if (sel.type === 'role') setData({ name: '', title: '', description: '', identity: '', can_write_code: true, rule_ids: [] });
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
        setData(loop ? normalizeLoopConfig(loop) : null);
      } else if (sel.type === 'team') {
        var t = teamsList.find(function (t) { return t.name === sel.name; });
        setData(t ? deepCopy(t) : null);
      } else if (sel.type === 'skill') {
        var sk = skillsList.find(function (s) { return s.id === sel.name; });
        setData(sk ? deepCopy(sk) : null);
      } else if (sel.type === 'role') {
        var rl = rolesList.find(function (r) { return r.name === sel.name; });
        setData(rl ? deepCopy(rl) : null);
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
          <div style={{ fontSize: 11, opacity: 0.6 }}>Choose a profile, loop, team, or skill from the left panel</div>
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
      } else if (sel.type === 'team') {
        var tOut = { name: data.name };
        if (data.description) tOut.description = data.description;
        if (data.delegation && data.delegation.profiles && data.delegation.profiles.length > 0) {
          tOut.delegation = cleanDelegation(data.delegation);
        }
        if (sel.isNew) {
          await apiCall('/api/config/teams', 'POST', tOut);
        } else {
          await apiCall('/api/config/teams/' + encodeURIComponent(data.name), 'PUT', tOut);
        }
      } else if (sel.type === 'skill') {
        var skOut = { id: data.id, short: data.short || '' };
        if (data.long) skOut.long = data.long;
        if (sel.isNew) {
          await apiCall('/api/config/skills', 'POST', skOut);
        } else {
          await apiCall('/api/config/skills/' + encodeURIComponent(data.id), 'PUT', skOut);
        }
      } else if (sel.type === 'role') {
        var rlOut = { name: data.name, can_write_code: !!data.can_write_code };
        if (data.title) rlOut.title = data.title;
        if (data.description) rlOut.description = data.description;
        if (data.identity) rlOut.identity = data.identity;
        if (data.rule_ids && data.rule_ids.length > 0) rlOut.rule_ids = data.rule_ids;
        if (sel.isNew) {
          await apiCall('/api/config/roles', 'POST', rlOut);
        } else {
          await apiCall('/api/config/roles/' + encodeURIComponent(data.name), 'PUT', rlOut);
        }
      }
      showToast('Saved', 'success');
      if (sel.isNew) {
        var savedName = sel.type === 'skill' ? data.id : data.name;
        dispatch({ type: 'SET_CONFIG_SELECTION', payload: { type: sel.type, name: savedName } });
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
    var itemName = sel.type === 'skill' ? data.id : data.name;
    if (!window.confirm('Delete "' + itemName + '"?')) return;
    try {
      var endpoint = sel.type === 'profile' ? '/api/config/profiles/' :
        sel.type === 'loop' ? '/api/config/loops/' :
        sel.type === 'skill' ? '/api/config/skills/' :
        sel.type === 'role' ? '/api/config/roles/' :
        '/api/config/teams/';
      await apiCall(endpoint + encodeURIComponent(itemName), 'DELETE');
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

  var typeLabel = sel.type === 'profile' ? 'Profile' : sel.type === 'loop' ? 'Loop' : sel.type === 'skill' ? 'Skill' : sel.type === 'role' ? 'Role' : 'Team';
  var typeColor = sel.type === 'team' ? 'var(--green)' : sel.type === 'skill' ? 'var(--pink)' : sel.type === 'role' ? 'var(--orange)' : 'var(--accent)';

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
            borderRadius: 3,
            background: typeColor + '15',
            color: typeColor,
            textTransform: 'uppercase', fontWeight: 600,
          }}>{typeLabel}</span>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 13, fontWeight: 600, color: 'var(--text-0)' }}>
            {sel.isNew ? 'New ' + typeLabel : (sel.type === 'skill' ? data.id : data.name)}
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
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <div style={{ flex: 1, overflow: 'auto', padding: '16px 20px', display: 'flex', flexDirection: 'column' }}>
          {sel.type === 'profile' && <ProfileEditor data={data} set={set} setData={setData} isNew={sel.isNew} agentsMeta={agentsMeta} onRefreshAgents={fetchAgentsMeta} showToast={showToast} />}
          {sel.type === 'loop' && <LoopEditor data={data} setData={setData} profiles={profiles} teams={teams} skills={skills} isNew={sel.isNew} projectID={state.currentProjectID} />}
          {sel.type === 'team' && <TeamEditor data={data} set={set} setData={setData} profiles={profiles} skills={skills} roleDefs={roleDefs} isNew={sel.isNew} projectID={state.currentProjectID} />}
          {sel.type === 'skill' && <SkillEditor data={data} set={set} isNew={sel.isNew} />}
          {sel.type === 'role' && <RoleEditor data={data} set={set} isNew={sel.isNew} />}
        </div>
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

  var sectionDivider = { borderTop: '1px solid var(--border)', paddingTop: 16 };

  return (
    <div style={{ maxWidth: 600, display: 'flex', flexDirection: 'column', gap: 16 }}>
      <Field label="Name" value={data.name} onChange={function (v) { set('name', v); }} disabled={!isNew} placeholder="my-profile" />
      <div>
        <label style={labelStyle}>Agent</label>
        <select value={data.agent || ''} onChange={function (e) { handleAgentChange(e.target.value); }} style={selectStyle}>
          {agentNames.map(function (a) { return <option key={a} value={a}>{a}</option>; })}
        </select>
      </div>
      <div style={sectionDivider}>
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
      <div style={sectionDivider}>
        <label style={labelStyle}>Speed</label>
        <select value={data.speed || ''} onChange={function (e) { set('speed', e.target.value); }} style={selectStyle}>
          <option value="">Not set</option>
          {SPEED_OPTIONS.filter(Boolean).map(function (s) { return <option key={s} value={s}>{s}</option>; })}
        </select>
      </div>
      <Field label="Intelligence (1-10, 0=unset)" value={data.intelligence || 0} type="number" onChange={function (v) { set('intelligence', Number(v)); }} />
      <Field label="Max Instances (0=unlimited)" value={data.max_instances || 0} type="number" onChange={function (v) { set('max_instances', Number(v)); }} />
      <div style={sectionDivider}>
        <label style={labelStyle}>Description</label>
        <textarea value={data.description || ''} onChange={function (e) { set('description', e.target.value); }} style={{ ...inputStyle, minHeight: 100, resize: 'vertical' }} placeholder="Strengths, weaknesses, best use cases..." />
      </div>
    </div>
  );
}

// ── Skill Editor ──

function SkillEditor({ data, set, isNew }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, height: '100%' }}>
      <Field label="Skill ID" value={data.id} onChange={function (v) { set('id', v); }} disabled={!isNew} placeholder="my_skill" />
      <div>
        <label style={labelStyle}>Short (embedded in prompt)</label>
        <textarea value={data.short || ''} onChange={function (e) { set('short', e.target.value); }} style={{ ...inputStyle, minHeight: 80, resize: 'vertical' }} placeholder="Concise instruction (1-4 sentences) for prompt embedding..." />
      </div>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 200 }}>
        <label style={labelStyle}>Long (full documentation, shown via `adaf skill`)</label>
        <textarea value={data.long || ''} onChange={function (e) { set('long', e.target.value); }} style={{ ...inputStyle, flex: 1, resize: 'vertical' }} placeholder="Full documentation in Markdown..." />
      </div>
    </div>
  );
}

// ── Role Editor ──

function RoleEditor({ data, set, isNew }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, height: '100%' }}>
      <Field label="Role Name" value={data.name} onChange={function (v) { set('name', v); }} disabled={!isNew} placeholder="my-role" />
      <Field label="Title" value={data.title || ''} onChange={function (v) { set('title', v); }} placeholder="ROLE TITLE (uppercase)" />
      <div>
        <label style={labelStyle}>Description</label>
        <textarea value={data.description || ''} onChange={function (e) { set('description', e.target.value); }} style={{ ...inputStyle, minHeight: 80, resize: 'vertical' }} placeholder="What this role does..." />
      </div>
      <Checkbox label="can_write_code" checked={data.can_write_code !== false} onChange={function (v) { set('can_write_code', v); }} />
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 200 }}>
        <label style={labelStyle}>Identity Prompt</label>
        <textarea value={data.identity || ''} onChange={function (e) { set('identity', e.target.value); }} style={{ ...inputStyle, flex: 1, resize: 'vertical' }} placeholder="System prompt / identity for this role..." />
      </div>
    </div>
  );
}

// ── Loop Editor ──

function LoopEditor({ data, setData, profiles, teams, skills, isNew, projectID }) {
  var [previewStepIndex, setPreviewStepIndex] = useState(0);
  var [previewScenarioID, setPreviewScenarioID] = useState('fresh_turn');
  var [promptPreview, setPromptPreview] = useState({ loading: false, error: '', data: null });
  var [previewItem, setPreviewItem] = useState(null);

  useEffect(function () {
    var stepCount = (data.steps || []).length;
    if (stepCount <= 0) {
      if (previewStepIndex !== 0) setPreviewStepIndex(0);
      return;
    }
    if (previewStepIndex >= stepCount) {
      setPreviewStepIndex(stepCount - 1);
    }
  }, [data.steps, previewStepIndex]);

  useEffect(function () {
    var steps = data.steps || [];
    if (!steps.length) {
      setPromptPreview({ loading: false, error: 'Add a step to preview prompts.', data: null });
      return;
    }
    if (previewStepIndex < 0 || previewStepIndex >= steps.length) return;

    var step = steps[previewStepIndex];
    if (!step || !String(step.profile || '').trim()) {
      setPromptPreview({ loading: false, error: 'Select a profile for this step to preview prompts.', data: null });
      return;
    }

    var cancelled = false;
    setPromptPreview(function (prev) { return { ...prev, loading: true, error: '' }; });

    var timer = setTimeout(function () {
      apiCall('/api/config/loops/prompt-preview', 'POST', {
        project_id: projectID || '',
        loop: {
          name: data.name || '',
          steps: steps.map(cleanStep),
        },
        step_index: previewStepIndex,
      })
        .then(function (resp) {
          if (cancelled) return;
          setPromptPreview({ loading: false, error: '', data: resp || null });
          var scenarios = resp && Array.isArray(resp.scenarios) ? resp.scenarios : [];
          setPreviewScenarioID(function (prev) {
            if (!scenarios.length) return '';
            return scenarios.some(function (s) { return s.id === prev; }) ? prev : scenarios[0].id;
          });
        })
        .catch(function (err) {
          if (cancelled || (err && err.authRequired)) return;
          setPromptPreview({ loading: false, error: err.message || String(err), data: null });
        });
    }, 180);

    return function () {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [data, previewStepIndex, projectID]);

  function setName(val) {
    setData(function (prev) { return { ...prev, name: val }; });
  }

  function setStep(idx, key, val) {
    setData(function (prev) {
      var steps = prev.steps.map(function (s, i) {
        if (i !== idx) return s;
        var next = { ...s, [key]: val };
        if (key === 'position' && val === 'supervisor') {
          next.team = '';
        }
        return next;
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

  return (
    <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', alignItems: 'flex-start' }}>
      <div style={{ flex: '999 1 560px', minWidth: 300, display: 'flex', flexDirection: 'column', gap: 16 }}>
        <Field label="Loop Name" value={data.name} onChange={function (v) { setName(v); }} disabled={!isNew} placeholder="my-loop" />

        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, fontWeight: 600, color: 'var(--text-1)' }}>
          Steps ({(data.steps || []).length})
        </div>

        {(data.steps || []).map(function (step, idx) {
          var previewing = idx === previewStepIndex;
          return (
            <div key={idx} style={{ padding: 16, border: previewing ? '1px solid var(--accent)55' : '1px solid var(--border)', borderRadius: 6, background: 'var(--bg-1)' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
                <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: previewing ? 'var(--accent)' : 'var(--text-1)' }}>
                  Step {idx + 1}
                </span>
                <div style={{ display: 'flex', gap: 8 }}>
                  <button onClick={function () { setPreviewStepIndex(idx); }} style={{ ...btnStyle, fontSize: 10, padding: '2px 8px', border: previewing ? '1px solid var(--accent)' : btnStyle.border, color: previewing ? '#000' : btnStyle.color, background: previewing ? 'var(--accent)' : btnStyle.background }}>
                    Preview
                  </button>
                  {(data.steps || []).length > 1 && (
                    <button onClick={function () { removeStep(idx); }} style={btnDangerStyle}>Remove Step</button>
                  )}
                </div>
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
                    <label style={labelStyle}>Position</label>
                    <select value={step.position || 'lead'} onChange={function (e) { setStep(idx, 'position', e.target.value); }} style={selectStyle}>
                      {LOOP_STEP_POSITIONS.map(function (p) { return <option key={p} value={p}>{p}</option>; })}
                    </select>
                  </div>
                  <div style={{ width: 100 }}>
                    <label style={labelStyle}>Turns</label>
                    <input type="number" min="1" value={step.turns || 1} onChange={function (e) { setStep(idx, 'turns', parseInt(e.target.value, 10) || 1); }} style={inputStyle} />
                  </div>
                </div>

                {/* Team dropdown */}
                <div>
                  <label style={labelStyle}>Team (optional)</label>
                  <select
                    value={step.team || ''}
                    onChange={function (e) { setStep(idx, 'team', e.target.value); }}
                    style={selectStyle}
                    disabled={step.position === 'supervisor'}
                  >
                    <option value="">No team</option>
                    {(teams || []).map(function (t) {
                      var subCount = t.delegation && t.delegation.profiles ? t.delegation.profiles.length : 0;
                      return <option key={t.name} value={t.name}>{t.name} ({subCount} sub-agents)</option>;
                    })}
                  </select>
                  {step.position === 'supervisor' && (
                    <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', marginTop: 4 }}>
                      Supervisor steps cannot have teams.
                    </div>
                  )}
                  {step.position === 'manager' && !step.team && (
                    <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', marginTop: 4 }}>
                      Manager steps require a team.
                    </div>
                  )}
                </div>

                {/* Skills picker */}
                <SkillsPicker
                  selected={step.skills || []}
                  available={skills}
                  onChange={function (v) {
                    setData(function (prev) {
                      var steps = (prev.steps || []).map(function (s, i) {
                        if (i !== idx) return s;
                        return { ...s, skills: v, skills_explicit: true };
                      });
                      return { ...prev, steps: steps };
                    });
                  }}
                  onPreview={setPreviewItem}
                />

                <div>
                  <label style={labelStyle}>Instructions (optional)</label>
                  <textarea value={step.instructions || ''} onChange={function (e) { setStep(idx, 'instructions', e.target.value); }} style={{ ...inputStyle, minHeight: 80, resize: 'vertical' }} placeholder="Step-specific instructions" />
                </div>
                <div style={{ display: 'grid', gap: 8 }}>
                  <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
                    Loop controls are built-in by position: supervisors can stop/message, managers can call supervisor.
                  </div>
                  <div style={{ display: 'flex', gap: 16 }}>
                    <Checkbox label="can_pushover" checked={!!step.can_pushover} onChange={function (v) { setStep(idx, 'can_pushover', v); }} />
                  </div>
                </div>
              </div>
            </div>
          );
        })}

        <button onClick={addStep} style={{ ...btnStyle, alignSelf: 'flex-start' }}>+ Add Step</button>
      </div>

      <div
        data-testid="loop-preview-rail"
        style={{
          flex: '1 1 360px',
          minWidth: 300,
          maxWidth: 520,
          position: 'sticky',
          top: 0,
          alignSelf: 'flex-start',
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
          maxHeight: 'calc(100vh - 210px)',
          overflow: 'hidden',
        }}
      >
        <HoverPreviewCard item={previewItem} testID="loop-hover-preview-card" maxHeight={200} />
        <LoopPromptPreviewPanel
          loopName={data.name}
          steps={data.steps || []}
          previewStepIndex={previewStepIndex}
          setPreviewStepIndex={setPreviewStepIndex}
          previewScenarioID={previewScenarioID}
          setPreviewScenarioID={setPreviewScenarioID}
          promptPreview={promptPreview}
        />
      </div>
    </div>
  );
}

function LoopPromptPreviewPanel({
  loopName,
  steps,
  previewStepIndex,
  setPreviewStepIndex,
  previewScenarioID,
  setPreviewScenarioID,
  promptPreview,
}) {
  var scenarios = promptPreview && promptPreview.data && Array.isArray(promptPreview.data.scenarios)
    ? promptPreview.data.scenarios
    : [];
  var activeScenario = scenarios.find(function (s) { return s.id === previewScenarioID; }) || (scenarios[0] || null);
  var runtimePath = promptPreview && promptPreview.data && promptPreview.data.runtime_path
    ? String(promptPreview.data.runtime_path)
    : '';

  return (
    <div data-testid="loop-prompt-preview-panel" style={{
      flex: 1,
      minHeight: 0,
      border: '1px solid var(--border)',
      borderRadius: 6,
      background: 'var(--bg-1)',
      display: 'flex',
      flexDirection: 'column',
      overflow: 'hidden',
    }}>
      <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, fontWeight: 600, color: 'var(--text-0)' }}>
          Prompt Preview
        </div>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
          This panel is generated by the same runtime prompt builders.
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <label style={{ ...labelStyle, marginBottom: 0 }}>Step</label>
          <select
            aria-label="Prompt preview step"
            value={String(previewStepIndex)}
            onChange={function (e) {
              setPreviewStepIndex(parseInt(e.target.value, 10) || 0);
            }}
            style={{ ...selectStyle, flex: 1, width: 'auto', minWidth: 0, padding: '4px 8px', fontSize: 11 }}
          >
            {(steps || []).map(function (step, idx) {
              var profileLabel = step && step.profile ? step.profile : 'no profile';
              var positionLabel = step && step.position ? String(step.position) : 'lead';
              return <option key={idx} value={idx}>Step {idx + 1}: {profileLabel} ({positionLabel})</option>;
            })}
          </select>
        </div>
        {!!loopName && (
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>
            Loop: {loopName}
          </div>
        )}
        {!!runtimePath && (
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
            Runtime path: {runtimePath}
          </div>
        )}
      </div>

      <div style={{ padding: 12, overflow: 'auto', minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
        {promptPreview.loading && (
          <div data-testid="loop-prompt-preview-loading" style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-3)' }}>
            Building prompt preview...
          </div>
        )}

        {!promptPreview.loading && promptPreview.error && (
          <div data-testid="loop-prompt-preview-error" style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--orange)', whiteSpace: 'pre-wrap' }}>
            {promptPreview.error}
          </div>
        )}

        {!promptPreview.loading && !promptPreview.error && scenarios.length > 0 && (
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {scenarios.map(function (scenario) {
              var selected = scenario.id === (activeScenario && activeScenario.id);
              return (
                <button
                  key={scenario.id}
                  type="button"
                  onClick={function () { setPreviewScenarioID(scenario.id); }}
                  style={{
                    ...btnStyle,
                    fontSize: 10,
                    padding: '3px 8px',
                    border: selected ? '1px solid var(--accent)' : '1px solid var(--border)',
                    color: selected ? '#000' : 'var(--text-1)',
                    background: selected ? 'var(--accent)' : 'var(--bg-2)',
                  }}
                >
                  {scenario.title}
                </button>
              );
            })}
          </div>
        )}

        {!promptPreview.loading && !promptPreview.error && activeScenario && (
          <>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)', lineHeight: 1.5 }}>
              {activeScenario.description}
            </div>
            <pre data-testid="loop-prompt-preview-body" style={{
              margin: 0,
              padding: '12px 14px',
              background: 'var(--bg-2)',
              border: '1px solid var(--border)',
              borderRadius: 4,
              color: 'var(--text-1)',
              fontFamily: "'JetBrains Mono', monospace",
              fontSize: 10,
              lineHeight: 1.5,
              whiteSpace: 'pre-wrap',
              overflowWrap: 'anywhere',
              wordBreak: 'break-word',
              overflow: 'auto',
              maxWidth: '100%',
              minHeight: 180,
            }}>
              {activeScenario.prompt || '(empty prompt)'}
            </pre>
          </>
        )}
      </div>
    </div>
  );
}

// ── Team Editor ──

function TeamEditor({ data, set, setData, profiles, skills, roleDefs, isNew, projectID }) {
  var [previewItem, setPreviewItem] = useState(null);
  var [previewChildProfile, setPreviewChildProfile] = useState('');
  var [previewChildRole, setPreviewChildRole] = useState('');
  var [previewScenarioID, setPreviewScenarioID] = useState('fresh_turn');
  var [promptPreview, setPromptPreview] = useState({ loading: false, error: '', data: null });

  var delegationProfiles = (data.delegation && Array.isArray(data.delegation.profiles)) ? data.delegation.profiles : [];

  useEffect(function () {
    if (!delegationProfiles.length) {
      if (previewChildProfile) setPreviewChildProfile('');
      return;
    }
    var names = delegationProfiles.map(function (dp) { return String(dp.name || '').trim(); }).filter(Boolean);
    if (!names.length) {
      if (previewChildProfile) setPreviewChildProfile('');
      return;
    }
    if (previewChildProfile && names.indexOf(previewChildProfile) >= 0) return;
    setPreviewChildProfile(names[0]);
  }, [delegationProfiles, previewChildProfile]);

  useEffect(function () {
    var selected = delegationProfiles.find(function (dp) { return String(dp.name || '').trim() === String(previewChildProfile || '').trim(); });
    var roleOptions = previewRoleOptionsForDelegationProfile(selected);
    if (previewChildRole && roleOptions.indexOf(previewChildRole) >= 0) return;
    setPreviewChildRole(roleOptions[0] || '');
  }, [delegationProfiles, previewChildProfile, previewChildRole]);

  useEffect(function () {
    if (!profiles || !profiles.length) {
      setPromptPreview({ loading: false, error: 'Create at least one profile to preview worker prompts.', data: null });
      return;
    }
    if (!delegationProfiles.length) {
      setPromptPreview({ loading: false, error: 'Enable delegation and add sub-agent profiles to preview worker prompts.', data: null });
      return;
    }
    if (!String(previewChildProfile || '').trim()) {
      setPromptPreview({ loading: false, error: 'Select a sub-agent profile to preview.', data: null });
      return;
    }

    var cancelled = false;
    setPromptPreview(function (prev) { return { ...prev, loading: true, error: '' }; });

    var timer = setTimeout(function () {
      var teamPayload = { name: data.name || '' };
      if (data.description) teamPayload.description = data.description;
      if (data.delegation) teamPayload.delegation = cleanDelegation(data.delegation);

      apiCall('/api/config/teams/prompt-preview', 'POST', {
        project_id: projectID || '',
        team: teamPayload,
        child_profile: previewChildProfile,
        child_role: previewChildRole || '',
        task: TEAM_SUBAGENT_PREVIEW_TASK,
      })
        .then(function (resp) {
          if (cancelled) return;
          setPromptPreview({ loading: false, error: '', data: resp || null });
          var scenarios = resp && Array.isArray(resp.scenarios) ? resp.scenarios : [];
          setPreviewScenarioID(function (prev) {
            if (!scenarios.length) return '';
            return scenarios.some(function (s) { return s.id === prev; }) ? prev : scenarios[0].id;
          });
        })
        .catch(function (err) {
          if (cancelled || (err && err.authRequired)) return;
          setPromptPreview({ loading: false, error: err.message || String(err), data: null });
        });
    }, 180);

    return function () {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [data, delegationProfiles, previewChildProfile, previewChildRole, projectID, profiles]);

  function setDelegation(deleg) {
    setData(function (prev) { return { ...prev, delegation: deleg }; });
  }

  return (
    <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', alignItems: 'flex-start' }}>
      <div style={{ flex: '999 1 560px', minWidth: 300, display: 'flex', flexDirection: 'column', gap: 16 }}>
        <Field label="Name" value={data.name} onChange={function (v) { set('name', v); }} disabled={!isNew} placeholder="my-team" />
        <div>
          <label style={labelStyle}>Description</label>
          <textarea value={data.description || ''} onChange={function (e) { set('description', e.target.value); }} style={{ ...inputStyle, minHeight: 80, resize: 'vertical' }} placeholder="What this team is good at..." />
        </div>

        {/* Delegation */}
        <DelegationEditor
          delegation={data.delegation}
          onChange={setDelegation}
          profiles={profiles}
          skills={skills}
          roleDefs={roleDefs}
          label="Team Sub-Agent Delegation"
          onPreview={setPreviewItem}
        />
      </div>
      <div style={{
        flex: '1 1 360px',
        minWidth: 300,
        maxWidth: 520,
        position: 'sticky',
        top: 0,
        alignSelf: 'flex-start',
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
        maxHeight: 'calc(100vh - 210px)',
        overflow: 'hidden',
      }}>
        <HoverPreviewCard item={previewItem} testID="team-hover-preview-card" maxHeight={200} />
        <TeamPromptPreviewPanel
          teamName={data.name}
          previewChildProfile={previewChildProfile}
          setPreviewChildProfile={setPreviewChildProfile}
          previewChildRole={previewChildRole}
          setPreviewChildRole={setPreviewChildRole}
          delegationProfiles={delegationProfiles}
          profiles={profiles}
          previewScenarioID={previewScenarioID}
          setPreviewScenarioID={setPreviewScenarioID}
          promptPreview={promptPreview}
        />
      </div>
    </div>
  );
}

function TeamPromptPreviewPanel({
  teamName,
  previewChildProfile,
  setPreviewChildProfile,
  previewChildRole,
  setPreviewChildRole,
  delegationProfiles,
  profiles,
  previewScenarioID,
  setPreviewScenarioID,
  promptPreview,
}) {
  var scenarios = promptPreview && promptPreview.data && Array.isArray(promptPreview.data.scenarios)
    ? promptPreview.data.scenarios
    : [];
  var activeScenario = scenarios.find(function (s) { return s.id === previewScenarioID; }) || (scenarios[0] || null);
  var runtimePath = promptPreview && promptPreview.data && promptPreview.data.runtime_path
    ? String(promptPreview.data.runtime_path)
    : '';
  var selectedDelegationProfile = (delegationProfiles || []).find(function (dp) { return String(dp.name || '').trim() === String(previewChildProfile || '').trim(); }) || null;
  var roleOptions = previewRoleOptionsForDelegationProfile(selectedDelegationProfile);

  return (
    <div data-testid="team-prompt-preview-panel" style={{
      flex: 1,
      minHeight: 0,
      border: '1px solid var(--border)',
      borderRadius: 6,
      background: 'var(--bg-1)',
      display: 'flex',
      flexDirection: 'column',
      overflow: 'hidden',
    }}>
      <div style={{ padding: '12px 14px', borderBottom: '1px solid var(--border)', display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 12, fontWeight: 600, color: 'var(--text-0)' }}>
          Prompt Preview
        </div>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
          Exact worker sub-agent prompt preview generated from runtime builders.
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <label style={{ ...labelStyle, marginBottom: 0 }}>Sub-Agent</label>
          <select
            aria-label="Team sub-agent preview profile"
            value={previewChildProfile}
            onChange={function (e) { setPreviewChildProfile(e.target.value); }}
            style={{ ...selectStyle, flex: 1, width: 'auto', minWidth: 0, padding: '4px 8px', fontSize: 11 }}
          >
            {(delegationProfiles || []).map(function (dp, idx) {
              var name = String(dp.name || '').trim() || ('sub-agent-' + (idx + 1));
              var profileMeta = (profiles || []).find(function (p) { return p.name === name; }) || null;
              return <option key={name + '-' + idx} value={name}>{name}{profileMeta ? ' (' + profileMeta.agent + ')' : ''}</option>;
            })}
          </select>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <label style={{ ...labelStyle, marginBottom: 0 }}>Role</label>
          <select
            aria-label="Team sub-agent preview role"
            value={previewChildRole}
            onChange={function (e) { setPreviewChildRole(e.target.value); }}
            style={{ ...selectStyle, flex: 1, width: 'auto', minWidth: 0, padding: '4px 8px', fontSize: 11 }}
          >
            {roleOptions.map(function (role) {
              return <option key={role || 'auto'} value={role}>{role || 'auto (default worker role)'}</option>;
            })}
          </select>
        </div>
        {!!teamName && (
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>
            Team: {teamName}
          </div>
        )}
        {!!previewChildProfile && (
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>
            Position: worker (sub-agents are always workers)
          </div>
        )}
        {!!runtimePath && (
          <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
            Runtime path: {runtimePath}
          </div>
        )}
      </div>

      <div style={{ padding: 12, overflow: 'auto', minHeight: 0, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
        {promptPreview.loading && (
          <div data-testid="team-prompt-preview-loading" style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-3)' }}>
            Building prompt preview...
          </div>
        )}

        {!promptPreview.loading && promptPreview.error && (
          <div data-testid="team-prompt-preview-error" style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--orange)', whiteSpace: 'pre-wrap' }}>
            {promptPreview.error}
          </div>
        )}

        {!promptPreview.loading && !promptPreview.error && scenarios.length > 0 && (
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            {scenarios.map(function (scenario) {
              var selected = scenario.id === (activeScenario && activeScenario.id);
              return (
                <button
                  key={scenario.id}
                  type="button"
                  onClick={function () { setPreviewScenarioID(scenario.id); }}
                  style={{
                    ...btnStyle,
                    fontSize: 10,
                    padding: '3px 8px',
                    border: selected ? '1px solid var(--accent)' : '1px solid var(--border)',
                    color: selected ? '#000' : 'var(--text-1)',
                    background: selected ? 'var(--accent)' : 'var(--bg-2)',
                  }}
                >
                  {scenario.title}
                </button>
              );
            })}
          </div>
        )}

        {!promptPreview.loading && !promptPreview.error && activeScenario && (
          <>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)', lineHeight: 1.5 }}>
              {activeScenario.description}
            </div>
            <pre data-testid="team-prompt-preview-body" style={{
              margin: 0,
              padding: '12px 14px',
              background: 'var(--bg-2)',
              border: '1px solid var(--border)',
              borderRadius: 4,
              color: 'var(--text-1)',
              fontFamily: "'JetBrains Mono', monospace",
              fontSize: 10,
              lineHeight: 1.5,
              whiteSpace: 'pre-wrap',
              overflowWrap: 'anywhere',
              wordBreak: 'break-word',
              overflow: 'auto',
              maxWidth: '100%',
              minHeight: 180,
            }}>
              {activeScenario.prompt || '(empty prompt)'}
            </pre>
          </>
        )}
      </div>
    </div>
  );
}

function previewRoleOptionsForDelegationProfile(dp) {
  if (!dp) return [''];
  var single = String(dp.role || '').trim();
  if (single) return [single];
  if (Array.isArray(dp.roles) && dp.roles.length > 0) {
    var seen = {};
    var out = [];
    dp.roles.forEach(function (rawRole) {
      var role = String(rawRole || '').trim();
      var key = role.toLowerCase();
      if (!role || seen[key]) return;
      seen[key] = true;
      out.push(role);
    });
    if (out.length > 0) return out;
  }
  return [''];
}

function HoverPreviewCard({ item, testID, maxHeight }) {
  var emptyTestID = (testID || 'hover-preview-card') + '-empty';
  var cardTestID = testID || 'hover-preview-card';
  if (!item) {
    return (
      <div data-testid={emptyTestID} style={{
        border: '1px solid var(--border)',
        borderRadius: 6,
        background: 'var(--bg-1)',
        padding: 12,
        flexShrink: 0,
      }}>
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-3)' }}>
          Hover a role or skill to preview it here while keeping prompt preview visible.
        </div>
      </div>
    );
  }

  var isRole = item.type === 'role';
  var badgeColor = isRole ? (ROLE_CHIP_COLORS[item.id] || 'var(--orange)') : 'var(--pink)';
  var badgeLabel = isRole ? 'ROLE' : 'SKILL';

  return (
    <div data-testid={cardTestID} style={{
      border: '1px solid var(--border)',
      borderRadius: 6,
      background: 'var(--bg-1)',
      display: 'flex',
      flexDirection: 'column',
      overflow: 'hidden',
      maxHeight: maxHeight || 260,
      flexShrink: 0,
    }}>
      <div style={{
        padding: '10px 12px',
        borderBottom: '1px solid var(--border)',
        display: 'flex',
        alignItems: 'center',
        gap: 8,
      }}>
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 9, padding: '2px 6px',
          borderRadius: 3, background: badgeColor + '15', color: badgeColor,
          textTransform: 'uppercase', fontWeight: 600, flexShrink: 0,
        }}>{badgeLabel}</span>
        <span style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
          color: 'var(--text-0)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{item.title || item.id}</span>
      </div>
      <div style={{ padding: 12, overflow: 'auto' }}>
        {isRole && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-1)', lineHeight: 1.5 }}>
              {item.description || 'No description available.'}
            </div>
            {item.identity && (
              <pre style={{
                margin: 0,
                padding: '8px 10px',
                background: 'var(--bg-2)',
                border: '1px solid var(--border)',
                borderRadius: 4,
                color: 'var(--text-2)',
                fontFamily: "'JetBrains Mono', monospace",
                fontSize: 9,
                lineHeight: 1.5,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}>
                {item.identity}
              </pre>
            )}
          </div>
        )}
        {!isRole && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {item.short && (
              <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-1)', lineHeight: 1.5 }}>
                {item.short}
              </div>
            )}
            {item.long && (
              <pre style={{
                margin: 0,
                padding: '8px 10px',
                background: 'var(--bg-2)',
                border: '1px solid var(--border)',
                borderRadius: 4,
                color: 'var(--text-2)',
                fontFamily: "'JetBrains Mono', monospace",
                fontSize: 9,
                lineHeight: 1.5,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}>
                {item.long}
              </pre>
            )}
            {!item.short && !item.long && (
              <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
                No description available.
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ── Role Picker (checkbox multi-select with descriptions) ──

var ROLE_DESCRIPTIONS = {
  'developer': 'Writes code, fixes bugs, implements features',
  'ui-designer': 'Focuses on UI/UX, frontend components, and visual design',
  'qa': 'Tests code, finds bugs, validates requirements',
  'backend-designer': 'Designs APIs, data models, and backend architecture',
  'documentator': 'Writes technical docs and handoff notes',
  'reviewer': 'Reviews changes for correctness and risks',
  'scout': 'Fast read-only investigation',
  'researcher': 'Deep option analysis and recommendations',
};

var ROLE_CHIP_COLORS = {
  'developer': 'var(--accent)',
  'ui-designer': '#7B8CFF',
  'qa': 'var(--green)',
  'backend-designer': '#5BCEFC',
  'documentator': '#F8C471',
  'reviewer': '#F39C12',
  'scout': '#7FDBB6',
  'researcher': '#85C1E9',
};

function RolePicker({ selected, onChange, roleDefs, onPreview }) {
  var selectedSet = {};
  (selected || []).forEach(function (v) { selectedSet[v] = true; });

  // Build a lookup from API role definitions
  var roleDefMap = {};
  (roleDefs || []).forEach(function (rd) { roleDefMap[rd.name] = rd; });
  var availableRoles = ((roleDefs || []).map(function (rd) { return rd.name; }).filter(function (name) { return !!name; }));
  if (!availableRoles.length) availableRoles = DEFAULT_ROLE_NAMES;

  function toggle(role) {
    if (selectedSet[role]) {
      onChange((selected || []).filter(function (v) { return v !== role; }));
    } else {
      onChange((selected || []).concat([role]));
    }
  }

  function handleRemove(val) {
    onChange((selected || []).filter(function (v) { return v !== val; }));
  }

  function handleHover(role) {
    if (!onPreview) return;
    var rd = roleDefMap[role];
    onPreview({
      type: 'role',
      id: role,
      title: rd ? rd.title : role,
      description: rd ? rd.description : (ROLE_DESCRIPTIONS[role] || ''),
      identity: rd ? rd.identity : '',
      can_write_code: rd ? rd.can_write_code : true,
      rule_ids: rd ? (rd.rule_ids || []) : [],
    });
  }

  return (
    <div>
      {/* Selected role chips */}
      {(selected || []).length > 0 ? (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginBottom: 8 }}>
          {(selected || []).map(function (val) {
            var chipColor = ROLE_CHIP_COLORS[val] || 'var(--text-2)';
            return (
              <span key={val} style={{
                display: 'inline-flex', alignItems: 'center', gap: 4,
                padding: '2px 8px', borderRadius: 10,
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                background: chipColor + '18', color: chipColor,
                border: '1px solid ' + chipColor + '35',
              }}>
                {val}
                <span
                  onClick={function () { handleRemove(val); }}
                  style={{ cursor: 'pointer', fontSize: 12, lineHeight: 1, opacity: 0.7 }}
                  onMouseEnter={function (e) { e.currentTarget.style.opacity = '1'; }}
                  onMouseLeave={function (e) { e.currentTarget.style.opacity = '0.7'; }}
                >{'\u00D7'}</span>
              </span>
            );
          })}
        </div>
      ) : (
        <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', marginBottom: 8 }}>
          Default (developer)
        </div>
      )}

      {/* Checkbox list of all roles */}
      <div style={{
        border: '1px solid var(--border)', borderRadius: 4, background: 'var(--bg-2)',
        padding: 4, overflow: 'auto',
      }}>
        {availableRoles.map(function (role) {
          var isChecked = !!selectedSet[role];
          var chipColor = ROLE_CHIP_COLORS[role] || 'var(--text-2)';
          var rd = roleDefMap[role];
          var desc = rd ? rd.description : (ROLE_DESCRIPTIONS[role] || '');
          return (
            <div key={role} style={{
              display: 'flex', alignItems: 'flex-start', gap: 6, padding: '5px 4px',
              borderRadius: 3,
            }}
              onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-3)'; handleHover(role); }}
              onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
            >
              <label style={{
                display: 'flex', alignItems: 'flex-start', gap: 6, cursor: 'pointer',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                color: isChecked ? 'var(--text-0)' : 'var(--text-2)',
                flex: 1, minWidth: 0,
              }}>
                <input type="checkbox" checked={isChecked} onChange={function () { toggle(role); }}
                  style={{ marginTop: 2, flexShrink: 0 }} />
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: isChecked ? 600 : 400, color: isChecked ? chipColor : undefined }}>{role}</div>
                  <div style={{ fontSize: 9, color: 'var(--text-3)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {desc}
                  </div>
                </div>
              </label>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ── Delegation Editor (reusable) ──

function DelegationEditor({ delegation, onChange, profiles, skills, roleDefs, label, isNested, onPreview }) {
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

  function setProfileMulti(idx, updates) {
    var profs = (delegation.profiles || []).map(function (p, i) {
      if (i !== idx) return p;
      var next = { ...p };
      Object.keys(updates).forEach(function (k) { next[k] = updates[k]; });
      return next;
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

  // Find profile metadata for display
  function profileMeta(name) {
    if (!name) return null;
    return profiles.find(function (p) { return p.name === name; });
  }

  var borderStyle = isNested ? '1px dashed var(--border)' : '1px solid var(--border)';

  return (
    <div style={{ border: borderStyle, borderRadius: 6, overflow: 'hidden' }}>
      <div
        onClick={function () { if (delegation) setExpanded(!expanded); }}
        style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '10px 14px', background: 'var(--bg-2)', cursor: delegation ? 'pointer' : 'default',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600,
            color: delegation ? 'var(--orange)' : 'var(--text-3)',
          }}>
            {label || 'Delegation'}
          </span>
          {delegation && delegation.profiles && (
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              padding: '1px 6px', borderRadius: 8,
              background: 'var(--orange)15', color: 'var(--orange)',
            }}>
              {delegation.profiles.length} sub-agent{delegation.profiles.length !== 1 ? 's' : ''}
            </span>
          )}
          {delegation && expanded && (
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>{'\u25BE'}</span>
          )}
          {delegation && !expanded && (
            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>{'\u25B8'}</span>
          )}
        </div>
        {!delegation ? (
          <button onClick={function (e) { e.stopPropagation(); enable(); }} style={{ ...btnStyle, fontSize: 10, padding: '3px 10px' }}>Enable</button>
        ) : (
          <button onClick={function (e) { e.stopPropagation(); disable(); }} style={{ ...btnDangerStyle, fontSize: 10, padding: '3px 10px' }}>Disable</button>
        )}
      </div>

      {delegation && expanded && (
        <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16 }}>
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
            var meta = profileMeta(dp.name);
            var agentName = meta ? meta.agent : '';
            var displayName = dp.name ? dp.name : 'Sub-Agent ' + (idx + 1);
            var advancedKey = 'adv_' + idx;

            return (
              <SubAgentCard
                key={idx}
                dp={dp}
                idx={idx}
                displayName={displayName}
                agentName={agentName}
                profiles={profiles}
                skills={skills}
                roleDefs={roleDefs}
                setProfile={setProfile}
                setProfileMulti={setProfileMulti}
                removeProfile={removeProfile}
                setProfileDelegation={setProfileDelegation}
                onPreview={onPreview}
              />
            );
          })}

          <button onClick={addProfile} style={{ ...btnStyle, fontSize: 10, alignSelf: 'flex-start' }}>+ Add Sub-Agent</button>
        </div>
      )}
    </div>
  );
}

// ── Sub-Agent Card (used inside DelegationEditor) ──

function SubAgentCard({ dp, idx, displayName, agentName, profiles, skills, roleDefs, setProfile, setProfileMulti, removeProfile, setProfileDelegation, onPreview }) {
  var [showAdvanced, setShowAdvanced] = useState(false);

  return (
    <div style={{ padding: 16, border: '1px solid var(--border)', borderRadius: 6, background: 'var(--bg-1)' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 600, color: 'var(--orange)' }}>
            {displayName}
          </span>
          {agentName && (
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              padding: '1px 6px', borderRadius: 8,
              background: 'var(--accent)15', color: 'var(--accent)',
            }}>{agentName}</span>
          )}
        </div>
        <button onClick={function () { removeProfile(idx); }} style={{ ...btnDangerStyle, fontSize: 9, padding: '2px 8px' }}>Remove</button>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {/* Essential: Profile + Roles always visible */}
        <div>
          <label style={labelStyle}>Profile</label>
          <select value={dp.name || ''} onChange={function (e) { setProfile(idx, 'name', e.target.value); }} style={selectStyle}>
            <option value="">Select profile</option>
            {profiles.map(function (p) { return <option key={p.name} value={p.name}>{p.name} ({p.agent})</option>; })}
          </select>
        </div>
        <div>
          <label style={labelStyle}>Roles</label>
          <RolePicker
            selected={Array.isArray(dp.roles) && dp.roles.length ? dp.roles : (dp.role ? [dp.role] : [])}
            onChange={function (roles) {
              setProfileMulti(idx, { roles: roles, role: '' });
            }}
            roleDefs={roleDefs}
            onPreview={onPreview}
          />
        </div>

        {/* Collapsible Advanced section */}
        <div
          onClick={function () { setShowAdvanced(!showAdvanced); }}
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            cursor: 'pointer', padding: '4px 0',
            borderTop: '1px solid var(--border)', marginTop: 2,
          }}
        >
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
            {showAdvanced ? '\u25BE' : '\u25B8'}
          </span>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', fontWeight: 500 }}>
            Advanced
          </span>
        </div>

        {showAdvanced && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingLeft: 4 }}>
            <div style={{ display: 'flex', gap: 8 }}>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Speed</label>
                <select value={dp.speed || ''} onChange={function (e) { setProfile(idx, 'speed', e.target.value); }} style={selectStyle}>
                  <option value="">Not set</option>
                  {SPEED_OPTIONS.filter(Boolean).map(function (s) { return <option key={s} value={s}>{s}</option>; })}
                </select>
              </div>
              <div style={{ width: 100 }}>
                <label style={labelStyle}>Max Instances</label>
                <input type="number" min="0" value={dp.max_instances || 0} onChange={function (e) { setProfile(idx, 'max_instances', parseInt(e.target.value, 10) || 0); }} style={inputStyle} />
              </div>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, paddingBottom: 4 }}>
              <Checkbox label="handoff" checked={!!dp.handoff} onChange={function (v) { setProfile(idx, 'handoff', v); }} />
            </div>
          </div>
        )}

        {/* Skills picker — always show pill summary */}
        <SkillsPicker
          selected={dp.skills || []}
          available={skills || []}
          onChange={function (v) { setProfile(idx, 'skills', v); }}
          onPreview={onPreview}
        />

        {/* Nested delegation for this sub-agent */}
        <DelegationEditor
          delegation={dp.delegation}
          onChange={function (d) { setProfileDelegation(idx, d); }}
          profiles={profiles}
          skills={skills}
          roleDefs={roleDefs}
          label={'Child Delegation'}
          isNested={true}
          onPreview={onPreview}
        />
      </div>
    </div>
  );
}

// ── Skills Picker (reusable multi-select) ──

function SkillsPicker({ selected, available, onChange, onPreview }) {
  var [isOpen, setIsOpen] = useState(false);
  var [filter, setFilter] = useState('');
  var selectedSet = {};
  (selected || []).forEach(function (id) { selectedSet[id] = true; });

  function toggle(id) {
    if (selectedSet[id]) {
      onChange((selected || []).filter(function (s) { return s !== id; }));
    } else {
      onChange((selected || []).concat([id]));
    }
  }

  function selectAll() {
    onChange((available || []).map(function (s) { return s.id; }));
  }

  function clearAll() {
    onChange([]);
  }

  var count = (selected || []).length;
  var skillMap = {};
  (available || []).forEach(function (sk) { if (sk && sk.id) skillMap[sk.id] = sk; });
  var filtered = (available || []).filter(function (sk) {
    if (!filter) return true;
    var q = filter.toLowerCase();
    return sk.id.toLowerCase().indexOf(q) >= 0 || (sk.short || '').toLowerCase().indexOf(q) >= 0;
  });

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 4 }}>
        <label style={{ ...labelStyle, marginBottom: 0 }}>Skills ({count} selected)</label>
        <div style={{ display: 'flex', gap: 4 }}>
          {count > 0 && (
            <button type="button" onClick={clearAll} style={{ ...btnStyle, fontSize: 9, padding: '1px 6px', color: 'var(--text-3)' }}>Clear</button>
          )}
        </div>
      </div>

      {/* Always show selected skill pills */}
      {count > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginBottom: 6 }}>
          {(selected || []).map(function (id) {
            var selectedSkill = skillMap[id] || null;
            return (
              <span key={id} style={{
                display: 'inline-flex', alignItems: 'center', gap: 3,
                padding: '2px 8px', borderRadius: 10, fontSize: 10,
                fontFamily: "'JetBrains Mono', monospace",
                background: 'var(--pink)15', color: 'var(--pink)',
                border: '1px solid var(--pink)30',
              }}
                onMouseEnter={function () {
                  if (!onPreview || !selectedSkill) return;
                  onPreview({ type: 'skill', id: selectedSkill.id, short: selectedSkill.short, long: selectedSkill.long });
                }}
              >
                {id}
                <span
                  onClick={function () { toggle(id); }}
                  style={{ cursor: 'pointer', fontSize: 11, lineHeight: 1, opacity: 0.7 }}
                  onMouseEnter={function (e) { e.currentTarget.style.opacity = '1'; }}
                  onMouseLeave={function (e) { e.currentTarget.style.opacity = '0.7'; }}
                >{'\u00D7'}</span>
              </span>
            );
          })}
        </div>
      )}

      {/* Toggle link */}
      <span
        onClick={function () { setIsOpen(!isOpen); setFilter(''); }}
        style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
          color: 'var(--accent)', cursor: 'pointer',
          textDecoration: 'none', opacity: 0.8,
        }}
        onMouseEnter={function (e) { e.currentTarget.style.opacity = '1'; }}
        onMouseLeave={function (e) { e.currentTarget.style.opacity = '0.8'; }}
      >
        {isOpen ? 'Close skills' : 'Edit skills\u2026'}
      </span>

      {/* Expanded picker */}
      {isOpen && (
        <div style={{
          border: '1px solid var(--border)', borderRadius: 4, background: 'var(--bg-2)',
          padding: 8, maxHeight: 280, overflow: 'auto', marginTop: 6,
        }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6, gap: 6 }}>
            {(available || []).length > 6 && (
              <input
                type="text"
                value={filter}
                onChange={function (e) { setFilter(e.target.value); }}
                placeholder="Filter skills\u2026"
                style={{ ...inputStyle, fontSize: 10, padding: '3px 8px', flex: 1 }}
              />
            )}
            <button type="button" onClick={selectAll} style={{ ...btnStyle, fontSize: 9, padding: '2px 8px', color: 'var(--text-3)', flexShrink: 0 }}>Select All</button>
          </div>
          {filtered.map(function (sk) {
            var isChecked = !!selectedSet[sk.id];
            var shortText = sk.short || '';
            if (shortText.length > 80) shortText = shortText.slice(0, 80) + '\u2026';
            return (
              <label key={sk.id} data-testid={'skills-option-' + sk.id} style={{
                display: 'flex', alignItems: 'flex-start', gap: 6, padding: '6px 4px',
                cursor: 'pointer', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                color: isChecked ? 'var(--text-0)' : 'var(--text-2)',
                borderRadius: 3,
              }}
                onMouseEnter={function (e) {
                  e.currentTarget.style.background = 'var(--bg-3)';
                  if (onPreview) onPreview({ type: 'skill', id: sk.id, short: sk.short, long: sk.long });
                }}
                onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
              >
                <input type="checkbox" checked={isChecked} onChange={function () { toggle(sk.id); }}
                  style={{ marginTop: 2, flexShrink: 0 }} />
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: isChecked ? 600 : 400 }}>{sk.id}</div>
                  <div style={{ fontSize: 9, color: 'var(--text-3)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {shortText}
                  </div>
                </div>
              </label>
            );
          })}
          {filtered.length === 0 && (available || []).length > 0 && (
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', textAlign: 'center', padding: 8 }}>
              No matching skills
            </div>
          )}
          {(available || []).length === 0 && (
            <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', textAlign: 'center', padding: 8 }}>
              No skills available
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── Shared components ──

function Field({ label, value, onChange, disabled, placeholder, type }) {
  var disabledStyle = disabled ? {
    opacity: 0.5, borderStyle: 'dashed', background: 'var(--bg-1)',
  } : {};
  return (
    <div>
      <label style={labelStyle}>{label}</label>
      <input
        type={type || 'text'}
        value={value || ''}
        onChange={function (e) { onChange(e.target.value); }}
        disabled={disabled}
        placeholder={placeholder}
        style={{ ...inputStyle, ...disabledStyle }}
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
  return { profile: '', position: 'lead', turns: 1, instructions: '', can_pushover: false, team: '', skills: [], skills_explicit: false };
}

function emptyDelegationProfile() {
  return { name: '', role: '', roles: [], max_instances: 0, speed: '', handoff: false, delegation: null, skills: [] };
}

function cleanStep(s) {
  var out = { profile: s.profile };
  if (s.position) out.position = s.position;
  if (s.turns && s.turns > 0) out.turns = Number(s.turns);
  if (s.instructions) out.instructions = s.instructions;
  if (s.can_pushover) out.can_pushover = true;
  if (s.team && s.position !== 'supervisor') out.team = s.team;
  if (s.skills_explicit) out.skills_explicit = true;
  if (s.skills && s.skills.length > 0) out.skills = s.skills;
  if (s.skills_explicit && (!s.skills || s.skills.length === 0)) out.skills = [];
  return out;
}

function normalizeLoopConfig(loop) {
  var copy = deepCopy(loop || {});
  var steps = Array.isArray(copy.steps) ? copy.steps : [];
  copy.steps = steps.map(function (step) {
    var normalized = {
      ...emptyStep(),
      ...step,
      position: step && step.position ? String(step.position) : 'lead',
      skills_explicit: !!(step && (step.skills_explicit || (Array.isArray(step.skills) && step.skills.length > 0))),
    };
    if (normalized.position === 'supervisor') {
      normalized.team = '';
    }
    return normalized;
  });
  if (!copy.steps.length) copy.steps = [emptyStep()];
  return copy;
}

function cleanDelegation(d) {
  if (!d) return null;
  var out = {};
  if (d.max_parallel) out.max_parallel = Number(d.max_parallel);
  if (d.style_preset) out.style_preset = d.style_preset;
  else if (d.style) out.style = d.style;
  out.profiles = (d.profiles || []).map(function (p) {
    var dp = { name: p.name };
    if (Array.isArray(p.roles) && p.roles.length) {
      dp.roles = p.roles.filter(function (r) { return r; });
    } else if (p.role) {
      dp.role = p.role;
    }
    if (p.max_instances) dp.max_instances = Number(p.max_instances);
    if (p.speed) dp.speed = p.speed;
    if (p.handoff) dp.handoff = true;
    if (p.skills && p.skills.length > 0) dp.skills = p.skills;
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
