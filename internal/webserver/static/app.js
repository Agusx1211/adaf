(function () {
  'use strict';

  var POLL_MS = 5000;
  var MAX_STREAM_EVENTS = 900;
  var MAX_ACTIVITY_EVENTS = 240;
  var STATUS_RUNNING = {
    starting: true,
    running: true,
    active: true,
    in_progress: true
  };

  var LEFT_VIEWS = ['agents', 'issues', 'docs', 'plan', 'logs'];
  var RIGHT_LAYERS = ['raw', 'activity', 'messages'];
  var SCOPE_COLOR_PALETTE = [
    '#89b4fa', '#94e2d5', '#cba6f7', '#fab387', '#a6e3a1', '#b4befe', '#89dceb', '#f2cdcd', '#74c7ec'
  ];

  var state = {
    // Auth
    authToken: '',

    // Multi-project
    projects: [],
    currentProjectID: '',

    // Layout state
    leftView: 'agents',
    rightLayer: 'raw',
    selectedScope: null,
    autoScroll: true,

    // Data
    sessions: [],
    spawns: [],
    messages: [],
    streamEvents: [],
    issues: [],
    plans: [],
    activePlan: null,
    docs: [],
    turns: [],
    loopRun: null,
    usage: null,

    // Selection
    selectedIssue: null,
    selectedPlan: null,

    // Tree state
    expandedNodes: new Set(),

    // WebSocket
    ws: null,
    wsConnected: false,

    // Terminal
    term: null,
    termWS: null,

    // Internal UI/runtime
    termWSConnected: false,
    currentPanel: 'left',
    loadingCount: 0,
    modal: null,
    projectMeta: null,
    pollTimer: null,
    reconnectTimer: null,
    currentSessionSocketID: 0,
    activeLoopIDForMessages: 0,
    sessionMessageDraft: '',
    viewLoaded: {
      issues: false,
      docs: false,
      plan: false,
      logs: false
    }
  };

  var content = document.getElementById('content');
  var modalRoot = document.getElementById('modal-root');
  var toastRoot = document.getElementById('toast-root');
  var loadingNode = document.getElementById('global-loading');

  var ICON_PATHS = {
    bot: 'M12 3a3 3 0 013 3v1h1a2 2 0 012 2v6a2 2 0 01-2 2h-1v1a3 3 0 11-6 0v-1H8a2 2 0 01-2-2V9a2 2 0 012-2h1V6a3 3 0 013-3zm-2 9a1 1 0 100 2 1 1 0 000-2zm4 0a1 1 0 100 2 1 1 0 000-2z',
    alert: 'M12 3l9 16H3l9-16zm0 5a1 1 0 00-1 1v4a1 1 0 102 0V9a1 1 0 00-1-1zm0 8a1.2 1.2 0 100 2.4 1.2 1.2 0 000-2.4z',
    file: 'M7 3h7l4 4v14H7a2 2 0 01-2-2V5a2 2 0 012-2zm7 1.5V8h3.5',
    list: 'M8 6h10M8 12h10M8 18h10M4 6h.01M4 12h.01M4 18h.01',
    scroll: 'M7 4h10a2 2 0 012 2v12a2 2 0 01-2 2H7a2 2 0 01-2-2V6a2 2 0 012-2zm2 4h6M9 11h6M9 14h4',
    terminal: 'M4 5h16v14H4zM7 9l3 3-3 3M12 15h5',
    activity: 'M3 12h4l3 7 4-14 3 7h4',
    message: 'M4 5h16v10H8l-4 4V5z',
    eye: 'M2 12s4-6 10-6 10 6 10 6-4 6-10 6-10-6-10-6zm10-3a3 3 0 100 6 3 3 0 000-6z',
    swap: 'M4 7h11l-2-2m2 2-2 2M20 17H9l2-2m-2 2 2 2',
    refresh: 'M20 12a8 8 0 10-2.34 5.66M20 12V7m0 5h-5',
    fork: 'M6 3a2 2 0 104 0 2 2 0 00-4 0zm8 0a2 2 0 104 0 2 2 0 00-4 0zM6 21a2 2 0 104 0 2 2 0 00-4 0zm2-16v6a4 4 0 004 4h4M16 5v10',
    branch: 'M6 3v8a4 4 0 004 4h8M6 3a2 2 0 114 0 2 2 0 01-4 0zm10 0a2 2 0 114 0 2 2 0 01-4 0zm0 18a2 2 0 114 0 2 2 0 01-4 0z',
    chevronDown: 'M6 9l6 6 6-6',
    chevronRight: 'M9 6l6 6-6 6'
  };

  function init() {
    if (!content || !modalRoot || !toastRoot) return;

    loadAuthToken();
    bindGlobalEvents();
    renderApp();

    initializeProjects()
      .then(function () {
        return refreshCoreData(true);
      })
      .then(function () {
        startPolling();
      })
      .catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Failed to initialize: ' + errorMessage(err), 'error');
        renderApp();
      });
  }

  function bindGlobalEvents() {
    content.addEventListener('click', onContentClick);
    content.addEventListener('change', onContentChange);
    content.addEventListener('input', onContentInput);
    content.addEventListener('submit', onContentSubmit);

    modalRoot.addEventListener('click', onModalClick);
    modalRoot.addEventListener('submit', onModalSubmit);

    document.addEventListener('keydown', onDocumentKeydown);
  }

  function onDocumentKeydown(event) {
    if (event.key === 'Escape' && state.modal) {
      closeModal();
      return;
    }

    if (isTypingTarget(event.target)) return;

    if (/^[1-5]$/.test(event.key)) {
      var idx = parseInt(event.key, 10) - 1;
      if (idx >= 0 && idx < LEFT_VIEWS.length) {
        event.preventDefault();
        setLeftView(LEFT_VIEWS[idx]);
      }
    }
  }

  function onContentClick(event) {
    var actionNode = event.target.closest('[data-action]');
    if (!actionNode) return;

    var action = actionNode.getAttribute('data-action');
    if (!action) return;

    event.preventDefault();

    if (action === 'set-left-view') {
      setLeftView(actionNode.getAttribute('data-view') || 'agents');
      return;
    }

    if (action === 'set-right-layer') {
      setRightLayer(actionNode.getAttribute('data-layer') || 'raw');
      return;
    }

    if (action === 'set-scope') {
      setSelectedScope(actionNode.getAttribute('data-scope') || null);
      return;
    }

    if (action === 'toggle-node') {
      toggleNode(actionNode.getAttribute('data-node') || '');
      return;
    }

    if (action === 'toggle-auto-scroll') {
      state.autoScroll = !state.autoScroll;
      renderApp();
      return;
    }

    if (action === 'toggle-mobile-panel') {
      state.currentPanel = state.currentPanel === 'left' ? 'right' : 'left';
      renderApp();
      return;
    }

    if (action === 'select-issue') {
      var issueID = parseInt(actionNode.getAttribute('data-issue-id') || '', 10);
      if (!Number.isNaN(issueID)) {
        state.selectedIssue = issueID;
        renderApp();
      }
      return;
    }

    if (action === 'select-plan') {
      var planID = actionNode.getAttribute('data-plan-id') || '';
      if (!planID) return;
      state.selectedPlan = planID;
      fetchSelectedPlanDetail(planID).then(function () {
        renderApp();
      }).catch(function (err) {
        showToast('Failed to load plan: ' + errorMessage(err), 'error');
        renderApp();
      });
      return;
    }

    if (action === 'clear-scope') {
      state.selectedScope = null;
      ensureSessionSocket();
      renderApp();
      return;
    }

    if (action === 'refresh-core') {
      refreshCoreData(false).catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Refresh failed: ' + errorMessage(err), 'error');
      });
      return;
    }

    if (action === 'open-new-session-modal') {
      openNewSessionModal().catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Failed to load session options: ' + errorMessage(err), 'error');
      });
      return;
    }

    if (action === 'stop-session') {
      var stopSessionID = parseInt(actionNode.getAttribute('data-session-id') || '', 10);
      if (Number.isNaN(stopSessionID) || stopSessionID <= 0) return;
      stopSessionByID(stopSessionID);
      return;
    }

    if (action === 'disconnect-auth') {
      clearAuthToken();
      closeModal();
      renderApp();
      return;
    }
  }

  function onContentChange(event) {
    var node = event.target;
    if (!node) return;

    if (node.id === 'project-select' || node.getAttribute('data-change') === 'project-select') {
      switchProject(node.value || '', true);
      return;
    }
  }

  function onContentInput(event) {
    var node = event.target;
    if (!node) return;

    if (node.getAttribute('data-input') === 'session-message') {
      state.sessionMessageDraft = String(node.value || '');
    }
  }

  function onContentSubmit(event) {
    var form = event.target.closest('form[data-form]');
    if (!form) return;

    event.preventDefault();

    var formName = form.getAttribute('data-form');
    if (formName === 'auth-inline') {
      submitAuthForm(form);
      return;
    }

    if (formName === 'session-message') {
      submitSessionMessageForm(form).catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Failed to send message: ' + errorMessage(err), 'error');
      });
    }
  }

  function onModalClick(event) {
    var actionNode = event.target.closest('[data-action]');
    if (!actionNode) return;
    var action = actionNode.getAttribute('data-action');
    if (!action) return;

    if (action === 'close-modal') {
      event.preventDefault();
      closeModal();
      return;
    }

    if (action === 'disconnect-auth') {
      event.preventDefault();
      clearAuthToken();
      closeModal();
      renderApp();
      return;
    }

    if (action === 'set-new-session-mode') {
      event.preventDefault();
      var form = actionNode.closest('form[data-modal-submit="new-session"]');
      if (!form) return;
      setNewSessionModeInForm(form, actionNode.getAttribute('data-mode') || 'ask');
    }
  }

  function onModalSubmit(event) {
    var form = event.target.closest('form[data-modal-submit]');
    if (!form) return;

    event.preventDefault();

    var submit = form.getAttribute('data-modal-submit');
    if (submit === 'auth') {
      submitAuthForm(form);
      return;
    }

    if (submit === 'new-session') {
      submitNewSessionForm(form).catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Failed to start session: ' + errorMessage(err), 'error');
      });
    }
  }

  function submitAuthForm(form) {
    var token = readString(form, 'auth_token');
    if (!token) {
      showToast('Token is required.', 'error');
      return;
    }

    saveAuthToken(token);
    closeModal();
    refreshCoreData(true)
      .then(function () {
        startPolling();
      })
      .catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Failed to authenticate: ' + errorMessage(err), 'error');
      });
  }

  async function openNewSessionModal() {
    var results = await Promise.all([
      apiCall('/api/config/profiles', 'GET', null, { allow404: true }),
      apiCall('/api/config/loops', 'GET', null, { allow404: true })
    ]);

    var profiles = arrayOrEmpty(results[0]).map(function (profile) {
      return {
        name: profile && profile.name ? String(profile.name) : '',
        agent: profile && profile.agent ? String(profile.agent) : '',
        model: profile && profile.model ? String(profile.model) : ''
      };
    }).filter(function (profile) {
      return !!profile.name;
    }).sort(function (a, b) {
      return a.name.localeCompare(b.name);
    });

    var loops = arrayOrEmpty(results[1]).map(function (loop) {
      return {
        name: loop && loop.name ? String(loop.name) : '',
        steps: arrayOrEmpty(loop && loop.steps)
      };
    }).filter(function (loop) {
      return !!loop.name;
    }).sort(function (a, b) {
      return a.name.localeCompare(b.name);
    });

    openModal('New Session', renderNewSessionModalBody(profiles, loops));
  }

  function renderNewSessionModalBody(profiles, loops) {
    var profileOptions = renderProfileOptions(profiles, 'Select profile');
    var loopOptions = renderLoopOptions(loops, 'Select loop');

    return '' +
      '<form class="new-session-form" data-modal-submit="new-session" data-mode="ask">' +
        '<input type="hidden" name="mode" value="ask">' +
        '<div class="session-mode-switch" role="tablist" aria-label="Session mode">' +
          '<button type="button" class="session-mode-btn active" data-action="set-new-session-mode" data-mode="ask">Ask</button>' +
          '<button type="button" class="session-mode-btn" data-action="set-new-session-mode" data-mode="pm">PM</button>' +
          '<button type="button" class="session-mode-btn" data-action="set-new-session-mode" data-mode="loop">Loop</button>' +
        '</div>' +

        '<section class="new-session-panel" data-mode-panel="ask">' +
          '<div class="form-grid">' +
            '<div class="form-field form-span-2"><label>Profile</label><select name="ask_profile">' + profileOptions + '</select></div>' +
            '<div class="form-field form-span-2"><label>Prompt</label><textarea name="ask_prompt" placeholder="Describe what the agent should do"></textarea></div>' +
            '<div class="form-field form-span-2"><label>Plan ID (optional)</label><input type="text" name="ask_plan_id" placeholder="plan-id"></div>' +
          '</div>' +
        '</section>' +

        '<section class="new-session-panel" data-mode-panel="pm">' +
          '<div class="form-grid">' +
            '<div class="form-field form-span-2"><label>Profile</label><select name="pm_profile">' + profileOptions + '</select></div>' +
            '<div class="form-field form-span-2"><label>Plan ID (optional)</label><input type="text" name="pm_plan_id" placeholder="plan-id"></div>' +
          '</div>' +
        '</section>' +

        '<section class="new-session-panel" data-mode-panel="loop">' +
          '<div class="form-grid">' +
            '<div class="form-field form-span-2"><label>Loop Definition</label><select name="loop_name">' + loopOptions + '</select></div>' +
            '<div class="form-field form-span-2"><label>Plan ID (optional)</label><input type="text" name="loop_plan_id" placeholder="plan-id"></div>' +
          '</div>' +
        '</section>' +

        '<div class="form-actions">' +
          '<button type="button" data-action="close-modal">Cancel</button>' +
          '<button type="submit" class="primary">Start Session</button>' +
        '</div>' +
      '</form>';
  }

  function renderProfileOptions(profiles, placeholder) {
    var list = arrayOrEmpty(profiles);
    if (!list.length) {
      return '<option value="">' + escapeHTML(placeholder || 'No profiles configured') + '</option>';
    }

    var options = '<option value="">' + escapeHTML(placeholder || 'Select profile') + '</option>';
    options += list.map(function (profile) {
      var label = profile.name;
      var meta = [];
      if (profile.agent) meta.push(profile.agent);
      if (profile.model) meta.push(profile.model);
      if (meta.length) label += ' (' + meta.join(' · ') + ')';
      return '<option value="' + escapeHTML(profile.name) + '">' + escapeHTML(label) + '</option>';
    }).join('');

    return options;
  }

  function renderLoopOptions(loops, placeholder) {
    var list = arrayOrEmpty(loops);
    if (!list.length) {
      return '<option value="">' + escapeHTML(placeholder || 'No loops configured') + '</option>';
    }

    var options = '<option value="">' + escapeHTML(placeholder || 'Select loop') + '</option>';
    options += list.map(function (loop) {
      var stepCount = arrayOrEmpty(loop.steps).length;
      var label = loop.name + (stepCount ? (' (' + stepCount + ' steps)') : '');
      return '<option value="' + escapeHTML(loop.name) + '">' + escapeHTML(label) + '</option>';
    }).join('');

    return options;
  }

  function setNewSessionModeInForm(form, mode) {
    if (!form) return;

    var nextMode = String(mode || 'ask').toLowerCase();
    if (nextMode !== 'ask' && nextMode !== 'pm' && nextMode !== 'loop') {
      nextMode = 'ask';
    }

    form.setAttribute('data-mode', nextMode);

    var hidden = form.querySelector('input[name="mode"]');
    if (hidden) hidden.value = nextMode;

    var buttons = form.querySelectorAll('[data-action="set-new-session-mode"]');
    for (var i = 0; i < buttons.length; i += 1) {
      var btn = buttons[i];
      var buttonMode = btn.getAttribute('data-mode') || 'ask';
      btn.classList.toggle('active', buttonMode === nextMode);
    }
  }

  async function submitNewSessionForm(form) {
    if (!form) return;

    var mode = readString(form, 'mode');
    if (mode !== 'ask' && mode !== 'pm' && mode !== 'loop') mode = 'ask';

    var endpoint = '';
    var payload = {};

    if (mode === 'ask') {
      var askProfile = readString(form, 'ask_profile');
      var askPrompt = readString(form, 'ask_prompt');
      var askPlanID = readString(form, 'ask_plan_id');

      if (!askProfile || !askPrompt) {
        showToast('Ask mode requires profile and prompt.', 'error');
        return;
      }

      endpoint = apiBase() + '/sessions/ask';
      payload = { profile: askProfile, prompt: askPrompt };
      if (askPlanID) payload.plan_id = askPlanID;
    } else if (mode === 'pm') {
      var pmProfile = readString(form, 'pm_profile');
      var pmPlanID = readString(form, 'pm_plan_id');

      if (!pmProfile) {
        showToast('PM mode requires a profile.', 'error');
        return;
      }

      endpoint = apiBase() + '/sessions/pm';
      payload = { profile: pmProfile };
      if (pmPlanID) payload.plan_id = pmPlanID;
    } else {
      var loopName = readString(form, 'loop_name');
      var loopPlanID = readString(form, 'loop_plan_id');

      if (!loopName) {
        showToast('Loop mode requires a loop definition.', 'error');
        return;
      }

      endpoint = apiBase() + '/sessions/loop';
      payload = {
        loop_name: loopName,
        loop: loopName
      };
      if (loopPlanID) payload.plan_id = loopPlanID;
    }

    var response = null;
    if (mode === 'pm') {
      try {
        response = await apiCall(endpoint, 'POST', payload);
      } catch (err) {
        if (/message are required/i.test(errorMessage(err))) {
          var fallbackPayload = Object.assign({}, payload, {
            message: 'Start PM session.'
          });
          response = await apiCall(endpoint, 'POST', fallbackPayload);
        } else {
          throw err;
        }
      }
    } else {
      response = await apiCall(endpoint, 'POST', payload);
    }

    var sessionID = Number(response && response.id);
    if (Number.isFinite(sessionID) && sessionID > 0) {
      state.selectedScope = 'session-' + sessionID;
      state.sessionMessageDraft = '';
    }

    closeModal();
    showToast('Session started.', 'success');
    await refreshCoreData(false);
  }

  async function stopSessionByID(sessionID) {
    var session = state.sessions.find(function (item) {
      return item.id === sessionID;
    }) || null;
    if (!session) {
      showToast('Session not found.', 'error');
      return;
    }

    if (!STATUS_RUNNING[normalizeStatus(session.status)]) {
      showToast('Session is not running.', 'error');
      return;
    }

    var shouldStop = window.confirm('Stop session #' + sessionID + '?');
    if (!shouldStop) return;

    try {
      await apiCall(apiBase() + '/sessions/' + encodeURIComponent(String(sessionID)) + '/stop', 'POST', {});
      showToast('Stop signal sent for session #' + sessionID + '.', 'success');
      await refreshCoreData(false);
    } catch (err) {
      if (err && err.authRequired) return;
      showToast('Failed to stop session: ' + errorMessage(err), 'error');
    }
  }

  async function initializeProjects() {
    try {
      var projects = arrayOrEmpty(await apiCall('/api/projects', 'GET', null, { allow404: true }));
      state.projects = projects;

      var savedID = '';
      try {
        savedID = localStorage.getItem('adaf_project_id') || '';
      } catch (_) {
        savedID = '';
      }

      if (savedID && findProjectByID(savedID)) {
        state.currentProjectID = savedID;
      } else {
        var defaultProject = projects.find(function (project) {
          return !!(project && project.is_default);
        }) || projects[0] || null;
        state.currentProjectID = defaultProject && defaultProject.id ? String(defaultProject.id) : '';
      }
    } catch (err) {
      if (!(err && err.authRequired)) {
        state.projects = [];
        state.currentProjectID = '';
      }
      throw err;
    } finally {
      updateDocumentTitle();
      renderApp();
    }
  }

  function findProjectByID(projectID) {
    var target = String(projectID || '');
    if (!target) return null;
    return state.projects.find(function (project) {
      return project && String(project.id || '') === target;
    }) || null;
  }

  function currentProject() {
    if (state.currentProjectID) {
      return findProjectByID(state.currentProjectID);
    }
    return state.projects.find(function (project) {
      return !!(project && project.is_default);
    }) || state.projects[0] || null;
  }

  function currentProjectName() {
    var project = currentProject();
    if (project && project.name) return project.name;
    if (state.projectMeta && state.projectMeta.name) return state.projectMeta.name;
    if (project && project.id) return String(project.id);
    return 'project';
  }

  function updateDocumentTitle() {
    var name = currentProjectName();
    document.title = name ? ('ADAF - ' + name) : 'ADAF';
  }

  function apiBase() {
    if (state.currentProjectID) {
      return '/api/projects/' + encodeURIComponent(state.currentProjectID);
    }
    return '/api';
  }

  function resetProjectScopedState() {
    stopPolling();
    disconnectSessionSocket();
    disconnectTerminal();

    state.sessions = [];
    state.spawns = [];
    state.messages = [];
    state.streamEvents = [];
    state.issues = [];
    state.plans = [];
    state.activePlan = null;
    state.docs = [];
    state.turns = [];
    state.loopRun = null;
    state.usage = null;

    state.selectedIssue = null;
    state.selectedPlan = null;
    state.selectedScope = null;
    state.expandedNodes = new Set();
    state.projectMeta = null;
    state.activeLoopIDForMessages = 0;

    state.viewLoaded = {
      issues: false,
      docs: false,
      plan: false,
      logs: false
    };
  }

  function switchProject(projectID, refresh) {
    var nextID = String(projectID || '');
    if (nextID && !findProjectByID(nextID)) return;

    if (nextID === state.currentProjectID && !refresh) return;

    resetProjectScopedState();
    state.currentProjectID = nextID;
    try {
      localStorage.setItem('adaf_project_id', nextID);
    } catch (_) {}

    updateDocumentTitle();
    renderApp();

    refreshCoreData(true)
      .then(function () {
        startPolling();
      })
      .catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Failed to switch project: ' + errorMessage(err), 'error');
      });
  }

  function setLeftView(view) {
    if (LEFT_VIEWS.indexOf(view) < 0) return;
    state.leftView = view;

    if (window.innerWidth <= 768) {
      state.currentPanel = 'left';
    }

    ensureViewData(view)
      .then(function () {
        renderApp();
      })
      .catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Failed to load ' + view + ': ' + errorMessage(err), 'error');
      });

    renderApp();
  }

  function setRightLayer(layer) {
    if (RIGHT_LAYERS.indexOf(layer) < 0) return;
    state.rightLayer = layer;

    if (window.innerWidth <= 768) {
      state.currentPanel = 'right';
    }

    renderApp();
  }

  function setSelectedScope(scope) {
    state.selectedScope = scope || null;
    expandScopeParents(scope);
    ensureSessionSocket();
    renderApp();
  }

  function toggleNode(nodeID) {
    if (!nodeID) return;
    if (state.expandedNodes.has(nodeID)) {
      state.expandedNodes.delete(nodeID);
    } else {
      state.expandedNodes.add(nodeID);
    }
    renderApp();
  }

  function expandScopeParents(scope) {
    if (!scope) return;
    if (scope.indexOf('session-') === 0) {
      state.expandedNodes.add(scope);
      return;
    }

    if (scope.indexOf('spawn-') === 0) {
      var spawnID = parseInt(scope.slice(6), 10);
      if (Number.isNaN(spawnID)) return;

      var byID = spawnMapByID();
      var current = byID[spawnID] || null;
      while (current) {
        state.expandedNodes.add('spawn-' + current.id);
        if (current.parent_spawn_id > 0) {
          current = byID[current.parent_spawn_id] || null;
          continue;
        }
        if (current.parent_turn_id > 0) {
          state.expandedNodes.add('session-' + current.parent_turn_id);
        }
        break;
      }
    }
  }

  function ensureTreeDefaults() {
    if (state.expandedNodes.size > 0) return;

    state.sessions.forEach(function (session) {
      state.expandedNodes.add('session-' + session.id);
    });

    state.spawns.forEach(function (spawn) {
      if (spawn.status === 'running' || spawn.status === 'awaiting_input') {
        state.expandedNodes.add('spawn-' + spawn.id);
      }
    });
  }

  async function refreshCoreData(initial) {
    var priorLoopID = state.loopRun && state.loopRun.id ? state.loopRun.id : 0;

    var results = await Promise.all([
      apiCall(apiBase() + '/project', 'GET', null, { allow404: true }),
      apiCall(apiBase() + '/sessions', 'GET', null, { allow404: true }),
      apiCall(apiBase() + '/spawns', 'GET', null, { allow404: true }),
      apiCall(apiBase() + '/loops', 'GET', null, { allow404: true }),
      apiCall(apiBase() + '/stats/profiles', 'GET', null, { allow404: true })
    ]);

    state.projectMeta = results[0] || null;
    updateDocumentTitle();

    state.sessions = normalizeSessions(results[1]);
    mergeSpawns(results[2], 'poll');

    var loopRuns = arrayOrEmpty(results[3]);
    state.loopRun = pickActiveLoopRun(loopRuns);

    var usageFromStats = aggregateUsageFromProfileStats(results[4]);
    if (usageFromStats) {
      state.usage = usageFromStats;
    } else if (!state.usage) {
      state.usage = {
        input_tokens: 0,
        output_tokens: 0,
        cost_usd: 0,
        num_turns: state.turns.length || 0
      };
    }

    ensureTreeDefaults();
    ensureDefaultScope();

    var currentLoopID = state.loopRun && state.loopRun.id ? state.loopRun.id : 0;
    if (currentLoopID !== priorLoopID || initial) {
      await refreshLoopMessages(currentLoopID);
    }

    if (state.leftView === 'agents') {
      ensureSessionSocket();
    } else {
      // keep socket in sync with selected scope for live right-panel data
      ensureSessionSocket();
    }

    await ensureViewData(state.leftView);
    renderApp();
  }

  function startPolling() {
    stopPolling();
    state.pollTimer = setInterval(function () {
      refreshCoreData(false).catch(function (err) {
        if (err && err.authRequired) return;
        showToast('Refresh error: ' + errorMessage(err), 'error');
      });
    }, POLL_MS);
  }

  function stopPolling() {
    if (state.pollTimer) {
      clearInterval(state.pollTimer);
      state.pollTimer = null;
    }
  }

  async function ensureViewData(view) {
    if (view === 'issues') {
      if (state.viewLoaded.issues) return;
      await fetchIssues();
      state.viewLoaded.issues = true;
      return;
    }

    if (view === 'docs') {
      if (state.viewLoaded.docs) return;
      await fetchDocs();
      state.viewLoaded.docs = true;
      return;
    }

    if (view === 'plan') {
      if (state.viewLoaded.plan) return;
      await fetchPlans();
      state.viewLoaded.plan = true;
      return;
    }

    if (view === 'logs') {
      if (state.viewLoaded.logs) return;
      await fetchTurns();
      state.viewLoaded.logs = true;
      return;
    }
  }

  async function fetchIssues() {
    state.issues = normalizeIssues(await apiCall(apiBase() + '/issues', 'GET', null, { allow404: true }));
    if (state.selectedIssue == null && state.issues.length) {
      state.selectedIssue = state.issues[0].id;
    }
  }

  async function fetchDocs() {
    state.docs = normalizeDocs(await apiCall(apiBase() + '/docs', 'GET', null, { allow404: true }));
  }

  async function fetchTurns() {
    state.turns = normalizeTurns(await apiCall(apiBase() + '/turns', 'GET', null, { allow404: true }));
    updateUsageTurnCountFromTurns();
  }

  async function fetchPlans() {
    state.plans = normalizePlans(await apiCall(apiBase() + '/plans', 'GET', null, { allow404: true }));

    var preferredPlanID = state.selectedPlan || activePlanIDFromProject() || '';
    if (!preferredPlanID && state.plans.length) {
      var active = state.plans.find(function (plan) {
        return normalizeStatus(plan.status) === 'active';
      });
      preferredPlanID = active ? active.id : state.plans[0].id;
    }

    if (preferredPlanID) {
      state.selectedPlan = preferredPlanID;
      await fetchSelectedPlanDetail(preferredPlanID);
    } else {
      state.activePlan = null;
      state.selectedPlan = null;
    }
  }

  async function fetchSelectedPlanDetail(planID) {
    if (!planID) {
      state.activePlan = null;
      return;
    }

    var detail = await apiCall(apiBase() + '/plans/' + encodeURIComponent(planID), 'GET', null, { allow404: true });
    if (detail) {
      state.activePlan = normalizePlan(detail);
      return;
    }

    state.activePlan = null;
  }

  function activePlanIDFromProject() {
    if (state.projectMeta && state.projectMeta.active_plan_id) {
      return String(state.projectMeta.active_plan_id);
    }
    return '';
  }

  async function refreshLoopMessages(loopID) {
    state.activeLoopIDForMessages = loopID || 0;
    if (!loopID) {
      state.messages = [];
      return;
    }

    var list = arrayOrEmpty(await apiCall(apiBase() + '/loops/' + encodeURIComponent(String(loopID)) + '/messages', 'GET', null, { allow404: true }));
    state.messages = normalizeLoopMessages(list);
  }

  function ensureDefaultScope() {
    if (state.selectedScope && scopeExists(state.selectedScope)) {
      return;
    }

    var runningSession = state.sessions.find(function (session) {
      return !!STATUS_RUNNING[normalizeStatus(session.status)];
    }) || null;

    if (runningSession) {
      state.selectedScope = 'session-' + runningSession.id;
      return;
    }

    if (state.sessions.length) {
      state.selectedScope = 'session-' + state.sessions[0].id;
      return;
    }

    if (state.spawns.length) {
      state.selectedScope = 'spawn-' + state.spawns[0].id;
      return;
    }

    state.selectedScope = null;
  }

  function scopeExists(scope) {
    if (!scope) return false;
    if (scope.indexOf('session-') === 0) {
      var sessionID = parseInt(scope.slice(8), 10);
      if (Number.isNaN(sessionID)) return false;
      return !!state.sessions.find(function (session) { return session.id === sessionID; });
    }
    if (scope.indexOf('spawn-') === 0) {
      var spawnID = parseInt(scope.slice(6), 10);
      if (Number.isNaN(spawnID)) return false;
      return !!state.spawns.find(function (spawn) { return spawn.id === spawnID; });
    }
    return false;
  }

  function resolveSessionIDForScope(scope) {
    var selected = scope || state.selectedScope;
    if (selected && selected.indexOf('session-') === 0) {
      var sessionID = parseInt(selected.slice(8), 10);
      if (!Number.isNaN(sessionID)) return sessionID;
    }

    if (selected && selected.indexOf('spawn-') === 0) {
      var spawnID = parseInt(selected.slice(6), 10);
      if (!Number.isNaN(spawnID)) {
        var spawn = state.spawns.find(function (item) {
          return item.id === spawnID;
        }) || null;
        if (spawn) {
          if (spawn.parent_turn_id > 0) return spawn.parent_turn_id;
          if (spawn.child_turn_id > 0) return spawn.child_turn_id;
        }
      }
    }

    var running = state.sessions.find(function (session) {
      return !!STATUS_RUNNING[normalizeStatus(session.status)];
    }) || null;
    if (running) return running.id;

    if (state.sessions.length) return state.sessions[0].id;
    return 0;
  }

  function ensureSessionSocket() {
    var targetSessionID = resolveSessionIDForScope(state.selectedScope);
    if (!targetSessionID) {
      disconnectSessionSocket();
      return;
    }

    if (
      state.ws &&
      state.currentSessionSocketID === targetSessionID &&
      (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    connectSessionSocket(targetSessionID);
  }

  function connectSessionSocket(sessionID) {
    if (!sessionID) return;

    disconnectSessionSocket();
    state.currentSessionSocketID = sessionID;

    try {
      state.ws = new WebSocket(buildWSURL('/ws/sessions/' + encodeURIComponent(String(sessionID))));
    } catch (err) {
      state.ws = null;
      state.wsConnected = false;
      renderApp();
      return;
    }

    state.ws.addEventListener('open', function () {
      state.wsConnected = true;
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: 'Connected to live session stream.'
      });
      renderApp();
    });

    state.ws.addEventListener('message', function (event) {
      var payload;
      try {
        payload = JSON.parse(event.data);
      } catch (_) {
        addStreamEvent({
          scope: 'session-' + sessionID,
          type: 'text',
          text: String(event.data || '')
        });
        renderApp();
        return;
      }

      ingestSessionEnvelope(sessionID, payload, false);
      renderApp();
    });

    state.ws.addEventListener('error', function () {
      state.wsConnected = false;
      renderApp();
    });

    state.ws.addEventListener('close', function () {
      var closedSessionID = state.currentSessionSocketID;
      state.wsConnected = false;
      state.ws = null;

      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: 'Session stream disconnected.'
      });

      if (state.reconnectTimer) {
        clearTimeout(state.reconnectTimer);
      }
      state.reconnectTimer = setTimeout(function () {
        if (resolveSessionIDForScope(state.selectedScope) === closedSessionID) {
          ensureSessionSocket();
        }
      }, 1800);

      renderApp();
    });
  }

  function disconnectSessionSocket() {
    if (state.reconnectTimer) {
      clearTimeout(state.reconnectTimer);
      state.reconnectTimer = null;
    }

    if (state.ws) {
      try {
        state.ws.close();
      } catch (_) {
        // Best effort.
      }
    }

    state.ws = null;
    state.wsConnected = false;
    state.currentSessionSocketID = 0;
  }

  function ingestSessionEnvelope(sessionID, envelope, replay) {
    if (!envelope || typeof envelope !== 'object') return;

    var type = envelope.type || 'event';
    var data = envelope.data;

    if (type === 'snapshot') {
      applySessionSnapshot(sessionID, data, replay);
      return;
    }

    if (type === 'event') {
      var wireEvent = data && data.event ? data.event : data;
      handleAgentStreamEvent(sessionID, wireEvent);
      return;
    }

    if (type === 'raw') {
      var rawText = rawPayloadText(data);
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: cropText(rawText)
      });
      return;
    }

    if (type === 'started') {
      addActivityFromEvent({
        scope: 'session-' + sessionID,
        ts: Date.now(),
        type: 'started',
        text: 'Agent turn started.'
      });
      return;
    }

    if (type === 'prompt') {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'thinking',
        text: data && data.prompt ? String(data.prompt) : 'Prompt emitted.'
      });
      return;
    }

    if (type === 'finished') {
      mergeUsageFromSessionSnapshot(data || {});
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: data && data.error ? 'tool_result' : 'text',
        text: 'Session finished with exit code ' + String((data && data.exit_code) || 0) + (data && data.error ? (' · ' + data.error) : '')
      });
      return;
    }

    if (type === 'done') {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: 'Session completed.' + (data && data.error ? ' ' + data.error : '')
      });
      return;
    }

    if (type === 'spawn') {
      mergeSpawns(data && data.spawns ? data.spawns : [], 'ws');
      return;
    }

    if (type === 'loop_step_start' || type === 'loop_step_end') {
      applyLoopWireUpdate(type, data || {});
      var actionText = type === 'loop_step_start'
        ? loopStepText(data, true)
        : loopStepText(data, false);
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: actionText
      });
      addActivityFromEvent({
        scope: 'session-' + sessionID,
        ts: Date.now(),
        type: type,
        text: actionText
      });
      return;
    }

    if (type === 'loop_done') {
      applyLoopDone(data || {});
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: 'Loop finished' + (data && data.reason ? ': ' + data.reason : '.')
      });
      addActivityFromEvent({
        scope: 'session-' + sessionID,
        ts: Date.now(),
        type: 'loop_done',
        text: 'Loop finished' + (data && data.reason ? ': ' + data.reason : '.')
      });
      return;
    }

    if (type === 'error') {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'tool_result',
        text: 'Error: ' + (data && data.error ? data.error : safeJSONString(data))
      });
      return;
    }

    addStreamEvent({
      scope: 'session-' + sessionID,
      type: 'text',
      text: '[' + type + '] ' + safeJSONString(data)
    });
  }

  function applySessionSnapshot(sessionID, snapshot, replay) {
    var data = asObject(snapshot);
    if (!data) return;

    if (data.session && typeof data.session === 'object') {
      mergeSessionSnapshot(sessionID, data.session);
      mergeUsageFromSessionSnapshot(data.session);
    }

    if (data.loop && typeof data.loop === 'object') {
      applyLoopWireUpdate('snapshot', data.loop);
    }

    if (Array.isArray(data.spawns)) {
      mergeSpawns(data.spawns, 'snapshot');
    }

    if (Array.isArray(data.recent)) {
      data.recent.forEach(function (recentMessage) {
        if (!recentMessage || recentMessage.type === 'snapshot') return;
        ingestSessionEnvelope(sessionID, recentMessage, true);
      });
    }

    if (!replay) {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: 'Snapshot received.'
      });
    }
  }

  function mergeSessionSnapshot(sessionID, sessionData) {
    if (!sessionData) return;

    var existing = state.sessions.find(function (session) {
      return session.id === sessionID;
    }) || null;

    if (existing) {
      existing.profile = sessionData.profile || existing.profile;
      existing.agent = sessionData.agent || existing.agent;
      existing.model = sessionData.model || existing.model;
      existing.status = sessionData.status || existing.status;
      existing.action = sessionData.action || existing.action;
      existing.started_at = sessionData.started_at || existing.started_at;
      existing.ended_at = sessionData.ended_at || existing.ended_at;
    } else {
      state.sessions.unshift({
        id: sessionID,
        profile: sessionData.profile || '',
        agent: sessionData.agent || '',
        model: sessionData.model || '',
        status: sessionData.status || 'running',
        action: sessionData.action || '',
        started_at: sessionData.started_at || '',
        ended_at: sessionData.ended_at || '',
        loop_name: ''
      });
    }
  }

  function mergeUsageFromSessionSnapshot(sessionData) {
    if (!sessionData || typeof sessionData !== 'object') return;

    if (!state.usage) {
      state.usage = {
        input_tokens: 0,
        output_tokens: 0,
        cost_usd: 0,
        num_turns: 0
      };
    }

    var input = Number(sessionData.input_tokens);
    var output = Number(sessionData.output_tokens);
    var cost = Number(sessionData.cost_usd);
    var turns = Number(sessionData.num_turns);

    if (Number.isFinite(input)) state.usage.input_tokens = Math.max(state.usage.input_tokens || 0, input);
    if (Number.isFinite(output)) state.usage.output_tokens = Math.max(state.usage.output_tokens || 0, output);
    if (Number.isFinite(cost)) state.usage.cost_usd = Math.max(state.usage.cost_usd || 0, cost);
    if (Number.isFinite(turns)) state.usage.num_turns = Math.max(state.usage.num_turns || 0, turns);
  }

  function handleAgentStreamEvent(sessionID, rawEvent) {
    var event = asObject(rawEvent);
    if (!event || typeof event !== 'object') {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: safeJSONString(rawEvent)
      });
      return;
    }

    if (event.type === 'assistant') {
      var blocks = extractContentBlocks(event);
      if (!blocks.length) {
        addStreamEvent({
          scope: 'session-' + sessionID,
          type: 'text',
          text: '[assistant event]'
        });
        return;
      }

      blocks.forEach(function (block) {
        ingestAssistantBlock(sessionID, block);
      });
      return;
    }

    if (event.type === 'user') {
      var userBlocks = extractContentBlocks(event);
      userBlocks.forEach(function (block) {
        if (!block || typeof block !== 'object') return;
        if (block.type === 'tool_result') {
          addStreamEvent({
            scope: 'session-' + sessionID,
            type: 'tool_result',
            tool: block.name || 'tool_result',
            result: stringifyToolPayload(block.content || block.output || block.text || safeJSONString(block))
          });
        }
      });
      return;
    }

    if (event.type === 'content_block_delta') {
      var delta = event.delta && (event.delta.text || event.delta.partial_json);
      if (delta) {
        addStreamEvent({
          scope: 'session-' + sessionID,
          type: 'text',
          text: String(delta)
        });
      }
      return;
    }

    if (event.type === 'result') {
      mergeUsageFromResultEvent(event);

      var resultParts = [];
      if (event.subtype) resultParts.push(String(event.subtype));
      if (state.usage && state.usage.input_tokens > 0) resultParts.push('in=' + formatNumber(state.usage.input_tokens));
      if (state.usage && state.usage.output_tokens > 0) resultParts.push('out=' + formatNumber(state.usage.output_tokens));
      if (state.usage && state.usage.cost_usd > 0) resultParts.push('cost=$' + Number(state.usage.cost_usd).toFixed(4));

      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'tool_result',
        text: resultParts.length ? resultParts.join(' · ') : 'Result received.'
      });
      return;
    }

    addStreamEvent({
      scope: 'session-' + sessionID,
      type: 'text',
      text: '[' + (event.type || 'event') + '] ' + safeJSONString(event)
    });
  }

  function ingestAssistantBlock(sessionID, block) {
    if (!block || typeof block !== 'object') return;

    if (block.type === 'text' && block.text) {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'text',
        text: String(block.text)
      });
      return;
    }

    if (block.type === 'thinking' && block.text) {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'thinking',
        text: String(block.text)
      });
      return;
    }

    if (block.type === 'tool_use') {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'tool_use',
        tool: block.name || 'tool',
        input: stringifyToolPayload(block.input || {})
      });
      addActivityFromEvent({
        scope: 'session-' + sessionID,
        ts: Date.now(),
        type: 'tool_use',
        tool: block.name || 'tool',
        text: stringifyToolPayload(block.input || {})
      });
      return;
    }

    if (block.type === 'tool_result') {
      addStreamEvent({
        scope: 'session-' + sessionID,
        type: 'tool_result',
        tool: block.name || 'tool_result',
        result: stringifyToolPayload(block.content || block.output || block.text || '')
      });
      return;
    }

    addStreamEvent({
      scope: 'session-' + sessionID,
      type: 'text',
      text: safeJSONString(block)
    });
  }

  function mergeUsageFromResultEvent(event) {
    if (!event || typeof event !== 'object') return;

    if (!state.usage) {
      state.usage = {
        input_tokens: 0,
        output_tokens: 0,
        cost_usd: 0,
        num_turns: 0
      };
    }

    var usage = event.usage && typeof event.usage === 'object' ? event.usage : {};

    var input = numberOr(state.usage.input_tokens, event.input_tokens, usage.input_tokens);
    var output = numberOr(state.usage.output_tokens, event.output_tokens, usage.output_tokens);
    var turns = numberOr(state.usage.num_turns, event.num_turns, usage.num_turns);
    var cost = numberOr(state.usage.cost_usd, event.total_cost_usd, event.cost_usd, usage.total_cost_usd, usage.cost_usd);

    if (Number.isFinite(input)) state.usage.input_tokens = input;
    if (Number.isFinite(output)) state.usage.output_tokens = output;
    if (Number.isFinite(turns)) state.usage.num_turns = turns;
    if (Number.isFinite(cost)) state.usage.cost_usd = cost;
  }

  function mergeSpawns(rawSpawns, source) {
    var nextSpawns = normalizeSpawns(rawSpawns);
    var previousByID = {};
    var seenIDs = {};

    state.spawns.forEach(function (spawn) {
      previousByID[spawn.id] = spawn;
    });

    if (!nextSpawns.length) {
      if (source === 'poll' && state.spawns.length === 0) return;
      state.spawns = nextSpawns;
      return;
    }

    var mergedUpdates = nextSpawns.map(function (spawn) {
      seenIDs[spawn.id] = true;
      return mergeSpawnRecord(previousByID[spawn.id], spawn);
    });

    if (source === 'poll') {
      state.spawns = mergedUpdates;
    } else {
      var retained = state.spawns.filter(function (spawn) {
        return !seenIDs[spawn.id];
      });
      state.spawns = mergedUpdates.concat(retained).sort(sortByStartTimeDesc);
    }

    mergedUpdates.forEach(function (spawn) {
      var prev = previousByID[spawn.id] || null;
      if (!prev) {
        addStreamEvent({
          scope: 'spawn-' + spawn.id,
          type: 'text',
          text: 'Spawn started: ' + (spawn.task || 'new task')
        });
        addActivityFromEvent({
          scope: 'spawn-' + spawn.id,
          ts: parseTimestamp(spawn.started_at),
          type: 'spawn_started',
          text: 'Spawn started: ' + (spawn.task || 'new task')
        });
        return;
      }

      if (normalizeStatus(prev.status) !== normalizeStatus(spawn.status)) {
        addStreamEvent({
          scope: 'spawn-' + spawn.id,
          type: 'text',
          text: 'Status: ' + (prev.status || 'unknown') + ' -> ' + (spawn.status || 'unknown')
        });
        addActivityFromEvent({
          scope: 'spawn-' + spawn.id,
          ts: Date.now(),
          type: 'spawn_status',
          text: 'Status changed to ' + (spawn.status || 'unknown')
        });
      }

      if (!prev.question && spawn.question && normalizeStatus(spawn.status) === 'awaiting_input') {
        addStreamEvent({
          scope: 'spawn-' + spawn.id,
          type: 'text',
          text: 'Awaiting input: ' + spawn.question
        });
      }
    });
  }

  function mergeSpawnRecord(previous, next) {
    if (!previous) return next;
    if (!next) return previous;

    return {
      id: next.id || previous.id,
      parent_turn_id: next.parent_turn_id || previous.parent_turn_id,
      parent_spawn_id: next.parent_spawn_id || previous.parent_spawn_id,
      child_turn_id: next.child_turn_id || previous.child_turn_id,
      profile: next.profile || previous.profile,
      role: next.role || previous.role,
      parent_profile: next.parent_profile || previous.parent_profile,
      status: next.status || previous.status,
      question: next.question || previous.question,
      task: next.task || previous.task,
      branch: next.branch || previous.branch,
      started_at: next.started_at || previous.started_at,
      completed_at: next.completed_at || previous.completed_at,
      summary: next.summary || previous.summary
    };
  }

  function pickActiveLoopRun(runs) {
    var list = arrayOrEmpty(runs).map(normalizeLoopRun);
    if (!list.length) return null;

    var running = list.filter(function (run) {
      return normalizeStatus(run.status) === 'running';
    });

    if (running.length) {
      running.sort(function (a, b) {
        return parseTimestamp(b.started_at) - parseTimestamp(a.started_at);
      });
      return running[0];
    }

    return null;
  }

  function applyLoopWireUpdate(type, data) {
    if (!data || typeof data !== 'object') return;

    var runID = Number(data.run_id || 0);
    if (!state.loopRun && runID > 0) {
      state.loopRun = {
        id: runID,
        hex_id: data.run_hex_id || '',
        loop_name: data.loop_name || 'loop',
        status: 'running',
        cycle: Number(data.cycle || 0),
        step_index: Number(data.step_index || 0),
        steps: [],
        started_at: ''
      };
    }

    if (!state.loopRun) return;
    if (runID > 0 && state.loopRun.id > 0 && state.loopRun.id !== runID) return;

    if (runID > 0) state.loopRun.id = runID;
    if (data.run_hex_id) state.loopRun.hex_id = data.run_hex_id;
    if (data.profile && (!state.loopRun.steps || !state.loopRun.steps.length)) {
      state.loopRun.steps = [{ profile: data.profile }];
    }

    if (Number.isFinite(Number(data.cycle))) state.loopRun.cycle = Number(data.cycle);
    if (Number.isFinite(Number(data.step_index))) state.loopRun.step_index = Number(data.step_index);
    if (Number.isFinite(Number(data.total_steps)) && Number(data.total_steps) > 0) {
      var total = Number(data.total_steps);
      if (!Array.isArray(state.loopRun.steps)) state.loopRun.steps = [];
      if (state.loopRun.steps.length < total) {
        while (state.loopRun.steps.length < total) {
          state.loopRun.steps.push({ profile: '' });
        }
      }
    }

    if (type === 'loop_step_start') {
      state.loopRun.status = 'running';
    }
  }

  function applyLoopDone(data) {
    if (!state.loopRun) return;

    if (data && typeof data === 'object') {
      if (data.run_id && state.loopRun.id && Number(data.run_id) !== Number(state.loopRun.id)) {
        return;
      }
      state.loopRun.status = data.reason || data.error ? (data.reason || 'completed') : 'completed';
    } else {
      state.loopRun.status = 'completed';
    }
  }

  function loopStepText(data, started) {
    var cycle = Number(data && data.cycle);
    var step = Number(data && data.step_index);
    var total = Number(data && data.total_steps);
    var profile = data && data.profile ? String(data.profile) : '';

    var cycleText = Number.isFinite(cycle) ? String(cycle + 1) : '?';
    var stepText = Number.isFinite(step) ? String(step + 1) : '?';
    var totalText = Number.isFinite(total) && total > 0 ? String(total) : '?';

    return (started ? 'Starting' : 'Finished') + ' cycle ' + cycleText + ' step ' + stepText + '/' + totalText + (profile ? (' · ' + profile) : '');
  }

  function addStreamEvent(entry) {
    if (!entry) return;

    var normalized = {
      id: Date.now().toString(36) + Math.random().toString(36).slice(2, 8),
      ts: Number.isFinite(Number(entry.ts)) ? Number(entry.ts) : Date.now(),
      scope: entry.scope || (state.currentSessionSocketID ? ('session-' + state.currentSessionSocketID) : 'session-0'),
      type: entry.type || 'text',
      text: entry.text != null ? String(entry.text) : '',
      tool: entry.tool || '',
      input: entry.input || '',
      result: entry.result || ''
    };

    var last = state.streamEvents[state.streamEvents.length - 1];
    if (
      last &&
      last.scope === normalized.scope &&
      last.type === normalized.type &&
      last.text === normalized.text &&
      last.tool === normalized.tool
    ) {
      return;
    }

    state.streamEvents.push(normalized);
    if (state.streamEvents.length > MAX_STREAM_EVENTS) {
      state.streamEvents = state.streamEvents.slice(state.streamEvents.length - MAX_STREAM_EVENTS);
    }

    addActivityFromEvent(normalized);
  }

  function addActivityFromEvent(event) {
    if (!event) return;

    var type = event.type || 'text';
    if (type === 'thinking') return;

    var description = '';
    if (type === 'tool_use') {
      description = (event.tool || 'tool') + ' → ' + stringifyToolPayload(event.input || '');
    } else if (type === 'tool_result') {
      description = (event.tool || 'result') + ': ' + stringifyToolPayload(event.result || event.text || '');
    } else {
      description = String(event.text || '').trim();
    }

    if (!description) return;

    var activity = {
      id: event.id || (Date.now().toString(36) + Math.random().toString(36).slice(2, 8)),
      ts: Number.isFinite(Number(event.ts)) ? Number(event.ts) : Date.now(),
      scope: event.scope || 'session-0',
      type: type,
      text: cropText(description, 200)
    };

    var last = state._activityLast;
    if (last && last.scope === activity.scope && last.type === activity.type && last.text === activity.text) {
      return;
    }

    state._activityLast = activity;

    if (!Array.isArray(state._activity)) state._activity = [];
    state._activity.push(activity);
    if (state._activity.length > MAX_ACTIVITY_EVENTS) {
      state._activity = state._activity.slice(state._activity.length - MAX_ACTIVITY_EVENTS);
    }
  }

  function filteredStreamEvents() {
    var events = state.streamEvents;
    var scope = state.selectedScope;
    if (!scope) return events;

    if (scope.indexOf('session-') === 0) {
      var sessionID = parseInt(scope.slice(8), 10);
      if (Number.isNaN(sessionID)) return events;
      return events.filter(function (event) {
        if (event.scope === scope) return true;
        if (event.scope.indexOf('spawn-') === 0) {
          var spawnID = parseInt(event.scope.slice(6), 10);
          if (Number.isNaN(spawnID)) return false;
          var spawn = state.spawns.find(function (item) { return item.id === spawnID; }) || null;
          if (!spawn) return false;
          return spawn.parent_turn_id === sessionID || spawn.child_turn_id === sessionID;
        }
        return false;
      });
    }

    if (scope.indexOf('spawn-') === 0) {
      return events.filter(function (event) {
        return event.scope === scope;
      });
    }

    return events;
  }

  function communicationMessages() {
    var list = state.messages.slice();

    var seenAsk = {};
    list.forEach(function (msg) {
      if (msg.type === 'ask' && msg.spawn_id) {
        seenAsk[msg.spawn_id] = true;
      }
    });

    state.spawns.forEach(function (spawn) {
      if (normalizeStatus(spawn.status) === 'awaiting_input' && spawn.question && !seenAsk[spawn.id]) {
        list.push({
          id: 'ask-' + spawn.id,
          spawn_id: spawn.id,
          type: 'ask',
          direction: 'child_to_parent',
          content: spawn.question,
          created_at: spawn.started_at || new Date().toISOString(),
          step_index: null
        });
      }

      if (spawn.summary && normalizeStatus(spawn.status) === 'completed') {
        list.push({
          id: 'reply-' + spawn.id,
          spawn_id: spawn.id,
          type: 'reply',
          direction: 'child_to_parent',
          content: spawn.summary,
          created_at: spawn.completed_at || spawn.started_at || new Date().toISOString(),
          step_index: null
        });
      }
    });

    list.sort(function (a, b) {
      return parseTimestamp(b.created_at) - parseTimestamp(a.created_at);
    });

    return list;
  }

  function spawnMapByID() {
    var map = {};
    state.spawns.forEach(function (spawn) {
      map[spawn.id] = spawn;
    });
    return map;
  }

  function renderApp() {
    if (!content) return;

    var rootClasses = 'app-root';
    if (state.currentPanel === 'right') {
      rootClasses += ' mobile-right';
    } else {
      rootClasses += ' mobile-left';
    }

    content.innerHTML = '' +
      '<div class="' + rootClasses + '">' +
        renderHeaderBar() +
        renderLoopBar() +
        '<div class="main-shell">' +
          renderLeftPanel() +
          renderRightPanel() +
        '</div>' +
      '</div>';

    applyPostRenderEffects();
    renderModal();
  }

  function renderHeaderBar() {
    var projectName = escapeHTML(currentProjectName());
    var usage = state.usage || {
      input_tokens: 0,
      output_tokens: 0,
      cost_usd: 0,
      num_turns: state.turns.length || 0
    };

    var usageTurns = Number(usage.num_turns) || 0;
    if (!usageTurns && state.turns.length) usageTurns = state.turns.length;

    var loopIndicator = '';
    if (state.loopRun && normalizeStatus(state.loopRun.status) === 'running') {
      loopIndicator = '' +
        '<span class="header-sep"></span>' +
        '<span class="loop-indicator">' + icon('refresh', 'spin') + escapeHTML(state.loopRun.loop_name || ('loop-' + state.loopRun.id)) + '</span>';
    }

    var projectPicker = '';
    if (state.projects.length > 1) {
      projectPicker = '' +
        '<select id="project-select" class="project-select" data-change="project-select" title="Switch project">' +
          state.projects.map(function (project) {
            var id = project && project.id ? String(project.id) : '';
            var label = project && project.name ? project.name : id || 'Unnamed project';
            if (project && project.is_default) label += ' (default)';
            return '<option value="' + escapeHTML(id) + '"' + (id === state.currentProjectID ? ' selected' : '') + '>' + escapeHTML(label) + '</option>';
          }).join('') +
        '</select>';
    }

    var wsOnline = !!(state.wsConnected || state.termWSConnected);

    return '' +
      '<div class="header-bar">' +
        '<span class="brand">adaf</span>' +
        '<span class="header-sep"></span>' +
        '<div class="project-block">' +
          '<span class="project-name">' + projectName + '</span>' +
          projectPicker +
        '</div>' +
        loopIndicator +
        '<span class="header-spacer"></span>' +
        '<span class="usage-stats mono">' +
          '<span>in=' + formatNumber(usage.input_tokens || 0) + '</span>' +
          '<span>out=' + formatNumber(usage.output_tokens || 0) + '</span>' +
          '<span class="cost">$' + Number(usage.cost_usd || 0).toFixed(4) + '</span>' +
          '<span>turns=' + formatNumber(usageTurns) + '</span>' +
        '</span>' +
        '<span class="ws-pill' + (wsOnline ? ' online' : '') + '"><span class="ws-pill-dot"></span>' + (wsOnline ? 'live' : 'offline') + '</span>' +
        '<button class="mobile-panel-toggle" data-action="toggle-mobile-panel">panel: ' + escapeHTML(state.currentPanel) + '</button>' +
      '</div>';
  }

  function renderLoopBar() {
    if (!state.loopRun) return '';
    if (normalizeStatus(state.loopRun.status) !== 'running') return '';

    var steps = arrayOrEmpty(state.loopRun.steps);
    var stepIndex = Number(state.loopRun.step_index) || 0;
    var cycle = Number(state.loopRun.cycle) || 0;
    var loopElapsed = formatElapsed(state.loopRun.started_at, state.loopRun.completed_at || state.loopRun.ended_at);

    var pills = steps.map(function (step, idx) {
      var cls = 'loop-pill';
      var marker = '○';
      if (idx < stepIndex) {
        cls += ' done';
        marker = '✓';
      } else if (idx === stepIndex) {
        cls += ' current';
        marker = '▶';
      }

      return '' +
        '<span class="' + cls + '">' +
          '<span class="marker">' + marker + '</span>' +
          '<span>' + escapeHTML(step.profile || ('step-' + (idx + 1))) + '</span>' +
        '</span>';
    }).join('');

    return '' +
      '<div class="loop-bar">' +
        '<span class="loop-bar-title">' + icon('refresh', 'spin') + 'Loop: ' + escapeHTML(state.loopRun.loop_name || ('loop-' + state.loopRun.id)) + '</span>' +
        renderStatusBadge(state.loopRun.status) +
        '<span class="loop-bar-meta mono">cycle ' + (cycle + 1) + ' · step ' + (stepIndex + 1) + '/' + (steps.length || '?') + '</span>' +
        '<span class="loop-pill-row">' + pills + '</span>' +
        '<span class="loop-hex">[' + escapeHTML(state.loopRun.hex_id || String(state.loopRun.id || '')) + '] ' + escapeHTML(loopElapsed) + '</span>' +
      '</div>';
  }

  function renderLeftPanel() {
    var tabs = [
      { id: 'agents', label: 'Agents', icon: 'bot', count: state.sessions.length + state.spawns.length },
      { id: 'issues', label: 'Issues', icon: 'alert', count: state.issues.length },
      { id: 'docs', label: 'Docs', icon: 'file', count: state.docs.length },
      { id: 'plan', label: 'Plan', icon: 'list', count: state.activePlan && state.activePlan.phases ? state.activePlan.phases.length : state.plans.length },
      { id: 'logs', label: 'Logs', icon: 'scroll', count: state.turns.length }
    ];

    var contentHTML = '';
    if (state.leftView === 'agents') contentHTML = renderAgentsView();
    if (state.leftView === 'issues') contentHTML = renderIssuesView();
    if (state.leftView === 'docs') contentHTML = renderDocsView();
    if (state.leftView === 'plan') contentHTML = renderPlanView();
    if (state.leftView === 'logs') contentHTML = renderLogsView();

    return '' +
      '<aside class="left-panel">' +
        '<div class="panel-tabs">' +
          tabs.map(function (tab, idx) {
            var active = tab.id === state.leftView;
            return '' +
              '<button class="view-tab' + (active ? ' active' : '') + '" data-action="set-left-view" data-view="' + tab.id + '">' +
                '<span class="tab-shortcut">' + (idx + 1) + '</span>' +
                icon(tab.icon, '') +
                '<span>' + escapeHTML(tab.label) + '</span>' +
                '<span class="tab-count">' + formatNumber(tab.count) + '</span>' +
              '</button>';
          }).join('') +
        '</div>' +
        renderLeftViewHeader() +
        '<div class="left-content">' + contentHTML + '</div>' +
        renderLeftFooter() +
      '</aside>';
  }

  function renderLeftViewHeader() {
    if (state.leftView !== 'agents') return '';

    return '' +
      '<div class="left-view-header">' +
        '<span class="left-view-title">Sessions</span>' +
        '<span class="left-view-meta mono">' + state.sessions.length + ' turns · ' + state.spawns.length + ' spawns</span>' +
        '<button class="left-view-action primary" data-action="open-new-session-modal">New Session</button>' +
      '</div>';
  }

  function renderLeftFooter() {
    var label = 'none';
    var elapsed = '--';

    if (state.selectedScope && state.selectedScope.indexOf('session-') === 0) {
      var sessionID = parseInt(state.selectedScope.slice(8), 10);
      var session = state.sessions.find(function (item) { return item.id === sessionID; }) || null;
      if (session) {
        label = session.agent || session.profile || ('session-' + session.id);
        elapsed = formatElapsed(session.started_at, session.ended_at);
      }
    } else if (state.selectedScope && state.selectedScope.indexOf('spawn-') === 0) {
      var spawnID = parseInt(state.selectedScope.slice(6), 10);
      var spawn = state.spawns.find(function (item) { return item.id === spawnID; }) || null;
      if (spawn) {
        label = spawn.profile || ('spawn-' + spawn.id);
        elapsed = formatElapsed(spawn.started_at, spawn.completed_at);
      }
    }

    return '' +
      '<div class="left-footer">' +
        '<span>Agent: <span class="value">' + escapeHTML(label) + '</span></span>' +
        '<span>Elapsed: <span class="value">' + escapeHTML(elapsed) + '</span></span>' +
      '</div>';
  }

  function renderAgentsView() {
    if (!state.sessions.length && !state.spawns.length) {
      return '<div class="empty-state">No sessions or spawns yet.</div>';
    }

    var childrenByParent = {};
    var rootsBySession = {};
    var spawnsByID = spawnMapByID();

    state.spawns.forEach(function (spawn) {
      if (spawn.parent_spawn_id > 0) {
        if (!childrenByParent[spawn.parent_spawn_id]) childrenByParent[spawn.parent_spawn_id] = [];
        childrenByParent[spawn.parent_spawn_id].push(spawn);
      } else {
        var sessionKey = spawn.parent_turn_id || 0;
        if (!rootsBySession[sessionKey]) rootsBySession[sessionKey] = [];
        rootsBySession[sessionKey].push(spawn);
      }
    });

    Object.keys(childrenByParent).forEach(function (key) {
      childrenByParent[key].sort(sortByStartTimeDesc);
    });
    Object.keys(rootsBySession).forEach(function (key) {
      rootsBySession[key].sort(sortByStartTimeDesc);
    });

    var msgCounts = {};
    communicationMessages().forEach(function (msg) {
      if (msg.spawn_id > 0) {
        msgCounts[msg.spawn_id] = (msgCounts[msg.spawn_id] || 0) + 1;
      }
    });

    var sessions = state.sessions.slice().sort(function (a, b) {
      return b.id - a.id;
    });

    var html = sessions.map(function (session) {
      var sessionNodeID = 'session-' + session.id;
      var selected = state.selectedScope === sessionNodeID;
      var rootSpawns = rootsBySession[session.id] || [];
      var expanded = state.expandedNodes.has(sessionNodeID);
      var status = normalizeStatus(session.status);
      var stopControl = STATUS_RUNNING[status]
        ? '<button class="session-stop-btn" data-action="stop-session" data-session-id="' + session.id + '" title="Stop session">■</button>'
        : '';

      var childrenHTML = '';
      if (expanded) {
        childrenHTML = rootSpawns.map(function (spawn) {
          return renderSpawnNode(spawn, 1, childrenByParent, msgCounts, spawnsByID);
        }).join('');
      }

      return '' +
        '<div class="scope-group">' +
          '<div class="scope-row session' + (selected ? ' selected' : '') + '" data-action="set-scope" data-scope="' + sessionNodeID + '">' +
            '<button class="expand-toggle" data-action="toggle-node" data-node="' + sessionNodeID + '">' +
              (rootSpawns.length ? icon(expanded ? 'chevronDown' : 'chevronRight', '') : '<span style="display:inline-block;width:12px"></span>') +
            '</button>' +
            '<span class="scope-dot' + (status === 'running' ? ' live' : '') + '" style="background:' + statusColor(session.status) + '"></span>' +
            '<div class="scope-main">' +
              '<div class="scope-head">' +
                '<span class="scope-id">turn #' + session.id + '</span>' +
                '<span class="scope-profile">' + escapeHTML(session.profile || 'unknown') + '</span>' +
                '<span class="scope-role">(' + escapeHTML(session.agent || 'agent') + ')</span>' +
                renderStatusBadge(session.status) +
              '</div>' +
              '<div class="scope-meta mono">' +
                escapeHTML(session.model || 'model n/a') + ' · ' +
                escapeHTML(formatElapsed(session.started_at, session.ended_at)) +
                (session.action ? (' · <span style="color:#a6adc8">' + escapeHTML(session.action) + '</span>') : '') +
              '</div>' +
            '</div>' +
            stopControl +
            (rootSpawns.length ? '<span class="count-chip spawn-chip">' + icon('fork', '') + rootSpawns.length + '</span>' : '') +
          '</div>' +
          childrenHTML +
        '</div>';
    }).join('');

    var orphanSpawns = state.spawns.filter(function (spawn) {
      return (!spawn.parent_turn_id || !state.sessions.find(function (session) { return session.id === spawn.parent_turn_id; })) && spawn.parent_spawn_id <= 0;
    });

    if (orphanSpawns.length) {
      html += '<div class="scope-group">' +
        '<div class="scope-meta" style="padding:6px 10px">Detached spawns</div>' +
        orphanSpawns.map(function (spawn) {
          return renderSpawnNode(spawn, 1, childrenByParent, msgCounts, spawnsByID);
        }).join('') +
      '</div>';
    }

    return html;
  }

  function renderSpawnNode(spawn, depth, childrenByParent, msgCounts, spawnsByID) {
    var nodeID = 'spawn-' + spawn.id;
    var selected = state.selectedScope === nodeID;
    var children = childrenByParent[spawn.id] || [];
    var expanded = state.expandedNodes.has(nodeID);
    var status = normalizeStatus(spawn.status);
    var msgCount = msgCounts[spawn.id] || 0;
    var hasPendingQuestion = status === 'awaiting_input' && !!spawn.question;

    var childrenHTML = '';
    if (expanded) {
      childrenHTML = children.map(function (child) {
        return renderSpawnNode(child, depth + 1, childrenByParent, msgCounts, spawnsByID);
      }).join('');
    }

    var connector = '';
    if (depth > 0) {
      var rails = '';
      for (var i = 1; i < depth; i += 1) {
        rails += '<span class="scope-rail"></span>';
      }
      connector = '<span class="scope-tree-connector">' + rails + '<span class="scope-corner">└</span></span>';
    }

    return '' +
      '<div class="scope-tree-indent" style="--depth:' + ((depth - 1) * 18) + 'px">' +
        '<div class="scope-row spawn' + (selected ? ' selected' : '') + '" data-action="set-scope" data-scope="' + nodeID + '">' +
          connector +
          '<button class="expand-toggle" data-action="toggle-node" data-node="' + nodeID + '">' +
            (children.length ? icon(expanded ? 'chevronDown' : 'chevronRight', '') : '<span style="display:inline-block;width:12px"></span>') +
          '</button>' +
          '<span class="scope-dot' + (status === 'running' ? ' live' : '') + '" style="background:' + statusColor(spawn.status) + '"></span>' +
          '<div class="scope-main">' +
            '<div class="scope-head">' +
              '<span class="scope-id">#' + spawn.id + '</span>' +
              '<span class="scope-profile">' + escapeHTML(spawn.profile || 'spawn') + '</span>' +
              (spawn.role ? '<span class="scope-role">as ' + escapeHTML(spawn.role) + '</span>' : '') +
              renderStatusBadge(spawn.status) +
              '<span class="scope-role mono">' + escapeHTML(formatElapsed(spawn.started_at, spawn.completed_at)) + '</span>' +
              (msgCount ? '<span class="count-chip">' + icon('message', '') + msgCount + '</span>' : '') +
            '</div>' +
            (spawn.task ? '<div class="scope-task">' + escapeHTML(spawn.task) + '</div>' : '') +
            (spawn.branch ? '<div class="scope-extra mono">' + icon('branch', '') + ' ' + escapeHTML(spawn.branch) + '</div>' : '') +
            (hasPendingQuestion ? '<div class="pending-question"><span class="pending-icon">' + icon('alert', '') + '</span><div><strong>AWAITING RESPONSE</strong>' + escapeHTML(spawn.question) + '</div></div>' : '') +
          '</div>' +
        '</div>' +
        childrenHTML +
      '</div>';
  }

  function renderIssuesView() {
    if (!state.issues.length) {
      return '<div class="empty-state">No issues found.</div>';
    }

    return state.issues.map(function (issue) {
      var selected = state.selectedIssue === issue.id;
      return '' +
        '<div class="list-item-card' + (selected ? ' selected' : '') + '" data-action="select-issue" data-issue-id="' + issue.id + '">' +
          '<div class="list-title-row">' +
            '<span class="list-id">#' + issue.id + '</span>' +
            '<span class="list-title">' + escapeHTML(issue.title || 'Untitled issue') + '</span>' +
          '</div>' +
          '<div class="meta-row">' +
            renderStatusBadge(issue.priority) +
            renderStatusBadge(issue.status) +
            arrayOrEmpty(issue.labels).map(function (label) {
              return '<span class="small-badge label">' + escapeHTML(label) + '</span>';
            }).join('') +
          '</div>' +
        '</div>';
    }).join('');
  }

  function renderPlanView() {
    var plan = state.activePlan;
    if (!plan) {
      if (!state.plans.length) {
        return '<div class="empty-state">No plans loaded.</div>';
      }

      return '' +
        '<div class="empty-state">Select a plan.</div>' +
        state.plans.map(function (item) {
          var selected = state.selectedPlan === item.id;
          return '<div class="list-item-card' + (selected ? ' selected' : '') + '" data-action="select-plan" data-plan-id="' + escapeHTML(item.id) + '">' +
            '<div class="list-title-row"><span class="list-id mono">' + escapeHTML(item.id) + '</span><span class="list-title">' + escapeHTML(item.title || item.id) + '</span>' + renderStatusBadge(item.status || 'active') + '</div>' +
          '</div>';
        }).join('');
    }

    var phases = arrayOrEmpty(plan.phases);
    var completeCount = phases.filter(function (phase) {
      return normalizeStatus(phase.status) === 'complete';
    }).length;
    var percent = phases.length ? Math.round((completeCount / phases.length) * 100) : 0;

    var planSelector = '';
    if (state.plans.length > 1) {
      planSelector = state.plans.map(function (item) {
        var selected = state.selectedPlan === item.id;
        return '<div class="list-item-card' + (selected ? ' selected' : '') + '" data-action="select-plan" data-plan-id="' + escapeHTML(item.id) + '">' +
          '<div class="list-title-row"><span class="list-id mono">' + escapeHTML(item.id) + '</span><span class="list-title">' + escapeHTML(item.title || item.id) + '</span></div>' +
        '</div>';
      }).join('');
    }

    return '' +
      '<div class="plan-overview">' +
        '<div class="plan-title">' + escapeHTML(plan.title || plan.id || 'Plan') + '</div>' +
        '<div class="progress-track"><div class="progress-fill" style="width:' + percent + '%"></div></div>' +
        '<div class="progress-meta">' + percent + '% complete</div>' +
      '</div>' +
      planSelector +
      phases.map(function (phase) {
        var status = normalizeStatus(phase.status || 'not_started');
        var marker = '○';
        if (status === 'complete') marker = '✓';
        else if (status === 'in_progress') marker = '◉';
        else if (status === 'blocked') marker = '✗';

        return '' +
          '<div class="phase-card" style="border-left-color:' + statusColor(status) + '">' +
            '<div class="phase-head">' +
              '<span class="phase-marker" style="color:' + statusColor(status) + '">' + marker + '</span>' +
              '<span class="phase-title">' + escapeHTML(phase.title || phase.id || 'Phase') + '</span>' +
              renderStatusBadge(phase.status || 'not_started') +
            '</div>' +
            (phase.description ? '<div class="phase-desc">' + escapeHTML(phase.description) + '</div>' : '') +
            (arrayOrEmpty(phase.depends_on).length ? '<div class="phase-deps mono">depends on: ' + escapeHTML(phase.depends_on.join(', ')) + '</div>' : '') +
          '</div>';
      }).join('');
  }

  function renderLogsView() {
    if (!state.turns.length) {
      return '<div class="empty-state">No turns recorded yet.</div>';
    }

    return state.turns.map(function (turn) {
      var status = normalizeStatus(turn.build_state || 'unknown');
      var border = status === 'passing' ? '#a6e3a1' : '#f38ba8';

      return '' +
        '<div class="turn-card" style="border-left-color:' + border + '">' +
          '<div class="turn-head">' +
            '<span class="turn-id">#' + escapeHTML(String(turn.id)) + ' [' + escapeHTML(turn.hex_id || '-') + ']</span>' +
            '<span class="turn-profile">' + escapeHTML(turn.profile_name || '-') + '</span>' +
            '<span class="turn-agent">(' + escapeHTML(turn.agent || '-') + ')</span>' +
            renderStatusBadge(turn.build_state || 'unknown') +
          '</div>' +
          '<div class="turn-objective">' + escapeHTML(turn.objective || 'No objective') + '</div>' +
          '<div class="turn-built">' + escapeHTML(turn.what_was_built || '') + '</div>' +
          '<div class="turn-meta">' + escapeHTML(turn.agent_model || 'model n/a') + ' · ' + escapeHTML(String(turn.duration_secs || 0)) + 's</div>' +
        '</div>';
    }).join('');
  }

  function renderDocsView() {
    if (!state.docs.length) {
      return '<div class="empty-state">No docs available.</div>';
    }

    return state.docs.map(function (doc) {
      return '' +
        '<div class="list-item-card doc-card">' +
          '<div class="list-title-row">' +
            icon('file', '') +
            '<span class="list-title">' + escapeHTML(doc.title || doc.id || 'Document') + '</span>' +
            '<span class="list-id mono">' + escapeHTML(doc.id || '') + '</span>' +
          '</div>' +
          '<div class="list-preview">' + escapeHTML(cropText((doc.content || '').replace(/\s+/g, ' ').trim(), 160)) + '</div>' +
        '</div>';
    }).join('');
  }

  function renderRightPanel() {
    return '' +
      '<section class="right-panel">' +
        renderRightTabBar() +
        '<div class="right-content">' + renderRightLayerContent() + '</div>' +
        renderSessionMessageBar() +
        renderStatusBar() +
      '</section>';
  }

  function renderRightTabBar() {
    var layers = [
      { id: 'raw', label: 'Raw', icon: 'terminal' },
      { id: 'activity', label: 'Activity', icon: 'activity' },
      { id: 'messages', label: 'Messages', icon: 'message' }
    ];

    var scopeLabel = state.selectedScope || 'all';

    return '' +
      '<div class="layer-tabs">' +
        layers.map(function (layer) {
          var active = state.rightLayer === layer.id;
          return '' +
            '<button class="layer-tab' + (active ? ' active' : '') + '" data-action="set-right-layer" data-layer="' + layer.id + '">' +
              icon(layer.icon, '') +
              '<span>' + escapeHTML(layer.label) + '</span>' +
            '</button>';
        }).join('') +
        '<span class="layer-tools">' +
          '<span class="scope-pill">' + icon('eye', '') + 'scope: <b>' + escapeHTML(scopeLabel) + '</b>' +
            (state.selectedScope ? '<button class="scope-pill-clear" data-action="clear-scope" title="Clear scope">×</button>' : '') +
          '</span>' +
          '<button class="toggle-chip' + (state.autoScroll ? ' active' : '') + '" data-action="toggle-auto-scroll">' + icon('swap', '') + 'auto-scroll ' + (state.autoScroll ? 'on' : 'off') + '</button>' +
        '</span>' +
      '</div>';
  }

  function renderRightLayerContent() {
    if (state.rightLayer === 'raw') return renderRawLayer();
    if (state.rightLayer === 'activity') return renderActivityLayer();
    if (state.rightLayer === 'messages') return renderMessagesLayer();
    return '<div class="empty-state">Unknown layer</div>';
  }

  function renderRawLayer() {
    var events = filteredStreamEvents();
    if (!events.length) {
      return '<div class="empty-state">' + (state.selectedScope ? 'No events for this scope.' : 'Waiting for stream events...') + '</div>';
    }

    return '' +
      '<div class="raw-feed" id="raw-feed">' +
        events.map(function (entry) {
          var kind = normalizeStatus(entry.type || 'text');
          var scopeColorValue = scopeColor(entry.scope);
          var typeMeta = typeDisplayMeta(entry.type);
          var rowClass = 'raw-row raw-' + kind.replace(/_/g, '-');

          var body = '';
          if (kind === 'tool_use') {
            body = '<span class="tool-name">' + escapeHTML(entry.tool || 'tool') + '</span> → <span class="tool-input">' + escapeHTML(stringifyToolPayload(entry.input || '')) + '</span>';
          } else if (kind === 'tool_result') {
            if (entry.tool) {
              body = '<span class="tool-name">' + escapeHTML(entry.tool) + ':</span> ' + escapeHTML(stringifyToolPayload(entry.result || entry.text || ''));
            } else {
              body = escapeHTML(stringifyToolPayload(entry.result || entry.text || ''));
            }
          } else {
            body = escapeHTML(entry.text || '');
          }

          return '' +
            '<div class="' + rowClass + '">' +
              '<span class="raw-scope">' +
                '<span class="raw-scope-dot" style="background:' + scopeColorValue + '"></span>' +
                '<span class="raw-scope-label" style="color:' + scopeColorValue + '">' + escapeHTML(scopeShortLabel(entry.scope)) + '</span>' +
              '</span>' +
              '<span class="raw-type" style="color:' + typeMeta.color + '">' + escapeHTML(typeMeta.label) + '</span>' +
              '<span class="raw-text">' + body + '</span>' +
            '</div>';
        }).join('') +
      '</div>';
  }

  function renderActivityLayer() {
    var source = Array.isArray(state._activity) ? state._activity.slice().reverse() : [];
    var entries = source.slice(0, 80);

    if (!entries.length) {
      return '<div class="empty-state">No activity yet.</div>';
    }

    return '' +
      '<div class="activity-feed">' +
        '<div class="layer-panel-title">Activity Feed</div>' +
        entries.map(function (entry, idx) {
          var display = activityDisplay(entry);
          return '' +
            '<div class="activity-entry" style="animation-delay:' + (idx * 0.03) + 's">' +
              '<div class="activity-rail">' +
                '<span class="activity-icon" style="color:' + display.color + ';border-color:' + withAlpha(display.color, 0.35) + ';background:' + withAlpha(display.color, 0.16) + '">' + display.icon + '</span>' +
                (idx < entries.length - 1 ? '<span class="activity-line"></span>' : '') +
              '</div>' +
              '<div class="activity-body">' +
                '<div class="activity-head"><span class="activity-scope">' + escapeHTML(scopeShortLabel(entry.scope)) + '</span><span class="activity-time">' + escapeHTML(formatRelativeTime(entry.ts)) + '</span></div>' +
                '<div class="activity-desc">' + escapeHTML(entry.text || '') + '</div>' +
              '</div>' +
            '</div>';
        }).join('') +
      '</div>';
  }

  function renderMessagesLayer() {
    var messages = communicationMessages();
    if (!messages.length) {
      return '<div class="empty-state">No agent communication yet.</div>';
    }

    var selectedScope = state.selectedScope;
    if (selectedScope && selectedScope.indexOf('spawn-') === 0) {
      var targetSpawn = parseInt(selectedScope.slice(6), 10);
      if (!Number.isNaN(targetSpawn)) {
        messages = messages.filter(function (msg) { return Number(msg.spawn_id) === targetSpawn; });
      }
    }

    if (selectedScope && selectedScope.indexOf('session-') === 0) {
      var targetSession = parseInt(selectedScope.slice(8), 10);
      if (!Number.isNaN(targetSession)) {
        messages = messages.filter(function (msg) {
          if (!msg.spawn_id) return true;
          var spawn = state.spawns.find(function (item) { return item.id === Number(msg.spawn_id); }) || null;
          if (!spawn) return false;
          return spawn.parent_turn_id === targetSession || spawn.child_turn_id === targetSession;
        });
      }
    }

    if (!messages.length) {
      return '<div class="empty-state">No messages for this scope.</div>';
    }

    return '' +
      '<div class="messages-feed">' +
        '<div class="layer-panel-title">Agent Communication</div>' +
        messages.map(function (msg) {
          var type = normalizeStatus(msg.type || 'message');
          if (type !== 'ask' && type !== 'reply') type = 'message';
          var direction = normalizeStatus(msg.direction) === 'parent_to_child' ? '↓' : '↑';
          var spawnLabel = msg.spawn_id ? ('spawn #' + msg.spawn_id) : (msg.step_index != null ? ('step ' + msg.step_index) : 'loop');
          var spawn = msg.spawn_id ? state.spawns.find(function (item) { return item.id === Number(msg.spawn_id); }) || null : null;
          var spawnProfile = (spawn && spawn.profile) ? '<span class="message-spawn-profile"> (' + escapeHTML(spawn.profile) + ')</span>' : '';

          return '' +
            '<div class="message-card ' + type + '">' +
              '<div class="message-head">' +
                '<span class="message-type ' + type + '">' + escapeHTML(type) + '</span>' +
                '<span class="message-meta">' + direction + ' ' + escapeHTML(spawnLabel) + spawnProfile + '</span>' +
                '<span class="message-time">' + escapeHTML(formatRelativeTime(msg.created_at)) + '</span>' +
              '</div>' +
              '<div class="message-content">' + escapeHTML(msg.content || '') + '</div>' +
            '</div>';
        }).join('') +
      '</div>';
  }

  function selectedRunningSession() {
    if (!state.selectedScope || state.selectedScope.indexOf('session-') !== 0) return null;

    var sessionID = parseInt(state.selectedScope.slice(8), 10);
    if (Number.isNaN(sessionID) || sessionID <= 0) return null;

    var session = state.sessions.find(function (item) {
      return item.id === sessionID;
    }) || null;
    if (!session) return null;

    if (!STATUS_RUNNING[normalizeStatus(session.status)]) return null;
    return session;
  }

  function resolveSessionMessageTarget() {
    var runningSession = selectedRunningSession();
    if (!runningSession) return null;

    var loopID = Number(state.loopRun && state.loopRun.id);
    var loopStatus = normalizeStatus(state.loopRun && state.loopRun.status);
    if (loopID > 0 && STATUS_RUNNING[loopStatus] && !!runningSession.loop_name) {
      return {
        kind: 'loop',
        id: loopID,
        sessionID: runningSession.id
      };
    }

    return {
      kind: 'session',
      id: runningSession.id,
      sessionID: runningSession.id
    };
  }

  function renderSessionMessageBar() {
    var target = resolveSessionMessageTarget();
    if (!target) return '';

    var targetLabel = target.kind === 'loop'
      ? ('loop #' + target.id + ' (session #' + target.sessionID + ')')
      : ('session #' + target.id);

    return '' +
      '<form class="session-message-bar" data-form="session-message">' +
        '<span class="session-message-target mono">' + escapeHTML(targetLabel) + '</span>' +
        '<input type="text" name="session_message" data-input="session-message" value="' + escapeHTML(state.sessionMessageDraft) + '" placeholder="Send message to running session" autocomplete="off">' +
        '<button type="submit" class="session-message-send">Send</button>' +
      '</form>';
  }

  async function submitSessionMessageForm(form) {
    var target = resolveSessionMessageTarget();
    if (!target) {
      showToast('Select a running session to send a message.', 'error');
      return;
    }

    var message = readString(form, 'session_message');
    if (!message) {
      showToast('Message is required.', 'error');
      return;
    }

    var path = '';
    if (target.kind === 'loop') {
      path = apiBase() + '/loops/' + encodeURIComponent(String(target.id)) + '/message';
    } else {
      path = apiBase() + '/sessions/' + encodeURIComponent(String(target.id)) + '/message';
    }

    await apiCall(path, 'POST', {
      message: message,
      content: message
    });

    state.sessionMessageDraft = '';

    if (target.kind === 'loop' && target.id > 0) {
      await refreshLoopMessages(target.id);
    }

    renderApp();
  }

  function renderStatusBar() {
    var running = state.spawns.filter(function (spawn) {
      return normalizeStatus(spawn.status) === 'running';
    }).length;

    var awaiting = state.spawns.filter(function (spawn) {
      return normalizeStatus(spawn.status) === 'awaiting_input';
    }).length;

    var done = state.spawns.filter(function (spawn) {
      var status = normalizeStatus(spawn.status);
      return status === 'completed' || status === 'merged';
    }).length;

    var messages = communicationMessages();

    return '' +
      '<div class="status-bar">' +
        '<span><span style="color:#6c7086">view=</span><span class="status-key">' + escapeHTML(state.leftView) + '</span></span>' +
        '<span><span style="color:#6c7086">layer=</span><span class="status-key">' + escapeHTML(state.rightLayer) + '</span></span>' +
        '<span><span style="color:#6c7086">detail=</span><span class="status-value">' + escapeHTML(state.selectedScope || 'all') + '</span></span>' +
        '<span class="status-spacer"></span>' +
        '<span><span style="color:#6c7086">spawns: </span><span style="color:#f9e2af">' + running + ' running</span> · <span style="color:#89b4fa">' + awaiting + ' awaiting</span> · <span style="color:#a6e3a1">' + done + ' done</span></span>' +
        '<span>' + messages.length + ' messages</span>' +
      '</div>';
  }

  function applyPostRenderEffects() {
    if (state.rightLayer !== 'raw') return;
    var feed = document.getElementById('raw-feed');
    if (!feed) return;

    if (state.autoScroll) {
      feed.scrollTop = feed.scrollHeight;
    }
  }

  function renderStatusBadge(status) {
    var normalized = normalizeStatus(status || 'unknown');
    var color = statusColor(normalized);

    return '' +
      '<span class="status-badge" style="--status-color:' + color + ';--status-bg:' + withAlpha(color, 0.14) + ';--status-border:' + withAlpha(color, 0.28) + '">' +
        '<span class="marker">' + statusIcon(normalized) + '</span>' +
        '<span>' + escapeHTML(normalized) + '</span>' +
      '</span>';
  }

  function statusColor(status) {
    var key = normalizeStatus(status);
    var map = {
      running: '#f9e2af',
      starting: '#f9e2af',
      awaiting_input: '#89b4fa',
      completed: '#a6e3a1',
      complete: '#a6e3a1',
      merged: '#a6e3a1',
      passing: '#a6e3a1',
      resolved: '#a6e3a1',
      done: '#a6e3a1',
      failed: '#f38ba8',
      failing: '#f38ba8',
      canceled: '#f38ba8',
      cancelled: '#f38ba8',
      rejected: '#f38ba8',
      blocked: '#f38ba8',
      stopped: '#6c7086',
      not_started: '#6c7086',
      open: '#f9e2af',
      in_progress: '#f9e2af',
      critical: '#f38ba8',
      high: '#fab387',
      medium: '#f9e2af',
      low: '#6c7086',
      active: '#89b4fa'
    };
    return map[key] || '#a6adc8';
  }

  function statusIcon(status) {
    var key = normalizeStatus(status);
    var map = {
      running: '◉',
      starting: '◉',
      awaiting_input: '◎',
      completed: '✓',
      complete: '✓',
      merged: '⊕',
      passing: '✓',
      failed: '✗',
      failing: '✗',
      canceled: '⊘',
      cancelled: '⊘',
      rejected: '⊗',
      blocked: '✗',
      stopped: '■',
      resolved: '✓',
      in_progress: '◉',
      open: '◉'
    };
    return map[key] || '○';
  }

  function typeDisplayMeta(type) {
    var normalized = normalizeStatus(type || 'text');
    if (normalized === 'thinking') return { label: 'thinking', color: '#9399b2' };
    if (normalized === 'tool_use') return { label: 'tool', color: '#f9e2af' };
    if (normalized === 'tool_result') return { label: 'result', color: '#a6e3a1' };
    if (normalized === 'text') return { label: 'text', color: '#cdd6f4' };
    return { label: normalized, color: '#a6adc8' };
  }

  function scopeColor(scope) {
    var key = String(scope || 'session-0');
    var parsed = key.match(/(session|spawn)-(\d+)/);
    if (!parsed) return '#7f849c';
    var idx = parseInt(parsed[2], 10);
    if (Number.isNaN(idx)) return '#7f849c';
    return SCOPE_COLOR_PALETTE[idx % SCOPE_COLOR_PALETTE.length];
  }

  function scopeShortLabel(scope) {
    var value = String(scope || 'session-0');
    if (value.indexOf('session-') === 0) return 's' + value.slice(8);
    if (value.indexOf('spawn-') === 0) return 'sp' + value.slice(6);
    return value;
  }

  function activityDisplay(entry) {
    var type = normalizeStatus(entry.type || 'text');

    if (type === 'tool_use') return { icon: '⚙', color: '#f9e2af' };
    if (type === 'tool_result') return { icon: '✓', color: '#a6e3a1' };
    if (type === 'spawn_started') return { icon: '⇢', color: '#cba6f7' };
    if (type === 'spawn_status') return { icon: '◉', color: '#89dceb' };
    if (type === 'loop_step_start') return { icon: '▶', color: '#89b4fa' };
    if (type === 'loop_step_end') return { icon: '■', color: '#fab387' };
    if (type === 'loop_done') return { icon: '✓', color: '#a6e3a1' };
    if (type === 'started') return { icon: '▶', color: '#89b4fa' };

    return { icon: '•', color: '#b4befe' };
  }

  function icon(name, extraClass) {
    var path = ICON_PATHS[name] || ICON_PATHS.bot;
    var className = 'icon' + (extraClass ? ' ' + extraClass : '');
    return '<svg class="' + className + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="' + path + '"></path></svg>';
  }

  function normalizeSessions(rawSessions) {
    return arrayOrEmpty(rawSessions).map(function (session) {
      return {
        id: Number(session && session.id) || 0,
        profile: session && (session.profile_name || session.profile) ? String(session.profile_name || session.profile) : '',
        agent: session && (session.agent_name || session.agent) ? String(session.agent_name || session.agent) : '',
        model: session && session.model ? String(session.model) : '',
        status: session && session.status ? String(session.status) : 'unknown',
        action: session && session.action ? String(session.action) : '',
        started_at: session && session.started_at ? session.started_at : '',
        ended_at: session && session.ended_at ? session.ended_at : '',
        loop_name: session && session.loop_name ? String(session.loop_name) : ''
      };
    }).filter(function (session) {
      return session.id > 0;
    }).sort(function (a, b) {
      return b.id - a.id;
    });
  }

  function normalizeSpawns(rawSpawns) {
    return arrayOrEmpty(rawSpawns).map(function (spawn) {
      var parentTurn = numberOr(0, spawn && spawn.parent_turn_id, spawn && spawn.parent_session_id);
      var childTurn = numberOr(0, spawn && spawn.child_turn_id, spawn && spawn.child_session_id);
      var parentSpawn = numberOr(0, spawn && spawn.parent_spawn_id, spawn && spawn.parent_id);
      var id = Number(spawn && spawn.id) || 0;

      return {
        id: id,
        parent_turn_id: parentTurn,
        parent_spawn_id: parentSpawn,
        child_turn_id: childTurn,
        profile: spawn && (spawn.profile || spawn.child_profile) ? String(spawn.profile || spawn.child_profile) : '',
        role: spawn && (spawn.role || spawn.child_role) ? String(spawn.role || spawn.child_role) : '',
        parent_profile: spawn && spawn.parent_profile ? String(spawn.parent_profile) : '',
        status: spawn && spawn.status ? String(spawn.status) : 'unknown',
        question: spawn && spawn.question ? String(spawn.question) : '',
        task: spawn && (spawn.task || spawn.objective || spawn.description) ? String(spawn.task || spawn.objective || spawn.description) : '',
        branch: spawn && spawn.branch ? String(spawn.branch) : '',
        started_at: spawn && (spawn.started_at || spawn.created_at) ? (spawn.started_at || spawn.created_at) : '',
        completed_at: spawn && spawn.completed_at ? spawn.completed_at : '',
        summary: spawn && spawn.summary ? String(spawn.summary) : ''
      };
    }).filter(function (spawn) {
      return spawn.id > 0;
    }).sort(sortByStartTimeDesc);
  }

  function normalizeLoopRun(run) {
    return {
      id: Number(run && run.id) || 0,
      hex_id: run && run.hex_id ? String(run.hex_id) : '',
      loop_name: run && run.loop_name ? String(run.loop_name) : 'loop',
      status: run && run.status ? String(run.status) : 'unknown',
      cycle: Number(run && run.cycle) || 0,
      step_index: Number(run && run.step_index) || 0,
      started_at: run && run.started_at ? run.started_at : '',
      steps: arrayOrEmpty(run && run.steps).map(function (step) {
        return {
          profile: step && step.profile ? String(step.profile) : '',
          role: step && step.role ? String(step.role) : ''
        };
      })
    };
  }

  function normalizeIssues(rawIssues) {
    return arrayOrEmpty(rawIssues).map(function (issue) {
      return {
        id: Number(issue && issue.id) || 0,
        title: issue && issue.title ? String(issue.title) : '',
        priority: issue && issue.priority ? String(issue.priority) : 'medium',
        status: issue && issue.status ? String(issue.status) : 'open',
        labels: arrayOrEmpty(issue && issue.labels),
        description: issue && issue.description ? String(issue.description) : ''
      };
    }).filter(function (issue) {
      return issue.id > 0;
    }).sort(function (a, b) {
      return b.id - a.id;
    });
  }

  function normalizeDocs(rawDocs) {
    return arrayOrEmpty(rawDocs).map(function (doc) {
      return {
        id: doc && doc.id ? String(doc.id) : '',
        title: doc && doc.title ? String(doc.title) : '',
        content: doc && doc.content ? String(doc.content) : '',
        plan_id: doc && doc.plan_id ? String(doc.plan_id) : ''
      };
    }).filter(function (doc) {
      return !!doc.id;
    });
  }

  function normalizePlans(rawPlans) {
    return arrayOrEmpty(rawPlans).map(normalizePlan).filter(function (plan) {
      return !!plan.id;
    });
  }

  function normalizePlan(plan) {
    if (!plan || typeof plan !== 'object') {
      return {
        id: '',
        title: '',
        status: 'active',
        description: '',
        phases: []
      };
    }

    return {
      id: plan.id ? String(plan.id) : '',
      title: plan.title ? String(plan.title) : (plan.id ? String(plan.id) : ''),
      status: plan.status ? String(plan.status) : 'active',
      description: plan.description ? String(plan.description) : '',
      phases: arrayOrEmpty(plan.phases).map(function (phase) {
        return {
          id: phase && phase.id ? String(phase.id) : '',
          title: phase && phase.title ? String(phase.title) : (phase && phase.id ? String(phase.id) : ''),
          status: phase && phase.status ? String(phase.status) : 'not_started',
          description: phase && phase.description ? String(phase.description) : '',
          depends_on: arrayOrEmpty(phase && phase.depends_on)
        };
      })
    };
  }

  function normalizeTurns(rawTurns) {
    return arrayOrEmpty(rawTurns).map(function (turn) {
      return {
        id: Number(turn && turn.id) || 0,
        hex_id: turn && turn.hex_id ? String(turn.hex_id) : '',
        profile_name: turn && turn.profile_name ? String(turn.profile_name) : '',
        agent: turn && turn.agent ? String(turn.agent) : '',
        build_state: turn && turn.build_state ? String(turn.build_state) : 'unknown',
        objective: turn && turn.objective ? String(turn.objective) : '',
        what_was_built: turn && turn.what_was_built ? String(turn.what_was_built) : '',
        agent_model: turn && turn.agent_model ? String(turn.agent_model) : '',
        duration_secs: Number(turn && turn.duration_secs) || 0
      };
    }).filter(function (turn) {
      return turn.id > 0;
    }).sort(function (a, b) {
      return b.id - a.id;
    });
  }

  function normalizeLoopMessages(rawMessages) {
    return arrayOrEmpty(rawMessages).map(function (msg) {
      return {
        id: Number(msg && msg.id) || 0,
        spawn_id: Number(msg && msg.spawn_id) || 0,
        type: msg && msg.type ? String(msg.type) : 'message',
        direction: msg && msg.direction ? String(msg.direction) : 'child_to_parent',
        content: msg && msg.content ? String(msg.content) : '',
        created_at: msg && msg.created_at ? msg.created_at : '',
        step_index: msg && Number.isFinite(Number(msg.step_index)) ? Number(msg.step_index) : null
      };
    }).filter(function (msg) {
      return msg.id > 0 || !!msg.content;
    });
  }

  function aggregateUsageFromProfileStats(stats) {
    var list = arrayOrEmpty(stats);
    if (!list.length) return null;

    var usage = {
      input_tokens: 0,
      output_tokens: 0,
      cost_usd: 0,
      num_turns: 0
    };

    list.forEach(function (item) {
      usage.input_tokens += Number(item && (item.total_input_tokens != null ? item.total_input_tokens : item.total_input_tok)) || 0;
      usage.output_tokens += Number(item && (item.total_output_tokens != null ? item.total_output_tokens : item.total_output_tok)) || 0;
      usage.cost_usd += Number(item && item.total_cost_usd) || 0;
      usage.num_turns += Number(item && item.total_turns) || 0;
    });

    return usage;
  }

  function updateUsageTurnCountFromTurns() {
    if (!state.usage) {
      state.usage = {
        input_tokens: 0,
        output_tokens: 0,
        cost_usd: 0,
        num_turns: state.turns.length
      };
      return;
    }

    if (!state.usage.num_turns || state.usage.num_turns < state.turns.length) {
      state.usage.num_turns = state.turns.length;
    }
  }

  function sortByStartTimeDesc(a, b) {
    return parseTimestamp(b && b.started_at) - parseTimestamp(a && a.started_at);
  }

  function connectTerminalSocket() {
    disconnectTerminal();

    try {
      state.termWS = new WebSocket(buildWSURL('/ws/terminal'));
    } catch (_) {
      state.termWS = null;
      state.termWSConnected = false;
      return;
    }

    state.termWS.addEventListener('open', function () {
      state.termWSConnected = true;
      renderApp();
    });

    state.termWS.addEventListener('close', function () {
      state.termWSConnected = false;
      state.termWS = null;
      renderApp();
    });

    state.termWS.addEventListener('error', function () {
      state.termWSConnected = false;
      renderApp();
    });
  }

  function disconnectTerminal() {
    if (state.termWS) {
      try {
        state.termWS.close();
      } catch (_) {}
      state.termWS = null;
    }
    state.termWSConnected = false;
  }

  function loadAuthToken() {
    var hash = window.location.hash || '';
    if (hash.indexOf('#token=') === 0) {
      saveAuthToken(hash.slice(7));
      window.location.hash = '';
      return;
    }

    try {
      state.authToken = localStorage.getItem('adaf_token') || '';
    } catch (_) {
      state.authToken = '';
    }
  }

  function saveAuthToken(token) {
    state.authToken = String(token || '').trim();
    try {
      localStorage.setItem('adaf_token', state.authToken);
    } catch (_) {}
  }

  function clearAuthToken() {
    state.authToken = '';
    try {
      localStorage.removeItem('adaf_token');
    } catch (_) {}

    disconnectSessionSocket();
    disconnectTerminal();
    stopPolling();
  }

  function showAuthPrompt() {
    openModal('Authentication Required', '' +
      '<form data-modal-submit="auth">' +
        '<div class="form-grid">' +
          '<div class="form-field form-span-2"><label>Bearer Token</label><input type="text" name="auth_token" placeholder="Paste your token" autocomplete="off" required></div>' +
        '</div>' +
        '<div class="form-actions">' +
          '<button type="button" data-action="disconnect-auth" class="danger">Clear Stored Token</button>' +
          '<button type="button" data-action="close-modal">Cancel</button>' +
          '<button type="submit" class="primary">Connect</button>' +
        '</div>' +
      '</form>');
  }

  function openModal(title, bodyHTML) {
    state.modal = {
      title: title || 'Dialog',
      bodyHTML: bodyHTML || ''
    };
    renderModal();
  }

  function closeModal() {
    state.modal = null;
    renderModal();
  }

  function renderModal() {
    if (!modalRoot) return;

    if (!state.modal) {
      modalRoot.innerHTML = '';
      return;
    }

    modalRoot.innerHTML = '' +
      '<div class="modal-backdrop" data-action="close-modal"></div>' +
      '<section class="modal" role="dialog" aria-modal="true" aria-label="' + escapeHTML(state.modal.title) + '">' +
        '<div class="modal-header">' +
          '<h3>' + escapeHTML(state.modal.title) + '</h3>' +
          '<button class="modal-close" data-action="close-modal" aria-label="Close">×</button>' +
        '</div>' +
        '<div class="modal-body">' + state.modal.bodyHTML + '</div>' +
      '</section>';
  }

  function showToast(message, type) {
    if (!toastRoot) return;

    var node = document.createElement('div');
    node.className = 'toast ' + (type === 'error' ? 'error' : 'success');
    node.textContent = String(message || '');

    toastRoot.appendChild(node);

    setTimeout(function () {
      if (node.parentNode) node.parentNode.removeChild(node);
    }, 4200);
  }

  function setLoading(active) {
    if (active) {
      state.loadingCount += 1;
    } else {
      state.loadingCount = Math.max(0, state.loadingCount - 1);
    }

    if (!loadingNode) return;

    var show = state.loadingCount > 0;
    loadingNode.classList.toggle('hidden', !show);
    loadingNode.setAttribute('aria-hidden', show ? 'false' : 'true');
  }

  async function apiCall(path, method, body, options) {
    setLoading(true);

    try {
      var headers = { Accept: 'application/json' };
      if (state.authToken) headers.Authorization = 'Bearer ' + state.authToken;

      var request = {
        method: method || 'GET',
        headers: headers
      };

      if (body != null) {
        headers['Content-Type'] = 'application/json';
        request.body = JSON.stringify(body);
      }

      var response = await fetch(path, request);

      if (response.ok) {
        if (response.status === 204) return null;
        var text = await response.text();
        if (!text) return null;

        try {
          return JSON.parse(text);
        } catch (_) {
          return text;
        }
      }

      if (response.status === 401) {
        showAuthPrompt();
        var authErr = new Error('Auth required');
        authErr.authRequired = true;
        throw authErr;
      }

      if (response.status === 404 && options && options.allow404) {
        return null;
      }

      if (response.status === 204) return null;

      var message = response.status + ' ' + response.statusText;
      try {
        var payload = await response.json();
        if (payload && payload.error) message = payload.error;
      } catch (_) {
        // Ignore JSON parse errors.
      }

      throw new Error(message);
    } finally {
      setLoading(false);
    }
  }

  function buildWSURL(path) {
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var url = proto + '//' + window.location.host + path;

    if (state.authToken) {
      url += (path.indexOf('?') >= 0 ? '&' : '?') + 'token=' + encodeURIComponent(state.authToken);
    }

    return url;
  }

  function stringifyToolPayload(value) {
    if (value == null) return '';
    if (typeof value === 'string') return value;
    try {
      return JSON.stringify(value);
    } catch (_) {
      return String(value);
    }
  }

  function rawPayloadText(data) {
    if (typeof data === 'string') return data;
    if (data && typeof data.data === 'string') return data.data;
    return safeJSONString(data);
  }

  function extractContentBlocks(event) {
    if (!event || typeof event !== 'object') return [];

    if (event.message && Array.isArray(event.message.content)) {
      return event.message.content.slice();
    }

    if (event.content_block && typeof event.content_block === 'object') {
      return [event.content_block];
    }

    return [];
  }

  function asObject(value) {
    if (value == null) return null;
    if (typeof value === 'object') return value;
    if (typeof value === 'string') {
      try {
        return JSON.parse(value);
      } catch (_) {
        return { text: value };
      }
    }
    return null;
  }

  function formatElapsed(start, end) {
    var startMS = parseTimestamp(start);
    if (!startMS) return '--';

    var endMS = end ? parseTimestamp(end) : Date.now();
    if (!endMS || endMS < startMS) endMS = Date.now();

    var sec = Math.floor((endMS - startMS) / 1000);
    if (sec < 60) return sec + 's';

    var min = Math.floor(sec / 60);
    if (min < 60) return min + 'm ' + (sec % 60) + 's';

    var hour = Math.floor(min / 60);
    return hour + 'h ' + (min % 60) + 'm';
  }

  function formatRelativeTime(value) {
    var ts = parseTimestamp(value);
    if (!ts) return 'unknown';

    var diff = Math.max(0, Date.now() - ts);
    var sec = Math.floor(diff / 1000);
    if (sec < 60) return sec + 's ago';

    var min = Math.floor(sec / 60);
    if (min < 60) return min + 'm ago';

    var hour = Math.floor(min / 60);
    if (hour < 24) return hour + 'h ago';

    var day = Math.floor(hour / 24);
    return day + 'd ago';
  }

  function parseTimestamp(value) {
    if (value == null || value === '') return 0;
    if (typeof value === 'number' && Number.isFinite(value)) return value;

    var ts = new Date(value).getTime();
    if (!Number.isFinite(ts)) return 0;
    return ts;
  }

  function formatNumber(value) {
    var num = Number(value || 0);
    if (!Number.isFinite(num)) return '0';
    return num.toLocaleString();
  }

  function cropText(input, limit) {
    var max = Number(limit || 120000);
    var text = String(input || '');
    if (text.length <= max) return text;
    return text.slice(0, max - 1) + '…';
  }

  function withAlpha(color, alpha) {
    var value = String(color || '').trim();
    var a = Number(alpha);
    if (!Number.isFinite(a)) a = 1;
    if (a < 0) a = 0;
    if (a > 1) a = 1;

    if (/^#([a-fA-F0-9]{6})$/.test(value)) {
      var hex = value.slice(1);
      var r = parseInt(hex.slice(0, 2), 16);
      var g = parseInt(hex.slice(2, 4), 16);
      var b = parseInt(hex.slice(4, 6), 16);
      return 'rgba(' + r + ', ' + g + ', ' + b + ', ' + a + ')';
    }

    return value;
  }

  function isTypingTarget(target) {
    if (!target || !target.tagName) return false;
    var tag = String(target.tagName).toLowerCase();
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
    if (target.isContentEditable) return true;
    return false;
  }

  function normalizeStatus(value) {
    return String(value || 'unknown').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_');
  }

  function safeJSONString(value) {
    if (value == null) return '';
    if (typeof value === 'string') return value;
    try {
      return JSON.stringify(value);
    } catch (_) {
      return String(value);
    }
  }

  function escapeHTML(value) {
    return String(value == null ? '' : value)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function errorMessage(err) {
    if (!err) return 'unknown error';
    return err.message || String(err);
  }

  function arrayOrEmpty(value) {
    return Array.isArray(value) ? value : [];
  }

  function readString(scope, name) {
    if (!scope || !name) return '';

    var node = null;
    if (typeof scope.querySelector === 'function') {
      node = scope.querySelector('[name="' + name + '"]');
    }

    if (!node || typeof node.value !== 'string') return '';
    return node.value.trim();
  }

  function numberOr(current) {
    for (var i = 1; i < arguments.length; i += 1) {
      var next = Number(arguments[i]);
      if (Number.isFinite(next)) return next;
    }
    return Number(current) || 0;
  }

  init();
})();
