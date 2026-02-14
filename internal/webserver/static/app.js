(function () {
  'use strict';
  var REFRESH_MS = 10000;
  var MAX_OUTPUT_CHARS = 120000;
  var state = {
    tab: '',
    dashboardTimer: null,
    selectedSessionID: null,
    selectedPlanID: null,
    issueFilter: 'all',
    ws: null,
    wsSessionID: null,
    term: null,
    termFit: null,
    termWS: null
  };
  var nav = document.getElementById('nav');
  var content = document.getElementById('content');
  function init() {
    if (!nav || !content) return;
    nav.addEventListener('click', function (event) {
      var link = event.target.closest('a[data-tab]');
      if (!link) return;
      event.preventDefault();
      switchTab(link.getAttribute('data-tab'));
    });
    content.addEventListener('click', onContentClick);
    switchTab('dashboard');
  }
  function switchTab(tab) {
    if (!tab) return;
    if (state.tab === tab) return renderCurrentTab();
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
    if (nextTab !== 'plans') state.selectedPlanID = null;
  }
  function updateNav() {
    nav.querySelectorAll('a[data-tab]').forEach(function (link) {
      var active = link.getAttribute('data-tab') === state.tab;
      link.classList.toggle('active', active);
      if (active) link.setAttribute('aria-current', 'page');
      else link.removeAttribute('aria-current');
    });
  }
  function onContentClick(event) {
    var target = event.target.closest('[data-switch-tab],[data-session-id],[data-plan-id],[data-issue-filter],[data-reconnect-session]');
    if (!target) return;
    event.preventDefault();
    if (target.hasAttribute('data-switch-tab')) {
      var nextTab = target.getAttribute('data-switch-tab');
      if (nextTab === 'sessions') {
        var jumpID = parseInt(target.getAttribute('data-session-id') || '', 10);
        if (!Number.isNaN(jumpID)) state.selectedSessionID = jumpID;
      }
      return switchTab(nextTab);
    }
    if (target.hasAttribute('data-session-id') && state.tab === 'sessions') {
      var sessionID = parseInt(target.getAttribute('data-session-id'), 10);
      if (!Number.isNaN(sessionID)) {
        state.selectedSessionID = sessionID;
        return renderSessions();
      }
    }
    if (target.hasAttribute('data-plan-id') && state.tab === 'plans') {
      state.selectedPlanID = target.getAttribute('data-plan-id') || '';
      return renderPlans();
    }
    if (target.hasAttribute('data-issue-filter') && state.tab === 'issues') {
      state.issueFilter = target.getAttribute('data-issue-filter') || 'all';
      return renderIssues();
    }
    if (target.hasAttribute('data-reconnect-session') && state.tab === 'sessions' && state.selectedSessionID) {
      connectSessionSocket(state.selectedSessionID, true);
    }
  }
  function renderCurrentTab() {
    if (state.tab === 'sessions') return renderSessions();
    if (state.tab === 'plans') return renderPlans();
    if (state.tab === 'issues') return renderIssues();
    if (state.tab === 'terminal') return renderTerminal();
    return renderDashboard();
  }
  function renderLoading(label) {
    content.innerHTML = '<section class="card"><p class="meta">Loading ' + escapeHTML(label) + '...</p></section>';
  }
  async function renderDashboard() {
    renderLoading('dashboard');
    try {
      var results = await Promise.all([api('/api/project', true), api('/api/plans'), api('/api/issues?status=open'), api('/api/sessions')]);
      var project = results[0];
      var plans = arrayOrEmpty(results[1]);
      var openIssues = arrayOrEmpty(results[2]);
      var sessions = arrayOrEmpty(results[3]);
      var activePlan = findActivePlan(project, plans);
      var summary = summarizePlanPhases(activePlan);
      content.innerHTML = '<section class="grid">' +
        '<article class="card span-6"><h2>Project</h2><div class="kv"><strong>' + escapeHTML(project && project.name ? project.name : 'Uninitialized') + '</strong></div><p class="meta">Repo: ' + escapeHTML(project && project.repo_path ? project.repo_path : 'N/A') + '</p><p class="meta">Active plan: ' + escapeHTML(activePlan ? activePlan.id : 'none') + '</p></article>' +
        '<article class="card span-6"><h2>Active Plan Summary</h2><div class="filters">' + createPill('complete', summary.complete) + createPill('in_progress', summary.in_progress) + createPill('not_started', summary.not_started) + createPill('blocked', summary.blocked) + '</div><p class="meta">Total phases: ' + summary.total + '</p></article>' +
        '<article class="card span-6"><h2>Open Issues (' + openIssues.length + ')</h2>' + renderIssueList(openIssues.slice(0, 6), true) + '</article>' +
        '<article class="card span-6"><h2>Recent Sessions</h2>' + renderSessionList(sessions.slice(0, 8), true) + '</article>' +
        '</section>';
      if (!state.dashboardTimer && state.tab === 'dashboard') {
        state.dashboardTimer = setInterval(function () {
          if (state.tab === 'dashboard') renderDashboard();
        }, REFRESH_MS);
      }
    } catch (err) {
      renderError('Failed to load dashboard: ' + errorMessage(err));
    }
  }
  async function renderSessions() {
    renderLoading('sessions');
    try {
      var sessions = arrayOrEmpty(await api('/api/sessions'));
      if (!state.selectedSessionID && sessions.length > 0) state.selectedSessionID = sessions[0].id;
      var selected = sessions.find(function (session) { return session.id === state.selectedSessionID; }) || null;
      content.innerHTML = '<section class="grid"><article class="card span-4"><h2>Sessions</h2>' + renderSessionList(sessions, false) + '</article><article class="card span-8">' + renderSessionDetail(selected) + '</article></section>';
      if (selected) connectSessionSocket(selected.id, false);
      else disconnectSessionSocket();
    } catch (err) {
      renderError('Failed to load sessions: ' + errorMessage(err));
    }
  }
  async function renderPlans() {
    renderLoading('plans');
    try {
      var plans = arrayOrEmpty(await api('/api/plans'));
      if (!state.selectedPlanID && plans.length > 0) state.selectedPlanID = plans[0].id;
      var detail = state.selectedPlanID ? await api('/api/plans/' + encodeURIComponent(state.selectedPlanID), true) : null;
      content.innerHTML = '<section class="grid"><article class="card span-4"><h2>Plans</h2>' + renderPlanList(plans) + '</article><article class="card span-8">' + renderPlanDetail(detail) + '</article></section>';
    } catch (err) {
      renderError('Failed to load plans: ' + errorMessage(err));
    }
  }
  async function renderIssues() {
    renderLoading('issues');
    try {
      var path = '/api/issues' + (state.issueFilter !== 'all' ? '?status=' + encodeURIComponent(state.issueFilter) : '');
      var issues = arrayOrEmpty(await api(path));
      content.innerHTML = '<section class="card"><h2>Issues</h2><div class="filters">' + renderFilterButton('all', 'All') + renderFilterButton('open', 'Open') + renderFilterButton('resolved', 'Resolved') + '</div>' + renderIssueList(issues, false) + '</section>';
    } catch (err) {
      renderError('Failed to load issues: ' + errorMessage(err));
    }
  }
  function renderTerminal() {
    if (typeof Terminal === 'undefined' || typeof FitAddon === 'undefined') {
      return renderError('Terminal runtime unavailable. Reload the page and try again.');
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
        // best effort
      }
    }
  }
  function connectTerminalSocket() {
    disconnectTerminal();
    state.termWS = new WebSocket(buildWSURL('/ws/terminal'));
    state.termWS.addEventListener('open', function () {
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
      if (state.term) state.term.write('\r\n[Connection closed]\r\n');
      state.termWS = null;
    });
    state.termWS.addEventListener('error', function () {
      if (state.term) state.term.write('\r\n[Connection error]\r\n');
    });
  }
  function disconnectTerminal() {
    window.removeEventListener('resize', fitTerminal);
    if (state.termWS) {
      try {
        state.termWS.close();
      } catch (_) {
        // best effort
      }
      state.termWS = null;
    }
  }
  function renderSessionList(sessions, compact) {
    if (!sessions.length) return '<p class="empty">No sessions found.</p>';
    return '<ul class="list">' + sessions.map(function (session) {
      var name = '#' + session.id + ' · ' + (session.profile_name || session.agent_name || 'session');
      var action = compact ? '<button data-switch-tab="sessions" data-session-id="' + session.id + '">Open</button>' : '<button data-session-id="' + session.id + '" class="' + (session.id === state.selectedSessionID ? 'active' : '') + '">View</button>';
      return '<li><div class="link-row"><div><strong>' + escapeHTML(name) + '</strong><br><span class="meta">' + escapeHTML(formatTime(session.started_at)) + '</span><br>' + createPill(session.status || 'unknown', session.agent_name || 'agent') + '</div><div>' + action + '</div></div></li>';
    }).join('') + '</ul>';
  }
  function renderSessionDetail(session) {
    if (!session) return '<h2>Session Detail</h2><p class="empty">Select a session to view details.</p>';
    return '<h2>Session #' + session.id + '</h2><p class="meta">' + escapeHTML(session.project_name || 'unknown project') + ' · ' + escapeHTML(session.agent_name || 'unknown agent') + '</p><div class="filters">' + createPill(session.status || 'unknown', 'status') + '<button data-reconnect-session="1">Reconnect Stream</button></div><div id="session-events" class="terminal" role="log" aria-live="polite">Waiting for stream...</div>';
  }
  function renderPlanList(plans) {
    if (!plans.length) return '<p class="empty">No plans found.</p>';
    return '<ul class="list">' + plans.map(function (plan) {
      return '<li><div class="link-row"><div><strong>' + escapeHTML(plan.title || plan.id) + '</strong><br><span class="meta">' + escapeHTML(plan.id) + '</span></div><div>' + createPill(plan.status || 'unknown', 'plan') + '<button data-plan-id="' + escapeHTML(plan.id) + '" class="' + (plan.id === state.selectedPlanID ? 'active' : '') + '">View</button></div></div></li>';
    }).join('') + '</ul>';
  }
  function renderPlanDetail(plan) {
    if (!plan) return '<h2>Plan Detail</h2><p class="empty">Select a plan to view phases.</p>';
    var phases = arrayOrEmpty(plan.phases);
    var phaseList = phases.length ? '<ul class="list">' + phases.map(function (phase) {
      return '<li><div class="link-row"><div><strong>' + escapeHTML(phase.title || phase.id || 'Untitled') + '</strong><br><span class="meta">' + escapeHTML(phase.description || 'No description') + '</span></div><div>' + createPill(phase.status || 'not_started', 'phase') + '</div></div></li>';
    }).join('') + '</ul>' : '<p class="empty">No phases defined.</p>';
    return '<h2>' + escapeHTML(plan.title || plan.id) + '</h2><p class="meta">' + escapeHTML(plan.description || 'No description') + '</p><div class="filters">' + createPill(plan.status || 'active', 'plan') + '<span class="meta">Updated ' + escapeHTML(formatTime(plan.updated)) + '</span></div>' + phaseList;
  }
  function renderIssueList(issues, compact) {
    if (!issues.length) return '<p class="empty">No matching issues.</p>';
    return '<ul class="list">' + issues.map(function (issue) {
      var updated = compact ? '' : '<br><span class="meta">Updated: ' + escapeHTML(formatTime(issue.updated)) + '</span>';
      return '<li><div class="link-row"><div><strong>#' + issue.id + ' ' + escapeHTML(issue.title || 'Untitled') + '</strong><br><span class="meta">Plan: ' + escapeHTML(issue.plan_id || 'none') + '</span>' + updated + '</div><div>' + createPill(issue.priority || 'low', 'priority') + createPill(issue.status || 'open', 'status') + '</div></div></li>';
    }).join('') + '</ul>';
  }
  function renderFilterButton(value, label) {
    return '<button data-issue-filter="' + value + '" class="' + (state.issueFilter === value ? 'active' : '') + '">' + label + '</button>';
  }
  function connectSessionSocket(sessionID, forceReconnect) {
    if (!sessionID) return;
    if (!forceReconnect && state.ws && state.wsSessionID === sessionID && state.ws.readyState <= 1) return;
    disconnectSessionSocket();
    var terminal = document.getElementById('session-events');
    if (terminal) terminal.textContent = 'Connecting to stream...\n';
    state.wsSessionID = sessionID;
    state.ws = new WebSocket(buildWSURL('/ws/sessions/' + encodeURIComponent(String(sessionID))));
    state.ws.addEventListener('open', function () { appendTerminal('[connected] live stream', true); });
    state.ws.addEventListener('message', function (event) { handleSessionMessage(event.data); });
    state.ws.addEventListener('error', function () { appendTerminal('[error] stream error', true); });
    state.ws.addEventListener('close', function () {
      appendTerminal('[closed] stream disconnected', true);
      state.ws = null;
      state.wsSessionID = null;
    });
  }
  function disconnectSessionSocket() {
    if (!state.ws) return;
    try {
      state.ws.close();
    } catch (_) {
      // best effort
    }
    state.ws = null;
    state.wsSessionID = null;
  }
  function handleSessionMessage(raw) {
    var msg;
    try {
      msg = JSON.parse(raw);
    } catch (_) {
      appendTerminal('[raw] ' + String(raw), true);
      return;
    }
    var data = msg.data;
    if (msg.type === 'event') return handleStreamEvent(data);
    if (msg.type === 'raw') return appendTerminal(rawPayloadText(data), false);
    if (msg.type === 'started') return appendTerminal('[started] turn started', true);
    if (msg.type === 'finished') {
      var code = data && typeof data.exit_code === 'number' ? data.exit_code : 0;
      var suffix = data && data.error ? ' error=' + data.error : '';
      return appendTerminal('[finished] exit_code=' + code + suffix, true);
    }
    if (msg.type === 'done') return appendTerminal('[done] session complete', true);
    if (msg.type === 'snapshot') return appendTerminal('[snapshot] status=' + ((data && data.session && data.session.status) || 'unknown'), true);
    if (msg.type === 'prompt') return appendTerminal('[prompt] prompt received', true);
    if (msg.type === 'spawn') return appendTerminal('[spawn] status updated', true);
    if (msg.type === 'loop_step_start') return appendTerminal('[loop] step started', true);
    if (msg.type === 'loop_step_end') return appendTerminal('[loop] step ended', true);
    if (msg.type === 'loop_done') return appendTerminal('[loop] done', true);
    if (msg.type === 'error') return appendTerminal('[error] ' + ((data && data.error) || 'unknown stream error'), true);
    appendTerminal('[' + msg.type + '] ' + safeJSONString(data), true);
  }
  function handleStreamEvent(payload) {
    var event = payload && payload.event ? payload.event : payload;
    if (!event || typeof event !== 'object') {
      appendTerminal('[event] ' + safeJSONString(payload), true);
      return;
    }
    if (event.type === 'assistant') {
      var blocks = event.message && Array.isArray(event.message.content) ? event.message.content : [];
      if (!blocks.length && event.content_block) blocks = [event.content_block];
      if (blocks.length) {
        blocks.forEach(function (block) {
          if (!block || typeof block !== 'object') return;
          if (block.type === 'text' && block.text) appendTerminal(block.text, true);
          if (block.type === 'tool_use') appendTerminal('[tool] ' + (block.name || 'unknown') + ' ' + safeJSONString(block.input || {}), true);
        });
        return;
      }
    }
    if (event.type === 'content_block_delta') {
      var delta = event.delta && (event.delta.text || event.delta.partial_json);
      if (delta) appendTerminal(delta, false);
      return;
    }
    if (event.type === 'result') {
      var parts = [];
      if (event.subtype) parts.push(event.subtype);
      if (typeof event.num_turns === 'number') parts.push('turns=' + event.num_turns);
      if (typeof event.total_cost_usd === 'number' && event.total_cost_usd > 0) parts.push('cost=$' + event.total_cost_usd.toFixed(4));
      appendTerminal('[result] ' + (parts.length ? parts.join(' ') : 'done'), true);
      return;
    }
    appendTerminal('[event:' + (event.type || 'unknown') + '] ' + safeJSONString(event), true);
  }
  function appendTerminal(text, newline) {
    var terminal = document.getElementById('session-events');
    if (!terminal) return;
    var next = terminal.textContent + String(text || '') + (newline ? '\n' : '');
    if (next.length > MAX_OUTPUT_CHARS) next = next.slice(next.length - MAX_OUTPUT_CHARS);
    terminal.textContent = next;
    terminal.scrollTop = terminal.scrollHeight;
  }
  function rawPayloadText(data) {
    if (typeof data === 'string') return data;
    if (data && typeof data.data === 'string') return data.data;
    return safeJSONString(data);
  }
  function createPill(status, label) {
    var normalized = normalizeStatus(status);
    return '<span class="pill status-' + normalized + '"><span class="dot"></span>' + escapeHTML(label == null ? status : label) + '</span>';
  }
  function summarizePlanPhases(plan) {
    var out = { total: 0, complete: 0, in_progress: 0, not_started: 0, blocked: 0 };
    if (!plan || !Array.isArray(plan.phases)) return out;
    plan.phases.forEach(function (phase) {
      out.total += 1;
      var key = normalizeStatus(phase.status);
      if (Object.prototype.hasOwnProperty.call(out, key)) out[key] += 1;
    });
    return out;
  }
  function findActivePlan(project, plans) {
    var list = arrayOrEmpty(plans);
    if (project && project.active_plan_id) {
      var configured = list.find(function (plan) { return plan.id === project.active_plan_id; });
      if (configured) return configured;
    }
    return list.find(function (plan) { return plan.status === 'active'; }) || list[0] || null;
  }
  async function api(path, allow404) {
    var response = await fetch(path, { method: 'GET', headers: { Accept: 'application/json' } });
    if (response.ok) return response.json();
    if (allow404 && response.status === 404) return null;
    var message = response.status + ' ' + response.statusText;
    try {
      var body = await response.json();
      if (body && body.error) message = body.error;
    } catch (_) {
      // ignore parsing errors
    }
    throw new Error(message);
  }
  function renderError(message) {
    content.innerHTML = '<section class="card error-card"><h2>Error</h2><p class="error-text">' + escapeHTML(message) + '</p></section>';
  }
  function formatTime(iso) {
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
  function buildWSURL(path) {
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return proto + '//' + window.location.host + path;
  }
  function escapeHTML(value) {
    return String(value == null ? '' : value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function errorMessage(err) {
    if (!err) return 'unknown error';
    return err.message || String(err);
  }
  function arrayOrEmpty(value) {
    return Array.isArray(value) ? value : [];
  }
  init();
})();
