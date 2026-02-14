(function () {
  'use strict';

  var REFRESH_MS = 10000;
  var MAX_EVENT_ENTRIES = 300;
  var MAX_OUTPUT_CHARS = 120000;

  var SESSION_ACTIVE = {
    starting: true,
    running: true
  };

  var ISSUE_STATUSES = ['open', 'in_progress', 'resolved', 'wontfix'];
  var ISSUE_PRIORITIES = ['critical', 'high', 'medium', 'low'];
  var PLAN_PHASE_STATUSES = ['not_started', 'in_progress', 'complete', 'blocked'];

  function createEmptyCache() {
    return {
      plans: [],
      planDetail: null,
      issues: [],
      docs: [],
      profiles: [],
      loops: [],
      roles: [],
      rules: [],
      pushover: {}
    };
  }

  var state = {
    tab: '',
    authToken: '',
    projects: [],
    currentProjectID: '',
    multiProject: false,
    dashboardTimer: null,
    selectedSessionID: null,
    selectedPlanID: '',
    selectedIssueID: null,
    selectedDocID: '',
    issueStatusFilter: 'all',
    issuePriorityFilter: 'all',
    docsPlanFilter: 'all',
    configSection: 'profiles',
    ws: null,
    wsConnected: false,
    wsSessionID: null,
    term: null,
    termFit: null,
    termWS: null,
    termWSConnected: false,
    loadingCount: 0,
    modal: null,
    sessionStreams: {},
    cache: createEmptyCache()
  };

  var nav = document.getElementById('nav');
  var content = document.getElementById('content');
  var modalRoot = document.getElementById('modal-root');
  var toastRoot = document.getElementById('toast-root');
  var loadingNode = document.getElementById('global-loading');
  var connDot = document.getElementById('conn-dot');
  var connLabel = document.getElementById('conn-label');
  var projectSelect = document.getElementById('project-select');

  function init() {
    if (!nav || !content || !modalRoot) return;

    loadAuthToken();
    updateConnectionStatus();

    nav.addEventListener('click', function (event) {
      var link = event.target.closest('a[data-tab]');
      if (!link) return;
      event.preventDefault();
      switchTab(link.getAttribute('data-tab'));
    });

    if (projectSelect) {
      projectSelect.addEventListener('change', function () {
        switchProject(projectSelect.value || '', true);
      });
    }

    content.addEventListener('click', onContentClick);
    content.addEventListener('submit', onContentSubmit);
    content.addEventListener('change', onContentChange);

    modalRoot.addEventListener('click', onModalClick);
    modalRoot.addEventListener('submit', onModalSubmit);
    modalRoot.addEventListener('change', onModalChange);

    document.addEventListener('keydown', function (event) {
      if (event.key === 'Escape' && state.modal) {
        closeModal();
      }
    });

    initializeProjects().finally(function () {
      switchTab('dashboard');
    });
  }

  async function initializeProjects() {
    try {
      var projects = arrayOrEmpty(await apiCall('/api/projects', 'GET', null, { allow404: true }));
      state.projects = projects;
      state.multiProject = projects.length > 1;

      if (state.multiProject) {
        var defaultProject = projects.find(function (project) {
          return !!(project && project.is_default);
        }) || projects[0] || null;
        state.currentProjectID = defaultProject && defaultProject.id ? String(defaultProject.id) : '';
      } else {
        state.currentProjectID = '';
      }
    } catch (err) {
      if (!(err && err.authRequired)) {
        state.projects = [];
        state.multiProject = false;
        state.currentProjectID = '';
      }
    }

    updateProjectSelect();
    updateDocumentTitle();
  }

  function updateProjectSelect() {
    if (!projectSelect) return;

    if (!state.multiProject) {
      projectSelect.innerHTML = '';
      projectSelect.style.display = 'none';
      return;
    }

    projectSelect.innerHTML = state.projects.map(function (project) {
      var id = project && project.id ? String(project.id) : '';
      var name = project && project.name ? project.name : id || 'Unnamed Project';
      var label = project && project.is_default ? (name + ' (default)') : name;
      return '<option value="' + escapeHTML(id) + '">' + escapeHTML(label) + '</option>';
    }).join('');

    projectSelect.value = state.currentProjectID;
    projectSelect.style.display = '';
  }

  function findProjectByID(projectID) {
    var id = String(projectID || '');
    if (!id) return null;
    return state.projects.find(function (project) {
      return project && String(project.id || '') === id;
    }) || null;
  }

  function currentProject() {
    if (state.multiProject && state.currentProjectID) {
      return findProjectByID(state.currentProjectID);
    }
    return state.projects.find(function (project) {
      return !!(project && project.is_default);
    }) || state.projects[0] || null;
  }

  function currentProjectName() {
    var project = currentProject();
    if (!project) return '';
    return project.name || project.id || '';
  }

  function updateDocumentTitle(preferredName) {
    var name = preferredName || currentProjectName();
    document.title = name ? ('ADAF - ' + name) : 'ADAF';
  }

  function resetProjectScopedState() {
    if (state.dashboardTimer) {
      clearInterval(state.dashboardTimer);
      state.dashboardTimer = null;
    }

    disconnectSessionSocket();
    state.selectedSessionID = null;
    state.selectedPlanID = '';
    state.selectedIssueID = null;
    state.selectedDocID = '';
    state.docsPlanFilter = 'all';
    state.issueStatusFilter = 'all';
    state.issuePriorityFilter = 'all';
    state.sessionStreams = {};
    state.cache = createEmptyCache();
  }

  function switchProject(projectID, refreshTab) {
    if (!state.multiProject) return;

    var nextID = String(projectID || '');
    if (!findProjectByID(nextID)) return;

    if (nextID === state.currentProjectID && !refreshTab) {
      updateProjectSelect();
      return;
    }

    closeModal();
    resetProjectScopedState();
    state.currentProjectID = nextID;
    updateProjectSelect();
    updateDocumentTitle();

    if (refreshTab && state.tab) {
      renderCurrentTab();
    }
  }

  function apiBase() {
    if (state.multiProject && state.currentProjectID) {
      return '/api/projects/' + encodeURIComponent(state.currentProjectID);
    }
    return '/api';
  }

  function loadAuthToken() {
    var hash = window.location.hash || '';
    if (hash.indexOf('#token=') === 0) {
      state.authToken = hash.slice(7);
      window.location.hash = '';
      try {
        localStorage.setItem('adaf_token', state.authToken);
      } catch (_) {
        // Best effort.
      }
      return;
    }

    try {
      state.authToken = localStorage.getItem('adaf_token') || '';
    } catch (_) {
      state.authToken = '';
    }
  }

  function updateConnectionStatus() {
    if (!connDot || !connLabel) return;
    var online = !!(state.wsConnected || state.termWSConnected);
    connDot.classList.toggle('online', online);
    connDot.classList.toggle('offline', !online);
    connLabel.textContent = online ? 'Online' : 'Offline';
  }

  function switchTab(tab) {
    if (!tab || tab === state.tab) {
      renderCurrentTab();
      return;
    }

    cleanupTabResources(tab);
    state.tab = tab;
    updateNav();
    renderCurrentTab();
  }

  function cleanupTabResources(nextTab) {
    if (state.dashboardTimer) {
      clearInterval(state.dashboardTimer);
      state.dashboardTimer = null;
    }

    if (nextTab !== 'sessions') {
      disconnectSessionSocket();
      state.selectedSessionID = null;
    }

    if (nextTab !== 'terminal') {
      disconnectTerminal();
    }

    if (nextTab !== 'plans') {
      state.selectedPlanID = '';
      state.cache.planDetail = null;
    }

    if (nextTab !== 'issues') {
      state.selectedIssueID = null;
    }

    if (nextTab !== 'docs') {
      state.selectedDocID = '';
    }
  }

  function updateNav() {
    nav.querySelectorAll('a[data-tab]').forEach(function (link) {
      var active = link.getAttribute('data-tab') === state.tab;
      link.classList.toggle('active', active);
      if (active) {
        link.setAttribute('aria-current', 'page');
      } else {
        link.removeAttribute('aria-current');
      }
    });
  }

  function onContentClick(event) {
    var actionNode = event.target.closest('[data-action]');
    if (!actionNode) return;

    var action = actionNode.getAttribute('data-action');
    if (!action) return;

    event.preventDefault();

    if (action === 'go-tab') {
      var targetTab = actionNode.getAttribute('data-tab') || 'dashboard';
      switchTab(targetTab);
      return;
    }

    if (action === 'switch-project') {
      var targetProjectID = actionNode.getAttribute('data-project-id') || '';
      switchProject(targetProjectID, true);
      return;
    }

    if (action === 'open-session-tab') {
      var jumpSession = parseInt(actionNode.getAttribute('data-session-id') || '', 10);
      if (!Number.isNaN(jumpSession)) state.selectedSessionID = jumpSession;
      switchTab('sessions');
      return;
    }

    if (action === 'open-plan-tab') {
      state.selectedPlanID = actionNode.getAttribute('data-plan-id') || '';
      switchTab('plans');
      return;
    }

    if (action === 'open-issue-tab') {
      var jumpIssue = parseInt(actionNode.getAttribute('data-issue-id') || '', 10);
      if (!Number.isNaN(jumpIssue)) state.selectedIssueID = jumpIssue;
      switchTab('issues');
      return;
    }

    if (action === 'open-doc-tab') {
      state.selectedDocID = actionNode.getAttribute('data-doc-id') || '';
      switchTab('docs');
      return;
    }

    if (action === 'select-session') {
      var sessionID = parseInt(actionNode.getAttribute('data-session-id') || '', 10);
      if (!Number.isNaN(sessionID)) {
        state.selectedSessionID = sessionID;
        renderSessions();
      }
      return;
    }

    if (action === 'reconnect-session') {
      if (state.selectedSessionID) connectSessionSocket(state.selectedSessionID, true);
      return;
    }

    if (action === 'open-session-start') {
      openStartSessionModal();
      return;
    }

    if (action === 'stop-session') {
      var stopID = parseInt(actionNode.getAttribute('data-session-id') || '', 10);
      if (!Number.isNaN(stopID)) stopSession(stopID);
      return;
    }

    if (action === 'select-plan') {
      state.selectedPlanID = actionNode.getAttribute('data-plan-id') || '';
      renderPlans();
      return;
    }

    if (action === 'open-create-plan') {
      openPlanModal('create', null);
      return;
    }

    if (action === 'open-edit-plan') {
      openPlanModal('edit', state.cache.planDetail);
      return;
    }

    if (action === 'activate-plan') {
      if (state.selectedPlanID) activatePlan(state.selectedPlanID);
      return;
    }

    if (action === 'delete-plan') {
      if (state.selectedPlanID) deletePlan(state.selectedPlanID);
      return;
    }

    if (action === 'open-create-phase') {
      openPhaseModal('create', state.cache.planDetail, null);
      return;
    }

    if (action === 'open-edit-phase') {
      var phaseID = actionNode.getAttribute('data-phase-id') || '';
      if (!state.cache.planDetail || !phaseID) return;
      var phase = findPhase(state.cache.planDetail, phaseID);
      if (!phase) return;
      openPhaseModal('edit', state.cache.planDetail, phase);
      return;
    }

    if (action === 'select-issue') {
      var issueID = parseInt(actionNode.getAttribute('data-issue-id') || '', 10);
      if (!Number.isNaN(issueID)) {
        state.selectedIssueID = issueID;
        renderIssues();
      }
      return;
    }

    if (action === 'open-create-issue') {
      openIssueModal('create', null);
      return;
    }

    if (action === 'open-edit-issue') {
      var editIssueID = parseInt(actionNode.getAttribute('data-issue-id') || '', 10);
      if (!Number.isNaN(editIssueID)) {
        var editIssue = state.cache.issues.find(function (it) { return it.id === editIssueID; }) || null;
        openIssueModal('edit', editIssue);
      }
      return;
    }

    if (action === 'delete-issue') {
      var deleteIssueID = parseInt(actionNode.getAttribute('data-issue-id') || '', 10);
      if (!Number.isNaN(deleteIssueID)) {
        deleteIssue(deleteIssueID);
      }
      return;
    }

    if (action === 'select-doc') {
      state.selectedDocID = actionNode.getAttribute('data-doc-id') || '';
      renderDocs();
      return;
    }

    if (action === 'open-create-doc') {
      openDocModal('create', null);
      return;
    }

    if (action === 'open-edit-doc') {
      var editDocID = actionNode.getAttribute('data-doc-id') || '';
      if (!editDocID) return;
      var editDoc = state.cache.docs.find(function (doc) { return doc.id === editDocID; }) || null;
      openDocModal('edit', editDoc);
      return;
    }

    if (action === 'delete-doc') {
      var deleteDocID = actionNode.getAttribute('data-doc-id') || '';
      if (deleteDocID) deleteDoc(deleteDocID);
      return;
    }

    if (action === 'set-config-section') {
      state.configSection = actionNode.getAttribute('data-section') || 'profiles';
      renderConfig();
      return;
    }

    if (action === 'open-create-profile') {
      openProfileModal('create', null);
      return;
    }

    if (action === 'open-edit-profile') {
      var profName = actionNode.getAttribute('data-profile-name') || '';
      var profile = state.cache.profiles.find(function (it) { return it.name === profName; }) || null;
      openProfileModal('edit', profile);
      return;
    }

    if (action === 'delete-profile') {
      deleteProfile(actionNode.getAttribute('data-profile-name') || '');
      return;
    }

    if (action === 'open-create-loop') {
      openLoopModal('create', null);
      return;
    }

    if (action === 'open-edit-loop') {
      var loopName = actionNode.getAttribute('data-loop-name') || '';
      var loop = state.cache.loops.find(function (it) { return it.name === loopName; }) || null;
      openLoopModal('edit', loop);
      return;
    }

    if (action === 'delete-loop') {
      deleteLoop(actionNode.getAttribute('data-loop-name') || '');
      return;
    }

    if (action === 'open-create-role') {
      openRoleModal();
      return;
    }

    if (action === 'delete-role') {
      deleteRole(actionNode.getAttribute('data-role-name') || '');
      return;
    }

    if (action === 'open-create-rule') {
      openRuleModal();
      return;
    }

    if (action === 'delete-rule') {
      deleteRule(actionNode.getAttribute('data-rule-id') || '');
      return;
    }
  }

  function onContentSubmit(event) {
    var form = event.target.closest('form[data-form]');
    if (!form) return;

    event.preventDefault();

    var name = form.getAttribute('data-form');
    if (!name) return;

    if (name === 'auth') {
      var token = readString(form, 'auth_token');
      state.authToken = token;
      try {
        localStorage.setItem('adaf_token', token);
      } catch (_) {
        // Best effort.
      }
      renderCurrentTab();
      return;
    }

    if (name === 'send-session-message') {
      submitSessionMessage(form);
      return;
    }

    if (name === 'save-pushover') {
      savePushover(form);
      return;
    }
  }

  function onContentChange(event) {
    var node = event.target;
    if (!node) return;

    var change = node.getAttribute('data-change');
    if (!change) return;

    if (change === 'issue-status-filter') {
      state.issueStatusFilter = node.value || 'all';
      renderIssues();
      return;
    }

    if (change === 'issue-priority-filter') {
      state.issuePriorityFilter = node.value || 'all';
      renderIssues();
      return;
    }

    if (change === 'docs-plan-filter') {
      state.docsPlanFilter = node.value || 'all';
      renderDocs();
      return;
    }
  }

  function onModalClick(event) {
    var actionNode = event.target.closest('[data-action]');
    if (!actionNode) return;
    var action = actionNode.getAttribute('data-action');

    if (action === 'close-modal') {
      event.preventDefault();
      closeModal();
      return;
    }

    if (action === 'add-loop-step') {
      event.preventDefault();
      addLoopStepCard(null);
      return;
    }

    if (action === 'remove-loop-step') {
      event.preventDefault();
      removeLoopStepCard(actionNode);
      return;
    }
  }

  function onModalSubmit(event) {
    var form = event.target.closest('form[data-modal-submit]');
    if (!form) return;

    event.preventDefault();

    var submit = form.getAttribute('data-modal-submit');
    if (!submit) return;

    if (submit === 'start-session') return submitStartSession(form);
    if (submit === 'issue-create') return submitIssueCreate(form);
    if (submit === 'issue-edit') return submitIssueEdit(form);
    if (submit === 'plan-create') return submitPlanCreate(form);
    if (submit === 'plan-edit') return submitPlanEdit(form);
    if (submit === 'phase-create') return submitPhaseCreate(form);
    if (submit === 'phase-edit') return submitPhaseEdit(form);
    if (submit === 'doc-create') return submitDocCreate(form);
    if (submit === 'doc-edit') return submitDocEdit(form);
    if (submit === 'profile-create') return submitProfileCreate(form);
    if (submit === 'profile-edit') return submitProfileEdit(form);
    if (submit === 'loop-create') return submitLoopCreate(form);
    if (submit === 'loop-edit') return submitLoopEdit(form);
    if (submit === 'role-create') return submitRoleCreate(form);
    if (submit === 'rule-create') return submitRuleCreate(form);
  }

  function onModalChange(event) {
    var node = event.target;
    if (!node) return;

    if (node.name === 'session_type') {
      var sessionForm = node.closest('form[data-modal-submit="start-session"]');
      if (sessionForm) updateStartSessionFormVisibility(sessionForm);
    }
  }

  function renderCurrentTab() {
    if (!state.tab) return;
    if (state.tab === 'dashboard') return renderDashboard();
    if (state.tab === 'sessions') return renderSessions();
    if (state.tab === 'plans') return renderPlans();
    if (state.tab === 'issues') return renderIssues();
    if (state.tab === 'docs') return renderDocs();
    if (state.tab === 'config') return renderConfig();
    if (state.tab === 'terminal') return renderTerminal();
  }

  function renderLoading(label) {
    content.innerHTML = '<section class="card"><p class="meta"><span class="spinner"></span> Loading ' + escapeHTML(label) + '...</p></section>';
  }

  function renderError(message) {
    content.innerHTML = '<section class="card error-card"><h2>Error</h2><p class="error-text">' + escapeHTML(message || 'Unknown error') + '</p></section>';
  }

  async function renderDashboard() {
    var tab = state.tab;
    renderLoading('dashboard');

    try {
      var results;
      var globalDashboard = null;

      if (state.multiProject) {
        var multiResults = await Promise.all([
          apiCall('/api/projects/dashboard', 'GET'),
          apiCall(apiBase() + '/project', 'GET', null, { allow404: true }),
          apiCall(apiBase() + '/plans', 'GET'),
          apiCall(apiBase() + '/issues?status=open', 'GET'),
          apiCall(apiBase() + '/sessions', 'GET'),
          apiCall(apiBase() + '/docs', 'GET')
        ]);
        globalDashboard = multiResults[0];
        results = multiResults.slice(1);
      } else {
        results = await Promise.all([
          apiCall(apiBase() + '/project', 'GET', null, { allow404: true }),
          apiCall(apiBase() + '/plans', 'GET'),
          apiCall(apiBase() + '/issues?status=open', 'GET'),
          apiCall(apiBase() + '/sessions', 'GET'),
          apiCall(apiBase() + '/docs', 'GET')
        ]);
      }

      if (state.tab !== tab) return;

      var project = results[0] || {};
      var plans = arrayOrEmpty(results[1]);
      var openIssues = arrayOrEmpty(results[2]);
      var sessions = arrayOrEmpty(results[3]);
      var docs = arrayOrEmpty(results[4]);

      state.cache.plans = plans;

      var titleName = currentProjectName() || project.name || '';
      updateDocumentTitle(titleName);

      if (state.multiProject) {
        var globalProjects = arrayOrEmpty(globalDashboard && globalDashboard.projects);
        content.innerHTML = '' +
          '<section class="grid">' +
            '<article class="card span-12">' +
              '<div class="card-title-row"><h2>Global Overview</h2><span class="meta">All registered projects</span></div>' +
              renderGlobalDashboardOverview(globalProjects) +
            '</article>' +
          '</section>' +
          '<section class="grid">' +
            '<article class="card span-12">' +
              '<h2>Current Project: ' + escapeHTML(titleName || 'Selected Project') + '</h2>' +
              '<p class="meta">Project-scoped details for the active selection.</p>' +
            '</article>' +
            renderProjectDashboardCards(project, plans, openIssues, sessions, docs) +
          '</section>';
      } else {
        content.innerHTML = '' +
          '<section class="grid">' +
            renderProjectDashboardCards(project, plans, openIssues, sessions, docs) +
          '</section>';
      }

      if (!state.dashboardTimer && state.tab === 'dashboard') {
        state.dashboardTimer = setInterval(function () {
          if (state.tab === 'dashboard') renderDashboard();
        }, REFRESH_MS);
      }
    } catch (err) {
      if (err && err.authRequired) return;
      renderError('Failed to load dashboard: ' + errorMessage(err));
    }
  }

  function renderProjectDashboardCards(project, plans, openIssues, sessions, docs) {
    var activePlan = findActivePlan(project, plans);
    var summary = summarizePlanPhases(activePlan);
    var name = project && project.name ? project.name : (currentProjectName() || 'Uninitialized');
    var repoPath = project && (project.repo_path || project.path) ? (project.repo_path || project.path) : 'N/A';

    return '' +
      '<article class="card span-6">' +
        '<h2>Project</h2>' +
        '<p><strong>' + escapeHTML(name) + '</strong></p>' +
        '<p class="meta">Repo: ' + escapeHTML(repoPath) + '</p>' +
        '<p class="meta">Active plan: ' + escapeHTML(activePlan ? activePlan.id : 'none') + '</p>' +
        '<div class="button-row"><button data-action="go-tab" data-tab="sessions">Open Sessions</button><button data-action="go-tab" data-tab="config">Open Config</button></div>' +
      '</article>' +
      '<article class="card span-6">' +
        '<h2>Plan Snapshot</h2>' +
        '<div class="filters">' +
          createPill('complete', summary.complete) +
          createPill('in_progress', summary.in_progress) +
          createPill('not_started', summary.not_started) +
          createPill('blocked', summary.blocked) +
        '</div>' +
        '<p class="meta">Total phases: ' + summary.total + '</p>' +
        '<div class="button-row"><button data-action="go-tab" data-tab="plans">Manage Plans</button></div>' +
      '</article>' +
      '<article class="card span-4">' +
        '<h2>Open Issues (' + openIssues.length + ')</h2>' +
        renderDashboardIssues(openIssues.slice(0, 6)) +
        '<div class="button-row"><button data-action="go-tab" data-tab="issues">View All Issues</button></div>' +
      '</article>' +
      '<article class="card span-4">' +
        '<h2>Recent Sessions</h2>' +
        renderDashboardSessions(sessions.slice(0, 6)) +
        '<div class="button-row"><button data-action="go-tab" data-tab="sessions">Control Sessions</button></div>' +
      '</article>' +
      '<article class="card span-4">' +
        '<h2>Docs (' + docs.length + ')</h2>' +
        renderDashboardDocs(docs.slice(0, 6)) +
        '<div class="button-row"><button data-action="go-tab" data-tab="docs">Manage Docs</button></div>' +
      '</article>';
  }

  function renderGlobalDashboardOverview(projects) {
    var list = arrayOrEmpty(projects);
    if (!list.length) return '<p class="empty">No project summary data available.</p>';

    var totals = summarizeGlobalProjects(list);

    return '' +
      '<div class="session-metrics">' +
        metricCard('Projects', formatNumber(totals.projects)) +
        metricCard('Open Issues', formatNumber(totals.openIssues)) +
        metricCard('Plans', formatNumber(totals.plans)) +
        metricCard('Turns', formatNumber(totals.turns)) +
      '</div>' +
      '<div class="project-cards">' +
        list.map(function (project) {
          var id = project && project.id ? String(project.id) : '';
          var name = project && project.name ? project.name : id || 'Unnamed Project';
          var active = id && id === state.currentProjectID;
          var badges = '';
          if (project && project.is_default) badges += createPill('active', 'default');
          if (active) badges += createPill('running', 'current');

          return '' +
            '<button type="button" class="project-card' + (active ? ' active' : '') + '" data-action="switch-project" data-project-id="' + escapeHTML(id) + '">' +
              '<div class="card-title-row">' +
                '<h3>' + escapeHTML(name) + '</h3>' +
                '<div class="filters">' + badges + '</div>' +
              '</div>' +
              '<p class="meta mono">' + escapeHTML((project && project.path) || 'unknown path') + '</p>' +
              '<div class="project-card-stats">' +
                '<span><strong>' + formatNumber(project && project.open_issue_count) + '</strong> open issues</span>' +
                '<span><strong>' + formatNumber(project && project.plan_count) + '</strong> plans</span>' +
                '<span><strong>' + formatNumber(project && project.turn_count) + '</strong> turns</span>' +
              '</div>' +
              '<p class="meta">Active plan: ' + escapeHTML((project && project.active_plan_id) || 'none') + '</p>' +
            '</button>';
        }).join('') +
      '</div>';
  }

  function summarizeGlobalProjects(projects) {
    var totals = {
      projects: 0,
      openIssues: 0,
      plans: 0,
      turns: 0
    };

    arrayOrEmpty(projects).forEach(function (project) {
      totals.projects += 1;
      totals.openIssues += Number(project && project.open_issue_count) || 0;
      totals.plans += Number(project && project.plan_count) || 0;
      totals.turns += Number(project && project.turn_count) || 0;
    });

    return totals;
  }

  function renderDashboardIssues(issues) {
    if (!issues.length) return '<p class="empty">No open issues.</p>';
    return '<ul class="list">' + issues.map(function (issue) {
      return '' +
        '<li>' +
          '<div class="list-item">' +
            '<div class="list-item-main"><strong>#' + issue.id + ' ' + escapeHTML(issue.title || 'Untitled') + '</strong><span class="meta">' + escapeHTML(issue.plan_id || 'no plan') + '</span></div>' +
            '<div class="list-item-actions"><button data-action="open-issue-tab" data-issue-id="' + issue.id + '">Open</button></div>' +
          '</div>' +
        '</li>';
    }).join('') + '</ul>';
  }

  function renderDashboardSessions(sessions) {
    if (!sessions.length) return '<p class="empty">No sessions found.</p>';
    return '<ul class="list">' + sessions.map(function (session) {
      var projectMeta = '';
      if (state.multiProject) {
        projectMeta = ' · ' + escapeHTML(session.project_name || 'Unknown project');
      }
      return '' +
        '<li>' +
          '<div class="list-item">' +
            '<div class="list-item-main"><strong>#' + session.id + ' · ' + escapeHTML(session.profile_name || session.agent_name || 'session') + '</strong><span class="meta">' + escapeHTML(formatRelativeTime(session.started_at)) + projectMeta + '</span></div>' +
            '<div class="list-item-actions">' + createPill(session.status || 'unknown', session.status || 'unknown') + '<button data-action="open-session-tab" data-session-id="' + session.id + '">Open</button></div>' +
          '</div>' +
        '</li>';
    }).join('') + '</ul>';
  }

  function renderDashboardDocs(docs) {
    if (!docs.length) return '<p class="empty">No docs created yet.</p>';
    return '<ul class="list">' + docs.map(function (doc) {
      return '' +
        '<li>' +
          '<div class="list-item">' +
            '<div class="list-item-main"><strong>' + escapeHTML(doc.title || doc.id) + '</strong><span class="meta">' + escapeHTML(doc.plan_id || 'shared') + '</span></div>' +
            '<div class="list-item-actions"><button data-action="open-doc-tab" data-doc-id="' + escapeHTML(doc.id) + '">Open</button></div>' +
          '</div>' +
        '</li>';
    }).join('') + '</ul>';
  }
  async function renderSessions() {
    var tab = state.tab;
    renderLoading('sessions');

    try {
      var results = await Promise.all([
        apiCall(apiBase() + '/sessions', 'GET'),
        apiCall('/api/config/profiles', 'GET'),
        apiCall('/api/config/loops', 'GET'),
        apiCall(apiBase() + '/plans', 'GET')
      ]);

      if (state.tab !== tab) return;

      var sessions = arrayOrEmpty(results[0]);
      var profiles = arrayOrEmpty(results[1]);
      var loops = arrayOrEmpty(results[2]);
      var plans = arrayOrEmpty(results[3]);

      state.cache.profiles = profiles;
      state.cache.loops = loops;
      state.cache.plans = plans;

      if (!state.selectedSessionID && sessions.length > 0) {
        state.selectedSessionID = sessions[0].id;
      }

      var selected = sessions.find(function (session) {
        return session.id === state.selectedSessionID;
      }) || null;

      content.innerHTML = '' +
        '<section class="grid">' +
          '<article class="card span-4">' +
            '<div class="card-title-row"><h2>Sessions</h2><button data-action="open-session-start">Start New Session</button></div>' +
            renderSessionList(sessions) +
          '</article>' +
          '<article class="card span-8">' +
            renderSessionDetail(selected) +
          '</article>' +
        '</section>';

      if (selected) {
        connectSessionSocket(selected.id, false);
        refreshSessionPanels(selected.id);
      } else {
        disconnectSessionSocket();
      }
    } catch (err) {
      if (err && err.authRequired) return;
      renderError('Failed to load sessions: ' + errorMessage(err));
    }
  }

  function renderSessionList(sessions) {
    if (!sessions.length) return '<p class="empty">No sessions found.</p>';

    return '<ul class="list">' + sessions.map(function (session) {
      var active = !!SESSION_ACTIVE[normalizeStatus(session.status)];
      var selected = session.id === state.selectedSessionID;
      var projectMeta = '';
      var messageForm = '';

      if (state.multiProject) {
        projectMeta = ' · ' + escapeHTML(session.project_name || 'Unknown project');
      }

      if (active) {
        messageForm = '' +
          '<form class="inline-form" data-form="send-session-message">' +
            '<input type="hidden" name="session_id" value="' + session.id + '">' +
            '<input type="text" name="content" placeholder="Send message" autocomplete="off" required>' +
            '<button type="submit">Send</button>' +
          '</form>';
      }

      return '' +
        '<li>' +
          '<div class="list-item">' +
            '<div class="list-item-main">' +
              '<strong>#' + session.id + ' · ' + escapeHTML(session.profile_name || session.agent_name || 'session') + '</strong>' +
              '<span class="meta">' + escapeHTML(formatRelativeTime(session.started_at)) + projectMeta + '</span>' +
              '<div class="filters">' + createPill(session.status || 'unknown', session.status || 'unknown') + '</div>' +
            '</div>' +
            '<div class="list-item-actions">' +
              '<button data-action="select-session" data-session-id="' + session.id + '" class="' + (selected ? 'active' : '') + '">View</button>' +
              (active ? '<button data-action="stop-session" data-session-id="' + session.id + '" class="danger">Stop</button>' : '') +
            '</div>' +
          '</div>' +
          messageForm +
        '</li>';
    }).join('') + '</ul>';
  }

  function renderSessionDetail(session) {
    if (!session) {
      return '<h2>Session Detail</h2><p class="empty">Select a session to inspect live events.</p>';
    }

    var active = !!SESSION_ACTIVE[normalizeStatus(session.status)];

    return '' +
      '<div class="card-title-row">' +
        '<h2>Session #' + session.id + '</h2>' +
        '<div class="button-row">' +
          '<button data-action="reconnect-session">Reconnect Stream</button>' +
          (active ? '<button data-action="stop-session" data-session-id="' + session.id + '" class="danger">Stop Session</button>' : '') +
        '</div>' +
      '</div>' +
      '<p class="meta">' + escapeHTML(session.project_name || 'Unknown project') + ' · ' + escapeHTML(session.agent_name || 'Unknown agent') + '</p>' +
      '<div class="filters">' + createPill(session.status || 'unknown', session.status || 'unknown') + '</div>' +
      '<div id="session-metrics" class="session-metrics"></div>' +
      '<div id="session-loop-progress"></div>' +
      '<div id="session-spawns"></div>' +
      '<section class="session-events">' +
        '<h3>Event Stream</h3>' +
        '<div id="session-events-feed" class="events-feed"></div>' +
      '</section>' +
      renderSessionMessageForm(session, active);
  }

  function renderSessionMessageForm(session, active) {
    if (!active) {
      return '<p class="meta">Session is not running. Messaging is disabled.</p>';
    }

    return '' +
      '<form class="inline-form" data-form="send-session-message">' +
        '<input type="hidden" name="session_id" value="' + session.id + '">' +
        '<input type="text" name="content" placeholder="Message running session" autocomplete="off" required>' +
        '<button type="submit" class="success">Send Message</button>' +
      '</form>';
  }

  async function submitSessionMessage(form) {
    var sessionID = parseInt(readString(form, 'session_id'), 10);
    var contentText = readString(form, 'content');

    if (Number.isNaN(sessionID) || !contentText) {
      showToast('Session message needs a target and content.', 'error');
      return;
    }

    try {
      await apiCall(apiBase() + '/sessions/' + encodeURIComponent(String(sessionID)) + '/message', 'POST', {
        content: contentText
      });
      form.reset();
      showToast('Message sent to session #' + sessionID + '.', 'success');
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'message',
        text: 'Message sent: ' + contentText
      });
      refreshSessionPanels(sessionID);
    } catch (err) {
      showToast('Failed to send message: ' + errorMessage(err), 'error');
    }
  }

  async function stopSession(sessionID) {
    if (!window.confirm('Stop session #' + sessionID + '?')) return;

    try {
      await apiCall(apiBase() + '/sessions/' + encodeURIComponent(String(sessionID)) + '/stop', 'POST');
      showToast('Stop signal sent to session #' + sessionID + '.', 'success');
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'control',
        text: 'Stop requested.'
      });
      renderSessions();
    } catch (err) {
      showToast('Failed to stop session: ' + errorMessage(err), 'error');
    }
  }

  function ensureSessionStream(sessionID) {
    if (!state.sessionStreams[sessionID]) {
      state.sessionStreams[sessionID] = {
        entries: [],
        metrics: {
          inputTokens: 0,
          outputTokens: 0,
          costUSD: 0,
          turns: 0,
          exitCode: null
        },
        spawns: [],
        loop: {
          cycle: 0,
          stepIndex: 0,
          totalSteps: 0,
          profile: ''
        },
        lastToolEntryIndex: -1
      };
    }
    return state.sessionStreams[sessionID];
  }

  function addSessionEntry(sessionID, entry) {
    var stream = ensureSessionStream(sessionID);
    var next = {
      kind: entry.kind || 'system',
      label: entry.label || entry.kind || 'event',
      text: entry.text || '',
      data: entry.data || null,
      name: entry.name || '',
      input: entry.input,
      output: entry.output,
      ts: Date.now(),
      isDelta: !!entry.isDelta
    };

    stream.entries.push(next);
    if (stream.entries.length > MAX_EVENT_ENTRIES) {
      stream.entries = stream.entries.slice(stream.entries.length - MAX_EVENT_ENTRIES);
      stream.lastToolEntryIndex = -1;
    }

    return next;
  }

  function connectSessionSocket(sessionID, forceReconnect) {
    if (!sessionID) return;

    if (!forceReconnect && state.ws && state.wsSessionID === sessionID && state.ws.readyState <= 1) {
      return;
    }

    disconnectSessionSocket();

    state.wsSessionID = sessionID;
    state.ws = new WebSocket(buildWSURL('/ws/sessions/' + encodeURIComponent(String(sessionID))));

    state.ws.addEventListener('open', function () {
      state.wsConnected = true;
      updateConnectionStatus();
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'stream',
        text: 'Connected to live stream.'
      });
      refreshSessionPanels(sessionID);
    });

    state.ws.addEventListener('message', function (event) {
      handleSessionMessage(sessionID, event.data);
    });

    state.ws.addEventListener('error', function () {
      state.wsConnected = false;
      updateConnectionStatus();
      addSessionEntry(sessionID, {
        kind: 'error',
        label: 'stream',
        text: 'Session stream error.'
      });
      refreshSessionPanels(sessionID);
    });

    state.ws.addEventListener('close', function () {
      state.wsConnected = false;
      updateConnectionStatus();
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'stream',
        text: 'Session stream disconnected.'
      });
      state.ws = null;
      state.wsSessionID = null;
      refreshSessionPanels(sessionID);
    });
  }

  function disconnectSessionSocket() {
    if (state.ws) {
      try {
        state.ws.close();
      } catch (_) {
        // Best effort.
      }
    }
    state.ws = null;
    state.wsSessionID = null;
    state.wsConnected = false;
    updateConnectionStatus();
  }

  function handleSessionMessage(sessionID, rawData) {
    var parsed;
    try {
      parsed = JSON.parse(rawData);
    } catch (_) {
      addSessionEntry(sessionID, {
        kind: 'raw',
        label: 'raw',
        text: String(rawData)
      });
      refreshSessionPanels(sessionID);
      return;
    }

    ingestSessionWireMessage(sessionID, parsed, false);
    refreshSessionPanels(sessionID);
  }

  function ingestSessionWireMessage(sessionID, message, replay) {
    if (!message || typeof message !== 'object') return;

    var type = message.type || 'event';
    var data = message.data;
    var stream = ensureSessionStream(sessionID);

    if (type === 'snapshot') {
      applySessionSnapshot(sessionID, data, replay);
      return;
    }

    if (type === 'event') {
      handleSessionStreamEvent(sessionID, data && data.event ? data.event : data);
      return;
    }

    if (type === 'raw') {
      var rawText = rawPayloadText(data);
      addSessionEntry(sessionID, {
        kind: 'raw',
        label: 'stdout',
        text: cropText(rawText)
      });
      return;
    }

    if (type === 'started') {
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'started',
        text: 'Agent turn started.'
      });
      return;
    }

    if (type === 'prompt') {
      addSessionEntry(sessionID, {
        kind: 'assistant',
        label: 'prompt',
        text: data && data.prompt ? String(data.prompt) : 'Prompt emitted.'
      });
      return;
    }

    if (type === 'finished') {
      if (data && typeof data.exit_code === 'number') {
        stream.metrics.exitCode = data.exit_code;
      }
      addSessionEntry(sessionID, {
        kind: data && data.error ? 'error' : 'result',
        label: 'finished',
        text: 'Exit code ' + String((data && typeof data.exit_code === 'number') ? data.exit_code : 0) + (data && data.error ? (' · ' + data.error) : '')
      });
      return;
    }

    if (type === 'done') {
      addSessionEntry(sessionID, {
        kind: 'result',
        label: 'done',
        text: 'Session complete.' + (data && data.error ? ' ' + data.error : '')
      });
      return;
    }

    if (type === 'spawn') {
      stream.spawns = arrayOrEmpty(data && data.spawns);
      addSessionEntry(sessionID, {
        kind: 'spawn',
        label: 'spawn',
        text: stream.spawns.length + ' spawn(s) tracked.'
      });
      return;
    }

    if (type === 'loop_step_start') {
      updateLoopProgress(stream, data);
      addSessionEntry(sessionID, {
        kind: 'loop',
        label: 'loop step start',
        text: loopProgressText(stream.loop, true)
      });
      return;
    }

    if (type === 'loop_step_end') {
      updateLoopProgress(stream, data);
      addSessionEntry(sessionID, {
        kind: 'loop',
        label: 'loop step end',
        text: loopProgressText(stream.loop, false)
      });
      return;
    }

    if (type === 'loop_done') {
      addSessionEntry(sessionID, {
        kind: data && data.error ? 'error' : 'loop',
        label: 'loop done',
        text: data && data.reason ? ('Reason: ' + data.reason) : 'Loop finished.'
      });
      return;
    }

    if (type === 'error') {
      addSessionEntry(sessionID, {
        kind: 'error',
        label: 'error',
        text: data && data.error ? data.error : safeJSONString(data)
      });
      return;
    }

    addSessionEntry(sessionID, {
      kind: 'system',
      label: type,
      text: safeJSONString(data)
    });
  }

  function applySessionSnapshot(sessionID, snapshot, replay) {
    var data = asObject(snapshot);
    if (!data || typeof data !== 'object') {
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'snapshot',
        text: safeJSONString(snapshot)
      });
      return;
    }

    var stream = ensureSessionStream(sessionID);

    if (data.session && typeof data.session === 'object') {
      stream.metrics.inputTokens = numberOr(stream.metrics.inputTokens, data.session.input_tokens);
      stream.metrics.outputTokens = numberOr(stream.metrics.outputTokens, data.session.output_tokens);
      stream.metrics.costUSD = numberOr(stream.metrics.costUSD, data.session.cost_usd);
      stream.metrics.turns = numberOr(stream.metrics.turns, data.session.num_turns);
    }

    if (data.loop && typeof data.loop === 'object') {
      updateLoopProgress(stream, data.loop);
    }

    if (Array.isArray(data.spawns)) {
      stream.spawns = data.spawns;
    }

    if (Array.isArray(data.recent)) {
      data.recent.forEach(function (recentMessage) {
        if (!recentMessage || recentMessage.type === 'snapshot') return;
        ingestSessionWireMessage(sessionID, recentMessage, true);
      });
    }

    if (!replay) {
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'snapshot',
        text: 'Stream snapshot received.'
      });
    }
  }

  function handleSessionStreamEvent(sessionID, rawEvent) {
    var event = asObject(rawEvent);
    if (!event || typeof event !== 'object') {
      addSessionEntry(sessionID, {
        kind: 'system',
        label: 'event',
        text: safeJSONString(rawEvent)
      });
      return;
    }

    var stream = ensureSessionStream(sessionID);

    if (event.type === 'assistant') {
      var blocks = extractContentBlocks(event);
      if (!blocks.length) {
        addSessionEntry(sessionID, {
          kind: 'assistant',
          label: 'assistant',
          text: '[assistant event]'
        });
        return;
      }
      blocks.forEach(function (block) {
        handleAssistantBlock(sessionID, block);
      });
      return;
    }

    if (event.type === 'user') {
      var userBlocks = extractContentBlocks(event);
      userBlocks.forEach(function (block) {
        if (!block || typeof block !== 'object') return;
        if (block.type === 'tool_result') {
          attachToolResult(stream, block.content || block.output || block.text || safeJSONString(block));
        }
      });
      return;
    }

    if (event.type === 'content_block_delta') {
      var delta = event.delta && (event.delta.text || event.delta.partial_json);
      if (delta) {
        var last = stream.entries[stream.entries.length - 1];
        if (last && last.kind === 'assistant' && last.isDelta) {
          last.text += delta;
          last.ts = Date.now();
        } else {
          addSessionEntry(sessionID, {
            kind: 'assistant',
            label: 'assistant',
            text: String(delta),
            isDelta: true
          });
        }
      }
      return;
    }

    if (event.type === 'result') {
      applyResultMetrics(stream, event);
      var resultBits = [];
      if (event.subtype) resultBits.push(String(event.subtype));
      if (typeof stream.metrics.inputTokens === 'number' && stream.metrics.inputTokens > 0) resultBits.push('in=' + stream.metrics.inputTokens);
      if (typeof stream.metrics.outputTokens === 'number' && stream.metrics.outputTokens > 0) resultBits.push('out=' + stream.metrics.outputTokens);
      if (typeof stream.metrics.costUSD === 'number' && stream.metrics.costUSD > 0) resultBits.push('cost=$' + stream.metrics.costUSD.toFixed(4));
      addSessionEntry(sessionID, {
        kind: 'result',
        label: 'result',
        text: resultBits.length ? resultBits.join(' · ') : 'Result event received.'
      });
      return;
    }

    addSessionEntry(sessionID, {
      kind: 'system',
      label: event.type || 'event',
      text: safeJSONString(event)
    });
  }

  function handleAssistantBlock(sessionID, block) {
    if (!block || typeof block !== 'object') return;

    var stream = ensureSessionStream(sessionID);

    if (block.type === 'text' && block.text) {
      addSessionEntry(sessionID, {
        kind: 'assistant',
        label: 'assistant',
        text: String(block.text)
      });
      return;
    }

    if (block.type === 'tool_use') {
      var toolEntry = addSessionEntry(sessionID, {
        kind: 'tool_use',
        label: 'tool',
        name: block.name || 'tool',
        input: block.input || {},
        output: null,
        text: 'Tool invoked.'
      });
      stream.lastToolEntryIndex = stream.entries.length - 1;
      return toolEntry;
    }

    if (block.type === 'tool_result') {
      attachToolResult(stream, block.content || block.output || block.text || safeJSONString(block));
      return;
    }

    addSessionEntry(sessionID, {
      kind: 'assistant',
      label: 'assistant',
      text: safeJSONString(block)
    });
  }

  function attachToolResult(stream, output) {
    if (stream.lastToolEntryIndex >= 0 && stream.lastToolEntryIndex < stream.entries.length) {
      var existing = stream.entries[stream.lastToolEntryIndex];
      if (existing && existing.kind === 'tool_use') {
        existing.output = output;
        existing.ts = Date.now();
        return;
      }
    }

    stream.entries.push({
      kind: 'tool_use',
      label: 'tool',
      name: 'tool_result',
      input: {},
      output: output,
      text: 'Tool result',
      ts: Date.now(),
      isDelta: false
    });
  }

  function applyResultMetrics(stream, event) {
    if (!stream || !event) return;

    var usage = event.usage && typeof event.usage === 'object' ? event.usage : {};

    stream.metrics.inputTokens = numberOr(stream.metrics.inputTokens, event.input_tokens, usage.input_tokens);
    stream.metrics.outputTokens = numberOr(stream.metrics.outputTokens, event.output_tokens, usage.output_tokens);
    stream.metrics.turns = numberOr(stream.metrics.turns, event.num_turns, usage.num_turns);
    stream.metrics.costUSD = numberOr(stream.metrics.costUSD, event.total_cost_usd, event.cost_usd, usage.total_cost_usd, usage.cost_usd);
  }

  function updateLoopProgress(stream, loopData) {
    if (!stream || !loopData) return;

    stream.loop.cycle = numberOr(stream.loop.cycle, loopData.cycle);
    stream.loop.stepIndex = numberOr(stream.loop.stepIndex, loopData.step_index);
    stream.loop.totalSteps = numberOr(stream.loop.totalSteps, loopData.total_steps);
    if (loopData.profile) {
      stream.loop.profile = String(loopData.profile);
    }
  }

  function loopProgressText(loop, started) {
    var step = (typeof loop.stepIndex === 'number' ? (loop.stepIndex + 1) : 0);
    var total = (typeof loop.totalSteps === 'number' && loop.totalSteps > 0) ? loop.totalSteps : '?';
    return (started ? 'Starting' : 'Finished') + ' cycle ' + (loop.cycle + 1) + ' step ' + step + ' of ' + total + (loop.profile ? (' · ' + loop.profile) : '');
  }

  function refreshSessionPanels(sessionID) {
    if (state.tab !== 'sessions') return;
    if (sessionID !== state.selectedSessionID) return;

    var stream = ensureSessionStream(sessionID);

    var metricsNode = document.getElementById('session-metrics');
    if (metricsNode) {
      metricsNode.innerHTML = renderSessionMetrics(stream.metrics);
    }

    var loopNode = document.getElementById('session-loop-progress');
    if (loopNode) {
      loopNode.innerHTML = renderLoopProgress(stream.loop);
    }

    var spawnNode = document.getElementById('session-spawns');
    if (spawnNode) {
      spawnNode.innerHTML = renderSpawnCards(stream.spawns);
    }

    var feed = document.getElementById('session-events-feed');
    if (feed) {
      var stickToBottom = feed.scrollTop + feed.clientHeight >= feed.scrollHeight - 18;
      feed.innerHTML = renderSessionEvents(stream.entries);
      if (stickToBottom) {
        feed.scrollTop = feed.scrollHeight;
      }
    }
  }

  function renderSessionMetrics(metrics) {
    var inputTokens = metrics && typeof metrics.inputTokens === 'number' ? metrics.inputTokens : 0;
    var outputTokens = metrics && typeof metrics.outputTokens === 'number' ? metrics.outputTokens : 0;
    var cost = metrics && typeof metrics.costUSD === 'number' ? metrics.costUSD : 0;
    var turns = metrics && typeof metrics.turns === 'number' ? metrics.turns : 0;
    var totalTokens = inputTokens + outputTokens;

    return '' +
      metricCard('Input Tokens', formatNumber(inputTokens), 'metric-input') +
      metricCard('Output Tokens', formatNumber(outputTokens), 'metric-output') +
      metricCard('Total Tokens', formatNumber(totalTokens), 'metric-total') +
      metricCard('Cost (USD)', cost > 0 ? ('$' + cost.toFixed(4)) : '$0.0000', 'metric-cost') +
      metricCard('Turns', formatNumber(turns), 'metric-turns') +
      metricCard('Avg Tokens/Turn', turns > 0 ? formatNumber(Math.round(totalTokens / turns)) : '-', 'metric-avg');
  }

  function metricCard(label, value, extraClass) {
    var cls = 'metric-card' + (extraClass ? ' ' + extraClass : '');
    return '<div class="' + cls + '"><div class="label">' + escapeHTML(label) + '</div><div class="value">' + escapeHTML(String(value)) + '</div></div>';
  }

  function renderLoopProgress(loop) {
    if (!loop) return '';

    var step = (typeof loop.stepIndex === 'number' ? loop.stepIndex + 1 : 0);
    var total = (typeof loop.totalSteps === 'number' && loop.totalSteps > 0) ? loop.totalSteps : '?';

    return '' +
      '<div class="loop-progress">' +
        '<strong>Loop Progress:</strong> cycle ' + (loop.cycle + 1) + ', step ' + step + ' of ' + total +
        (loop.profile ? (' · profile ' + escapeHTML(loop.profile)) : '') +
      '</div>';
  }

  function renderSpawnCards(spawns) {
    var list = arrayOrEmpty(spawns);
    if (!list.length) return '';

    var roots = [];
    var childMap = {};
    list.forEach(function (spawn) {
      var parentID = spawn.parent_id || spawn.parent_spawn_id || 0;
      if (!parentID) {
        roots.push(spawn);
      } else {
        if (!childMap[parentID]) childMap[parentID] = [];
        childMap[parentID].push(spawn);
      }
    });

    if (!roots.length) roots = list;

    function renderSpawnNode(spawn, depth) {
      var children = childMap[spawn.id] || [];
      var statusNorm = normalizeStatus(spawn.status || 'unknown');
      var statusIcon = statusNorm === 'running' || statusNorm === 'active' ? '\u{1F7E2}' :
                       statusNorm === 'complete' || statusNorm === 'done' ? '\u{2705}' :
                       statusNorm === 'error' ? '\u{1F534}' : '\u{26AA}';

      var html = '' +
        '<div class="spawn-tree-node" style="margin-left:' + (depth * 1.2) + 'rem">' +
          '<details class="spawn-details"' + (children.length || spawn.objective ? '' : ' open') + '>' +
            '<summary class="spawn-summary">' +
              '<span class="spawn-status-icon">' + statusIcon + '</span>' +
              '<strong>Spawn #' + escapeHTML(String(spawn.id)) + '</strong>' +
              '<span class="meta"> ' + escapeHTML(spawn.profile || spawn.agent || 'unknown') + '</span>' +
              (spawn.role ? '<span class="spawn-role-tag">' + escapeHTML(spawn.role) + '</span>' : '') +
              '<span class="pill status-' + statusNorm + '" style="margin-left:0.4rem"><span class="dot"></span>' + escapeHTML(spawn.status || 'unknown') + '</span>' +
            '</summary>' +
            '<div class="spawn-tree-body">' +
              (spawn.objective ? '<div class="spawn-objective">' + escapeHTML(String(spawn.objective).slice(0, 200)) + '</div>' : '') +
              (spawn.description ? '<div class="spawn-description meta">' + escapeHTML(String(spawn.description).slice(0, 300)) + '</div>' : '') +
            '</div>' +
          '</details>' +
          (children.length ? children.map(function (child) { return renderSpawnNode(child, depth + 1); }).join('') : '') +
        '</div>';

      return html;
    }

    return '<div class="spawn-tree">' +
      '<h4 class="spawn-tree-title">Spawn Tree (' + list.length + ')</h4>' +
      roots.map(function (root) { return renderSpawnNode(root, 0); }).join('') +
    '</div>';
  }

  var TOOL_ICONS = {
    Read: '\u{1F4C4}', Write: '\u{1F4DD}', Edit: '\u{270F}\uFE0F',
    Bash: '\u{1F4BB}', Grep: '\u{1F50D}', Glob: '\u{1F4C2}',
    WebFetch: '\u{1F310}', WebSearch: '\u{1F50E}', Task: '\u{1F9E9}',
    TodoWrite: '\u{2705}', NotebookEdit: '\u{1F4D3}',
    AskUserQuestion: '\u{2753}', Skill: '\u{26A1}',
    shell: '\u{1F4BB}', command_execution: '\u{1F4BB}'
  };

  function toolIcon(name) {
    return TOOL_ICONS[name] || '\u{1F527}';
  }

  function isDiffContent(text) {
    if (typeof text !== 'string' || text.length < 10) return false;
    var lines = text.split('\n');
    var diffIndicators = 0;
    for (var i = 0; i < Math.min(lines.length, 20); i++) {
      if (/^[+-]{3}\s/.test(lines[i]) || /^@@\s/.test(lines[i]) || /^diff\s--git/.test(lines[i])) diffIndicators++;
    }
    return diffIndicators >= 2;
  }

  function renderDiff(text) {
    var lines = String(text).split('\n');
    return '<div class="diff-block">' + lines.map(function (line) {
      var cls = 'diff-line';
      if (/^@@/.test(line)) cls += ' diff-hunk';
      else if (/^\+/.test(line)) cls += ' diff-add';
      else if (/^-/.test(line)) cls += ' diff-del';
      else if (/^diff\s--git/.test(line) || /^index\s/.test(line)) cls += ' diff-meta';
      return '<div class="' + cls + '">' + escapeHTML(line) + '</div>';
    }).join('') + '</div>';
  }

  function renderToolOutput(output) {
    if (output == null || output === '') {
      return '<div class="meta tool-output-pending">Waiting for tool output...</div>';
    }
    var text = typeof output === 'object' ? safeJSONString(output) : String(output);
    if (isDiffContent(text)) {
      return '<div class="tool-output-label">Output</div>' + renderDiff(text);
    }
    if (typeof output === 'object') {
      return '<div class="tool-output-label">Output</div><pre class="code-block">' + highlightJSON(output) + '</pre>';
    }
    return '<div class="tool-output-label">Output</div><pre class="code-block">' + escapeHTML(text) + '</pre>';
  }

  function renderToolInput(toolName, input) {
    if (input == null) return '';
    var inputObj = typeof input === 'object' ? input : {};

    if (toolName === 'Edit' && inputObj.file_path) {
      var parts = [];
      parts.push('<div class="tool-file-path">' + escapeHTML(inputObj.file_path) + '</div>');
      if (inputObj.old_string) {
        parts.push('<div class="tool-diff-inline">');
        parts.push('<div class="diff-block"><div class="diff-line diff-del">' + escapeHTML(inputObj.old_string).replace(/\n/g, '</div><div class="diff-line diff-del">') + '</div></div>');
        if (inputObj.new_string) {
          parts.push('<div class="diff-block"><div class="diff-line diff-add">' + escapeHTML(inputObj.new_string).replace(/\n/g, '</div><div class="diff-line diff-add">') + '</div></div>');
        }
        parts.push('</div>');
      }
      return parts.join('');
    }

    if ((toolName === 'Read' || toolName === 'Write' || toolName === 'Glob' || toolName === 'Grep') && inputObj.file_path) {
      return '<div class="tool-file-path">' + escapeHTML(inputObj.file_path) + '</div>' +
        '<pre class="code-block">' + highlightJSON(inputObj) + '</pre>';
    }

    if (toolName === 'Bash' && inputObj.command) {
      var cmdHTML = escapeHTML(inputObj.command);
      if (typeof hljs !== 'undefined') {
        try { cmdHTML = hljs.highlight(inputObj.command, { language: 'bash' }).value; } catch (_) {}
      }
      return '<pre class="code-block hljs"><code>' + cmdHTML + '</code></pre>';
    }

    return '<pre class="code-block">' + highlightJSON(inputObj) + '</pre>';
  }

  function renderSessionEvents(entries) {
    if (!entries || !entries.length) {
      return '<p class="empty">Waiting for stream events...</p>';
    }
    return entries.map(function (entry, index) {
      return renderSessionEventCard(entry, index, entries);
    }).join('');
  }

  function renderSessionEventCard(entry, index, entries) {
    var kind = entry.kind || 'system';
    var eventClass = 'event-card event-' + normalizeStatus(kind.replace(/_use$/, '').replace(/_result$/, ''));
    if (kind === 'tool_use') eventClass = 'event-card event-tool';
    if (kind === 'assistant') eventClass = 'event-card event-assistant';
    if (kind === 'result') eventClass = 'event-card event-result';
    if (kind === 'spawn') eventClass = 'event-card event-spawn';
    if (kind === 'loop') eventClass = 'event-card event-loop';
    if (kind === 'error') eventClass = 'event-card event-error';
    if (kind === 'raw' || kind === 'system') eventClass = 'event-card event-raw';

    var turnBoundary = '';
    if (kind === 'result' && entry.label === 'result') {
      var turnNum = 0;
      for (var t = 0; t <= index; t++) {
        if (entries[t] && entries[t].kind === 'result' && entries[t].label === 'result') turnNum++;
      }
      turnBoundary = '<div class="turn-boundary"><span class="turn-boundary-line"></span><span class="turn-boundary-label">Turn ' + turnNum + ' complete</span><span class="turn-boundary-line"></span></div>';
    }
    if (kind === 'loop' && entry.label === 'loop step start') {
      turnBoundary = '<div class="turn-boundary turn-boundary-loop"><span class="turn-boundary-line"></span><span class="turn-boundary-label">' + escapeHTML(entry.text || 'Loop step') + '</span><span class="turn-boundary-line"></span></div>';
    }

    var bodyHTML = '';

    if (kind === 'assistant') {
      bodyHTML = '<div class="markdown">' + renderMarkdown(entry.text || '') + '</div>';
    } else if (kind === 'tool_use') {
      var toolName = entry.name || 'tool';
      var icon = toolIcon(toolName);
      var defaultOpen = (toolName === 'Edit' || toolName === 'Write') ? ' open' : '';

      bodyHTML = '' +
        '<details class="tool-section"' + defaultOpen + '>' +
          '<summary class="tool-summary">' +
            '<span class="tool-icon">' + icon + '</span>' +
            '<span class="tool-name">' + escapeHTML(toolName) + '</span>' +
            (entry.output != null && entry.output !== '' ? '<span class="tool-done-badge">done</span>' : '<span class="tool-pending-badge">running</span>') +
          '</summary>' +
          '<div class="tool-body">' +
            '<div class="tool-input-label">Input</div>' +
            renderToolInput(toolName, entry.input) +
            renderToolOutput(entry.output) +
          '</div>' +
        '</details>';
    } else if (kind === 'raw') {
      bodyHTML = '<pre class="code-block">' + escapeHTML(entry.text || '') + '</pre>';
    } else if (kind === 'system' || kind === 'loop' || kind === 'spawn' || kind === 'result' || kind === 'error') {
      bodyHTML = '<div>' + escapeHTML(entry.text || '') + '</div>';
      if (entry.data) {
        bodyHTML += '<pre class="code-block">' + highlightJSON(entry.data) + '</pre>';
      }
    } else {
      bodyHTML = '<pre class="code-block">' + escapeHTML(entry.text || safeJSONString(entry.data)) + '</pre>';
    }

    return turnBoundary +
      '<article class="' + eventClass + '">' +
        '<div class="event-header"><span>' + escapeHTML(entry.label || entry.kind || 'event') + '</span><span>' + escapeHTML(formatClock(entry.ts)) + '</span></div>' +
        '<div class="event-body">' + bodyHTML + '</div>' +
      '</article>';
  }
  async function renderPlans() {
    var tab = state.tab;
    renderLoading('plans');

    try {
      var plans = arrayOrEmpty(await apiCall(apiBase() + '/plans', 'GET'));
      if (state.tab !== tab) return;

      state.cache.plans = plans;
      if (!state.selectedPlanID && plans.length > 0) {
        state.selectedPlanID = plans[0].id;
      }

      var detail = null;
      if (state.selectedPlanID) {
        detail = await apiCall(apiBase() + '/plans/' + encodeURIComponent(state.selectedPlanID), 'GET', null, { allow404: true });
      }
      if (state.tab !== tab) return;

      state.cache.planDetail = detail || null;

      content.innerHTML = '' +
        '<section class="grid">' +
          '<article class="card span-4">' +
            '<div class="card-title-row"><h2>Plans</h2><button data-action="open-create-plan">Create Plan</button></div>' +
            renderPlanList(plans) +
          '</article>' +
          '<article class="card span-8">' +
            renderPlanDetail(detail) +
          '</article>' +
        '</section>';
    } catch (err) {
      if (err && err.authRequired) return;
      renderError('Failed to load plans: ' + errorMessage(err));
    }
  }

  function renderPlanList(plans) {
    if (!plans.length) return '<p class="empty">No plans found.</p>';

    return '<ul class="list">' + plans.map(function (plan) {
      var active = plan.id === state.selectedPlanID;
      return '' +
        '<li>' +
          '<div class="list-item">' +
            '<div class="list-item-main">' +
              '<strong>' + escapeHTML(plan.title || plan.id) + '</strong>' +
              '<span class="meta mono">' + escapeHTML(plan.id) + '</span>' +
              '<div class="filters">' + createPill(plan.status || 'active', plan.status || 'active') + '</div>' +
            '</div>' +
            '<div class="list-item-actions"><button data-action="select-plan" data-plan-id="' + escapeHTML(plan.id) + '" class="' + (active ? 'active' : '') + '">View</button></div>' +
          '</div>' +
        '</li>';
    }).join('') + '</ul>';
  }

  function renderPlanDetail(plan) {
    if (!plan) {
      return '<h2>Plan Detail</h2><p class="empty">Select a plan to inspect phases.</p>';
    }

    var phases = arrayOrEmpty(plan.phases);

    return '' +
      '<div class="card-title-row">' +
        '<h2>' + escapeHTML(plan.title || plan.id) + '</h2>' +
        '<div class="button-row">' +
          '<button data-action="open-edit-plan">Edit Plan</button>' +
          '<button data-action="activate-plan" class="success">Activate Plan</button>' +
          '<button data-action="delete-plan" class="danger">Delete Plan</button>' +
        '</div>' +
      '</div>' +
      '<p class="meta mono">ID: ' + escapeHTML(plan.id) + '</p>' +
      '<div class="filters">' + createPill(plan.status || 'active', plan.status || 'active') + '</div>' +
      '<div class="markdown">' + renderMarkdown(plan.description || '_No description._') + '</div>' +
      '<div class="card-title-row" style="margin-top:0.8rem"><h3>Phases</h3><button data-action="open-create-phase">Create Phase</button></div>' +
      (phases.length ? '<ul class="list">' + phases.map(function (phase) {
        return '' +
          '<li>' +
            '<div class="list-item">' +
              '<div class="list-item-main">' +
                '<strong>' + escapeHTML(phase.title || phase.id || 'Untitled') + '</strong>' +
                '<span class="meta mono">' + escapeHTML(phase.id || '-') + '</span>' +
                '<div class="item-preview">' + escapeHTML(phase.description || 'No description') + '</div>' +
                '<div class="filters">' + createPill(phase.status || 'not_started', phase.status || 'not_started') + '<span class="meta">Priority ' + escapeHTML(String(phase.priority || 0)) + '</span></div>' +
              '</div>' +
              '<div class="list-item-actions"><button data-action="open-edit-phase" data-phase-id="' + escapeHTML(phase.id || '') + '">Edit</button></div>' +
            '</div>' +
          '</li>';
      }).join('') + '</ul>' : '<p class="empty">No phases defined.</p>');
  }

  async function activatePlan(planID) {
    try {
      await apiCall(apiBase() + '/plans/' + encodeURIComponent(planID) + '/activate', 'POST');
      showToast('Plan ' + planID + ' activated.', 'success');
      renderPlans();
    } catch (err) {
      showToast('Failed to activate plan: ' + errorMessage(err), 'error');
    }
  }

  async function deletePlan(planID) {
    if (!window.confirm('Delete plan "' + planID + '"?')) return;

    try {
      await apiCall(apiBase() + '/plans/' + encodeURIComponent(planID), 'DELETE');
      showToast('Plan deleted: ' + planID, 'success');
      if (state.selectedPlanID === planID) state.selectedPlanID = '';
      renderPlans();
    } catch (err) {
      showToast('Failed to delete plan: ' + errorMessage(err), 'error');
    }
  }

  async function renderIssues() {
    var tab = state.tab;
    renderLoading('issues');

    try {
      var issuePath = apiBase() + '/issues';
      if (state.issueStatusFilter !== 'all') {
        issuePath += '?status=' + encodeURIComponent(state.issueStatusFilter);
      }

      var results = await Promise.all([
        apiCall(issuePath, 'GET'),
        apiCall(apiBase() + '/plans', 'GET')
      ]);

      if (state.tab !== tab) return;

      var issues = arrayOrEmpty(results[0]);
      var plans = arrayOrEmpty(results[1]);
      state.cache.plans = plans;

      if (state.issuePriorityFilter !== 'all') {
        issues = issues.filter(function (issue) {
          return normalizeStatus(issue.priority) === normalizeStatus(state.issuePriorityFilter);
        });
      }

      state.cache.issues = issues;

      if (!state.selectedIssueID && issues.length > 0) {
        state.selectedIssueID = issues[0].id;
      }

      var selected = issues.find(function (issue) {
        return issue.id === state.selectedIssueID;
      }) || null;

      content.innerHTML = '' +
        '<section class="grid">' +
          '<article class="card span-5">' +
            '<div class="card-title-row"><h2>Issues</h2><button data-action="open-create-issue">Create Issue</button></div>' +
            '<div class="filters">' +
              '<select data-change="issue-status-filter">' +
                issueFilterOptions('all', state.issueStatusFilter, 'All statuses') +
                issueFilterOptions('open', state.issueStatusFilter, 'Open') +
                issueFilterOptions('in_progress', state.issueStatusFilter, 'In Progress') +
                issueFilterOptions('resolved', state.issueStatusFilter, 'Resolved') +
                issueFilterOptions('wontfix', state.issueStatusFilter, 'Wontfix') +
              '</select>' +
              '<select data-change="issue-priority-filter">' +
                issueFilterOptions('all', state.issuePriorityFilter, 'All priorities') +
                issueFilterOptions('critical', state.issuePriorityFilter, 'Critical') +
                issueFilterOptions('high', state.issuePriorityFilter, 'High') +
                issueFilterOptions('medium', state.issuePriorityFilter, 'Medium') +
                issueFilterOptions('low', state.issuePriorityFilter, 'Low') +
              '</select>' +
            '</div>' +
            renderIssueList(issues) +
          '</article>' +
          '<article class="card span-7">' +
            renderIssueDetail(selected) +
          '</article>' +
        '</section>';
    } catch (err) {
      if (err && err.authRequired) return;
      renderError('Failed to load issues: ' + errorMessage(err));
    }
  }

  function issueFilterOptions(value, current, label) {
    return '<option value="' + value + '"' + (value === current ? ' selected' : '') + '>' + escapeHTML(label) + '</option>';
  }

  function renderIssueList(issues) {
    if (!issues.length) return '<p class="empty">No matching issues.</p>';

    return '<ul class="list">' + issues.map(function (issue) {
      return '' +
        '<li>' +
          '<div class="list-item">' +
            '<div class="list-item-main">' +
              '<strong>#' + issue.id + ' ' + escapeHTML(issue.title || 'Untitled') + '</strong>' +
              '<span class="meta">Plan: ' + escapeHTML(issue.plan_id || 'none') + ' · Updated ' + escapeHTML(formatRelativeTime(issue.updated)) + '</span>' +
              '<div class="item-preview">' + escapeHTML((issue.description || '').slice(0, 160) || 'No description') + '</div>' +
              '<div class="filters">' + createPill(issue.priority || 'medium', issue.priority || 'medium') + createPill(issue.status || 'open', issue.status || 'open') + '</div>' +
            '</div>' +
            '<div class="list-item-actions">' +
              '<button data-action="select-issue" data-issue-id="' + issue.id + '" class="' + (issue.id === state.selectedIssueID ? 'active' : '') + '">View</button>' +
              '<button data-action="open-edit-issue" data-issue-id="' + issue.id + '">Edit</button>' +
              '<button data-action="delete-issue" data-issue-id="' + issue.id + '" class="danger">Delete</button>' +
            '</div>' +
          '</div>' +
        '</li>';
    }).join('') + '</ul>';
  }

  function renderIssueDetail(issue) {
    if (!issue) {
      return '<h2>Issue Detail</h2><p class="empty">Select an issue to view full details.</p>';
    }

    return '' +
      '<div class="card-title-row"><h2>#' + issue.id + ' ' + escapeHTML(issue.title || 'Untitled') + '</h2><div class="button-row"><button data-action="open-edit-issue" data-issue-id="' + issue.id + '">Edit</button><button data-action="delete-issue" data-issue-id="' + issue.id + '" class="danger">Delete</button></div></div>' +
      '<div class="filters">' + createPill(issue.status || 'open', issue.status || 'open') + createPill(issue.priority || 'medium', issue.priority || 'medium') + '</div>' +
      '<div class="kv-grid">' +
        '<div class="kv"><div class="label">Plan</div><div class="value mono">' + escapeHTML(issue.plan_id || 'none') + '</div></div>' +
        '<div class="kv"><div class="label">Labels</div><div class="value">' + escapeHTML((issue.labels || []).join(', ') || 'none') + '</div></div>' +
        '<div class="kv"><div class="label">Created</div><div class="value">' + escapeHTML(formatAbsolute(issue.created)) + '</div></div>' +
        '<div class="kv"><div class="label">Updated</div><div class="value">' + escapeHTML(formatAbsolute(issue.updated)) + '</div></div>' +
      '</div>' +
      '<section class="markdown">' + renderMarkdown(issue.description || '_No description provided._') + '</section>';
  }

  async function deleteIssue(issueID) {
    if (!window.confirm('Delete issue #' + issueID + '?')) return;

    try {
      await apiCall(apiBase() + '/issues/' + encodeURIComponent(String(issueID)), 'DELETE');
      showToast('Issue deleted: #' + issueID, 'success');
      if (state.selectedIssueID === issueID) state.selectedIssueID = null;
      renderIssues();
    } catch (err) {
      showToast('Failed to delete issue: ' + errorMessage(err), 'error');
    }
  }

  async function renderDocs() {
    var tab = state.tab;
    renderLoading('docs');

    try {
      var docsPath = apiBase() + '/docs';
      if (state.docsPlanFilter !== 'all') {
        docsPath += '?plan=' + encodeURIComponent(state.docsPlanFilter);
      }

      var results = await Promise.all([
        apiCall(docsPath, 'GET'),
        apiCall(apiBase() + '/plans', 'GET')
      ]);

      if (state.tab !== tab) return;

      var docs = arrayOrEmpty(results[0]);
      var plans = arrayOrEmpty(results[1]);

      state.cache.docs = docs;
      state.cache.plans = plans;

      if (!state.selectedDocID && docs.length > 0) {
        state.selectedDocID = docs[0].id;
      }

      var selected = docs.find(function (doc) { return doc.id === state.selectedDocID; }) || null;

      content.innerHTML = '' +
        '<section class="grid">' +
          '<article class="card span-5">' +
            '<div class="card-title-row"><h2>Docs</h2><button data-action="open-create-doc">Create Doc</button></div>' +
            '<div class="filters"><select data-change="docs-plan-filter">' +
              '<option value="all">All plans</option>' +
              plans.map(function (plan) {
                return '<option value="' + escapeHTML(plan.id) + '"' + (plan.id === state.docsPlanFilter ? ' selected' : '') + '>' + escapeHTML(plan.id) + '</option>';
              }).join('') +
            '</select></div>' +
            renderDocList(docs) +
          '</article>' +
          '<article class="card span-7">' +
            renderDocDetail(selected) +
          '</article>' +
        '</section>';
    } catch (err) {
      if (err && err.authRequired) return;
      renderError('Failed to load docs: ' + errorMessage(err));
    }
  }

  function renderDocList(docs) {
    if (!docs.length) return '<p class="empty">No docs found.</p>';

    return '<ul class="list">' + docs.map(function (doc) {
      return '' +
        '<li>' +
          '<div class="list-item">' +
            '<div class="list-item-main">' +
              '<strong>' + escapeHTML(doc.title || doc.id) + '</strong>' +
              '<span class="meta mono">' + escapeHTML(doc.id) + ' · plan ' + escapeHTML(doc.plan_id || 'shared') + '</span>' +
              '<div class="item-preview">' + escapeHTML((doc.content || '').slice(0, 160) || 'No content') + '</div>' +
            '</div>' +
            '<div class="list-item-actions">' +
              '<button data-action="select-doc" data-doc-id="' + escapeHTML(doc.id) + '" class="' + (state.selectedDocID === doc.id ? 'active' : '') + '">View</button>' +
              '<button data-action="open-edit-doc" data-doc-id="' + escapeHTML(doc.id) + '">Edit</button>' +
              '<button data-action="delete-doc" data-doc-id="' + escapeHTML(doc.id) + '" class="danger">Delete</button>' +
            '</div>' +
          '</div>' +
        '</li>';
    }).join('') + '</ul>';
  }

  function renderDocDetail(doc) {
    if (!doc) {
      return '<h2>Doc Detail</h2><p class="empty">Select a document to view details.</p>';
    }

    return '' +
      '<div class="card-title-row"><h2>' + escapeHTML(doc.title || doc.id) + '</h2><div class="button-row"><button data-action="open-edit-doc" data-doc-id="' + escapeHTML(doc.id) + '">Edit</button><button data-action="delete-doc" data-doc-id="' + escapeHTML(doc.id) + '" class="danger">Delete</button></div></div>' +
      '<div class="kv-grid">' +
        '<div class="kv"><div class="label">ID</div><div class="value mono">' + escapeHTML(doc.id) + '</div></div>' +
        '<div class="kv"><div class="label">Plan</div><div class="value mono">' + escapeHTML(doc.plan_id || 'shared') + '</div></div>' +
        '<div class="kv"><div class="label">Created</div><div class="value">' + escapeHTML(formatAbsolute(doc.created)) + '</div></div>' +
        '<div class="kv"><div class="label">Updated</div><div class="value">' + escapeHTML(formatAbsolute(doc.updated)) + '</div></div>' +
      '</div>' +
      '<section class="markdown">' + renderMarkdown(doc.content || '_No content._') + '</section>';
  }

  async function deleteDoc(docID) {
    if (!window.confirm('Delete doc "' + docID + '"?')) return;

    try {
      await apiCall(apiBase() + '/docs/' + encodeURIComponent(docID), 'DELETE');
      showToast('Doc deleted: ' + docID, 'success');
      if (state.selectedDocID === docID) state.selectedDocID = '';
      renderDocs();
    } catch (err) {
      showToast('Failed to delete doc: ' + errorMessage(err), 'error');
    }
  }

  async function renderConfig() {
    var tab = state.tab;
    renderLoading('config');

    try {
      var results = await Promise.all([
        apiCall('/api/config/profiles', 'GET'),
        apiCall('/api/config/loops', 'GET'),
        apiCall('/api/config/roles', 'GET'),
        apiCall('/api/config/rules', 'GET'),
        apiCall('/api/config/pushover', 'GET')
      ]);

      if (state.tab !== tab) return;

      state.cache.profiles = arrayOrEmpty(results[0]);
      state.cache.loops = arrayOrEmpty(results[1]);
      state.cache.roles = arrayOrEmpty(results[2]);
      state.cache.rules = arrayOrEmpty(results[3]);
      state.cache.pushover = results[4] || {};

      var sectionHTML = '';
      if (state.configSection === 'profiles') sectionHTML = renderProfileSection(state.cache.profiles);
      if (state.configSection === 'loops') sectionHTML = renderLoopSection(state.cache.loops);
      if (state.configSection === 'roles') sectionHTML = renderRoleSection(state.cache.roles);
      if (state.configSection === 'rules') sectionHTML = renderRuleSection(state.cache.rules);
      if (state.configSection === 'pushover') sectionHTML = renderPushoverSection(state.cache.pushover);

      content.innerHTML = '' +
        '<section class="card">' +
          '<h2>Config</h2>' +
          '<div class="subtabs">' +
            configTabButton('profiles', 'Profiles') +
            configTabButton('loops', 'Loop Definitions') +
            configTabButton('roles', 'Roles') +
            configTabButton('rules', 'Rules') +
            configTabButton('pushover', 'Pushover') +
          '</div>' +
          sectionHTML +
        '</section>';
    } catch (err) {
      if (err && err.authRequired) return;
      renderError('Failed to load config: ' + errorMessage(err));
    }
  }

  function configTabButton(section, label) {
    return '<button data-action="set-config-section" data-section="' + section + '" class="' + (section === state.configSection ? 'active' : '') + '">' + escapeHTML(label) + '</button>';
  }

  function renderProfileSection(profiles) {
    return '' +
      '<div class="card-title-row"><h3>Profiles</h3><button data-action="open-create-profile">Create Profile</button></div>' +
      (profiles.length ? '<ul class="list">' + profiles.map(function (profile) {
        return '' +
          '<li>' +
            '<div class="list-item">' +
              '<div class="list-item-main">' +
                '<strong>' + escapeHTML(profile.name || 'unnamed') + '</strong>' +
                '<span class="meta">' + escapeHTML(profile.agent || 'agent') + ' · ' + escapeHTML(profile.model || 'default model') + '</span>' +
                '<div class="item-preview">Reasoning: ' + escapeHTML(profile.reasoning_level || 'default') + ' · Intelligence: ' + escapeHTML(String(profile.intelligence || 0)) + ' · Max instances: ' + escapeHTML(String(profile.max_instances || 0)) + ' · Speed: ' + escapeHTML(profile.speed || 'n/a') + '</div>' +
              '</div>' +
              '<div class="list-item-actions">' +
                '<button data-action="open-edit-profile" data-profile-name="' + escapeHTML(profile.name) + '">Edit</button>' +
                '<button data-action="delete-profile" data-profile-name="' + escapeHTML(profile.name) + '" class="danger">Delete</button>' +
              '</div>' +
            '</div>' +
          '</li>';
      }).join('') + '</ul>' : '<p class="empty">No profiles configured.</p>');
  }
  function renderLoopSection(loops) {
    return '' +
      '<div class="card-title-row"><h3>Loop Definitions</h3><button data-action="open-create-loop">Create Loop</button></div>' +
      (loops.length ? '<ul class="list">' + loops.map(function (loop) {
        var steps = arrayOrEmpty(loop.steps);
        return '' +
          '<li>' +
            '<div class="list-item">' +
              '<div class="list-item-main">' +
                '<strong>' + escapeHTML(loop.name || 'unnamed-loop') + '</strong>' +
                '<span class="meta">' + steps.length + ' steps</span>' +
                '<div class="item-preview">' + escapeHTML(steps.map(function (step, index) {
                  return (index + 1) + '. ' + (step.profile || 'profile?') + (step.role ? (' [' + step.role + ']') : '');
                }).join('  |  ')) + '</div>' +
              '</div>' +
              '<div class="list-item-actions">' +
                '<button data-action="open-edit-loop" data-loop-name="' + escapeHTML(loop.name) + '">Edit</button>' +
                '<button data-action="delete-loop" data-loop-name="' + escapeHTML(loop.name) + '" class="danger">Delete</button>' +
              '</div>' +
            '</div>' +
          '</li>';
      }).join('') + '</ul>' : '<p class="empty">No loop definitions configured.</p>');
  }

  function renderRoleSection(roles) {
    return '' +
      '<div class="card-title-row"><h3>Roles</h3><button data-action="open-create-role">Create Role</button></div>' +
      (roles.length ? '<ul class="list">' + roles.map(function (role) {
        return '' +
          '<li>' +
            '<div class="list-item">' +
              '<div class="list-item-main">' +
                '<strong>' + escapeHTML(role.name || 'unnamed-role') + '</strong>' +
                '<span class="meta">' + escapeHTML(role.title || '') + '</span>' +
                '<div class="item-preview">' + escapeHTML(role.description || '') + '</div>' +
              '</div>' +
              '<div class="list-item-actions"><button data-action="delete-role" data-role-name="' + escapeHTML(role.name || '') + '" class="danger">Delete</button></div>' +
            '</div>' +
          '</li>';
      }).join('') + '</ul>' : '<p class="empty">No roles configured.</p>');
  }

  function renderRuleSection(rules) {
    return '' +
      '<div class="card-title-row"><h3>Rules</h3><button data-action="open-create-rule">Create Rule</button></div>' +
      (rules.length ? '<ul class="list">' + rules.map(function (rule) {
        return '' +
          '<li>' +
            '<div class="list-item">' +
              '<div class="list-item-main">' +
                '<strong class="mono">' + escapeHTML(rule.id || 'rule') + '</strong>' +
                '<div class="item-preview">' + escapeHTML((rule.body || '').slice(0, 220) || 'No body') + '</div>' +
              '</div>' +
              '<div class="list-item-actions"><button data-action="delete-rule" data-rule-id="' + escapeHTML(rule.id || '') + '" class="danger">Delete</button></div>' +
            '</div>' +
          '</li>';
      }).join('') + '</ul>' : '<p class="empty">No prompt rules configured.</p>');
  }

  function renderPushoverSection(pushover) {
    var cfg = pushover || {};
    return '' +
      '<div class="card-title-row"><h3>Pushover</h3></div>' +
      '<form data-form="save-pushover">' +
        '<div class="form-grid">' +
          '<div class="form-field"><label>User Key</label><input type="text" name="user_key" value="' + escapeHTML(cfg.user_key || '') + '"></div>' +
          '<div class="form-field"><label>App Token</label><input type="text" name="app_token" value="' + escapeHTML(cfg.app_token || '') + '"></div>' +
        '</div>' +
        '<div class="form-actions"><button type="submit" class="success">Save Pushover</button></div>' +
      '</form>';
  }

  async function savePushover(form) {
    var payload = {
      user_key: readString(form, 'user_key'),
      app_token: readString(form, 'app_token')
    };

    try {
      await apiCall('/api/config/pushover', 'PUT', payload);
      showToast('Pushover config saved.', 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to save pushover config: ' + errorMessage(err), 'error');
    }
  }

  function renderTerminal() {
    if (typeof Terminal === 'undefined' || typeof FitAddon === 'undefined' || typeof FitAddon.FitAddon === 'undefined') {
      renderError('Terminal runtime unavailable. Verify bundled xterm assets.');
      return;
    }

    content.innerHTML = '<section class="card terminal-card"><div id="terminal-container"></div></section>';

    var container = document.getElementById('terminal-container');
    if (!container) return;

    if (!state.term) {
      state.term = new Terminal({
        cursorBlink: true,
        fontSize: 13,
        fontFamily: '"IBM Plex Mono", "SFMono-Regular", monospace',
        theme: {
          background: '#11111b',
          foreground: '#cdd6f4',
          cursor: '#f5e0dc',
          selectionBackground: '#45475a',
          black: '#45475a',
          red: '#f38ba8',
          green: '#a6e3a1',
          yellow: '#f9e2af',
          blue: '#89b4fa',
          magenta: '#cba6f7',
          cyan: '#89dceb',
          white: '#bac2de',
          brightBlack: '#585b70',
          brightRed: '#f38ba8',
          brightGreen: '#a6e3a1',
          brightYellow: '#f9e2af',
          brightBlue: '#89b4fa',
          brightMagenta: '#cba6f7',
          brightCyan: '#89dceb',
          brightWhite: '#a6adc8'
        }
      });

      state.termFit = new FitAddon.FitAddon();
      state.term.loadAddon(state.termFit);

      state.term.onData(function (data) {
        if (state.termWS && state.termWS.readyState === WebSocket.OPEN) {
          state.termWS.send(JSON.stringify({ type: 'input', data: btoa(data) }));
        }
      });

      state.term.onResize(function (size) {
        if (state.termWS && state.termWS.readyState === WebSocket.OPEN) {
          state.termWS.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
        }
      });

      state.term.open(container);
    } else if (state.term.element) {
      container.appendChild(state.term.element);
    } else {
      state.term.open(container);
    }

    fitTerminal();
    connectTerminalSocket();

    window.removeEventListener('resize', fitTerminal);
    window.addEventListener('resize', fitTerminal);

    state.term.focus();
  }

  function fitTerminal() {
    if (state.termFit) {
      try {
        state.termFit.fit();
      } catch (_) {
        // Best effort.
      }
    }
  }

  function connectTerminalSocket() {
    disconnectTerminal();

    state.termWS = new WebSocket(buildWSURL('/ws/terminal'));

    state.termWS.addEventListener('open', function () {
      state.termWSConnected = true;
      updateConnectionStatus();
      fitTerminal();
      if (state.term) {
        state.termWS.send(JSON.stringify({
          type: 'resize',
          cols: state.term.cols,
          rows: state.term.rows
        }));
      }
    });

    state.termWS.addEventListener('message', function (event) {
      var msg;
      try {
        msg = JSON.parse(event.data);
      } catch (_) {
        return;
      }

      if (msg.type === 'output' && msg.data && state.term) {
        state.term.write(atob(msg.data));
      } else if (msg.type === 'exit' && state.term) {
        state.term.write('\r\n[Process exited with code ' + (msg.code || 0) + ']\r\n');
      }
    });

    state.termWS.addEventListener('close', function () {
      state.termWSConnected = false;
      updateConnectionStatus();
      if (state.term) state.term.write('\r\n[Connection closed]\r\n');
      state.termWS = null;
    });

    state.termWS.addEventListener('error', function () {
      state.termWSConnected = false;
      updateConnectionStatus();
      if (state.term) state.term.write('\r\n[Connection error]\r\n');
    });
  }

  function disconnectTerminal() {
    window.removeEventListener('resize', fitTerminal);

    if (state.termWS) {
      try {
        state.termWS.close();
      } catch (_) {
        // Best effort.
      }
      state.termWS = null;
    }

    state.termWSConnected = false;
    updateConnectionStatus();
  }

  function openStartSessionModal() {
    var profiles = state.cache.profiles;
    var loops = state.cache.loops;
    var plans = state.cache.plans;

    openModal('Start New Session', '' +
      '<form data-modal-submit="start-session">' +
        '<div class="form-grid">' +
          '<div class="form-field"><label>Session Type</label>' +
            '<select name="session_type">' +
              '<option value="ask">Ask</option>' +
              '<option value="pm">PM</option>' +
              '<option value="loop">Loop</option>' +
            '</select>' +
          '</div>' +
          '<div class="form-field" data-session-only="ask,pm"><label>Profile</label>' +
            '<select name="profile">' +
              profiles.map(function (profile) {
                return '<option value="' + escapeHTML(profile.name) + '">' + escapeHTML(profile.name) + '</option>';
              }).join('') +
            '</select>' +
          '</div>' +
          '<div class="form-field" data-session-only="loop"><label>Loop Definition</label>' +
            '<select name="loop">' +
              loops.map(function (loop) {
                return '<option value="' + escapeHTML(loop.name) + '">' + escapeHTML(loop.name) + '</option>';
              }).join('') +
            '</select>' +
          '</div>' +
          '<div class="form-field" data-session-only="ask,pm"><label>Model Override (optional)</label><input type="text" name="model" placeholder="optional model"></div>' +
          '<div class="form-field" data-session-only="loop"><label>Max Cycles (optional)</label><input type="number" min="1" name="max_cycles" placeholder="e.g. 5"></div>' +
          '<div class="form-field"><label>Plan (optional)</label>' +
            '<select name="plan_id">' +
              '<option value="">Use active plan</option>' +
              plans.map(function (plan) {
                return '<option value="' + escapeHTML(plan.id) + '">' + escapeHTML(plan.id) + '</option>';
              }).join('') +
            '</select>' +
          '</div>' +
          '<div class="form-field span-2"><label>Prompt / Message</label><textarea name="prompt" required placeholder="What should the session do?"></textarea></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">Start Session</button></div>' +
      '</form>'
    );

    var form = modalRoot.querySelector('form[data-modal-submit="start-session"]');
    if (form) updateStartSessionFormVisibility(form);
  }

  function updateStartSessionFormVisibility(form) {
    if (!form) return;

    var type = readString(form, 'session_type') || 'ask';
    form.querySelectorAll('[data-session-only]').forEach(function (node) {
      var list = (node.getAttribute('data-session-only') || '').split(',').map(function (item) {
        return item.trim();
      });
      var visible = list.indexOf(type) >= 0;
      node.classList.toggle('hidden', !visible);
    });
  }

  async function submitStartSession(form) {
    var sessionType = readString(form, 'session_type') || 'ask';
    var profile = readString(form, 'profile');
    var loop = readString(form, 'loop');
    var prompt = readString(form, 'prompt');
    var model = readString(form, 'model');
    var planID = readString(form, 'plan_id');
    var maxCycles = parseInt(readString(form, 'max_cycles'), 10);

    if ((sessionType === 'ask' || sessionType === 'pm') && (!profile || !prompt)) {
      showToast('Profile and prompt are required for ask/pm sessions.', 'error');
      return;
    }

    if (sessionType === 'loop' && !loop) {
      showToast('Loop definition is required for loop sessions.', 'error');
      return;
    }

    var path = apiBase() + '/sessions/ask';
    var payload = {};

    if (sessionType === 'ask') {
      path = apiBase() + '/sessions/ask';
      payload = {
        profile: profile,
        prompt: prompt,
        model: model || ''
      };
    } else if (sessionType === 'pm') {
      path = apiBase() + '/sessions/pm';
      payload = {
        profile: profile,
        prompt: prompt,
        model: model || ''
      };
    } else {
      path = apiBase() + '/sessions/loop';
      payload = {
        loop: loop,
        prompt: prompt
      };
      if (!Number.isNaN(maxCycles) && maxCycles > 0) payload.max_cycles = maxCycles;
    }

    if (planID) payload.plan_id = planID;

    try {
      var created = await apiCall(path, 'POST', payload);
      closeModal();
      showToast('Session started successfully.', 'success');
      if (created && typeof created.id === 'number') {
        state.selectedSessionID = created.id;
      }
      renderSessions();
    } catch (err) {
      showToast('Failed to start session: ' + errorMessage(err), 'error');
    }
  }
  function openIssueModal(mode, issue) {
    var plans = state.cache.plans;
    var current = issue || {};
    var title = mode === 'create' ? 'Create Issue' : 'Edit Issue #' + current.id;

    openModal(title, '' +
      '<form data-modal-submit="' + (mode === 'create' ? 'issue-create' : 'issue-edit') + '"' + (mode === 'edit' ? (' data-issue-id="' + current.id + '"') : '') + '>' +
        '<div class="form-grid">' +
          '<div class="form-field span-2"><label>Title</label><input type="text" name="title" value="' + escapeHTML(current.title || '') + '" required></div>' +
          '<div class="form-field span-2"><label>Description</label><textarea name="description" placeholder="Describe the issue">' + escapeHTML(current.description || '') + '</textarea></div>' +
          '<div class="form-field"><label>Priority</label><select name="priority">' + ISSUE_PRIORITIES.map(function (priority) {
            var selected = normalizeStatus(current.priority || 'medium') === priority;
            return '<option value="' + priority + '"' + (selected ? ' selected' : '') + '>' + capitalize(priority) + '</option>';
          }).join('') + '</select></div>' +
          '<div class="form-field"><label>Status</label><select name="status">' + ISSUE_STATUSES.map(function (status) {
            var selected = normalizeStatus(current.status || 'open') === status;
            return '<option value="' + status + '"' + (selected ? ' selected' : '') + '>' + capitalize(status.replace('_', ' ')) + '</option>';
          }).join('') + '</select></div>' +
          '<div class="form-field"><label>Plan (optional)</label><select name="plan_id"><option value="">No plan</option>' + plans.map(function (plan) {
            return '<option value="' + escapeHTML(plan.id) + '"' + (plan.id === (current.plan_id || '') ? ' selected' : '') + '>' + escapeHTML(plan.id) + '</option>';
          }).join('') + '</select></div>' +
          '<div class="form-field"><label>Labels (comma separated)</label><input type="text" name="labels" value="' + escapeHTML((current.labels || []).join(', ')) + '" placeholder="bug, ui"></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">' + (mode === 'create' ? 'Create Issue' : 'Save Issue') + '</button></div>' +
      '</form>'
    );
  }

  async function submitIssueCreate(form) {
    var payload = issuePayloadFromForm(form);
    if (!payload.title) {
      showToast('Issue title is required.', 'error');
      return;
    }

    try {
      await apiCall(apiBase() + '/issues', 'POST', payload);
      closeModal();
      showToast('Issue created.', 'success');
      renderIssues();
    } catch (err) {
      showToast('Failed to create issue: ' + errorMessage(err), 'error');
    }
  }

  async function submitIssueEdit(form) {
    var issueID = parseInt(form.getAttribute('data-issue-id') || '', 10);
    if (Number.isNaN(issueID)) return;

    var payload = issuePayloadFromForm(form);

    try {
      await apiCall(apiBase() + '/issues/' + encodeURIComponent(String(issueID)), 'PUT', payload);
      closeModal();
      showToast('Issue updated.', 'success');
      renderIssues();
    } catch (err) {
      showToast('Failed to update issue: ' + errorMessage(err), 'error');
    }
  }

  function issuePayloadFromForm(form) {
    var labelsRaw = readString(form, 'labels');

    return {
      title: readString(form, 'title'),
      description: readString(form, 'description'),
      priority: readString(form, 'priority') || 'medium',
      status: readString(form, 'status') || 'open',
      plan_id: readString(form, 'plan_id'),
      labels: labelsRaw ? labelsRaw.split(',').map(function (item) { return item.trim(); }).filter(Boolean) : []
    };
  }

  function openPlanModal(mode, plan) {
    var current = plan || {};
    var title = mode === 'create' ? 'Create Plan' : 'Edit Plan ' + (current.id || '');

    openModal(title, '' +
      '<form data-modal-submit="' + (mode === 'create' ? 'plan-create' : 'plan-edit') + '"' + (mode === 'edit' ? (' data-plan-id="' + escapeHTML(current.id || '') + '"') : '') + '>' +
        '<div class="form-grid">' +
          '<div class="form-field"><label>ID (slug)</label><input type="text" name="id" value="' + escapeHTML(current.id || '') + '" ' + (mode === 'edit' ? 'disabled' : 'required') + '></div>' +
          '<div class="form-field"><label>Title</label><input type="text" name="title" value="' + escapeHTML(current.title || '') + '" required></div>' +
          '<div class="form-field span-2"><label>Description</label><textarea name="description" placeholder="Plan description">' + escapeHTML(current.description || '') + '</textarea></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">' + (mode === 'create' ? 'Create Plan' : 'Save Plan') + '</button></div>' +
      '</form>'
    );
  }

  async function submitPlanCreate(form) {
    var payload = {
      id: slugify(readString(form, 'id')),
      title: readString(form, 'title'),
      description: readString(form, 'description')
    };

    if (!payload.id || !payload.title) {
      showToast('Plan ID and title are required.', 'error');
      return;
    }

    try {
      await apiCall(apiBase() + '/plans', 'POST', payload);
      closeModal();
      state.selectedPlanID = payload.id;
      showToast('Plan created.', 'success');
      renderPlans();
    } catch (err) {
      showToast('Failed to create plan: ' + errorMessage(err), 'error');
    }
  }

  async function submitPlanEdit(form) {
    var planID = form.getAttribute('data-plan-id') || state.selectedPlanID;
    if (!planID) return;

    var payload = {
      title: readString(form, 'title'),
      description: readString(form, 'description')
    };

    try {
      await apiCall(apiBase() + '/plans/' + encodeURIComponent(planID), 'PUT', payload);
      closeModal();
      showToast('Plan updated.', 'success');
      renderPlans();
    } catch (err) {
      showToast('Failed to update plan: ' + errorMessage(err), 'error');
    }
  }

  function openPhaseModal(mode, plan, phase) {
    if (!plan) {
      showToast('Select a plan first.', 'error');
      return;
    }

    var current = phase || {};

    openModal((mode === 'create' ? 'Create Phase' : 'Edit Phase') + ' · ' + escapeHTML(plan.id), '' +
      '<form data-modal-submit="' + (mode === 'create' ? 'phase-create' : 'phase-edit') + '" data-plan-id="' + escapeHTML(plan.id) + '"' + (mode === 'edit' ? (' data-phase-id="' + escapeHTML(current.id || '') + '"') : '') + '>' +
        '<div class="form-grid">' +
          '<div class="form-field"><label>Phase ID</label><input type="text" name="id" value="' + escapeHTML(current.id || '') + '" ' + (mode === 'edit' ? 'disabled' : 'required') + '></div>' +
          '<div class="form-field"><label>Status</label><select name="status">' + PLAN_PHASE_STATUSES.map(function (status) {
            var selected = normalizeStatus(current.status || 'not_started') === status;
            return '<option value="' + status + '"' + (selected ? ' selected' : '') + '>' + capitalize(status.replace('_', ' ')) + '</option>';
          }).join('') + '</select></div>' +
          '<div class="form-field span-2"><label>Title</label><input type="text" name="title" value="' + escapeHTML(current.title || '') + '" required></div>' +
          '<div class="form-field span-2"><label>Description</label><textarea name="description" placeholder="Phase description">' + escapeHTML(current.description || '') + '</textarea></div>' +
          '<div class="form-field"><label>Priority</label><input type="number" name="priority" value="' + escapeHTML(String(current.priority || 0)) + '"></div>' +
          '<div class="form-field"><label>Depends On (comma separated IDs)</label><input type="text" name="depends_on" value="' + escapeHTML((current.depends_on || []).join(', ')) + '"></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">' + (mode === 'create' ? 'Create Phase' : 'Save Phase') + '</button></div>' +
      '</form>'
    );
  }

  async function submitPhaseEdit(form) {
    var planID = form.getAttribute('data-plan-id') || state.selectedPlanID;
    var phaseID = form.getAttribute('data-phase-id') || '';
    if (!planID || !phaseID) return;

    var payload = {
      title: readString(form, 'title'),
      description: readString(form, 'description'),
      status: readString(form, 'status') || 'not_started'
    };

    var priority = parseInt(readString(form, 'priority'), 10);
    if (!Number.isNaN(priority)) payload.priority = priority;

    var dependsOn = readString(form, 'depends_on');
    if (dependsOn) {
      payload.depends_on = dependsOn.split(',').map(function (item) { return item.trim(); }).filter(Boolean);
    }

    try {
      await apiCall(apiBase() + '/plans/' + encodeURIComponent(planID) + '/phases/' + encodeURIComponent(phaseID), 'PUT', payload);
      closeModal();
      showToast('Phase updated.', 'success');
      renderPlans();
    } catch (err) {
      showToast('Failed to update phase: ' + errorMessage(err), 'error');
    }
  }

  async function submitPhaseCreate(form) {
    var planID = form.getAttribute('data-plan-id') || state.selectedPlanID;
    if (!planID || !state.cache.planDetail) return;

    var phaseID = slugify(readString(form, 'id') || readString(form, 'title'));
    if (!phaseID) {
      showToast('Phase ID is required.', 'error');
      return;
    }

    var priority = parseInt(readString(form, 'priority'), 10);
    if (Number.isNaN(priority)) priority = 0;

    var dependsOn = readString(form, 'depends_on');
    var nextPhase = {
      id: phaseID,
      title: readString(form, 'title'),
      description: readString(form, 'description'),
      status: readString(form, 'status') || 'not_started',
      priority: priority,
      depends_on: dependsOn ? dependsOn.split(',').map(function (item) { return item.trim(); }).filter(Boolean) : []
    };

    var existingPhases = arrayOrEmpty(state.cache.planDetail.phases).slice();
    existingPhases.push(nextPhase);

    try {
      await apiCall(apiBase() + '/plans/' + encodeURIComponent(planID), 'PUT', {
        phases: existingPhases
      });
      closeModal();
      showToast('Phase created.', 'success');
      renderPlans();
    } catch (err) {
      showToast('Failed to create phase: ' + errorMessage(err), 'error');
    }
  }

  function openDocModal(mode, doc) {
    var current = doc || {};
    var plans = state.cache.plans;

    openModal((mode === 'create' ? 'Create Doc' : 'Edit Doc ' + escapeHTML(current.id || '')), '' +
      '<form data-modal-submit="' + (mode === 'create' ? 'doc-create' : 'doc-edit') + '"' + (mode === 'edit' ? (' data-doc-id="' + escapeHTML(current.id || '') + '"') : '') + '>' +
        '<div class="form-grid">' +
          '<div class="form-field span-2"><label>Title</label><input type="text" name="title" value="' + escapeHTML(current.title || '') + '" required></div>' +
          '<div class="form-field"><label>Plan (optional)</label><select name="plan_id"><option value="">Shared</option>' + plans.map(function (plan) {
            return '<option value="' + escapeHTML(plan.id) + '"' + (plan.id === (current.plan_id || '') ? ' selected' : '') + '>' + escapeHTML(plan.id) + '</option>';
          }).join('') + '</select></div>' +
          '<div class="form-field span-2"><label>Content</label><textarea name="content" placeholder="Markdown content">' + escapeHTML(current.content || '') + '</textarea></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">' + (mode === 'create' ? 'Create Doc' : 'Save Doc') + '</button></div>' +
      '</form>'
    );
  }
  async function submitDocCreate(form) {
    var payload = {
      title: readString(form, 'title'),
      content: readString(form, 'content'),
      plan_id: readString(form, 'plan_id')
    };

    if (!payload.title) {
      showToast('Doc title is required.', 'error');
      return;
    }

    try {
      var created = await apiCall(apiBase() + '/docs', 'POST', payload);
      closeModal();
      if (created && created.id) state.selectedDocID = created.id;
      showToast('Doc created.', 'success');
      renderDocs();
    } catch (err) {
      showToast('Failed to create doc: ' + errorMessage(err), 'error');
    }
  }

  async function submitDocEdit(form) {
    var docID = form.getAttribute('data-doc-id') || '';
    if (!docID) return;

    var payload = {
      title: readString(form, 'title'),
      content: readString(form, 'content'),
      plan_id: readString(form, 'plan_id')
    };

    try {
      await apiCall(apiBase() + '/docs/' + encodeURIComponent(docID), 'PUT', payload);
      closeModal();
      showToast('Doc updated.', 'success');
      renderDocs();
    } catch (err) {
      showToast('Failed to update doc: ' + errorMessage(err), 'error');
    }
  }

  function openProfileModal(mode, profile) {
    var current = profile || {};

    openModal((mode === 'create' ? 'Create Profile' : 'Edit Profile ' + escapeHTML(current.name || '')), '' +
      '<form data-modal-submit="' + (mode === 'create' ? 'profile-create' : 'profile-edit') + '"' + (mode === 'edit' ? (' data-profile-name="' + escapeHTML(current.name || '') + '"') : '') + '>' +
        '<div class="form-grid">' +
          '<div class="form-field"><label>Name</label><input type="text" name="name" value="' + escapeHTML(current.name || '') + '" ' + (mode === 'edit' ? 'disabled' : 'required') + '></div>' +
          '<div class="form-field"><label>Agent</label><input type="text" name="agent" value="' + escapeHTML(current.agent || '') + '" required></div>' +
          '<div class="form-field"><label>Model</label><input type="text" name="model" value="' + escapeHTML(current.model || '') + '"></div>' +
          '<div class="form-field"><label>Reasoning Level</label><input type="text" name="reasoning_level" value="' + escapeHTML(current.reasoning_level || '') + '"></div>' +
          '<div class="form-field"><label>Intelligence (1-10)</label><input type="number" min="1" max="10" name="intelligence" value="' + escapeHTML(String(current.intelligence || 5)) + '"></div>' +
          '<div class="form-field"><label>Max Instances</label><input type="number" min="0" name="max_instances" value="' + escapeHTML(String(current.max_instances || 0)) + '"></div>' +
          '<div class="form-field"><label>Speed</label><input type="text" name="speed" value="' + escapeHTML(current.speed || '') + '"></div>' +
          '<div class="form-field span-2"><label>Description</label><textarea name="description">' + escapeHTML(current.description || '') + '</textarea></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">' + (mode === 'create' ? 'Create Profile' : 'Save Profile') + '</button></div>' +
      '</form>'
    );
  }

  function profilePayloadFromForm(form, fixedName) {
    var intelligence = parseInt(readString(form, 'intelligence'), 10);
    if (Number.isNaN(intelligence)) intelligence = 0;

    var maxInstances = parseInt(readString(form, 'max_instances'), 10);
    if (Number.isNaN(maxInstances)) maxInstances = 0;

    return {
      name: fixedName || readString(form, 'name'),
      agent: readString(form, 'agent'),
      model: readString(form, 'model'),
      reasoning_level: readString(form, 'reasoning_level'),
      intelligence: intelligence,
      description: readString(form, 'description'),
      max_instances: maxInstances,
      speed: readString(form, 'speed')
    };
  }

  async function submitProfileCreate(form) {
    var payload = profilePayloadFromForm(form, '');

    if (!payload.name || !payload.agent) {
      showToast('Profile name and agent are required.', 'error');
      return;
    }

    try {
      await apiCall('/api/config/profiles', 'POST', payload);
      closeModal();
      showToast('Profile created.', 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to create profile: ' + errorMessage(err), 'error');
    }
  }

  async function submitProfileEdit(form) {
    var name = form.getAttribute('data-profile-name') || '';
    if (!name) return;

    var payload = profilePayloadFromForm(form, name);

    try {
      await apiCall('/api/config/profiles/' + encodeURIComponent(name), 'PUT', payload);
      closeModal();
      showToast('Profile updated.', 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to update profile: ' + errorMessage(err), 'error');
    }
  }

  async function deleteProfile(name) {
    if (!name) return;
    if (!window.confirm('Delete profile "' + name + '"?')) return;

    try {
      await apiCall('/api/config/profiles/' + encodeURIComponent(name), 'DELETE');
      showToast('Profile deleted: ' + name, 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to delete profile: ' + errorMessage(err), 'error');
    }
  }

  function openLoopModal(mode, loop) {
    var current = loop || { name: '', steps: [{ profile: '', role: '', turns: 1, instructions: '' }] };
    if (!current.steps || !current.steps.length) {
      current.steps = [{ profile: '', role: '', turns: 1, instructions: '' }];
    }

    openModal((mode === 'create' ? 'Create Loop Definition' : 'Edit Loop ' + escapeHTML(current.name || '')), '' +
      '<form data-modal-submit="' + (mode === 'create' ? 'loop-create' : 'loop-edit') + '"' + (mode === 'edit' ? (' data-loop-name="' + escapeHTML(current.name || '') + '"') : '') + '>' +
        '<div class="form-grid">' +
          '<div class="form-field span-2"><label>Name</label><input type="text" name="name" value="' + escapeHTML(current.name || '') + '" ' + (mode === 'edit' ? 'disabled' : 'required') + '></div>' +
        '</div>' +
        '<div class="card-title-row" style="margin-top:0.7rem"><h4>Steps</h4><button type="button" data-action="add-loop-step">Add Step</button></div>' +
        '<div class="step-list" data-loop-steps>' + current.steps.map(function (step, index) {
          return loopStepCardHTML(step, index);
        }).join('') + '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">' + (mode === 'create' ? 'Create Loop' : 'Save Loop') + '</button></div>' +
      '</form>'
    );
  }

  function loopStepCardHTML(step, index) {
    var safeStep = step || {};

    return '' +
      '<section class="step-card" data-loop-step>' +
        '<div class="step-head"><strong>Step ' + (index + 1) + '</strong><button type="button" data-action="remove-loop-step" class="danger">Remove</button></div>' +
        '<div class="form-grid">' +
          '<div class="form-field"><label>Profile</label><input type="text" name="profile" value="' + escapeHTML(safeStep.profile || '') + '" required></div>' +
          '<div class="form-field"><label>Role</label><input type="text" name="role" value="' + escapeHTML(safeStep.role || '') + '"></div>' +
          '<div class="form-field"><label>Turns</label><input type="number" name="turns" min="1" value="' + escapeHTML(String(safeStep.turns || 1)) + '"></div>' +
          '<div class="form-field span-2"><label>Instructions</label><textarea name="instructions">' + escapeHTML(safeStep.instructions || '') + '</textarea></div>' +
        '</div>' +
      '</section>';
  }

  function addLoopStepCard(step) {
    var list = modalRoot.querySelector('[data-loop-steps]');
    if (!list) return;

    var count = list.querySelectorAll('[data-loop-step]').length;
    list.insertAdjacentHTML('beforeend', loopStepCardHTML(step || { profile: '', role: '', turns: 1, instructions: '' }, count));
    refreshLoopStepIndices(list);
  }

  function removeLoopStepCard(actionNode) {
    var card = actionNode.closest('[data-loop-step]');
    var list = modalRoot.querySelector('[data-loop-steps]');
    if (!card || !list) return;

    var cards = list.querySelectorAll('[data-loop-step]');
    if (cards.length <= 1) {
      showToast('Loop must have at least one step.', 'error');
      return;
    }

    card.remove();
    refreshLoopStepIndices(list);
  }

  function refreshLoopStepIndices(list) {
    if (!list) return;
    list.querySelectorAll('[data-loop-step]').forEach(function (node, index) {
      var head = node.querySelector('.step-head strong');
      if (head) head.textContent = 'Step ' + (index + 1);
    });
  }

  function loopPayloadFromForm(form, fixedName) {
    var steps = [];

    form.querySelectorAll('[data-loop-step]').forEach(function (stepNode) {
      var stepProfile = readString(stepNode, 'profile');
      var stepRole = readString(stepNode, 'role');
      var turns = parseInt(readString(stepNode, 'turns'), 10);
      var instructions = readString(stepNode, 'instructions');

      if (!stepProfile) return;

      var step = {
        profile: stepProfile,
        role: stepRole,
        turns: Number.isNaN(turns) || turns <= 0 ? 1 : turns,
        instructions: instructions
      };
      steps.push(step);
    });

    return {
      name: fixedName || readString(form, 'name'),
      steps: steps
    };
  }

  async function submitLoopCreate(form) {
    var payload = loopPayloadFromForm(form, '');

    if (!payload.name || !payload.steps.length) {
      showToast('Loop name and at least one step are required.', 'error');
      return;
    }

    try {
      await apiCall('/api/config/loops', 'POST', payload);
      closeModal();
      showToast('Loop definition created.', 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to create loop: ' + errorMessage(err), 'error');
    }
  }

  async function submitLoopEdit(form) {
    var name = form.getAttribute('data-loop-name') || '';
    if (!name) return;

    var payload = loopPayloadFromForm(form, name);

    try {
      await apiCall('/api/config/loops/' + encodeURIComponent(name), 'PUT', payload);
      closeModal();
      showToast('Loop definition updated.', 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to update loop: ' + errorMessage(err), 'error');
    }
  }

  async function deleteLoop(name) {
    if (!name) return;
    if (!window.confirm('Delete loop definition "' + name + '"?')) return;

    try {
      await apiCall('/api/config/loops/' + encodeURIComponent(name), 'DELETE');
      showToast('Loop deleted: ' + name, 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to delete loop: ' + errorMessage(err), 'error');
    }
  }

  function openRoleModal() {
    openModal('Create Role', '' +
      '<form data-modal-submit="role-create">' +
        '<div class="form-grid">' +
          '<div class="form-field span-2"><label>Name</label><input type="text" name="name" required></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">Create Role</button></div>' +
      '</form>'
    );
  }

  async function submitRoleCreate(form) {
    var name = readString(form, 'name');
    if (!name) {
      showToast('Role name is required.', 'error');
      return;
    }

    try {
      await apiCall('/api/config/roles', 'POST', { name: name });
      closeModal();
      showToast('Role created.', 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to create role: ' + errorMessage(err), 'error');
    }
  }

  async function deleteRole(name) {
    if (!name) return;
    if (!window.confirm('Delete role "' + name + '"?')) return;

    try {
      await apiCall('/api/config/roles/' + encodeURIComponent(name), 'DELETE');
      showToast('Role deleted: ' + name, 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to delete role: ' + errorMessage(err), 'error');
    }
  }

  function openRuleModal() {
    openModal('Create Rule', '' +
      '<form data-modal-submit="rule-create">' +
        '<div class="form-grid">' +
          '<div class="form-field"><label>Rule ID</label><input type="text" name="id" required></div>' +
          '<div class="form-field span-2"><label>Body</label><textarea name="body" required></textarea></div>' +
        '</div>' +
        '<div class="form-actions"><button type="button" data-action="close-modal">Cancel</button><button type="submit" class="success">Create Rule</button></div>' +
      '</form>'
    );
  }

  async function submitRuleCreate(form) {
    var id = readString(form, 'id');
    var body = readString(form, 'body');

    if (!id || !body) {
      showToast('Rule id and body are required.', 'error');
      return;
    }

    try {
      await apiCall('/api/config/rules', 'POST', { id: id, body: body });
      closeModal();
      showToast('Rule created.', 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to create rule: ' + errorMessage(err), 'error');
    }
  }

  async function deleteRule(id) {
    if (!id) return;
    if (!window.confirm('Delete rule "' + id + '"?')) return;

    try {
      await apiCall('/api/config/rules/' + encodeURIComponent(id), 'DELETE');
      showToast('Rule deleted: ' + id, 'success');
      renderConfig();
    } catch (err) {
      showToast('Failed to delete rule: ' + errorMessage(err), 'error');
    }
  }

  function openModal(title, bodyHTML) {
    state.modal = {
      title: title || 'Form',
      bodyHTML: bodyHTML || ''
    };
    renderModal();
  }

  function closeModal() {
    state.modal = null;
    renderModal();
  }

  function renderModal() {
    if (!state.modal) {
      modalRoot.innerHTML = '';
      return;
    }

    modalRoot.innerHTML = '' +
      '<div class="modal-backdrop" data-action="close-modal"></div>' +
      '<section class="modal" role="dialog" aria-modal="true" aria-label="' + escapeHTML(state.modal.title) + '">' +
        '<header class="modal-header">' +
          '<h3>' + escapeHTML(state.modal.title) + '</h3>' +
          '<button class="modal-close" data-action="close-modal" aria-label="Close">×</button>' +
        '</header>' +
        '<div class="modal-body">' + state.modal.bodyHTML + '</div>' +
      '</section>';
  }

  function showAuthPrompt() {
    content.innerHTML = '' +
      '<section class="card">' +
        '<h2>Authentication Required</h2>' +
        '<p class="meta">Enter the auth token to access ADAF API endpoints.</p>' +
        '<form class="inline-form" data-form="auth">' +
          '<input type="text" name="auth_token" placeholder="Auth token" autocomplete="off" required>' +
          '<button type="submit" class="success">Connect</button>' +
        '</form>' +
      '</section>';
  }

  function showToast(message, type) {
    if (!toastRoot) return;

    var node = document.createElement('div');
    node.className = 'toast ' + (type === 'error' ? 'error' : 'success');
    node.textContent = message || '';

    toastRoot.appendChild(node);

    window.setTimeout(function () {
      if (node.parentNode) node.parentNode.removeChild(node);
    }, 4200);
  }
  function setLoading(active) {
    if (active) state.loadingCount += 1;
    else state.loadingCount = Math.max(0, state.loadingCount - 1);

    if (loadingNode) {
      loadingNode.classList.toggle('hidden', state.loadingCount === 0);
      loadingNode.setAttribute('aria-hidden', state.loadingCount === 0 ? 'true' : 'false');
    }
  }

  async function apiCall(path, method, body, options) {
    setLoading(true);

    try {
      var headers = { Accept: 'application/json' };
      if (state.authToken) headers.Authorization = 'Bearer ' + state.authToken;

      var opts = {
        method: method || 'GET',
        headers: headers
      };

      if (body != null) {
        headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(body);
      }

      var response = await fetch(path, opts);

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

      if (response.status === 204) return null;
      if (options && options.allow404 && response.status === 404) return null;

      var message = response.status + ' ' + response.statusText;
      try {
        var payload = await response.json();
        if (payload && payload.error) message = payload.error;
      } catch (_) {
        // Ignore non-JSON payload.
      }

      throw new Error(message);
    } finally {
      setLoading(false);
    }
  }

  function findActivePlan(project, plans) {
    var list = arrayOrEmpty(plans);

    if (project && project.active_plan_id) {
      var configured = list.find(function (plan) {
        return plan.id === project.active_plan_id;
      });
      if (configured) return configured;
    }

    return list.find(function (plan) { return normalizeStatus(plan.status) === 'active'; }) || list[0] || null;
  }

  function summarizePlanPhases(plan) {
    var out = {
      total: 0,
      complete: 0,
      in_progress: 0,
      not_started: 0,
      blocked: 0
    };

    if (!plan || !Array.isArray(plan.phases)) return out;

    plan.phases.forEach(function (phase) {
      out.total += 1;
      var key = normalizeStatus(phase.status || 'not_started');
      if (Object.prototype.hasOwnProperty.call(out, key)) out[key] += 1;
    });

    return out;
  }

  function findPhase(plan, phaseID) {
    if (!plan || !Array.isArray(plan.phases)) return null;
    return plan.phases.find(function (phase) {
      return phase.id === phaseID;
    }) || null;
  }

  function buildWSURL(path) {
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    var url = proto + '//' + window.location.host + path;

    if (state.authToken) {
      url += (path.indexOf('?') >= 0 ? '&' : '?') + 'token=' + encodeURIComponent(state.authToken);
    }

    return url;
  }

  function createPill(status, label) {
    var normalized = normalizeStatus(status);
    return '<span class="pill status-' + normalized + '"><span class="dot"></span>' + escapeHTML(label == null ? status : label) + '</span>';
  }

  var markedRenderer = (function () {
    if (typeof marked === 'undefined') return null;

    marked.setOptions({
      breaks: true,
      gfm: true,
      highlight: function (code, lang) {
        if (typeof hljs !== 'undefined' && lang && hljs.getLanguage(lang)) {
          try { return hljs.highlight(code, { language: lang }).value; } catch (_) {}
        }
        if (typeof hljs !== 'undefined') {
          try { return hljs.highlightAuto(code).value; } catch (_) {}
        }
        return escapeHTML(code);
      }
    });

    var renderer = new marked.Renderer();

    renderer.link = function (href, title, text) {
      var url = sanitizeURL(typeof href === 'object' ? href.href : href);
      var linkText = typeof href === 'object' ? href.text : text;
      return '<a href="' + escapeHTML(url) + '" target="_blank" rel="noopener noreferrer">' + (linkText || url) + '</a>';
    };

    renderer.code = function (code, infostring) {
      var text = typeof code === 'object' ? code.text : code;
      var lang = typeof code === 'object' ? code.lang : infostring;
      lang = String(lang || '').trim();

      var highlighted;
      if (typeof hljs !== 'undefined' && lang && hljs.getLanguage(lang)) {
        try { highlighted = hljs.highlight(text, { language: lang }).value; } catch (_) { highlighted = escapeHTML(text); }
      } else if (typeof hljs !== 'undefined') {
        try { highlighted = hljs.highlightAuto(text).value; } catch (_) { highlighted = escapeHTML(text); }
      } else {
        highlighted = escapeHTML(text);
      }

      var langLabel = lang ? '<span class="code-lang-label">' + escapeHTML(lang) + '</span>' : '';
      return '<div class="code-block-wrapper">' + langLabel + '<pre class="hljs"><code>' + highlighted + '</code></pre></div>';
    };

    return renderer;
  })();

  function renderMarkdown(text) {
    var source = String(text == null ? '' : text);
    if (!source.trim()) return '';

    if (markedRenderer) {
      try {
        return marked.parse(source, { renderer: markedRenderer });
      } catch (_) {}
    }

    return '<p>' + escapeHTML(source) + '</p>';
  }

  function sanitizeURL(raw) {
    var url = String(raw || '').trim();
    if (/^(https?:|mailto:)/i.test(url)) return url;
    return '#';
  }

  function highlightJSON(value) {
    var json;
    try {
      json = JSON.stringify(value == null ? null : value, null, 2);
    } catch (_) {
      json = String(value);
    }

    var safe = escapeHTML(json);

    safe = safe.replace(/(&quot;[^&]*&quot;)(\s*:)/g, '<span class="json-key">$1</span>$2');
    safe = safe.replace(/(:\s*)(&quot;[^&]*&quot;)/g, '$1<span class="json-string">$2</span>');
    safe = safe.replace(/(:\s*)(-?\d+(?:\.\d+)?)/g, '$1<span class="json-number">$2</span>');
    safe = safe.replace(/\b(true|false)\b/g, '<span class="json-bool">$1</span>');
    safe = safe.replace(/\bnull\b/g, '<span class="json-null">null</span>');

    return safe;
  }

  function extractContentBlocks(event) {
    if (!event || typeof event !== 'object') return [];

    var blocks = [];

    if (event.message && Array.isArray(event.message.content)) {
      blocks = event.message.content.slice();
    } else if (event.content_block && typeof event.content_block === 'object') {
      blocks = [event.content_block];
    }

    return blocks;
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

  function rawPayloadText(data) {
    if (typeof data === 'string') return data;
    if (data && typeof data.data === 'string') return data.data;
    return safeJSONString(data);
  }

  function cropText(input) {
    var text = String(input || '');
    if (text.length <= MAX_OUTPUT_CHARS) return text;
    return text.slice(text.length - MAX_OUTPUT_CHARS);
  }

  function formatRelativeTime(iso) {
    if (!iso) return 'unknown';
    var then = new Date(iso);
    if (Number.isNaN(then.getTime())) return String(iso);

    var diff = Math.abs(Date.now() - then.getTime());
    var minute = 60 * 1000;
    var hour = 60 * minute;
    var day = 24 * hour;

    if (diff < minute) return 'just now';
    if (diff < hour) return Math.round(diff / minute) + 'm ago';
    if (diff < day) return Math.round(diff / hour) + 'h ago';
    return Math.round(diff / day) + 'd ago';
  }

  function formatAbsolute(iso) {
    if (!iso) return 'unknown';
    var then = new Date(iso);
    if (Number.isNaN(then.getTime())) return String(iso);
    return then.toLocaleString();
  }

  function formatClock(timestamp) {
    if (!timestamp) return '--:--:--';
    var date = new Date(timestamp);
    if (Number.isNaN(date.getTime())) return '--:--:--';
    return date.toLocaleTimeString();
  }

  function formatNumber(value) {
    var num = Number(value || 0);
    if (!Number.isFinite(num)) return '0';
    return num.toLocaleString();
  }

  function capitalize(text) {
    var value = String(text || '');
    if (!value) return '';
    return value.charAt(0).toUpperCase() + value.slice(1);
  }

  function slugify(raw) {
    return String(raw || '')
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');
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

    var node;
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
