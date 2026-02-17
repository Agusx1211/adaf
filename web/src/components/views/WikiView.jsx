import { useEffect, useMemo, useState } from 'react';
import { useAppState, useDispatch, normalizeWiki } from '../../state/store.js';
import { apiBase, apiCall } from '../../api/client.js';
import { cropText, parseTimestamp } from '../../utils/format.js';
import { useToast } from '../common/Toast.jsx';

export default function WikiView() {
  var state = useAppState();
  var dispatch = useDispatch();
  var toast = useToast();
  var base = apiBase(state.currentProjectID);

  var selectedWiki = state.selectedWiki;
  var [query, setQuery] = useState('');
  var [searching, setSearching] = useState(false);
  var [searchResults, setSearchResults] = useState(null);
  var [creating, setCreating] = useState(false);
  var [showCreate, setShowCreate] = useState(false);
  var [newTitle, setNewTitle] = useState('');
  var [newContent, setNewContent] = useState('');
  var [newBy, setNewBy] = useState('web-ui');

  var wiki = useMemo(function () {
    return (state.wiki || []).slice().sort(function (a, b) {
      return parseTimestamp(b.updated || b.created) - parseTimestamp(a.updated || a.created);
    });
  }, [state.wiki]);

  useEffect(function () {
    var trimmed = query.trim();
    if (!trimmed) {
      setSearchResults(null);
      setSearching(false);
      return;
    }

    var cancelled = false;
    var timer = setTimeout(function () {
      setSearching(true);
      apiCall(base + '/wiki/search?q=' + encodeURIComponent(trimmed), 'GET', null, { allow404: true })
        .then(function (results) {
          if (cancelled) return;
          setSearchResults(normalizeWiki(results || []));
        })
        .catch(function (err) {
          if (cancelled) return;
          setSearchResults([]);
          if (!err.authRequired) {
            toast('Wiki search failed: ' + (err.message || err), 'error');
          }
        })
        .finally(function () {
          if (!cancelled) setSearching(false);
        });
    }, 180);

    return function () {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [query, base, toast]);

  var list = query.trim() ? (searchResults || []) : wiki;

  async function createWikiEntry() {
    var title = newTitle.trim();
    if (!title) {
      toast('Title is required', 'error');
      return;
    }

    setCreating(true);
    try {
      var created = await apiCall(base + '/wiki', 'POST', {
        title: title,
        content: newContent || '',
        updated_by: (newBy || '').trim() || 'web-ui',
      });
      var next = [created].concat(state.wiki || []);
      var dedup = {};
      next = next.filter(function (entry) {
        if (!entry || !entry.id) return false;
        if (dedup[entry.id]) return false;
        dedup[entry.id] = true;
        return true;
      });
      dispatch({ type: 'SET', payload: { wiki: normalizeWiki(next), selectedWiki: created.id } });
      setShowCreate(false);
      setNewTitle('');
      setNewContent('');
      toast('Wiki entry created', 'success');
    } catch (err) {
      if (!err.authRequired) {
        toast('Failed to create wiki entry: ' + (err.message || err), 'error');
      }
    } finally {
      setCreating(false);
    }
  }

  return (
    <div style={{ overflow: 'hidden', flex: 1, display: 'flex', flexDirection: 'column' }}>
      <div style={{ padding: '10px 12px', borderBottom: '1px solid var(--border)', display: 'grid', gap: 8 }}>
        <div style={{ display: 'flex', gap: 8 }}>
          <input
            value={query}
            onChange={function (e) { setQuery(e.target.value); }}
            style={inputStyle}
            placeholder="Fuzzy search wiki..."
          />
          <button
            onClick={function () { setShowCreate(!showCreate); }}
            style={newButtonStyle}
          >
            {showCreate ? 'Close' : '+ New'}
          </button>
        </div>
        {showCreate && (
          <div style={{ display: 'grid', gap: 7 }}>
            <input
              value={newTitle}
              onChange={function (e) { setNewTitle(e.target.value); }}
              style={inputStyle}
              placeholder="Wiki title"
            />
            <input
              value={newBy}
              onChange={function (e) { setNewBy(e.target.value); }}
              style={inputStyle}
              placeholder="Edited by (agent/profile/user)"
            />
            <textarea
              value={newContent}
              onChange={function (e) { setNewContent(e.target.value); }}
              rows={5}
              style={textareaStyle}
              placeholder="Concise, high-signal wiki content..."
            />
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button
                onClick={createWikiEntry}
                disabled={creating}
                style={saveButtonStyle}
              >
                {creating ? 'Creating...' : 'Create Wiki Entry'}
              </button>
            </div>
          </div>
        )}
      </div>

      {searching && (
        <div style={{ padding: '6px 12px', fontSize: 10, color: 'var(--text-3)', borderBottom: '1px solid var(--border)' }}>
          Searching wiki...
        </div>
      )}

      <div style={{ overflow: 'auto', flex: 1 }}>
        {!list.length ? (
          <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-3)', fontSize: 12 }}>
            {query.trim() ? 'No wiki matches.' : 'No wiki entries available.'}
          </div>
        ) : list.map(function (entry) {
          var selected = selectedWiki === entry.id;
          var updatedBy = (entry.updated_by || entry.created_by || 'unknown').trim();
          var version = Number(entry.version) || 0;
          return (
            <div key={entry.id} style={{
              padding: '10px 14px', borderBottom: '1px solid var(--border)', cursor: 'pointer',
              background: selected ? 'var(--bg-3)' : 'transparent',
              transition: 'background 0.15s ease',
            }}
            onClick={function () { dispatch({ type: 'SET_SELECTED_WIKI', payload: entry.id }); }}
            onMouseEnter={function (e) { if (!selected) e.currentTarget.style.background = 'var(--bg-2)'; }}
            onMouseLeave={function (e) { if (!selected) e.currentTarget.style.background = 'transparent'; }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-0)' }}>{entry.title || entry.id || 'Wiki Entry'}</span>
                <span style={{ fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>{entry.id}</span>
              </div>
              <div style={{ fontSize: 11, color: 'var(--text-2)', lineHeight: 1.4 }}>
                {cropText((entry.content || '').replace(/\s+/g, ' ').trim(), 160)}
              </div>
              <div style={{ marginTop: 6, display: 'flex', gap: 10, fontFamily: "'JetBrains Mono', monospace", fontSize: 9, color: 'var(--text-3)' }}>
                <span>v{version || 1}</span>
                <span>by {updatedBy || 'unknown'}</span>
                <span>{entry.plan_id ? 'plan:' + entry.plan_id : 'shared'}</span>
              </div>
            </div>
          );
        })}
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
  fontSize: 11,
  outline: 'none',
};

var textareaStyle = {
  width: '100%',
  padding: '8px 10px',
  borderRadius: 6,
  border: '1px solid var(--border)',
  background: 'var(--bg-0)',
  color: 'var(--text-0)',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 11,
  lineHeight: 1.5,
  resize: 'vertical',
  minHeight: 90,
};

var newButtonStyle = {
  padding: '0 10px',
  border: '1px solid var(--accent)40',
  background: 'var(--accent)15',
  color: 'var(--accent)',
  borderRadius: 6,
  cursor: 'pointer',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  fontWeight: 600,
};

var saveButtonStyle = {
  padding: '6px 10px',
  border: '1px solid var(--green)',
  background: 'var(--green)',
  color: '#000',
  borderRadius: 6,
  cursor: 'pointer',
  fontFamily: "'JetBrains Mono', monospace",
  fontSize: 10,
  fontWeight: 700,
};
