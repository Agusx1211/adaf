import { useEffect, useMemo, useState } from 'react';
import { useAppState, useDispatch, normalizeIssues } from '../../state/store.js';
import { apiBase, apiCall } from '../../api/client.js';
import { timeAgo, normalizeStatus, withAlpha } from '../../utils/format.js';
import { statusColor, statusIcon } from '../../utils/colors.js';
import Modal from '../common/Modal.jsx';
import { useToast } from '../common/Toast.jsx';

var ISSUE_COLUMNS = [
  { id: 'open', label: 'Open' },
  { id: 'ongoing', label: 'Ongoing' },
  { id: 'in_review', label: 'In Review' },
  { id: 'closed', label: 'Closed' },
];

var PRIORITIES = ['critical', 'high', 'medium', 'low'];
var LIVE_REFRESH_MS = 2000;

export default function IssuesView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var toast = useToast();
  var base = apiBase(state.currentProjectID);
  var issues = state.issues || [];
  var selectedIssueID = state.selectedIssue;

  var [detailIssue, setDetailIssue] = useState(null);
  var [detailLoading, setDetailLoading] = useState(false);
  var [saving, setSaving] = useState(false);
  var [commentSaving, setCommentSaving] = useState(false);

  var [showCreateModal, setShowCreateModal] = useState(false);
  var [creating, setCreating] = useState(false);

  var [editTitle, setEditTitle] = useState('');
  var [editStatus, setEditStatus] = useState('open');
  var [editPriority, setEditPriority] = useState('medium');
  var [editPlanID, setEditPlanID] = useState('');
  var [editLabels, setEditLabels] = useState('');
  var [editDescription, setEditDescription] = useState('');
  var [commentBody, setCommentBody] = useState('');

  var [createTitle, setCreateTitle] = useState('');
  var [createPriority, setCreatePriority] = useState('medium');
  var [createPlanID, setCreatePlanID] = useState('');
  var [createLabels, setCreateLabels] = useState('');
  var [createDescription, setCreateDescription] = useState('');

  var issuesByColumn = useMemo(function () {
    var grouped = { open: [], ongoing: [], in_review: [], closed: [] };
    issues.forEach(function (issue) {
      var key = normalizeStatus(issue.status || 'open');
      if (!grouped[key]) grouped[key] = [];
      grouped[key].push(issue);
    });
    ISSUE_COLUMNS.forEach(function (column) {
      grouped[column.id].sort(function (a, b) { return b.id - a.id; });
    });
    return grouped;
  }, [issues]);

  useEffect(function () {
    if (!selectedIssueID) {
      setDetailIssue(null);
      return;
    }
    var selected = issues.find(function (issue) { return issue.id === selectedIssueID; }) || null;
    if (selected) setDetailIssue(selected);

    setDetailLoading(true);
    apiCall(base + '/issues/' + encodeURIComponent(String(selectedIssueID)), 'GET', null, { allow404: true })
      .then(function (data) {
        var normalized = normalizeIssues(data ? [data] : [])[0] || null;
        if (!normalized) {
          dispatch({ type: 'SET_SELECTED_ISSUE', payload: null });
          setDetailIssue(null);
          return;
        }
        setDetailIssue(normalized);
      })
      .catch(function (err) {
        if (!err.authRequired) {
          toast('Failed to load issue: ' + (err.message || err), 'error');
        }
      })
      .finally(function () {
        setDetailLoading(false);
      });
  }, [selectedIssueID, base, dispatch, issues, toast]);

  useEffect(function () {
    if (!detailIssue) return;
    setEditTitle(detailIssue.title || '');
    setEditStatus(normalizeStatus(detailIssue.status || 'open') || 'open');
    setEditPriority(normalizeStatus(detailIssue.priority || 'medium') || 'medium');
    setEditPlanID(detailIssue.plan_id || '');
    setEditLabels((detailIssue.labels || []).join(', '));
    setEditDescription(detailIssue.description || '');
    setCommentBody('');
  }, [detailIssue && detailIssue.id]);

  async function refreshIssues() {
    var next = normalizeIssues(await apiCall(base + '/issues', 'GET', null, { allow404: true }));
    dispatch({ type: 'SET', payload: { issues: next } });
    return next;
  }

  useEffect(function () {
    var cancelled = false;

    async function refreshQuietly() {
      try {
        var next = normalizeIssues(await apiCall(base + '/issues', 'GET', null, { allow404: true }));
        if (cancelled) return;
        dispatch({ type: 'SET', payload: { issues: next } });
      } catch (err) {
        if (err && err.authRequired) {
          dispatch({ type: 'SET', payload: { authRequired: true } });
        }
      }
    }

    refreshQuietly();
    var timer = setInterval(refreshQuietly, LIVE_REFRESH_MS);
    return function () {
      cancelled = true;
      clearInterval(timer);
    };
  }, [base, dispatch]);

  function openIssue(issueID) {
    dispatch({ type: 'SET_SELECTED_ISSUE', payload: issueID });
  }

  function closeIssueModal() {
    dispatch({ type: 'SET_SELECTED_ISSUE', payload: null });
    setDetailIssue(null);
  }

  async function createIssue() {
    var title = String(createTitle || '').trim();
    if (!title) {
      toast('Issue title is required', 'error');
      return;
    }
    setCreating(true);
    try {
      var created = await apiCall(base + '/issues', 'POST', {
        title: title,
        description: createDescription || '',
        status: 'open',
        priority: createPriority || 'medium',
        plan_id: String(createPlanID || '').trim(),
        labels: String(createLabels || '').split(',').map(function (label) { return label.trim(); }).filter(function (label) { return !!label; }),
        created_by: 'web-ui',
        updated_by: 'web-ui',
      });
      await refreshIssues();
      setShowCreateModal(false);
      setCreateTitle('');
      setCreatePriority('medium');
      setCreatePlanID('');
      setCreateLabels('');
      setCreateDescription('');
      if (created && created.id) {
        openIssue(Number(created.id));
      }
      toast('Issue created', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to create issue: ' + (err.message || err), 'error');
      }
    } finally {
      setCreating(false);
    }
  }

  async function saveIssueChanges() {
    if (!detailIssue) return;

    setSaving(true);
    try {
      var payload = {
        title: editTitle.trim(),
        description: editDescription,
        status: editStatus,
        priority: editPriority,
        plan_id: String(editPlanID || '').trim(),
        labels: String(editLabels || '').split(',').map(function (label) { return label.trim(); }).filter(function (label) { return !!label; }),
        depends_on: detailIssue.depends_on || [],
        updated_by: 'web-ui',
      };
      var updated = await apiCall(base + '/issues/' + encodeURIComponent(String(detailIssue.id)), 'PUT', payload);
      var normalized = normalizeIssues([updated])[0] || null;
      if (normalized) {
        setDetailIssue(normalized);
      }
      await refreshIssues();
      toast('Issue updated', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to update issue: ' + (err.message || err), 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  async function addComment() {
    if (!detailIssue) return;
    var body = String(commentBody || '').trim();
    if (!body) {
      toast('Comment body is required', 'error');
      return;
    }

    setCommentSaving(true);
    try {
      var updated = await apiCall(
        base + '/issues/' + encodeURIComponent(String(detailIssue.id)) + '/comments',
        'POST',
        { body: body, by: 'web-ui' }
      );
      var normalized = normalizeIssues([updated])[0] || null;
      if (normalized) {
        setDetailIssue(normalized);
      }
      await refreshIssues();
      setCommentBody('');
      toast('Comment added', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to add comment: ' + (err.message || err), 'error');
      }
    } finally {
      setCommentSaving(false);
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: 'var(--bg-1)' }}>
      <div style={{
        padding: '10px 14px',
        borderBottom: '1px solid var(--border)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 11, fontWeight: 700, color: 'var(--text-1)' }}>
            Issue Board
          </span>
          <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)' }}>
            {issues.length} issue(s)
          </span>
        </div>
        <button
          onClick={function () { setShowCreateModal(true); }}
          style={{
            border: '1px solid var(--accent)40',
            background: 'var(--accent)15',
            color: 'var(--accent)',
            borderRadius: 4,
            padding: '4px 10px',
            cursor: 'pointer',
            fontFamily: "'JetBrains Mono', monospace",
            fontSize: 10,
            fontWeight: 700,
          }}
        >+ New Issue</button>
      </div>

      <div style={{ flex: 1, overflowX: 'auto', overflowY: 'hidden', padding: 12 }}>
        <div style={{ display: 'flex', gap: 12, minWidth: 1120, height: '100%' }}>
          {ISSUE_COLUMNS.map(function (column) {
            var columnIssues = issuesByColumn[column.id] || [];
            return (
              <div key={column.id} style={{
                width: 270,
                minWidth: 270,
                border: '1px solid var(--border)',
                borderRadius: 8,
                background: 'var(--bg-2)',
                display: 'flex',
                flexDirection: 'column',
              }}>
                <div style={{
                  padding: '10px 12px',
                  borderBottom: '1px solid var(--border)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                }}>
                  <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, fontWeight: 700, color: 'var(--text-1)' }}>
                    {column.label}
                  </span>
                  <span style={{
                    fontFamily: "'JetBrains Mono', monospace",
                    fontSize: 10,
                    color: 'var(--text-2)',
                    padding: '1px 6px',
                    borderRadius: 10,
                    background: 'var(--bg-4)',
                  }}>{columnIssues.length}</span>
                </div>

                <div style={{ flex: 1, overflow: 'auto', padding: 8, display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {columnIssues.map(function (issue) {
                    return (
                      <IssueCard key={issue.id} issue={issue} onOpen={openIssue} />
                    );
                  })}
                  {columnIssues.length === 0 && (
                    <div style={{
                      border: '1px dashed var(--border)',
                      borderRadius: 6,
                      padding: 12,
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: 10,
                      color: 'var(--text-3)',
                      textAlign: 'center',
                    }}>
                      No issues
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {showCreateModal && (
        <Modal title="Create Issue" onClose={function () { setShowCreateModal(false); }} maxWidth={680}>
          <div style={{ display: 'grid', gap: 10 }}>
            <label style={labelStyle}>Title</label>
            <input value={createTitle} onChange={function (event) { setCreateTitle(event.target.value); }} style={inputStyle} />

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
              <div>
                <label style={labelStyle}>Priority</label>
                <select value={createPriority} onChange={function (event) { setCreatePriority(event.target.value); }} style={inputStyle}>
                  {PRIORITIES.map(function (priority) {
                    return <option key={priority} value={priority}>{priority}</option>;
                  })}
                </select>
              </div>
              <div>
                <label style={labelStyle}>Plan ID (optional)</label>
                <input value={createPlanID} onChange={function (event) { setCreatePlanID(event.target.value); }} style={inputStyle} />
              </div>
            </div>

            <label style={labelStyle}>Labels</label>
            <input value={createLabels} onChange={function (event) { setCreateLabels(event.target.value); }} style={inputStyle} placeholder="comma separated" />

            <label style={labelStyle}>Description</label>
            <textarea
              value={createDescription}
              onChange={function (event) { setCreateDescription(event.target.value); }}
              style={textareaStyle}
              rows={10}
            />

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <button onClick={function () { setShowCreateModal(false); }} style={buttonGhostStyle}>Cancel</button>
              <button onClick={createIssue} disabled={creating} style={buttonPrimaryStyle}>{creating ? 'Creating...' : 'Create Issue'}</button>
            </div>
          </div>
        </Modal>
      )}

      {selectedIssueID && (
        <Modal title={'Issue #' + selectedIssueID} onClose={closeIssueModal} maxWidth={1120}>
          {!detailIssue || detailLoading ? (
            <div style={{ padding: 12, fontFamily: "'JetBrains Mono', monospace", fontSize: 11, color: 'var(--text-3)' }}>Loading issue...</div>
          ) : (
            <div style={{ display: 'grid', gridTemplateColumns: '1.1fr 0.9fr', gap: 14, maxHeight: '72vh' }}>
              <div style={{ minHeight: 0, overflow: 'auto', paddingRight: 4 }}>
                <div style={{ display: 'grid', gap: 10 }}>
                  <label style={labelStyle}>Title</label>
                  <input value={editTitle} onChange={function (event) { setEditTitle(event.target.value); }} style={inputStyle} />

                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                    <div>
                      <label style={labelStyle}>Status</label>
                      <select value={editStatus} onChange={function (event) { setEditStatus(event.target.value); }} style={inputStyle}>
                        {ISSUE_COLUMNS.map(function (column) {
                          return <option key={column.id} value={column.id}>{column.label}</option>;
                        })}
                      </select>
                    </div>
                    <div>
                      <label style={labelStyle}>Priority</label>
                      <select value={editPriority} onChange={function (event) { setEditPriority(event.target.value); }} style={inputStyle}>
                        {PRIORITIES.map(function (priority) {
                          return <option key={priority} value={priority}>{priority}</option>;
                        })}
                      </select>
                    </div>
                  </div>

                  <label style={labelStyle}>Plan ID</label>
                  <input value={editPlanID} onChange={function (event) { setEditPlanID(event.target.value); }} style={inputStyle} />

                  <label style={labelStyle}>Labels</label>
                  <input value={editLabels} onChange={function (event) { setEditLabels(event.target.value); }} style={inputStyle} placeholder="comma separated" />

                  <label style={labelStyle}>Description</label>
                  <textarea
                    value={editDescription}
                    onChange={function (event) { setEditDescription(event.target.value); }}
                    style={textareaStyle}
                    rows={12}
                  />

                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                    <InfoPill label="Created" value={formatIssueMeta(detailIssue.created, detailIssue.created_by)} />
                    <InfoPill label="Updated" value={formatIssueMeta(detailIssue.updated, detailIssue.updated_by)} />
                  </div>

                  <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                    <button onClick={closeIssueModal} style={buttonGhostStyle}>Close</button>
                    <button onClick={saveIssueChanges} disabled={saving} style={buttonPrimaryStyle}>
                      {saving ? 'Saving...' : 'Save Changes'}
                    </button>
                  </div>
                </div>
              </div>

              <div style={{ minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div style={panelStyle}>
                  <div style={panelHeaderStyle}>Comments ({(detailIssue.comments || []).length})</div>
                  <div style={{ maxHeight: 220, overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 8 }}>
                    {(detailIssue.comments || []).length === 0 && (
                      <div style={emptyTextStyle}>No comments yet.</div>
                    )}
                    {(detailIssue.comments || []).map(function (comment) {
                      return (
                        <div key={comment.id} style={commentStyle}>
                          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
                            <span style={commentMetaStyle}>{comment.by || 'unknown'} #{comment.id}</span>
                            <span style={commentMetaStyle}>{timeAgo(comment.created)}</span>
                          </div>
                          <div style={{ fontFamily: "'Outfit', sans-serif", fontSize: 12, color: 'var(--text-1)', lineHeight: 1.5, whiteSpace: 'pre-wrap' }}>
                            {comment.body}
                          </div>
                        </div>
                      );
                    })}
                  </div>

                  <label style={labelStyle}>Add Comment</label>
                  <textarea value={commentBody} onChange={function (event) { setCommentBody(event.target.value); }} style={commentInputStyle} rows={4} />
                  <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                    <button onClick={addComment} disabled={commentSaving} style={buttonPrimaryStyle}>
                      {commentSaving ? 'Posting...' : 'Post Comment'}
                    </button>
                  </div>
                </div>

                <div style={panelStyle}>
                  <div style={panelHeaderStyle}>History ({(detailIssue.history || []).length})</div>
                  <div style={{ maxHeight: 260, overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 8 }}>
                    {(detailIssue.history || []).length === 0 && (
                      <div style={emptyTextStyle}>No history yet.</div>
                    )}
                    {(detailIssue.history || []).slice().reverse().map(function (item) {
                      return (
                        <div key={String(item.id) + '-' + String(item.type)} style={historyItemStyle}>
                          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                            <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-1)' }}>
                              {formatHistoryLabel(item)}
                            </span>
                            <span style={commentMetaStyle}>{timeAgo(item.at)}</span>
                          </div>
                          <div style={{ fontFamily: "'Outfit', sans-serif", fontSize: 11, color: 'var(--text-2)', marginTop: 4 }}>
                            {formatHistoryDetail(item)}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>
            </div>
          )}
        </Modal>
      )}
    </div>
  );
}

function IssueCard(props) {
  var issue = props.issue;
  var onOpen = props.onOpen;
  var status = normalizeStatus(issue.status || 'open');
  var color = statusColor(status);

  return (
    <button
      type="button"
      onClick={function () { onOpen(issue.id); }}
      style={{
        textAlign: 'left',
        width: '100%',
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: '10px 10px 8px 10px',
        background: 'var(--bg-1)',
        cursor: 'pointer',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
        <span style={{ fontFamily: "'Outfit', sans-serif", fontSize: 12, fontWeight: 600, color: 'var(--text-0)', lineHeight: 1.4 }}>
          {issue.title || 'Untitled issue'}
        </span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-3)', flexShrink: 0 }}>#{issue.id}</span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8, flexWrap: 'wrap' }}>
        <Badge value={issue.priority || 'medium'} />
        <span style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 4,
          fontFamily: "'JetBrains Mono', monospace",
          fontSize: 9,
          color: color,
          background: withAlpha(color, 0.14),
          border: '1px solid ' + withAlpha(color, 0.28),
          borderRadius: 999,
          padding: '1px 7px',
        }}>
          <span>{statusIcon(status)}</span>
          <span>{status}</span>
        </span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: 8 }}>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
          {((issue.comments || []).length) + ' comment(s)'}
        </span>
        <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
          {timeAgo(issue.updated || issue.created)}
        </span>
      </div>
    </button>
  );
}

function Badge(props) {
  var value = String(props.value || '').trim().toLowerCase() || 'medium';
  var color = statusColor(value);
  return (
    <span style={{
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 9,
      color: color,
      border: '1px solid ' + withAlpha(color, 0.35),
      background: withAlpha(color, 0.16),
      borderRadius: 999,
      padding: '1px 7px',
      textTransform: 'lowercase',
    }}>
      {value}
    </span>
  );
}

function InfoPill(props) {
  return (
    <div style={{
      border: '1px solid var(--border)',
      borderRadius: 6,
      background: 'var(--bg-2)',
      padding: '8px 10px',
    }}>
      <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)', marginBottom: 3 }}>
        {props.label}
      </div>
      <div style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-1)' }}>
        {props.value}
      </div>
    </div>
  );
}

function formatIssueMeta(ts, actor) {
  var who = String(actor || '').trim() || 'unknown';
  if (!ts) return who;
  return timeAgo(ts) + ' by ' + who;
}

function formatHistoryLabel(item) {
  var by = String(item.by || '').trim() || 'unknown';
  var type = String(item.type || '').trim() || 'updated';
  return by + ' · ' + type;
}

function formatHistoryDetail(item) {
  if (item.type === 'status_changed') {
    return 'status: ' + (item.from || '-') + ' -> ' + (item.to || '-');
  }
  if (item.type === 'moved') {
    return 'scope: ' + (item.from || '-') + ' -> ' + (item.to || '-');
  }
  if (item.type === 'commented') {
    return item.message || 'comment added';
  }
  if (item.field) {
    return item.field + ': ' + (item.from || '-') + ' -> ' + (item.to || '-');
  }
  return item.message || item.type || 'updated';
}

var labelStyle = {
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  color: 'var(--text-2)',
};

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
};

var commentInputStyle = {
  width: '100%',
  padding: '9px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-0)',
  color: 'var(--text-0)',
  fontFamily: "'Outfit', sans-serif",
  fontSize: 12,
  lineHeight: 1.5,
  resize: 'vertical',
};

var buttonPrimaryStyle = {
  border: '1px solid var(--accent)',
  background: 'var(--accent)',
  color: '#000',
  borderRadius: 4,
  padding: '6px 12px',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  fontWeight: 700,
  cursor: 'pointer',
};

var buttonGhostStyle = {
  border: '1px solid var(--border)',
  background: 'transparent',
  color: 'var(--text-2)',
  borderRadius: 4,
  padding: '6px 12px',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  fontWeight: 600,
  cursor: 'pointer',
};

var panelStyle = {
  border: '1px solid var(--border)',
  borderRadius: 8,
  background: 'var(--bg-2)',
  padding: 10,
  display: 'flex',
  flexDirection: 'column',
  gap: 8,
};

var panelHeaderStyle = {
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  fontWeight: 700,
  color: 'var(--text-1)',
};

var commentStyle = {
  border: '1px solid var(--border)',
  borderRadius: 6,
  padding: '8px 10px',
  background: 'var(--bg-1)',
};

var historyItemStyle = {
  border: '1px solid var(--border)',
  borderRadius: 6,
  padding: '8px 10px',
  background: 'var(--bg-1)',
};

var commentMetaStyle = {
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 9,
  color: 'var(--text-3)',
};

var emptyTextStyle = {
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  color: 'var(--text-3)',
};
