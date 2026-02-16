import { useState, useEffect, useCallback } from 'react';
import { useDispatch } from '../../state/store.js';
import { apiFSBrowse, apiFSMkdir, apiProjectInit, apiProjectOpen } from '../../api/client.js';
import { persistProjectSelection } from '../../utils/projectLink.js';
import Modal from '../common/Modal.jsx';

export default function ProjectBrowser({ onClose }) {
  var dispatch = useDispatch();
  var [path, setPath] = useState('');
  var [parent, setParent] = useState('');
  var [entries, setEntries] = useState([]);
  var [loading, setLoading] = useState(false);
  var [error, setError] = useState('');
  var [showNewFolder, setShowNewFolder] = useState(false);
  var [newFolderName, setNewFolderName] = useState('');

  var browse = useCallback(function (targetPath) {
    setLoading(true);
    setError('');
    apiFSBrowse(targetPath || '')
      .then(function (data) {
        setPath(data.path || '');
        setParent(data.parent || '');
        setEntries(data.entries || []);
      })
      .catch(function (err) {
        setError(err.message || 'Failed to browse');
      })
      .finally(function () {
        setLoading(false);
      });
  }, []);

  useEffect(function () { browse(''); }, [browse]);

  function handleNavigate(entry) {
    if (!entry.is_dir) return;
    var next = path + '/' + entry.name;
    browse(next);
  }

  function handleNavigateUp() {
    if (parent) browse(parent);
  }

  function handleBreadcrumb(targetPath) {
    browse(targetPath);
  }

  function handleNewFolder() {
    var name = newFolderName.trim();
    if (!name) return;
    var folderPath = path + '/' + name;
    apiFSMkdir(folderPath)
      .then(function () {
        setShowNewFolder(false);
        setNewFolderName('');
        browse(path);
      })
      .catch(function (err) {
        setError(err.message || 'Failed to create folder');
      });
  }

  function handleInitProject(entryName) {
    var initPath = path + '/' + entryName;
    apiProjectInit(initPath)
      .then(function () {
        browse(path);
      })
      .catch(function (err) {
        setError(err.message || 'Failed to init project');
      });
  }

  function handleOpenProject(entryName) {
    var openPath = path + '/' + entryName;
    apiProjectOpen(openPath)
      .then(function (data) {
        if (data && data.id) {
          var nextProjectID = String(data.id);
          dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
          dispatch({ type: 'RESET_PROJECT_STATE' });
          persistProjectSelection(nextProjectID);
          onClose();
        }
      })
      .catch(function (err) {
        setError(err.message || 'Failed to open project');
      });
  }

  // Build breadcrumb segments from the absolute path
  var breadcrumbs = [];
  if (path) {
    var parts = path.split('/').filter(function (p) { return p !== ''; });
    var accumulated = '';
    for (var i = 0; i < parts.length; i++) {
      accumulated += '/' + parts[i];
      breadcrumbs.push({ label: parts[i], path: accumulated });
    }
  }

  return (
    <Modal title="File Explorer" onClose={onClose} maxWidth={640}>
      {/* Breadcrumb nav */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 2, marginBottom: 12,
        fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
        flexWrap: 'wrap',
      }}>
        <span
          onClick={function () { handleBreadcrumb('/'); }}
          style={{
            color: breadcrumbs.length > 0 ? 'var(--accent)' : 'var(--text-0)',
            cursor: breadcrumbs.length > 0 ? 'pointer' : 'default',
            fontWeight: breadcrumbs.length === 0 ? 600 : 400,
          }}
        >/</span>
        {breadcrumbs.map(function (crumb, i) {
          var isLast = i === breadcrumbs.length - 1;
          return (
            <span key={crumb.path} style={{ display: 'flex', alignItems: 'center', gap: 2 }}>
              <span
                onClick={isLast ? undefined : function () { handleBreadcrumb(crumb.path); }}
                style={{
                  color: isLast ? 'var(--text-0)' : 'var(--accent)',
                  cursor: isLast ? 'default' : 'pointer',
                  fontWeight: isLast ? 600 : 400,
                }}
              >{crumb.label}</span>
              {!isLast && <span style={{ color: 'var(--text-3)' }}>/</span>}
            </span>
          );
        })}
      </div>

      {error && (
        <div style={{
          padding: '6px 10px', marginBottom: 8, borderRadius: 4,
          background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
          color: '#ef4444', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
        }}>{error}</div>
      )}

      {/* Toolbar */}
      <div style={{ display: 'flex', gap: 6, marginBottom: 10 }}>
        {parent && (
          <button onClick={handleNavigateUp} style={toolBtnStyle}>.. Up</button>
        )}
        <button
          onClick={function () { setShowNewFolder(!showNewFolder); }}
          style={toolBtnStyle}
        >+ New Folder</button>
      </div>

      {/* New folder inline form */}
      {showNewFolder && (
        <div style={{ display: 'flex', gap: 6, marginBottom: 10 }}>
          <input
            type="text"
            value={newFolderName}
            onChange={function (e) { setNewFolderName(e.target.value); }}
            onKeyDown={function (e) { if (e.key === 'Enter') handleNewFolder(); }}
            placeholder="Folder name"
            autoFocus
            style={{
              flex: 1, padding: '4px 8px', background: 'var(--bg-2)',
              border: '1px solid var(--border)', borderRadius: 4,
              color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
            }}
          />
          <button onClick={handleNewFolder} style={toolBtnStyle}>Create</button>
          <button onClick={function () { setShowNewFolder(false); setNewFolderName(''); }} style={toolBtnStyle}>Cancel</button>
        </div>
      )}

      {/* Directory listing */}
      <div style={{
        maxHeight: 400, overflow: 'auto',
        border: '1px solid var(--border)', borderRadius: 4,
      }}>
        {loading ? (
          <div style={emptyStyle}>Loading...</div>
        ) : entries.length === 0 ? (
          <div style={emptyStyle}>Empty directory</div>
        ) : entries.map(function (entry) {
          var icon = entry.is_project ? '\u25C6' : entry.is_dir ? '\u25B7' : '\u2500';
          var iconColor = entry.is_project ? 'var(--accent)' : entry.is_dir ? 'var(--text-2)' : 'var(--text-3)';
          var nameColor = entry.is_project ? 'var(--text-0)' : entry.is_dir ? 'var(--text-1)' : 'var(--text-2)';
          var nameWeight = entry.is_project ? 600 : 400;

          return (
            <div
              key={entry.name}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '6px 12px', borderBottom: '1px solid var(--bg-3)',
                cursor: entry.is_dir ? 'pointer' : 'default',
              }}
              onClick={entry.is_dir ? function () { handleNavigate(entry); } : undefined}
              onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
              onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
            >
              {/* Icon */}
              <span style={{ fontSize: 12, flexShrink: 0, color: iconColor }}>
                {icon}
              </span>

              {/* Name */}
              <span style={{
                flex: 1, fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
                color: nameColor, fontWeight: nameWeight,
                overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
              }}>{entry.name}</span>

              {/* Project badge + actions for directories */}
              {entry.is_project && (
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                  padding: '1px 6px', borderRadius: 3,
                  background: 'var(--accent)15', color: 'var(--accent)',
                  flexShrink: 0,
                }}>project</span>
              )}

              {entry.is_project ? (
                <button
                  onClick={function (e) { e.stopPropagation(); handleOpenProject(entry.name); }}
                  style={actionBtnStyle}
                >Open</button>
              ) : entry.is_dir ? (
                <button
                  onClick={function (e) { e.stopPropagation(); handleInitProject(entry.name); }}
                  style={{
                    ...actionBtnStyle,
                    background: 'var(--bg-3)',
                    color: 'var(--text-2)',
                  }}
                >Init</button>
              ) : null}
            </div>
          );
        })}
      </div>
    </Modal>
  );
}

var toolBtnStyle = {
  padding: '4px 10px', border: '1px solid var(--border)',
  background: 'var(--bg-2)', borderRadius: 4, cursor: 'pointer',
  fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
  color: 'var(--text-1)',
};

var actionBtnStyle = {
  padding: '2px 8px', border: '1px solid var(--accent)40',
  background: 'var(--accent)10', borderRadius: 3, cursor: 'pointer',
  fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
  color: 'var(--accent)', fontWeight: 600, flexShrink: 0,
};

var emptyStyle = {
  padding: 20, textAlign: 'center', color: 'var(--text-3)',
  fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
};
