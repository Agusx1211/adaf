import { useEffect, useRef, useCallback } from 'react';
import { apiCall, apiBase } from './client.js';
import { useAppState, useDispatch, normalizeSessions, normalizeSpawns, normalizeIssues, normalizeWiki, normalizePlans, normalizePlan, normalizeTurns, normalizeLoopMessages, pickActiveLoopRun, normalizeAllLoopRuns, aggregateUsageFromProfileStats } from '../state/store.js';
import { arrayOrEmpty, normalizeStatus, parseTimestamp } from '../utils/format.js';
import { readProjectIDFromURL, persistProjectSelection } from '../utils/projectLink.js';

var POLL_MS = 5000;
var USAGE_POLL_MS = 60000;
var HISTORY_TAIL_LINES = 2000;

export function usePolling() {
  var state = useAppState();
  var dispatch = useDispatch();
  var timerRef = useRef(null);
  var mountedRef = useRef(true);

  var refresh = useCallback(async function (initial) {
    if (!mountedRef.current) return;
    var base = apiBase(state.currentProjectID);

    try {
      var results = await Promise.all([
        apiCall(base + '/project', 'GET', null, { allow404: true }),
        apiCall(base + '/sessions', 'GET', null, { allow404: true }),
        apiCall(base + '/spawns', 'GET', null, { allow404: true }),
        apiCall(base + '/loops', 'GET', null, { allow404: true }),
        apiCall(base + '/stats/profiles', 'GET', null, { allow404: true }),
        apiCall(base + '/turns?limit=1000', 'GET', null, { allow404: true }),
      ]);

      if (!mountedRef.current) return;

      var projectMeta = results[0] || null;
      var sessions = normalizeSessions(results[1]);
      var spawns = normalizeSpawns(results[2]);
      var loopRun = pickActiveLoopRun(results[3]);
      var loopRuns = normalizeAllLoopRuns(results[3]);
      var turns = normalizeTurns(results[5]);

      var usageFromStats = aggregateUsageFromProfileStats(results[4]);

      dispatch({
        type: 'SET_CORE_DATA',
        payload: { projectMeta, sessions, spawns, turns, loopRun, loopRuns, usage: usageFromStats },
      });

      return { loopRun, sessions, spawns };
    } catch (err) {
      if (err && err.authRequired) {
        dispatch({ type: 'SET', payload: { authRequired: true } });
      }
      throw err;
    }
  }, [state.currentProjectID, dispatch]);

  useEffect(function () {
    mountedRef.current = true;
    return function () { mountedRef.current = false; };
  }, []);

  useEffect(function () {
    refresh(true).catch(function () {});

    timerRef.current = setInterval(function () {
      refresh(false).catch(function () {});
    }, POLL_MS);

    return function () {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [refresh]);

  return refresh;
}

export function useViewData(view, projectID) {
  var dispatch = useDispatch();
  var loadedRef = useRef({});

  var loadView = useCallback(async function (viewName) {
    if (loadedRef.current[viewName]) return;
    var base = apiBase(projectID);

    try {
      if (viewName === 'issues') {
        var issues = normalizeIssues(await apiCall(base + '/issues', 'GET', null, { allow404: true }));
        dispatch({ type: 'SET', payload: { issues } });
      } else if (viewName === 'wiki') {
        var wiki = normalizeWiki(await apiCall(base + '/wiki', 'GET', null, { allow404: true }));
        dispatch({ type: 'SET', payload: { wiki } });
      } else if (viewName === 'plan') {
        var plans = normalizePlans(await apiCall(base + '/plans', 'GET', null, { allow404: true }));
        dispatch({ type: 'SET', payload: { plans } });
      } else if (viewName === 'logs') {
        var turns = normalizeTurns(await apiCall(base + '/turns', 'GET', null, { allow404: true }));
        dispatch({ type: 'SET', payload: { turns } });
      }
      loadedRef.current[viewName] = true;
    } catch (err) {
      if (err && err.authRequired) {
        dispatch({ type: 'SET', payload: { authRequired: true } });
      }
    }
  }, [projectID, dispatch]);

  useEffect(function () {
    if (view && view !== 'loops') {
      loadView(view);
    }
  }, [view, loadView]);

  // Reset loaded state when project changes
  useEffect(function () {
    loadedRef.current = {};
  }, [projectID]);

  return loadView;
}

export function usePlanDetail(planID, projectID) {
  var dispatch = useDispatch();

  useEffect(function () {
    if (!planID) return;
    var base = apiBase(projectID);
    apiCall(base + '/plans/' + encodeURIComponent(planID), 'GET', null, { allow404: true })
      .then(function (detail) {
        if (detail) {
          dispatch({ type: 'SET', payload: { activePlan: normalizePlan(detail) } });
        }
      })
      .catch(function () {});
  }, [planID, projectID, dispatch]);
}

export function useLoopMessages(loopID, projectID) {
  var dispatch = useDispatch();

  useEffect(function () {
    if (!loopID) {
      dispatch({ type: 'SET', payload: { messages: [] } });
      return;
    }
    var base = apiBase(projectID);
    apiCall(base + '/loops/' + encodeURIComponent(String(loopID)) + '/messages', 'GET', null, { allow404: true })
      .then(function (list) {
        dispatch({ type: 'SET', payload: { messages: normalizeLoopMessages(arrayOrEmpty(list)) } });
      })
      .catch(function () {});
  }, [loopID, projectID, dispatch]);
}

export function fetchTurnRecordingEvents(turnID, projectID, dispatch) {
  var base = apiBase(projectID);
  var url = base + '/turns/' + encodeURIComponent(String(turnID)) + '/events';
  return fetchRecordingEvents(url, turnID, dispatch);
}

export function fetchSessionRecordingEvents(sessionID, projectID, dispatch) {
  var base = apiBase(projectID);
  var url = base + '/sessions/' + encodeURIComponent(String(sessionID)) + '/events?tail=' + encodeURIComponent(String(HISTORY_TAIL_LINES));
  return fetchRecordingEvents(url, sessionID, dispatch);
}

function fetchRecordingEvents(url, cacheID, dispatch) {
  var headers = { Accept: 'application/x-ndjson' };
  var token = '';
  try { token = localStorage.getItem('adaf_token') || ''; } catch (_) {}
  if (token) headers.Authorization = 'Bearer ' + token;

  return fetch(url, { headers: headers })
    .then(function (res) {
      if (!res.ok) return null;
      return res.text();
    })
    .then(function (text) {
      if (!text) return;
      var events = [];
      var lines = text.split('\n');
      for (var i = 0; i < lines.length; i++) {
        var line = lines[i].trim();
        if (!line) continue;
        try {
          var ev = JSON.parse(line);
          events.push(ev);
        } catch (_) {}
      }
      dispatch({ type: 'SET_HISTORICAL_EVENTS', payload: { turnID: cacheID, events: events } });
    })
    .catch(function () {});
}

export function useInitProjects() {
  var dispatch = useDispatch();

  useEffect(function () {
    apiCall('/api/projects', 'GET', null, { allow404: true })
      .then(function (projects) {
        var list = arrayOrEmpty(projects);
        dispatch({ type: 'SET_PROJECTS', payload: list });

        var urlID = readProjectIDFromURL();
        var savedID = '';
        try { savedID = localStorage.getItem('adaf_project_id') || ''; } catch (_) {}

        var nextProjectID = '';
        if (urlID && list.find(function (p) { return p && String(p.id || '') === urlID; })) {
          nextProjectID = urlID;
        } else if (savedID && list.find(function (p) { return p && String(p.id || '') === savedID; })) {
          nextProjectID = savedID;
        } else {
          var defaultProject = list.find(function (p) { return !!(p && p.is_default); }) || list[0] || null;
          nextProjectID = defaultProject && defaultProject.id ? String(defaultProject.id) : '';
        }

        // Detect unresolved project: URL has ?project=X but X is not registered
        if (urlID && !nextProjectID) {
          dispatch({ type: 'SET', payload: { needsProjectPicker: true, unresolvedProjectID: urlID } });
        } else if (!nextProjectID && list.length === 0) {
          // No projects at all — show picker
          dispatch({ type: 'SET', payload: { needsProjectPicker: true, unresolvedProjectID: '' } });
        }

        dispatch({ type: 'SET_PROJECT_ID', payload: nextProjectID });
        persistProjectSelection(nextProjectID);
      })
      .catch(function (err) {
        if (err && err.authRequired) {
          dispatch({ type: 'SET', payload: { authRequired: true } });
        }
      });
  }, [dispatch]);
}

export function useUsageLimits() {
  var dispatch = useDispatch();
  var timerRef = useRef(null);
  var mountedRef = useRef(true);

  var fetchUsage = useCallback(async function () {
    if (!mountedRef.current) return;
    try {
      var data = await apiCall('/api/usage', 'GET', null, { allow404: true });
      if (!mountedRef.current) return;
      dispatch({ type: 'SET', payload: { usageLimits: data || null } });
    } catch (err) {
      // Silently ignore usage errors
    }
  }, [dispatch]);

  useEffect(function () {
    mountedRef.current = true;
    fetchUsage();

    timerRef.current = setInterval(function () {
      fetchUsage();
    }, USAGE_POLL_MS);

    return function () {
      mountedRef.current = false;
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [fetchUsage]);

  return fetchUsage;
}
