import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useAppState, useDispatch } from '../../state/store.js';
import { apiCall, apiProjectOpen, apiFSBrowse, apiFSMkdir, apiProjectInit, apiFSSearch, apiRemoveRecentProject } from '../../api/client.js';
import { persistProjectSelection } from '../../utils/projectLink.js';

function timeAgo(dateStr) {
  if (!dateStr) return '';
  var d = new Date(dateStr);
  var now = new Date();
  var diff = now - d;
  if (diff < 0) return 'just now';
  var secs = Math.floor(diff / 1000);
  if (secs < 60) return 'just now';
  var mins = Math.floor(secs / 60);
  if (mins < 60) return mins + 'm ago';
  var hours = Math.floor(mins / 60);
  if (hours < 24) return hours + 'h ago';
  var days = Math.floor(hours / 24);
  if (days < 30) return days + 'd ago';
  var months = Math.floor(days / 30);
  if (months < 12) return months + 'mo ago';
  return Math.floor(months / 12) + 'y ago';
}

function matchesSearch(text, query) {
  if (!query) return true;
  var lower = (text || '').toLowerCase();
  var terms = query.toLowerCase().split(/\s+/);
  for (var i = 0; i < terms.length; i++) {
    if (terms[i] && lower.indexOf(terms[i]) < 0) return false;
  }
  return true;
}

function looksLikePath(s) {
  return s && (s.charAt(0) === '/' || s.charAt(0) === '~' || s.indexOf('/') >= 0);
}

