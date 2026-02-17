import { createContext, useContext, useReducer, useCallback } from 'react';
import { arrayOrEmpty, normalizeStatus, numberOr, parseTimestamp, safeJSONString, stringifyToolPayload, cropText } from '../utils/format.js';

var MAX_STREAM_EVENTS = 0; // 0 means unlimited
var MAX_ACTIVITY_EVENTS = 240;

var initialState = {
  projects: [],
  currentProjectID: '',
  authRequired: false,

  sessions: [],
  spawns: [],
  messages: [],
  streamEvents: [],
  activity: [],
  activityLast: null,
  issues: [],
  plans: [],
  activePlan: null,
  docs: [],
  turns: [],
  loopRun: null,
  usage: null,
  projectMeta: null,
  usageLimits: null,

  selectedScope: null,
  selectedIssue: null,
  selectedPlan: null,
  selectedDoc: null,
  selectedTurn: null,
  expandedNodes: {},
  leftView: 'loops',
  rightLayer: 'raw',
  autoScroll: true,

  wsConnected: false,
  currentSessionSocketID: 0,
  termWSConnected: false,

  sessionMessageDraft: '',
  activeLoopIDForMessages: 0,
  viewLoaded: { issues: false, docs: false, plan: false, logs: false },

  configSelection: null, // { type: 'profile'|'loop'|'team'|'skill'|'role', name: string } or null
  standaloneChatID: '', // active chat instance ID
  standaloneChatStatuses: {}, // { [chatID]: 'thinking' | 'responding' }
  loopRuns: [], // all loop runs (not just active)
  historicalEvents: {}, // { [turnID]: [ event, ... ] } cached recording events
  needsProjectPicker: false, // show project picker overlay
  unresolvedProjectID: '', // project ID from URL that wasn't found
};

function reducer(state, action) {
  switch (action.type) {
    case 'SET':
      return { ...state, ...action.payload };

    case 'SET_PROJECTS':
      return { ...state, projects: action.payload };

    case 'SET_PROJECT_ID':
      return { ...state, currentProjectID: action.payload };

    case 'SET_CORE_DATA': {
      var d = action.payload;
      return { ...state, ...d };
    }

    case 'SET_LEFT_VIEW':
      return { ...state, leftView: action.payload };

    case 'SET_RIGHT_LAYER':
      return { ...state, rightLayer: action.payload };

    case 'SET_SELECTED_SCOPE':
      return { ...state, selectedScope: action.payload };

    case 'SET_SELECTED_ISSUE':
      return { ...state, selectedIssue: action.payload };

    case 'SET_SELECTED_PLAN':
      return { ...state, selectedPlan: action.payload };

    case 'SET_SELECTED_DOC':
      return { ...state, selectedDoc: action.payload };

    case 'SET_SELECTED_TURN':
      return { ...state, selectedTurn: action.payload };

    case 'SET_CONFIG_SELECTION':
      return { ...state, configSelection: action.payload };

    case 'SET_STANDALONE_CHAT_ID':
      return { ...state, standaloneChatID: action.payload || '' };

    case 'SET_STANDALONE_CHAT_STATUS': {
      var statusChatID = action.payload.chatID;
      var chatStatus = action.payload.status;
      var nextStatuses = { ...state.standaloneChatStatuses };
      if (!chatStatus || chatStatus === 'idle') {
        delete nextStatuses[statusChatID];
      } else {
        nextStatuses[statusChatID] = chatStatus;
      }
      return { ...state, standaloneChatStatuses: nextStatuses };
    }

    case 'SET_LOOP_RUNS':
      return { ...state, loopRuns: action.payload };

    case 'SET_HISTORICAL_EVENTS': {
      var hTurnID = action.payload.turnID;
      var hEvents = action.payload.events;
      var nextHistorical = { ...state.historicalEvents };
      nextHistorical[hTurnID] = hEvents;
      return { ...state, historicalEvents: nextHistorical };
    }

    case 'TOGGLE_NODE': {
      var nodeID = action.payload;
      var next = { ...state.expandedNodes };
      // Use explicit true/false so nodes with either default can be toggled.
      next[nodeID] = !next[nodeID];
      return { ...state, expandedNodes: next };
    }

    case 'EXPAND_NODES': {
      var nodes = { ...state.expandedNodes };
      action.payload.forEach(function (id) { nodes[id] = true; });
      return { ...state, expandedNodes: nodes };
    }

    case 'TOGGLE_AUTO_SCROLL':
      return { ...state, autoScroll: !state.autoScroll };

    case 'ADD_STREAM_EVENT': {
      var entry = action.payload;
      var events = state.streamEvents;
      var last = events[events.length - 1];
      if (last && last.scope === entry.scope && last.type === entry.type && last.text === entry.text && last.tool === entry.tool) {
        return state;
      }
      var nextEvents = events.concat([entry]);
      if (MAX_STREAM_EVENTS > 0 && nextEvents.length > MAX_STREAM_EVENTS) {
        nextEvents = nextEvents.slice(nextEvents.length - MAX_STREAM_EVENTS);
      }

      var nextActivity = state.activity;
      var nextActivityLast = state.activityLast;
      var actEntry = buildActivityEntry(entry);
      if (actEntry) {
        if (!nextActivityLast || nextActivityLast.scope !== actEntry.scope || nextActivityLast.type !== actEntry.type || nextActivityLast.text !== actEntry.text) {
          nextActivity = state.activity.concat([actEntry]);
          if (nextActivity.length > MAX_ACTIVITY_EVENTS) {
            nextActivity = nextActivity.slice(nextActivity.length - MAX_ACTIVITY_EVENTS);
          }
          nextActivityLast = actEntry;
        }
      }

      return { ...state, streamEvents: nextEvents, activity: nextActivity, activityLast: nextActivityLast };
    }

    case 'MERGE_SPAWNS': {
      return { ...state, spawns: action.payload };
    }

    case 'RESET_PROJECT_STATE':
      return {
        ...state,
        sessions: [], spawns: [], messages: [], streamEvents: [], activity: [], activityLast: null,
        issues: [], plans: [], activePlan: null, docs: [], turns: [], loopRun: null, loopRuns: [], usage: null,
        selectedIssue: null, selectedPlan: null, selectedDoc: null, selectedTurn: null, selectedScope: null,
        expandedNodes: {}, projectMeta: null, activeLoopIDForMessages: 0, standaloneChatStatuses: {},
        historicalEvents: {},
        viewLoaded: { issues: false, docs: false, plan: false, logs: false },
        needsProjectPicker: false, unresolvedProjectID: '',
      };

    default:
      return state;
  }
}

