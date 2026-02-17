import { useEffect, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiBase, apiCall } from '../../api/client.js';
import { MarkdownContent, injectEventBlockStyles } from '../common/EventBlocks.jsx';
import SectionHeader from '../common/SectionHeader.jsx';
import { useToast } from '../common/Toast.jsx';

export default function WikiDetailPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var toast = useToast();
  var base = apiBase(state.currentProjectID);
  var entry = state.wiki.find(function (item) { return item.id === state.selectedWiki; });

  var [isEditing, setIsEditing] = useState(false);
  var [saving, setSaving] = useState(false);
  var [deleting, setDeleting] = useState(false);
  var [editTitle, setEditTitle] = useState('');
  var [editPlanID, setEditPlanID] = useState('');
  var [editContent, setEditContent] = useState('');
  var [editBy, setEditBy] = useState('web-ui');

  useEffect(function () {
    injectEventBlockStyles();
  }, []);

  useEffect(function () {
    if (!entry) {
      setIsEditing(false);
      setEditTitle('');
      setEditPlanID('');
      setEditContent('');
      setEditBy('web-ui');
      return;
    }
    if (!isEditing) {
      setEditTitle(entry.title || '');
      setEditPlanID(entry.plan_id || '');
      setEditContent(entry.content || '');
      setEditBy(entry.updated_by || entry.created_by || 'web-ui');
    }
  }, [entry && entry.id]);

  if (!entry) {
    return (
      <div style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)' }}>
        Select a wiki entry
      </div>
    );
  }

  function buildMarkdown(data) {
    var history = Array.isArray(data.history) ? data.history : [];
    var lastHistory = history.slice(history.length > 5 ? history.length - 5 : 0);
    var historyLines = lastHistory.map(function (change) {
      var action = change && change.action ? change.action : 'update';
      var by = change && change.by ? change.by : 'unknown';
      var at = change && change.at ? String(change.at) : '';
      return '- v' + (Number(change && change.version) || '?') + ' ' + action + ' by ' + by + (at ? ' @ ' + at : '');
    }).join('\n');
    return (
      '# ' + (data.title || 'Untitled Wiki Entry') + '\n\n' +
      '**Wiki ID:** `' + (data.id || 'n/a') + '`  \n' +
      '**Plan:** ' + (data.plan_id || 'shared') + '  \n\n' +
      '**Version:** ' + (Number(data.version) || 1) + '  \n' +
      '**Created By:** ' + (data.created_by || 'unknown') + '  \n' +
      '**Updated By:** ' + (data.updated_by || data.created_by || 'unknown') + '  \n' +
      '**Updated At:** ' + (data.updated || 'n/a') + '  \n\n' +
      (data.content || '_No content yet._') +
      '\n\n## Recent Changes\n\n' +
      (historyLines || '_No history yet._')
    );
  }

  function beginEdit() {
    setEditTitle(entry.title || '');
    setEditPlanID(entry.plan_id || '');
    setEditContent(entry.content || '');
    setEditBy(entry.updated_by || entry.created_by || 'web-ui');
    setIsEditing(true);
  }

  function cancelEdit() {
    setEditTitle(entry.title || '');
    setEditPlanID(entry.plan_id || '');
    setEditContent(entry.content || '');
    setEditBy(entry.updated_by || entry.created_by || 'web-ui');
    setIsEditing(false);
  }

  async function saveWiki() {
    setSaving(true);

    var payload = {
      title: editTitle.trim() || entry.title,
      plan_id: editPlanID.trim(),
      content: editContent,
      updated_by: editBy.trim() || 'web-ui',
    };

    try {
      var updated = await apiCall(base + '/wiki/' + encodeURIComponent(entry.id), 'PUT', payload);
      dispatch({
        type: 'SET',
        payload: {
          wiki: state.wiki.map(function (item) {
            return item.id === updated.id ? updated : item;
          }),
        },
      });
      setIsEditing(false);
      toast('Wiki entry updated', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to save wiki entry: ' + (err.message || err), 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  async function deleteWiki() {
    var confirmed = window.confirm('Delete wiki entry "' + (entry.title || entry.id) + '"?');
    if (!confirmed) return;

    setDeleting(true);
    try {
      await apiCall(base + '/wiki/' + encodeURIComponent(entry.id), 'DELETE');
      dispatch({
        type: 'SET',
        payload: {
          wiki: state.wiki.filter(function (item) { return item.id !== entry.id; }),
          selectedWiki: null,
        },
      });
      setIsEditing(false);
      toast('Wiki entry deleted', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to delete wiki entry: ' + (err.message || err), 'error');
      }
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column', animation: 'fadeIn 0.2s ease' }}>
      <SectionHeader
        count={entry.id ? 1 : 0}
        action={(
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            {isEditing ? (
              <>
                <button
                  onClick={cancelEdit}
                  disabled={saving || deleting}
                  style={{
                    background: 'transparent', color: 'var(--text-3)', border: '1px solid var(--border)',
                    borderRadius: 4, padding: '4px 8px', fontFamily: "'JetBrains Mono', monospace",
                    fontSize: 10, cursor: saving || deleting ? 'not-allowed' : 'pointer',
                  }}
                >Cancel</button>
                <button
                  onClick={saveWiki}
                  disabled={saving || deleting}
                  style={{
                    background: saving || deleting ? 'var(--bg-3)' : 'var(--green)', color: '#000',
                    border: '1px solid transparent', borderRadius: 4, padding: '4px 8px',
                    fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                    fontWeight: 600, cursor: saving || deleting ? 'not-allowed' : 'pointer',
                  }}
                >{saving ? 'Saving...' : 'Save'}</button>
              </>
            ) : (
              <button
                onClick={beginEdit}
                disabled={deleting}
                style={{
                  background: 'transparent', color: 'var(--accent)', border: '1px solid var(--accent)40',
                  borderRadius: 4, padding: '4px 8px', fontFamily: "'JetBrains Mono', monospace",
                  fontSize: 10, cursor: deleting ? 'not-allowed' : 'pointer',
                }}
              >Edit</button>
            )}
            <button
              onClick={deleteWiki}
              disabled={saving || deleting}
              style={{
                background: 'transparent', color: 'var(--red)', border: '1px solid var(--red)40',
                borderRadius: 4, padding: '4px 8px', fontFamily: "'JetBrains Mono', monospace",
                fontSize: 10, cursor: saving || deleting ? 'not-allowed' : 'pointer',
              }}
            >{deleting ? 'Deleting...' : 'Delete'}</button>
          </div>
        )}
      >
        Wiki
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
                placeholder="Wiki title"
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Plan ID</label>
              <input
                value={editPlanID}
                onChange={function (event) { setEditPlanID(event.target.value); }}
                style={inputStyle}
                placeholder="shared"
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Content (Markdown)</label>
              <textarea
                value={editContent}
                onChange={function (event) { setEditContent(event.target.value); }}
                style={textareaStyle}
                rows={18}
              />

              <label style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: 'var(--text-2)' }}>Updated By</label>
              <input
                value={editBy}
                onChange={function (event) { setEditBy(event.target.value); }}
                style={inputStyle}
                placeholder="agent/profile/user"
              />
            </div>
          ) : (
            <MarkdownContent text={buildMarkdown(entry)} style={markdownContentStyle} />
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
  minHeight: 220,
};

var markdownContentStyle = {
  width: '100%',
  margin: 0,
  padding: 0,
};
