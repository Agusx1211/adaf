import { useEffect, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiBase, apiCall } from '../../api/client.js';
import { MarkdownContent, injectEventBlockStyles } from '../common/EventBlocks.jsx';
import SectionHeader from '../common/SectionHeader.jsx';
import { useToast } from '../common/Toast.jsx';

var planStatusOptions = ['active', 'done', 'cancelled', 'frozen'];

export default function PlanDetailPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var toast = useToast();
  var base = apiBase(state.currentProjectID);
  var plan = state.plans.find(function (item) { return item.id === state.selectedPlan; });

  if (!plan && state.activePlan) {
    plan = state.activePlan;
  }

  var [isEditing, setIsEditing] = useState(false);
  var [saving, setSaving] = useState(false);
  var [editTitle, setEditTitle] = useState('');
  var [editStatus, setEditStatus] = useState('active');
  var [editDescription, setEditDescription] = useState('');

  useEffect(function () {
    injectEventBlockStyles();
  }, []);

  useEffect(function () {
    if (!plan) {
      setIsEditing(false);
      setEditTitle('');
      setEditStatus('active');
      setEditDescription('');
      return;
    }
    if (!isEditing) {
      setEditTitle(plan.title || '');
      setEditStatus(plan.status || 'active');
      setEditDescription(plan.description || '');
    }
  }, [plan && plan.id]);

  if (!plan) {
    return (
      <div style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)' }}>
        Select a plan
      </div>
    );
  }

  function buildMarkdown(data) {
    return (
      '# ' + (data.title || data.id || 'Plan') + '\n\n' +
      '**Plan ID:** ' + data.id + '  \n' +
      '**Status:** ' + data.status + '  \n\n' +
      (data.description || '_No description yet._')
    );
  }

  function beginEdit() {
    setEditTitle(plan.title || '');
    setEditStatus(plan.status || 'active');
    setEditDescription(plan.description || '');
    setIsEditing(true);
  }

  function cancelEdit() {
    setEditTitle(plan.title || '');
    setEditStatus(plan.status || 'active');
    setEditDescription(plan.description || '');
    setIsEditing(false);
  }

  async function savePlan() {
    setSaving(true);

    var payload = {
      title: editTitle.trim() || plan.title,
      status: editStatus,
      description: editDescription,
    };

    try {
      var updated = await apiCall(base + '/plans/' + encodeURIComponent(plan.id), 'PUT', payload);
      var nextPlans = state.plans.map(function (item) { return item.id === updated.id ? updated : item; });
      dispatch({ type: 'SET', payload: { plans: nextPlans, activePlan: state.activePlan && state.activePlan.id === updated.id ? updated : state.activePlan } });
      setIsEditing(false);
      toast('Plan updated', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to save plan: ' + (err.message || err), 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column', animation: 'fadeIn 0.2s ease' }}>
      <SectionHeader
        action={(
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            {isEditing ? (
              <>
                <button
                  onClick={cancelEdit}
                  disabled={saving}
                  style={{
                    background: 'transparent', color: 'var(--text-3)', border: '1px solid var(--border)',
                    borderRadius: 4, padding: '4px 8px', fontFamily: "'JetBrains Mono', monospace",
                    fontSize: 10, cursor: saving ? 'not-allowed' : 'pointer',
                  }}
                >Cancel</button>
                <button
                  onClick={savePlan}
                  disabled={saving}
                  style={{
                    background: saving ? 'var(--bg-3)' : 'var(--green)', color: '#000',
                    border: '1px solid transparent', borderRadius: 4, padding: '4px 8px',
                    fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                    fontWeight: 600, cursor: saving ? 'not-allowed' : 'pointer',
                  }}
                >{saving ? 'Saving...' : 'Save'}</button>
              </>
            ) : (
              <button
                onClick={beginEdit}
                style={{
                  background: 'transparent', color: 'var(--accent)', border: '1px solid var(--accent)40',
                  borderRadius: 4, padding: '4px 8px', fontFamily: "'JetBrains Mono', monospace",
                  fontSize: 10, cursor: 'pointer',
                }}
              >Edit</button>
            )}
          </div>
        )}
      >
        {'Plan: ' + (plan.title || plan.id)}
      </SectionHeader>

      <div style={{ flex: 1, overflow: 'auto', padding: '0 0 16px 0' }}>
        <div style={{ padding: '12px 16px' }}>
          {isEditing ? (
            <div style={{ display: 'grid', gap: 10 }}>
              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Title</label>
              <input
                value={editTitle}
                onChange={function (event) { setEditTitle(event.target.value); }}
                style={inputStyle}
                placeholder="Plan title"
              />

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div>
                  <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Status</label>
                  <select value={editStatus} onChange={function (event) { setEditStatus(event.target.value); }} style={selectStyle}>
                    {planStatusOptions.map(function (status) {
                      return <option key={status} value={status}>{status}</option>;
                    })}
                  </select>
                </div>
                <div>
                  <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Plan ID</label>
                  <input value={plan.id || ''} disabled style={inputStyleDisabled} />
                </div>
              </div>

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Description (Markdown)</label>
              <textarea
                value={editDescription}
                onChange={function (event) { setEditDescription(event.target.value); }}
                style={textareaStyle}
                rows={14}
              />
            </div>
          ) : (
            <MarkdownContent text={buildMarkdown(plan)} style={markdownContentStyle} />
          )}
        </div>
      </div>
    </div>
  );
}

var inputStyle = {
  width: '100%',
  padding: '7px 10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-3)',
  color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 12,
  outline: 'none',
};

var inputStyleDisabled = {
  width: '100%',
  padding: '7px 10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-0)',
  color: 'var(--text-3)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 12,
  outline: 'none',
};

var selectStyle = {
  width: '100%',
  padding: '7px 10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-3)',
  color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 12,
};

var textareaStyle = {
  width: '100%',
  padding: '10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-0)',
  color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 12,
  lineHeight: 1.5,
  resize: 'vertical',
  minHeight: 200,
};

var markdownContentStyle = {
  width: '100%',
  margin: 0,
  padding: 0,
};