function buildActivityEntry(event) {
  if (!event) return null;
  var type = event.type || 'text';
  if (type === 'thinking') return null;

  var description = '';
  if (type === 'tool_use') {
    description = (event.tool || 'tool') + ' \u2192 ' + stringifyToolPayload(event.input || '');
  } else if (type === 'tool_result') {
    description = (event.tool || 'result') + ': ' + stringifyToolPayload(event.result || event.text || '');
  } else {
    description = String(event.text || '').trim();
  }
  if (!description) return null;

  return {
    id: event.id || (Date.now().toString(36) + Math.random().toString(36).slice(2, 8)),
    ts: Number.isFinite(Number(event.ts)) ? Number(event.ts) : Date.now(),
    scope: event.scope || 'session-0',
    type: type,
    text: cropText(description, 200),
  };
}

var AppContext = createContext(null);

export function AppProvider({ children }) {
  var [state, dispatch] = useReducer(reducer, initialState);
  return (
    <AppContext.Provider value={{ state, dispatch }}>
      {children}
    </AppContext.Provider>
  );
}

export function useAppState() {
  var ctx = useContext(AppContext);
  if (!ctx) throw new Error('useAppState must be used within AppProvider');
  return ctx.state;
}

export function useDispatch() {
  var ctx = useContext(AppContext);
  if (!ctx) throw new Error('useDispatch must be used within AppProvider');
  return ctx.dispatch;
}

export function useApp() {
  var ctx = useContext(AppContext);
  if (!ctx) throw new Error('useApp must be used within AppProvider');
  return ctx;
}

// Normalize functions ported from app.js

export function normalizeSessions(rawSessions) {
  return arrayOrEmpty(rawSessions).map(function (session) {
    return {
      id: Number(session && session.id) || 0,
      profile: session && (session.profile_name || session.profile) ? String(session.profile_name || session.profile) : '',
      agent: session && (session.agent_name || session.agent) ? String(session.agent_name || session.agent) : '',
      model: session && (session.model || session.agent_model) ? String(session.model || session.agent_model) : '',
      status: session && session.status ? String(session.status) : 'unknown',
      action: session && session.action ? String(session.action) : '',
      started_at: session && session.started_at ? session.started_at : '',
      ended_at: session && session.ended_at ? session.ended_at : '',
      loop_name: session && session.loop_name ? String(session.loop_name) : '',
    };
  }).filter(function (s) { return s.id > 0; }).sort(function (a, b) { return b.id - a.id; });
}

