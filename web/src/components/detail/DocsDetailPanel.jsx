import { useEffect, useState } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiBase, apiCall } from '../../api/client.js';
import { MarkdownContent, injectEventBlockStyles } from '../common/EventBlocks.jsx';
import SectionHeader from '../common/SectionHeader.jsx';
import { useToast } from '../common/Toast.jsx';

export default function DocsDetailPanel() {
  var state = useAppState();
  var dispatch = useDispatch();
  var toast = useToast();
  var base = apiBase(state.currentProjectID);
  var doc = state.docs.find(function (item) { return item.id === state.selectedDoc; });

  var [isEditing, setIsEditing] = useState(false);
  var [saving, setSaving] = useState(false);
  var [editTitle, setEditTitle] = useState('');
  var [editPlanID, setEditPlanID] = useState('');
  var [editContent, setEditContent] = useState('');

  useEffect(function () {
    injectEventBlockStyles();
  }, []);

  useEffect(function () {
    if (!doc) {
      setIsEditing(false);
      setEditTitle('');
      setEditPlanID('');
      setEditContent('');
      return;
    }
    if (!isEditing) {
      setEditTitle(doc.title || '');
      setEditPlanID(doc.plan_id || '');
      setEditContent(doc.content || '');
    }
  }, [doc && doc.id]);

  if (!doc) {
    return (
      <div style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-3)' }}>
        Select a doc
      </div>
    );
  }

  function buildMarkdown(data) {
    return (
      '# ' + (data.title || 'Untitled Doc') + '\n\n' +
      '**Document ID:** `' + (data.id || 'n/a') + '`  \n' +
      '**Plan:** ' + (data.plan_id || 'shared') + '  \n\n' +
      (data.content || '_No content yet._')
    );
  }

  function beginEdit() {
    setEditTitle(doc.title || '');
    setEditPlanID(doc.plan_id || '');
    setEditContent(doc.content || '');
    setIsEditing(true);
  }

  function cancelEdit() {
    setEditTitle(doc.title || '');
    setEditPlanID(doc.plan_id || '');
    setEditContent(doc.content || '');
    setIsEditing(false);
  }

  async function saveDoc() {
    setSaving(true);

    var payload = {
      title: editTitle.trim() || doc.title,
      plan_id: editPlanID.trim(),
      content: editContent,
    };

    try {
      var updated = await apiCall(base + '/docs/' + encodeURIComponent(doc.id), 'PUT', payload);
      dispatch({
        type: 'SET',
        payload: {
          docs: state.docs.map(function (item) {
            return item.id === updated.id ? updated : item;
          }),
        },
      });
      setIsEditing(false);
      toast('Doc updated', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to save doc: ' + (err.message || err), 'error');
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <div style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column', animation: 'fadeIn 0.2s ease' }}>
      <SectionHeader
        count={doc.id ? 1 : 0}
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
                  onClick={saveDoc}
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
        Documentation
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
                placeholder="Doc title"
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
            </div>
          ) : (
            <MarkdownContent text={buildMarkdown(doc)} style={markdownContentStyle} />
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
