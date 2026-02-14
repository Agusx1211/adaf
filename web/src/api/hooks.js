import { useEffect, useRef, useCallback } from 'react';
import { apiCall, apiBase } from './client.js';
import { useAppState, useDispatch, normalizeSessions, normalizeSpawns, normalizeIssues, normalizeDocs, normalizePlans, normalizePlan, normalizeTurns, normalizeLoopMessages, pickActiveLoopRun, aggregateUsageFromProfileStats } from '../state/store.js';
import { arrayOrEmpty, normalizeStatus, parseTimestamp } from '../utils/format.js';

var POLL_MS = 5000;

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
      ]);

      if (!mountedRef.current) return;

      var projectMeta = results[0] || null;
      var sessions = normalizeSessions(results[1]);
      var spawns = normalizeSpawns(results[2]);
      var loopRun = pickActiveLoopRun(results[3]);

      var usageFromStats = aggregateUsageFromProfileStats(results[4]);

      dispatch({
        type: 'SET_CORE_DATA',
        payload: { projectMeta, sessions, spawns, loopRun, usage: usageFromStats },
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
      } else if (viewName === 'docs') {
        var docs = normalizeDocs(await apiCall(base + '/docs', 'GET', null, { allow404: true }));
        dispatch({ type: 'SET', payload: { docs } });
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
    if (view && view !== 'agents') {
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

export function useInitProjects() {
  var dispatch = useDispatch();

  useEffect(function () {
    apiCall('/api/projects', 'GET', null, { allow404: true })
      .then(function (projects) {
        var list = arrayOrEmpty(projects);
        dispatch({ type: 'SET_PROJECTS', payload: list });

        var savedID = '';
        try { savedID = localStorage.getItem('adaf_project_id') || ''; } catch (_) {}

        if (savedID && list.find(function (p) { return p && String(p.id || '') === savedID; })) {
          dispatch({ type: 'SET_PROJECT_ID', payload: savedID });
        } else {
          var defaultProject = list.find(function (p) { return !!(p && p.is_default); }) || list[0] || null;
          dispatch({ type: 'SET_PROJECT_ID', payload: defaultProject && defaultProject.id ? String(defaultProject.id) : '' });
        }
      })
      .catch(function (err) {
        if (err && err.authRequired) {
          dispatch({ type: 'SET', payload: { authRequired: true } });
        }
      });
  }, [dispatch]);
}