export function normalizeSpawns(rawSpawns) {
  return arrayOrEmpty(rawSpawns).map(function (spawn) {
    var parentTurn = numberOr(0, spawn && spawn.parent_turn_id, spawn && spawn.parent_session_id);
    var childTurn = numberOr(0, spawn && spawn.child_turn_id, spawn && spawn.child_session_id);
    var parentDaemonSession = numberOr(0, spawn && spawn.parent_daemon_session_id);
    var childDaemonSession = numberOr(0, spawn && spawn.child_daemon_session_id);
    var parentSpawn = numberOr(0, spawn && spawn.parent_spawn_id, spawn && spawn.parent_id);
    return {
      id: Number(spawn && spawn.id) || 0,
      parent_turn_id: parentTurn,
      parent_session_id: numberOr(0, spawn && spawn.parent_session_id),
      parent_daemon_session_id: parentDaemonSession,
      parent_spawn_id: parentSpawn,
      child_turn_id: childTurn,
      child_session_id: numberOr(0, spawn && spawn.child_session_id),
      child_daemon_session_id: childDaemonSession,
      profile: spawn && (spawn.profile || spawn.child_profile) ? String(spawn.profile || spawn.child_profile) : '',
      role: spawn && (spawn.role || spawn.child_role) ? String(spawn.role || spawn.child_role) : '',
      parent_profile: spawn && spawn.parent_profile ? String(spawn.parent_profile) : '',
      status: spawn && spawn.status ? String(spawn.status) : 'unknown',
      question: spawn && spawn.question ? String(spawn.question) : '',
      task: spawn && (spawn.task || spawn.objective || spawn.description) ? String(spawn.task || spawn.objective || spawn.description) : '',
      branch: spawn && spawn.branch ? String(spawn.branch) : '',
      started_at: spawn && (spawn.started_at || spawn.created_at) ? (spawn.started_at || spawn.created_at) : '',
      completed_at: spawn && spawn.completed_at ? spawn.completed_at : '',
      summary: spawn && spawn.summary ? String(spawn.summary) : '',
    };
  }).filter(function (s) { return s.id > 0; }).sort(function (a, b) {
    return parseTimestamp(b.started_at) - parseTimestamp(a.started_at);
  });
}

export function normalizeIssues(rawIssues) {
  return arrayOrEmpty(rawIssues).map(function (issue) {
    return {
      id: Number(issue && issue.id) || 0,
      title: issue && issue.title ? String(issue.title) : '',
      plan_id: issue && issue.plan_id ? String(issue.plan_id) : '',
      priority: issue && issue.priority ? String(issue.priority) : 'medium',
      status: issue && issue.status ? String(issue.status) : 'open',
      labels: arrayOrEmpty(issue && issue.labels),
      depends_on: arrayOrEmpty(issue && issue.depends_on).map(function (id) { return Number(id) || 0; }).filter(function (id) { return id > 0; }),
      description: issue && issue.description ? String(issue.description) : '',
    };
  }).filter(function (i) { return i.id > 0; }).sort(function (a, b) { return b.id - a.id; });
}

export function normalizeDocs(rawDocs) {
  return arrayOrEmpty(rawDocs).map(function (doc) {
    return {
      id: doc && doc.id ? String(doc.id) : '',
      title: doc && doc.title ? String(doc.title) : '',
      content: doc && doc.content ? String(doc.content) : '',
      plan_id: doc && doc.plan_id ? String(doc.plan_id) : '',
    };
  }).filter(function (d) { return !!d.id; });
}

export function normalizePlans(rawPlans) {
  return arrayOrEmpty(rawPlans).map(normalizePlan).filter(function (p) { return !!p.id; });
}

export function normalizePlan(plan) {
  if (!plan || typeof plan !== 'object') {
    return { id: '', title: '', status: 'active', description: '' };
  }
  return {
    id: plan.id ? String(plan.id) : '',
    title: plan.title ? String(plan.title) : (plan.id ? String(plan.id) : ''),
    status: plan.status ? String(plan.status) : 'active',
    description: plan.description ? String(plan.description) : '',
  };
}

