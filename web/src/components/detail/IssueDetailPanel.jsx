import { useEffect, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiBase, apiCall } from '../../api/client.js';
import { MarkdownContent, injectEventBlockStyles } from '../common/EventBlocks.jsx';
import SectionHeader from '../common/SectionHeader.jsx';
import { useToast } from '../common/Toast.jsx';
import { normalizeStatus } from '../../utils/format.js';

var issueStatusOptions = ['open', 'in_progress', 'resolved', 'wontfix'];
var issuePriorityOptions = ['critical', 'high', 'medium', 'low'];

export default function IssueDetailPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var toast = useToast();
  var base = apiBase(state.currentProjectID);
  var issue = state.issues.find(function (item) { return item.id === state.selectedIssue; });

  var [isEditing, setIsEditing] = useState(false);
  var [saving, setSaving] = useState(false);
  var [editTitle, setEditTitle] = useState('');
  var [editPlanID, setEditPlanID] = useState('');
  var [editStatus, setEditStatus] = useState('open');
  var [editPriority, setEditPriority] = useState('medium');
  var [editLabels, setEditLabels] = useState('');
  var [editDependsOn, setEditDependsOn] = useState('');
  var [editDescription, setEditDescription] = useState('');

  useEffect(function () { injectEventBlockStyles(); }, []);

  useEffect(function () {
    if (!issue) {
      setIsEditing(false);
      setEditTitle('');
      setEditPlanID('');
      setEditStatus('open');
      setEditPriority('medium');
      setEditLabels('');
      setEditDependsOn('');
      setEditDescription('');
      return;
    }

    if (!isEditing) {
      setEditTitle(issue.title || '');
      setEditPlanID(issue.plan_id || '');
      setEditStatus(normalizeStatus(issue.status || 'open') || 'open');
      setEditPriority(normalizeStatus(issue.priority || 'medium') || 'medium');
      setEditLabels((issue.labels || []).join(', '));
      setEditDependsOn((issue.depends_on || []).join(', '));
      setEditDescription(issue.description || '');
    }
  }, [issue && issue.id]);

  if (!issue) {
    return <div style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)' }}>Select an issue</div>;
  }

  function buildMarkdown(data) {
    var labels = data.labels ? data.labels.join(', ') : '';
    var dependsOn = data.depends_on ? data.depends_on.join(', ') : '';
    return (
      '# Issue #' + (issue.id || '') + '\n\n' +
      '**Plan:** ' + (data.plan_id || 'shared') + '  \n' +
      '**Status:** ' + (data.status || 'open') + '  \n' +
      '**Priority:** ' + (data.priority || 'medium') + '  \n' +
      (labels ? ('**Labels:** ' + labels + '  \n') : '') +
      (dependsOn ? ('**Depends on:** ' + dependsOn + '  \n\n') : '\n') +
      '## Details\n\n' +
      (data.title ? ('# ' + data.title + '\n\n') : '') +
      (data.description || '_No description yet._')
    );
  }

  function beginEdit() {
    if (!issue) return;
    setEditTitle(issue.title || '');
    setEditPlanID(issue.plan_id || '');
    setEditStatus(normalizeStatus(issue.status || 'open') || 'open');
    setEditPriority(normalizeStatus(issue.priority || 'medium') || 'medium');
    setEditLabels((issue.labels || []).join(', '));
    setEditDependsOn((issue.depends_on || []).join(', '));
    setEditDescription(issue.description || '');
    setIsEditing(true);
  }

  function cancelEdit() {
    if (!issue) return;
    setEditTitle(issue.title || '');
    setEditPlanID(issue.plan_id || '');
    setEditStatus(normalizeStatus(issue.status || 'open') || 'open');
    setEditPriority(normalizeStatus(issue.priority || 'medium') || 'medium');
    setEditLabels((issue.labels || []).join(', '));
    setEditDependsOn((issue.depends_on || []).join(', '));
    setEditDescription(issue.description || '');
    setIsEditing(false);
  }

  async function saveIssue() {
    if (!issue) return;
    setSaving(true);

    var payload = {
      title: editTitle.trim() || issue.title,
      plan_id: editPlanID.trim() || issue.plan_id,
      status: editStatus,
      priority: editPriority,
      description: editDescription,
      labels: editLabels
        .split(',')
        .map(function (item) { return item.trim(); })
        .filter(function (item) { return item.length > 0; }),
      depends_on: editDependsOn
        .split(',')
        .map(function (item) { return Number(item.trim()); })
        .filter(function (id) { return Number.isFinite(id) && id > 0; }),
    };

    try {
      var updated = await apiCall(base + '/issues/' + encodeURIComponent(String(issue.id)), 'PUT', payload);
      var next = state.issues.map(function (item) {
        if (item.id === updated.id) return updated;
        return item;
      });
      dispatch({ type: 'SET', payload: { issues: next } });
      toast('Issue updated', 'success');
      setIsEditing(false);
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to save issue: ' + (err.message || err), 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column', animation: 'fadeIn 0.2s ease' }}>
      <SectionHeader
        count={1}
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
                  onClick={saveIssue}
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
        {'Issue #' + issue.id}
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
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Plan ID</label>
              <input
                value={editPlanID}
                onChange={function (event) { setEditPlanID(event.target.value); }}
                style={inputStyle}
                placeholder="optional shared plan id"
              />

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div>
                  <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Status</label>
                  <select value={editStatus} onChange={function (event) { setEditStatus(event.target.value); }} style={selectStyle}>
                    {issueStatusOptions.map(function (status) {
                      return <option key={status} value={status}>{status}</option>;
                    })}
                  </select>
                </div>
                <div>
                  <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Priority</label>
                  <select value={editPriority} onChange={function (event) { setEditPriority(event.target.value); }} style={selectStyle}>
                    {issuePriorityOptions.map(function (priority) {
                      return <option key={priority} value={priority}>{priority}</option>;
                    })}
                  </select>
                </div>
              </div>

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Labels</label>
              <input
                value={editLabels}
                onChange={function (event) { setEditLabels(event.target.value); }}
                style={inputStyle}
                placeholder="comma separated"
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Depends On (Issue IDs)</label>
              <input
                value={editDependsOn}
                onChange={function (event) { setEditDependsOn(event.target.value); }}
                style={inputStyle}
                placeholder="e.g. 12, 18"
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Description (Markdown)</label>
              <textarea
                value={editDescription}
                onChange={function (event) { setEditDescription(event.target.value); }}
                style={textareaStyle}
                rows={12}
              />
            </div>
          ) : (
            <MarkdownContent
              text={buildMarkdown({
                title: issue.title,
                plan_id: issue.plan_id || '',
                status: issue.status || 'open',
                priority: issue.priority || 'medium',
                labels: issue.labels || [],
                depends_on: issue.depends_on || [],
                description: issue.description || '',
              })}
              style={markdownContentStyle}
            />
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
  minHeight: 240,
};

var markdownContentStyle = {
  width: '100%',
  margin: 0,
  padding: 0,
};
