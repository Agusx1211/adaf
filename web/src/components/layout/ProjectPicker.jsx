import { useState, useEffect, useCallback } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall, apiProjectOpen } from '../../api/client.js';
import { persistProjectSelection } from '../../utils/projectLink.js';
import ProjectBrowser from './ProjectBrowser.jsx';

export default function ProjectPicker() {
  var state = useAppState();
  var dispatch = useDispatch();
  var [recentProjects, setRecentProjects] = useState([]);
  var [loading, setLoading] = useState(true);
  var [showBrowser, setShowBrowser] = useState(false);
  var [error, setError] = useState('');

  useEffect(function () {
    apiCall('/api/projects/recent', 'GET', null, { allow404: true })
      .then(function (data) {
        setRecentProjects(Array.isArray(data) ? data : []);
      })
      .catch(function () {
        setRecentProjects([]);
      })
      .finally(function () {
        setLoading(false);
      });
  }, []);

  var handleSelectProject = useCallback(function (project) {
    setError('');
    // Try opening via path
    apiProjectOpen(project.path)
      .then(function (data) {
        if (data && data.id) {
          var nextProjectID = String(data.id);
          dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
          dispatch({ type: 'RESET_PROJECT_STATE' });
          persistProjectSelection(nextProjectID);
          // Re-fetch project list
          apiCall('/api/projects', 'GET', null, { allow404: true })
            .then(function (projects) {
              dispatch({ type: 'SET_PROJECTS', payload: Array.isArray(projects) ? projects : [] });
            })
            .catch(function () {});
        }
      })
      .catch(function (err) {
        setError(err.message || 'Failed to open project');
      });
  }, [dispatch]);

  var handleSelectRegistered = useCallback(function (project) {
    var nextProjectID = String(project.id || '');
    dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
    dispatch({ type: 'RESET_PROJECT_STATE' });
    persistProjectSelection(nextProjectID);
  }, [dispatch]);

  var registeredProjects = state.projects || [];

  if (showBrowser) {
    return <ProjectBrowser onClose={function () { setShowBrowser(false); }} />;
  }

  return (
    <div style={{
      width: '100vw', height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg-0)',
    }}>
      <div style={{
        width: '100%', maxWidth: 520, padding: 32,
        background: 'var(--bg-1)', border: '1px solid var(--border)', borderRadius: 8,
      }}>
        <h2 style={{
          margin: '0 0 8px 0', fontFamily: "'JetBrains Mono', monospace",
          fontSize: 16, fontWeight: 600, color: 'var(--text-0)',
        }}>Select Project</h2>

        {state.unresolvedProjectID && (
          <div style={{
            padding: '6px 10px', marginBottom: 12, borderRadius: 4,
            background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: '#ef4444',
          }}>
            Project "{state.unresolvedProjectID}" not found
          </div>
        )}

        {error && (
          <div style={{
            padding: '6px 10px', marginBottom: 12, borderRadius: 4,
            background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: '#ef4444',
          }}>{error}</div>
        )}

        {/* Currently registered projects */}
        {registeredProjects.length > 0 && (
          <div style={{ marginBottom: 16 }}>
            <div style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
              color: 'var(--text-3)', marginBottom: 6, textTransform: 'uppercase',
              letterSpacing: '0.05em',
            }}>Open Projects</div>
            <div style={{
              border: '1px solid var(--border)', borderRadius: 4, overflow: 'hidden',
            }}>
              {registeredProjects.map(function (p) {
                return (
                  <div
                    key={p.id}
                    onClick={function () { handleSelectRegistered(p); }}
                    onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
                    onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
                    style={{
                      padding: '8px 12px', cursor: 'pointer',
                      borderBottom: '1px solid var(--bg-3)',
                      display: 'flex', alignItems: 'center', gap: 8,
                    }}
                  >
                    <span style={{
                      fontSize: 12, color: 'var(--accent)', flexShrink: 0,
                    }}>{'\u25C6'}</span>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{
                        fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
                        color: 'var(--text-0)', fontWeight: 600,
                        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                      }}>{p.name || p.id}</div>
                      <div style={{
                        fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                        color: 'var(--text-3)', marginTop: 2,
                        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                      }}>{p.path || p.id}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Recent projects */}
        {loading ? (
          <div style={{
            padding: 16, textAlign: 'center', color: 'var(--text-3)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          }}>Loading recent projects...</div>
        ) : recentProjects.length > 0 ? (
          <div style={{ marginBottom: 16 }}>
            <div style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
              color: 'var(--text-3)', marginBottom: 6, textTransform: 'uppercase',
              letterSpacing: '0.05em',
            }}>Recent Projects</div>
            <div style={{
              border: '1px solid var(--border)', borderRadius: 4,
              maxHeight: 280, overflow: 'auto',
            }}>
              {recentProjects.filter(function (rp) {
                // Filter out already-registered projects
                return !registeredProjects.find(function (reg) {
                  return reg.path === rp.path || reg.id === rp.id;
                });
              }).map(function (rp) {
                return (
                  <div
                    key={rp.path}
                    onClick={function () { handleSelectProject(rp); }}
                    onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
                    onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
                    style={{
                      padding: '8px 12px', cursor: 'pointer',
                      borderBottom: '1px solid var(--bg-3)',
                      display: 'flex', alignItems: 'center', gap: 8,
                    }}
                  >
                    <span style={{
                      fontSize: 12, color: 'var(--text-2)', flexShrink: 0,
                    }}>{'\u25CB'}</span>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{
                        fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
                        color: 'var(--text-1)',
                        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                      }}>{rp.name || rp.id}</div>
                      <div style={{
                        fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                        color: 'var(--text-3)', marginTop: 2,
                        overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                      }}>{rp.path}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ) : null}

        {/* Browse button */}
        <button
          onClick={function () { setShowBrowser(true); }}
          style={{
            width: '100%', padding: '10px 16px',
            border: '1px solid var(--border)', borderRadius: 4,
            background: 'var(--bg-2)', color: 'var(--text-1)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
            cursor: 'pointer', textAlign: 'center',
          }}
          onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-3)'; }}
          onMouseLeave={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
        >Browse...</button>
      </div>
    </div>
  );
}