export function normalizeTurns(rawTurns) {
  return arrayOrEmpty(rawTurns).map(function (turn) {
    return {
      id: Number(turn && turn.id) || 0,
      hex_id: turn && turn.hex_id ? String(turn.hex_id) : '',
      loop_run_hex_id: turn && turn.loop_run_hex_id ? String(turn.loop_run_hex_id) : '',
      step_hex_id: turn && turn.step_hex_id ? String(turn.step_hex_id) : '',
      profile_name: turn && turn.profile_name ? String(turn.profile_name) : '',
      agent: turn && turn.agent ? String(turn.agent) : '',
      agent_model: turn && turn.agent_model ? String(turn.agent_model) : '',
      plan_id: turn && turn.plan_id ? String(turn.plan_id) : '',
      commit_hash: turn && turn.commit_hash ? String(turn.commit_hash) : '',
      build_state: turn && turn.build_state ? String(turn.build_state) : 'unknown',
      date: turn && turn.date ? turn.date : '',
      objective: turn && turn.objective ? String(turn.objective) : '',
      what_was_built: turn && turn.what_was_built ? String(turn.what_was_built) : '',
      key_decisions: turn && turn.key_decisions ? String(turn.key_decisions) : '',
      challenges: turn && turn.challenges ? String(turn.challenges) : '',
      current_state: turn && turn.current_state ? String(turn.current_state) : '',
      known_issues: turn && turn.known_issues ? String(turn.known_issues) : '',
      next_steps: turn && turn.next_steps ? String(turn.next_steps) : '',
      duration_secs: Number(turn && turn.duration_secs) || 0,
    };
  }).filter(function (t) { return t.id > 0; }).sort(function (a, b) { return b.id - a.id; });
}

export function normalizeLoopRun(run) {
  var stepHexIDs = {};
  if (run && typeof run.step_hex_ids === 'object') {
    Object.keys(run.step_hex_ids).forEach(function (cycleStep) {
      var hex = String(run.step_hex_ids[cycleStep] || '').trim();
      if (!hex) return;
      stepHexIDs[String(cycleStep)] = hex;
    });
  }
  return {
    id: Number(run && run.id) || 0,
    hex_id: run && run.hex_id ? String(run.hex_id) : '',
    loop_name: run && run.loop_name ? String(run.loop_name) : 'loop',
    status: run && run.status ? String(run.status) : 'unknown',
    cycle: Number(run && run.cycle) || 0,
    step_index: Number(run && run.step_index) || 0,
    started_at: run && run.started_at ? run.started_at : '',
    stopped_at: run && run.stopped_at ? run.stopped_at : '',
    daemon_session_id: Number(run && run.daemon_session_id) || 0,
    turn_ids: arrayOrEmpty(run && (run.turn_ids || run.session_ids)).map(function (id) { return Number(id) || 0; }),
    step_hex_ids: stepHexIDs,
    steps: arrayOrEmpty(run && run.steps).map(function (step) {
      return {
        profile: step && step.profile ? String(step.profile) : '',
        position: step && step.position ? String(step.position) : 'lead',
        turns: Number(step && step.turns) || 1,
      };
    }),
  };
}

export function normalizeLoopMessages(rawMessages) {
  return arrayOrEmpty(rawMessages).map(function (msg) {
    return {
      id: Number(msg && msg.id) || 0,
      spawn_id: Number(msg && msg.spawn_id) || 0,
      type: msg && msg.type ? String(msg.type) : 'message',
      direction: msg && msg.direction ? String(msg.direction) : 'child_to_parent',
      content: msg && msg.content ? String(msg.content) : '',
      created_at: msg && msg.created_at ? msg.created_at : '',
      step_index: msg && Number.isFinite(Number(msg.step_index)) ? Number(msg.step_index) : null,
    };
  }).filter(function (m) { return m.id > 0 || !!m.content; });
}

export function normalizeAllLoopRuns(runs) {
  return arrayOrEmpty(runs).map(normalizeLoopRun).filter(function (r) { return r.id > 0; })
    .sort(function (a, b) { return parseTimestamp(b.started_at) - parseTimestamp(a.started_at); });
}

export function pickActiveLoopRun(runs) {
  var list = arrayOrEmpty(runs).map(normalizeLoopRun);
  if (!list.length) return null;
  var running = list.filter(function (run) { return normalizeStatus(run.status) === 'running'; });
  if (running.length) {
    running.sort(function (a, b) { return parseTimestamp(b.started_at) - parseTimestamp(a.started_at); });
    return running[0];
  }
  return null;
}

export function aggregateUsageFromProfileStats(stats) {
  var list = arrayOrEmpty(stats);
  if (!list.length) return null;
  var usage = { input_tokens: 0, output_tokens: 0, cost_usd: 0, num_turns: 0 };
  list.forEach(function (item) {
    usage.input_tokens += Number(item && (item.total_input_tokens != null ? item.total_input_tokens : item.total_input_tok)) || 0;
    usage.output_tokens += Number(item && (item.total_output_tokens != null ? item.total_output_tokens : item.total_output_tok)) || 0;
    usage.cost_usd += Number(item && item.total_cost_usd) || 0;
    usage.num_turns += Number(item && item.total_turns) || 0;
  });
  return usage;
}