export default function ProjectPicker({ inline, onClose }) {
  var state = useAppState();
  var dispatch = useDispatch();
  var [recentProjects, setRecentProjects] = useState([]);
  var [loading, setLoading] = useState(true);
  var [error, setError] = useState('');
  var [search, setSearch] = useState('');
  var [selectedIdx, setSelectedIdx] = useState(0);

  // File browser state
  var [browserPath, setBrowserPath] = useState('');
  var [browserParent, setBrowserParent] = useState('');
  var [browserEntries, setBrowserEntries] = useState([]);
  var [browserLoading, setBrowserLoading] = useState(false);
  var [browserError, setBrowserError] = useState('');
  var [showBrowser, setShowBrowser] = useState(false);
  var [showHidden, setShowHidden] = useState(false);
  var [showNewFolder, setShowNewFolder] = useState(false);
  var [newFolderName, setNewFolderName] = useState('');
  var [pathEditing, setPathEditing] = useState(false);
  var [pathInput, setPathInput] = useState('');
  var [browserFilter, setBrowserFilter] = useState('');
  var [pathSearchResults, setPathSearchResults] = useState([]);

  var searchRef = useRef(null);
  var listRef = useRef(null);
  var searchTimerRef = useRef(null);

  // Debounced path search
  useEffect(function () {
    if (!search || !looksLikePath(search)) {
      setPathSearchResults([]);
      return;
    }
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    searchTimerRef.current = setTimeout(function () {
      apiFSSearch(search)
        .then(function (data) {
          setPathSearchResults(Array.isArray(data) ? data : []);
        })
        .catch(function () {
          setPathSearchResults([]);
        });
    }, 200);
    return function () {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    };
  }, [search]);

  // Load recent projects
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

  // Autofocus search
  useEffect(function () {
    if (searchRef.current) searchRef.current.focus();
  }, []);

  // Browse filesystem
  var browse = useCallback(function (targetPath) {
    setBrowserLoading(true);
    setBrowserError('');
    apiFSBrowse(targetPath || '')
      .then(function (data) {
        setBrowserPath(data.path || '');
        setBrowserParent(data.parent || '');
        setBrowserEntries(data.entries || []);
        setPathInput(data.path || '');
        setBrowserFilter('');
      })
      .catch(function (err) {
        setBrowserError(err.message || 'Failed to browse');
      })
      .finally(function () {
        setBrowserLoading(false);
      });
  }, []);

  // Open browser on first expand
  useEffect(function () {
    if (showBrowser && browserEntries.length === 0 && !browserLoading) {
      browse('');
    }
  }, [showBrowser, browserEntries.length, browserLoading, browse]);

  var handleSelectProject = useCallback(function (project) {
    setError('');
    apiProjectOpen(project.path)
      .then(function (data) {
        if (data && data.id) {
          var nextProjectID = String(data.id);
          dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
          dispatch({ type: 'RESET_PROJECT_STATE' });
          persistProjectSelection(nextProjectID);
          apiCall('/api/projects', 'GET', null, { allow404: true })
            .then(function (projects) {
              dispatch({ type: 'SET_PROJECTS', payload: Array.isArray(projects) ? projects : [] });
            })
            .catch(function () {});
          if (onClose) onClose();
        }
      })
      .catch(function (err) {
        setError(err.message || 'Failed to open project');
      });
  }, [dispatch, onClose]);

  var handleSelectRegistered = useCallback(function (project) {
    var nextProjectID = String(project.id || '');
    dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
    dispatch({ type: 'RESET_PROJECT_STATE' });
    persistProjectSelection(nextProjectID);
    if (onClose) onClose();
  }, [dispatch, onClose]);

  var registeredProjects = state.projects || [];

  // Build combined list of projects for search
  var allProjects = useMemo(function () {
    var items = [];
    var seenPaths = {};

    // Registered (open) projects first
    registeredProjects.forEach(function (p) {
      items.push({
        type: 'open',
        id: p.id,
        name: p.name || p.id,
        path: p.path || p.id,
        raw: p,
      });
      if (p.path) seenPaths[p.path] = true;
    });

    // Recent projects (not already in open)
    recentProjects.forEach(function (rp) {
      if (seenPaths[rp.path]) return;
      items.push({
        type: 'recent',
        id: rp.id,
        name: rp.name || rp.id,
        path: rp.path,
        openedAt: rp.opened_at,
        raw: rp,
      });
      seenPaths[rp.path] = true;
    });

    return items;
  }, [registeredProjects, recentProjects]);

  // Filtered projects
  var filteredProjects = useMemo(function () {
    if (!search) return allProjects;
    return allProjects.filter(function (p) {
      return matchesSearch(p.name + ' ' + p.path, search);
    });
  }, [allProjects, search]);

  // Split into sections for display
  var openProjects = useMemo(function () {
    return filteredProjects.filter(function (p) { return p.type === 'open'; });
  }, [filteredProjects]);

  var recentFiltered = useMemo(function () {
    return filteredProjects.filter(function (p) { return p.type === 'recent'; });
  }, [filteredProjects]);

  // Filter browser entries
  var filteredBrowserEntries = useMemo(function () {
    var entries = browserEntries;
    if (!showHidden) {
      entries = entries.filter(function (e) { return e.name.charAt(0) !== '.'; });
    }
    if (browserFilter) {
      entries = entries.filter(function (e) {
        return matchesSearch(e.name, browserFilter);
      });
    }
    return entries;
  }, [browserEntries, showHidden, browserFilter]);

  // Keyboard navigation
  var totalItems = filteredProjects.length;
  useEffect(function () {
    setSelectedIdx(0);
  }, [search]);

  function handleKeyDown(e) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIdx(function (prev) { return Math.min(prev + 1, totalItems - 1); });
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIdx(function (prev) { return Math.max(prev - 1, 0); });
    } else if (e.key === 'Enter') {
      e.preventDefault();
      // If search looks like a path, navigate browser there
      if (looksLikePath(search)) {
        setShowBrowser(true);
        browse(search.charAt(0) === '~' ? search : search);
        return;
      }
      var item = filteredProjects[selectedIdx];
      if (item) {
        if (item.type === 'open') {
          handleSelectRegistered(item.raw);
        } else {
          handleSelectProject(item.raw);
        }
      }
    } else if (e.key === 'Escape') {
      if (onClose) onClose();
    }
  }

  // Scroll selected item into view
  useEffect(function () {
    if (!listRef.current) return;
    var items = listRef.current.querySelectorAll('[data-project-item]');
    if (items[selectedIdx]) {
      items[selectedIdx].scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIdx]);

  // Browser handlers
  function handleBrowserNavigate(entry) {
    if (!entry.is_dir) return;
    browse(browserPath + '/' + entry.name);
  }

  function handleBrowserUp() {
    if (browserParent) browse(browserParent);
  }

  function handleBreadcrumb(targetPath) {
    browse(targetPath);
  }

  function handlePathSubmit(e) {
    e.preventDefault();
    if (pathInput.trim()) {
      browse(pathInput.trim());
    }
    setPathEditing(false);
  }

  function handleNewFolder() {
    var name = newFolderName.trim();
    if (!name) return;
    var folderPath = browserPath + '/' + name;
    apiFSMkdir(folderPath)
      .then(function () {
        setShowNewFolder(false);
        setNewFolderName('');
        browse(browserPath);
      })
      .catch(function (err) {
        setBrowserError(err.message || 'Failed to create folder');
      });
  }

  function handleInitProject(entryName) {
    var initPath = browserPath + '/' + entryName;
    apiProjectInit(initPath)
      .then(function () {
        browse(browserPath);
      })
      .catch(function (err) {
        setBrowserError(err.message || 'Failed to init project');
      });
  }

  function handleOpenProject(entryName) {
    var openPath = browserPath + '/' + entryName;
    apiProjectOpen(openPath)
      .then(function (data) {
        if (data && data.id) {
          var nextProjectID = String(data.id);
          dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
          dispatch({ type: 'RESET_PROJECT_STATE' });
          persistProjectSelection(nextProjectID);
          if (onClose) onClose();
        }
      })
      .catch(function (err) {
        setBrowserError(err.message || 'Failed to open project');
      });
  }

  // Breadcrumbs
  var breadcrumbs = [];
  if (browserPath) {
    var parts = browserPath.split('/').filter(function (p) { return p !== ''; });
    var accumulated = '';
    for (var i = 0; i < parts.length; i++) {
      accumulated += '/' + parts[i];
      breadcrumbs.push({ label: parts[i], path: accumulated });
    }
  }

  function handleRemoveRecent(e, item) {
    e.stopPropagation();
    apiRemoveRecentProject(item.path)
      .then(function () {
        setRecentProjects(function (prev) {
          return prev.filter(function (rp) { return rp.path !== item.path; });
        });
      })
      .catch(function () {});
  }

  function renderProjectCard(item, idx, globalIdx) {
    var isSelected = globalIdx === selectedIdx;
    return (
      <div
        key={item.path + '-' + item.type}
        data-project-item
        onClick={function () {
          if (item.type === 'open') handleSelectRegistered(item.raw);
          else handleSelectProject(item.raw);
        }}
        onMouseEnter={function (e) {
          setSelectedIdx(globalIdx);
          e.currentTarget.style.background = 'var(--bg-2)';
        }}
        onMouseLeave={function (e) {
          e.currentTarget.style.background = isSelected ? 'var(--bg-2)' : 'transparent';
        }}
        style={{
          padding: '10px 12px', cursor: 'pointer',
          border: '1px solid ' + (isSelected ? 'var(--accent)40' : 'var(--border)'),
          borderRadius: 6,
          background: isSelected ? 'var(--bg-2)' : 'transparent',
          transition: 'all 0.1s ease',
          display: 'flex', flexDirection: 'column', gap: 4,
          position: 'relative', minWidth: 0,
        }}
      >
        {/* Remove button for recent items */}
        {item.type === 'recent' && (
          <button
            onClick={function (e) { handleRemoveRecent(e, item); }}
            title="Remove from recent"
            style={{
              position: 'absolute', top: 4, right: 4,
              background: 'none', border: 'none',
              color: 'var(--text-3)', cursor: 'pointer',
              fontSize: 11, padding: '0 3px', lineHeight: 1,
              fontFamily: "'JetBrains Mono', monospace",
              opacity: 0.5, borderRadius: 3,
            }}
            onMouseEnter={function (e) { e.currentTarget.style.opacity = '1'; e.currentTarget.style.color = '#ef4444'; }}
            onMouseLeave={function (e) { e.currentTarget.style.opacity = '0.5'; e.currentTarget.style.color = 'var(--text-3)'; }}
          >{'\u00D7'}</button>
        )}

        {/* Top row: icon + name + badge */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
          <span style={{
            fontSize: 10, flexShrink: 0,
            color: item.type === 'open' ? 'var(--accent)' : 'var(--text-2)',
          }}>{item.type === 'open' ? '\u25C6' : '\u25CB'}</span>
          <span style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
            color: item.type === 'open' ? 'var(--text-0)' : 'var(--text-1)',
            fontWeight: item.type === 'open' ? 600 : 400,
            overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
            flex: 1, minWidth: 0,
          }}>
            {search ? highlightMatch(item.name, search) : item.name}
          </span>
          {item.type === 'open' && (
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
              padding: '1px 5px', borderRadius: 3,
              background: 'rgba(74,230,138,0.1)', color: 'var(--green)',
              flexShrink: 0,
            }}>active</span>
          )}
        </div>

        {/* Path */}
        <div style={{
          fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
          color: 'var(--text-3)',
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          paddingLeft: 16,
        }}>
          {search ? highlightMatch(shortenPath(item.path), search) : shortenPath(item.path)}
        </div>

        {/* Time ago */}
        {item.openedAt && (
          <div style={{
            fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
            color: 'var(--text-3)', paddingLeft: 16, opacity: 0.7,
          }}>{timeAgo(item.openedAt)}</div>
        )}
      </div>
    );
  }

  var content = (
    <div style={{
      width: '100%', maxWidth: 720, padding: 0,
      background: 'var(--bg-1)', border: '1px solid var(--border)', borderRadius: 8,
      display: 'flex', flexDirection: 'column',
      maxHeight: 'calc(100vh - 80px)',
      boxShadow: '0 20px 60px rgba(0,0,0,0.5)',
    }}>
      {/* Header */}
      <div style={{
        padding: '16px 20px 12px', borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      }}>
        <h2 style={{
          margin: 0, fontFamily: "'JetBrains Mono', monospace",
          fontSize: 14, fontWeight: 600, color: 'var(--text-0)',
        }}>Open Project</h2>
        {onClose && (
          <button
            onClick={onClose}
            style={{
              background: 'none', border: 'none', color: 'var(--text-3)',
              cursor: 'pointer', fontSize: 16, padding: '0 4px',
              fontFamily: "'JetBrains Mono', monospace",
            }}
          >X</button>
        )}
      </div>

      {/* Search bar */}
      <div style={{ padding: '12px 20px 8px' }}>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '8px 12px',
          background: 'var(--bg-0)', border: '1px solid var(--border)',
          borderRadius: 6,
        }}>
          <span style={{
            fontSize: 13, color: 'var(--text-3)', flexShrink: 0,
            fontFamily: "'JetBrains Mono', monospace",
          }}>{'\u2315'}</span>
          <input
            ref={searchRef}
            type="text"
            value={search}
            onChange={function (e) { setSearch(e.target.value); }}
            onKeyDown={handleKeyDown}
            placeholder="Search projects or type a path..."
            style={{
              flex: 1, background: 'transparent', border: 'none', outline: 'none',
              color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 12,
            }}
          />
          {search && (
            <button
              onClick={function () { setSearch(''); if (searchRef.current) searchRef.current.focus(); }}
              style={{
                background: 'none', border: 'none', color: 'var(--text-3)',
                cursor: 'pointer', fontSize: 12, padding: 0,
                fontFamily: "'JetBrains Mono', monospace",
              }}
            >X</button>
          )}
        </div>
      </div>

      {state.unresolvedProjectID && (
        <div style={{
          padding: '6px 20px', margin: '0 20px 8px',
          borderRadius: 4,
          background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
          fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: '#ef4444',
        }}>
          Project "{state.unresolvedProjectID}" not found
        </div>
      )}

      {error && (
        <div style={{
          padding: '6px 10px', margin: '0 20px 8px',
          borderRadius: 4,
          background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
          fontFamily: "'JetBrains Mono', monospace", fontSize: 10, color: '#ef4444',
        }}>{error}</div>
      )}

      {/* Projects list */}
      <div ref={listRef} style={{
        flex: 1, overflow: 'auto', minHeight: 0,
        padding: '0 20px',
      }}>
        {loading ? (
          <div style={{
            padding: 24, textAlign: 'center', color: 'var(--text-3)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          }}>Loading projects...</div>
        ) : filteredProjects.length === 0 && search && !looksLikePath(search) ? (
          <div style={{
            padding: 24, textAlign: 'center', color: 'var(--text-3)',
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
          }}>No projects match "{search}"</div>
        ) : (
          <>
            {/* Open projects section */}
            {openProjects.length > 0 && (
              <div style={{ marginBottom: 12 }}>
                <div style={sectionLabelStyle}>Open Projects</div>
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(2, 1fr)',
                  gap: 6,
                }}>
                  {openProjects.map(function (p, idx) {
                    return renderProjectCard(p, idx, filteredProjects.indexOf(p));
                  })}
                </div>
              </div>
            )}

            {/* Recent projects section */}
            {recentFiltered.length > 0 && (
              <div style={{ marginBottom: 12 }}>
                <div style={sectionLabelStyle}>Recent Projects</div>
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(2, 1fr)',
                  gap: 6,
                  maxHeight: 280, overflowY: 'auto',
                  paddingRight: 2,
                }}>
                  {recentFiltered.map(function (p, idx) {
                    return renderProjectCard(p, idx, filteredProjects.indexOf(p));
                  })}
                </div>
              </div>
            )}

            {/* Path search results */}
            {search && looksLikePath(search) && (
              <div style={{ marginBottom: 8 }}>
                {pathSearchResults.length > 0 && (
                  <>
                    <div style={sectionLabelStyle}>Filesystem Matches</div>
                    <div style={{
                      border: '1px solid var(--border)', borderRadius: 4, overflow: 'hidden',
                      maxHeight: 200, overflowY: 'auto',
                    }}>
                      {pathSearchResults.map(function (entry) {
                        return (
                          <div
                            key={entry.full_path || entry.name}
                            style={{
                              display: 'flex', alignItems: 'center', gap: 8,
                              padding: '6px 12px', borderBottom: '1px solid var(--bg-3)',
                              cursor: 'pointer',
                            }}
                            onClick={function () {
                              if (entry.is_project && entry.full_path) {
                                apiProjectOpen(entry.full_path)
                                  .then(function (data) {
                                    if (data && data.id) {
                                      var nextProjectID = String(data.id);
                                      dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
                                      dispatch({ type: 'RESET_PROJECT_STATE' });
                                      persistProjectSelection(nextProjectID);
                                      if (onClose) onClose();
                                    }
                                  })
                                  .catch(function (err) {
                                    setError(err.message || 'Failed to open project');
                                  });
                              } else if (entry.full_path) {
                                setShowBrowser(true);
                                browse(entry.full_path);
                                setSearch('');
                              }
                            }}
                            onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
                            onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
                          >
                            <span style={{
                              fontSize: 11, flexShrink: 0,
                              color: entry.is_project ? 'var(--accent)' : 'var(--text-2)',
                            }}>{entry.is_project ? '\u25C6' : '\u25B7'}</span>
                            <div style={{ flex: 1, minWidth: 0 }}>
                              <div style={{
                                fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                                color: entry.is_project ? 'var(--text-0)' : 'var(--text-1)',
                                fontWeight: entry.is_project ? 600 : 400,
                                overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                              }}>{entry.name}</div>
                              {entry.full_path && (
                                <div style={{
                                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                                  color: 'var(--text-3)', marginTop: 1,
                                  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                                }}>{shortenPath(entry.full_path)}</div>
                              )}
                            </div>
                            {entry.is_project && (
                              <span style={{
                                fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
                                padding: '1px 5px', borderRadius: 3,
                                background: 'var(--accent)15', color: 'var(--accent)',
                                flexShrink: 0,
                              }}>project</span>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </>
                )}
                <div style={{
                  padding: '8px 12px', marginTop: pathSearchResults.length > 0 ? 4 : 0,
                  border: '1px solid var(--border)', borderRadius: 4,
                  background: 'var(--bg-2)',
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                  color: 'var(--text-2)', cursor: 'pointer',
                  display: 'flex', alignItems: 'center', gap: 8,
                }}
                onClick={function () {
                  setShowBrowser(true);
                  browse(search);
                  setSearch('');
                }}
                onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-3)'; }}
                onMouseLeave={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
                >
                  <span style={{ color: 'var(--accent)' }}>{'\u2192'}</span>
                  <span>Browse to <strong style={{ color: 'var(--text-0)' }}>{search}</strong></span>
                  <span style={{ color: 'var(--text-3)', fontSize: 9, marginLeft: 'auto' }}>Enter</span>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* File Browser Section */}
      <div style={{
        borderTop: '1px solid var(--border)',
        background: 'var(--bg-0)',
        borderRadius: '0 0 8px 8px',
      }}>
        {/* Browser toggle header */}
        <div
          onClick={function () { setShowBrowser(!showBrowser); }}
          style={{
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            padding: '10px 20px', cursor: 'pointer',
          }}
          onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-1)'; }}
          onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
        >
          <div style={{
            display: 'flex', alignItems: 'center', gap: 8,
            fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
            color: 'var(--text-2)',
          }}>
            <span style={{
              fontSize: 9, transition: 'transform 0.15s ease',
              transform: showBrowser ? 'rotate(90deg)' : 'rotate(0)',
              display: 'inline-block',
            }}>{'\u25B6'}</span>
            <span>Browse Filesystem</span>
          </div>
          {browserPath && showBrowser && (
            <span style={{
              fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
              color: 'var(--text-3)',
            }}>{shortenPath(browserPath)}</span>
          )}
        </div>

        {showBrowser && (
          <div style={{ padding: '0 20px 16px' }}>
            {/* Path bar */}
            <div style={{
              display: 'flex', alignItems: 'center', gap: 4, marginBottom: 8,
              padding: '6px 10px',
              background: 'var(--bg-1)', border: '1px solid var(--border)',
              borderRadius: 4, minHeight: 28,
            }}>
              {pathEditing ? (
                <form onSubmit={handlePathSubmit} style={{ display: 'flex', flex: 1, gap: 4 }}>
                  <input
                    type="text"
                    value={pathInput}
                    onChange={function (e) { setPathInput(e.target.value); }}
                    autoFocus
                    onBlur={function () { setPathEditing(false); }}
                    style={{
                      flex: 1, background: 'transparent', border: 'none', outline: 'none',
                      color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                    }}
                  />
                </form>
              ) : (
                <div
                  onClick={function () { setPathEditing(true); setPathInput(browserPath); }}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 2, flex: 1,
                    fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                    cursor: 'text', flexWrap: 'wrap',
                  }}
                >
                  <span
                    onClick={function (e) { e.stopPropagation(); handleBreadcrumb('/'); }}
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
                          onClick={isLast ? function (e) { e.stopPropagation(); } : function (e) { e.stopPropagation(); handleBreadcrumb(crumb.path); }}
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
              )}
            </div>

            {browserError && (
              <div style={{
                padding: '6px 10px', marginBottom: 8, borderRadius: 4,
                background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.3)',
                color: '#ef4444', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
              }}>{browserError}</div>
            )}

            {/* Browser toolbar */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8 }}>
              {browserParent && (
                <button onClick={handleBrowserUp} style={toolBtnStyle}>
                  {'\u2191'} Up
                </button>
              )}
              <button
                onClick={function () { setShowNewFolder(!showNewFolder); }}
                style={toolBtnStyle}
              >+ New Folder</button>
              <div style={{ flex: 1 }} />
              <div style={{
                display: 'flex', alignItems: 'center', gap: 4,
                padding: '3px 8px',
                background: 'var(--bg-1)', border: '1px solid var(--border)',
                borderRadius: 4,
              }}>
                <span style={{
                  fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                  color: 'var(--text-3)',
                }}>{'\u2315'}</span>
                <input
                  type="text"
                  value={browserFilter}
                  onChange={function (e) { setBrowserFilter(e.target.value); }}
                  placeholder="Filter..."
                  style={{
                    width: 80, background: 'transparent', border: 'none', outline: 'none',
                    color: 'var(--text-0)', fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
                  }}
                />
              </div>
              <label style={{
                display: 'flex', alignItems: 'center', gap: 4, cursor: 'pointer',
                fontFamily: "'JetBrains Mono', monospace", fontSize: 9,
                color: 'var(--text-3)',
              }}>
                <input
                  type="checkbox"
                  checked={showHidden}
                  onChange={function (e) { setShowHidden(e.target.checked); }}
                  style={{ width: 11, height: 11, margin: 0 }}
                />
                Hidden
              </label>
            </div>

            {/* New folder form */}
            {showNewFolder && (
              <div style={{ display: 'flex', gap: 6, marginBottom: 8 }}>
                <input
                  type="text"
                  value={newFolderName}
                  onChange={function (e) { setNewFolderName(e.target.value); }}
                  onKeyDown={function (e) { if (e.key === 'Enter') handleNewFolder(); if (e.key === 'Escape') { setShowNewFolder(false); setNewFolderName(''); } }}
                  placeholder="Folder name"
                  autoFocus
                  style={{
                    flex: 1, padding: '4px 8px', background: 'var(--bg-1)',
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
              maxHeight: 280, overflow: 'auto',
              border: '1px solid var(--border)', borderRadius: 4,
              background: 'var(--bg-1)',
            }}>
              {browserLoading ? (
                <div style={emptyStyle}>Loading...</div>
              ) : filteredBrowserEntries.length === 0 ? (
                <div style={emptyStyle}>
                  {browserFilter ? 'No matches for "' + browserFilter + '"' : 'Empty directory'}
                </div>
              ) : filteredBrowserEntries.map(function (entry) {
                var icon = entry.is_project ? '\u25C6' : entry.is_dir ? '\u25B7' : '\u2500';
                var iconColor = entry.is_project ? 'var(--accent)' : entry.is_dir ? 'var(--text-2)' : 'var(--text-3)';
                var nameColor = entry.is_project ? 'var(--text-0)' : entry.is_dir ? 'var(--text-1)' : 'var(--text-2)';
                var nameWeight = entry.is_project ? 600 : 400;

                return (
                  <div
                    key={entry.name}
                    style={{
                      display: 'flex', alignItems: 'center', gap: 8,
                      padding: '5px 12px', borderBottom: '1px solid var(--bg-3)',
                      cursor: entry.is_dir ? 'pointer' : 'default',
                    }}
                    onClick={entry.is_dir ? function () { handleBrowserNavigate(entry); } : undefined}
                    onMouseEnter={function (e) { e.currentTarget.style.background = 'var(--bg-2)'; }}
                    onMouseLeave={function (e) { e.currentTarget.style.background = 'transparent'; }}
                  >
                    <span style={{ fontSize: 11, flexShrink: 0, color: iconColor }}>{icon}</span>
                    <span style={{
                      flex: 1, fontFamily: "'JetBrains Mono', monospace", fontSize: 11,
                      color: nameColor, fontWeight: nameWeight,
                      overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                    }}>
                      {browserFilter ? highlightMatch(entry.name, browserFilter) : entry.name}
                    </span>
                    {entry.is_project && (
                      <span style={{
                        fontFamily: "'JetBrains Mono', monospace", fontSize: 8,
                        padding: '1px 5px', borderRadius: 3,
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
                          border: '1px solid var(--border)',
                          color: 'var(--text-2)',
                        }}
                      >Init</button>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  );

  if (inline) return content;

  return (
    <div style={{
      width: '100vw', height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg-0)',
    }}>
      {content}
    </div>
  );
}

function highlightMatch(text, query) {
  if (!query || !text) return text;
  var terms = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (terms.length === 0) return text;

  // Find all match ranges
  var lower = text.toLowerCase();
  var ranges = [];
  for (var t = 0; t < terms.length; t++) {
    var term = terms[t];
    var pos = 0;
    while (true) {
      var idx = lower.indexOf(term, pos);
      if (idx < 0) break;
      ranges.push([idx, idx + term.length]);
      pos = idx + 1;
    }
  }

  if (ranges.length === 0) return text;

  // Merge overlapping ranges
  ranges.sort(function (a, b) { return a[0] - b[0]; });
  var merged = [ranges[0]];
  for (var i = 1; i < ranges.length; i++) {
    var last = merged[merged.length - 1];
    if (ranges[i][0] <= last[1]) {
      last[1] = Math.max(last[1], ranges[i][1]);
    } else {
      merged.push(ranges[i]);
    }
  }

  // Build result
  var parts = [];
  var prev = 0;
  for (var j = 0; j < merged.length; j++) {
    if (merged[j][0] > prev) {
      parts.push(<span key={'t' + j}>{text.slice(prev, merged[j][0])}</span>);
    }
    parts.push(
      <span key={'h' + j} style={{ color: 'var(--accent)', fontWeight: 600 }}>
        {text.slice(merged[j][0], merged[j][1])}
      </span>
    );
    prev = merged[j][1];
  }
  if (prev < text.length) {
    parts.push(<span key="end">{text.slice(prev)}</span>);
  }
  return parts;
}

function shortenPath(path) {
  if (!path) return '';
  try {
    // Replace home dir prefix with ~
    var home = null;
    if (path.indexOf('/home/') === 0) {
      var parts = path.split('/');
      if (parts.length >= 3) {
        home = '/' + parts[1] + '/' + parts[2];
      }
    } else if (path.indexOf('/Users/') === 0) {
      var parts2 = path.split('/');
      if (parts2.length >= 3) {
        home = '/' + parts2[1] + '/' + parts2[2];
      }
    }
    if (home && path.indexOf(home) === 0) {
      return '~' + path.slice(home.length);
    }
  } catch (_) {}
  return path;
}

var sectionLabelStyle = {
  fontFamily: "'JetBrains Mono', monospace", fontSize: 10,
  color: 'var(--text-3)', marginBottom: 4, textTransform: 'uppercase',
  letterSpacing: '0.05em', padding: '4px 0',
};

var toolBtnStyle = {
  padding: '3px 8px', border: '1px solid var(--border)',
  background: 'var(--bg-1)', borderRadius: 4, cursor: 'pointer',
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
