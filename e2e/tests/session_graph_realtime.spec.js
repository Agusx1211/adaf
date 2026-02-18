const { expect, test } = require('@playwright/test');
const {
  fixtureProject,
  loadState,
  projectAppURL,
  projectBaseURL,
} = require('./helpers.js');

const WAIT_FIXTURE_SESSION_ID = 4301;
const WAIT_FIXTURE_TURN_ID = 901;

function installFakeSessionWebSocket(page, wsMessages) {
  return page.addInitScript(({ queuedMessages }) => {
    function emit(ws, type, event) {
      var list = ws._listeners[type] || [];
      list.slice().forEach(function (handler) {
        try { handler(event); } catch (_) {}
      });
      var propHandler = ws['on' + type];
      if (typeof propHandler === 'function') {
        try { propHandler.call(ws, event); } catch (_) {}
      }
    }

    function FakeWebSocket(url) {
      this.url = String(url || '');
      this.readyState = FakeWebSocket.CONNECTING;
      this._listeners = { open: [], message: [], error: [], close: [] };

      var self = this;
      setTimeout(function () {
        if (self.readyState === FakeWebSocket.CLOSED) return;
        self.readyState = FakeWebSocket.OPEN;
        emit(self, 'open', {});

        if (self.url.indexOf('/ws/sessions/') >= 0) {
          (Array.isArray(queuedMessages) ? queuedMessages : []).forEach(function (item, idx) {
            var envelope = item && item.envelope ? item.envelope : item;
            var delay = Number(item && item.delay_ms);
            if (!Number.isFinite(delay) || delay < 0) delay = 20 + idx * 45;
            setTimeout(function () {
              if (self.readyState !== FakeWebSocket.OPEN) return;
              emit(self, 'message', { data: JSON.stringify(envelope) });
            }, delay);
          });
        }
      }, 0);
    }

    FakeWebSocket.CONNECTING = 0;
    FakeWebSocket.OPEN = 1;
    FakeWebSocket.CLOSING = 2;
    FakeWebSocket.CLOSED = 3;

    FakeWebSocket.prototype.addEventListener = function (type, handler) {
      if (!this._listeners[type]) this._listeners[type] = [];
      this._listeners[type].push(handler);
    };

    FakeWebSocket.prototype.removeEventListener = function (type, handler) {
      if (!this._listeners[type]) return;
      this._listeners[type] = this._listeners[type].filter(function (candidate) {
        return candidate !== handler;
      });
    };

    FakeWebSocket.prototype.send = function () {};

    FakeWebSocket.prototype.close = function () {
      if (this.readyState === FakeWebSocket.CLOSED) return;
      this.readyState = FakeWebSocket.CLOSED;
      emit(this, 'close', {});
    };

    window.WebSocket = FakeWebSocket;
  }, { queuedMessages: wsMessages || [] });
}

function disableMainPollingInterval(page) {
  return page.addInitScript(() => {
    var originalSetInterval = window.setInterval;
    window.setInterval = function (fn, delay) {
      if (Number(delay) === 5000) {
        return originalSetInterval(function () {}, 24 * 60 * 60 * 1000);
      }
      return originalSetInterval(fn, delay);
    };
  });
}

async function routeSessionsAsRunning(page, state, fixture) {
  var sessionsURL = `${projectBaseURL(state, fixture.id)}/sessions`;
  await page.route(sessionsURL, async (route) => {
    var response = await route.fetch();
    var payload = await response.json();
    var sessions = Array.isArray(payload) ? payload : [];
    sessions.forEach(function (session) {
      if (Number(session && session.id) === WAIT_FIXTURE_SESSION_ID) {
        session.status = 'running';
        session.ended_at = '';
      }
    });
    await route.fulfill({ response, json: sessions });
  });
}

async function routeLoopRunAsRunning(page, state, fixture) {
  var loopsURL = `${projectBaseURL(state, fixture.id)}/loops`;
  await page.route(loopsURL, async (route) => {
    var response = await route.fetch();
    var payload = await response.json();
    var runs = Array.isArray(payload) ? payload : [];
    runs.forEach(function (run) {
      if (String(run && run.loop_name || '') !== 'turn-scope-fixture') return;
      run.status = 'running';
      if (!run.started_at) run.started_at = new Date().toISOString();
    });
    await route.fulfill({ response, json: runs });
  });
}

async function routeLoopRunAsRunningWithoutDaemonSession(page, state, fixture) {
  var loopsURL = `${projectBaseURL(state, fixture.id)}/loops`;
  await page.route(loopsURL, async (route) => {
    var response = await route.fetch();
    var payload = await response.json();
    var runs = Array.isArray(payload) ? payload : [];
    runs.forEach(function (run) {
      if (String(run && run.loop_name || '') === 'turn-scope-fixture') {
        run.status = 'running';
        run.daemon_session_id = 0;
        if (!run.started_at) run.started_at = new Date().toISOString();
        return;
      }
      if (String(run && run.status || '').toLowerCase() === 'running') {
        run.status = 'completed';
      }
    });
    await route.fulfill({ response, json: runs });
  });
}

test('loop topology graph updates when websocket reports new sub-agent spawn', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var now = new Date().toISOString();

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 750,
      envelope: {
        type: 'spawn',
        data: {
          spawns: [
            {
              id: 101,
              parent_turn_id: WAIT_FIXTURE_TURN_ID,
              parent_daemon_session_id: WAIT_FIXTURE_SESSION_ID,
              profile: 'worker-b',
              role: 'developer',
              status: 'running',
              task: 'Validate stream ordering',
              started_at: now,
            },
          ],
        },
      },
    },
  ]);

  await routeSessionsAsRunning(page, state, fixture);
  await routeLoopRunAsRunning(page, state, fixture);

  var spawnsURL = `${projectBaseURL(state, fixture.id)}/spawns`;
  await page.route(spawnsURL, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([
        {
          id: 100,
          parent_turn_id: WAIT_FIXTURE_TURN_ID,
          parent_daemon_session_id: WAIT_FIXTURE_SESSION_ID,
          profile: 'worker-a',
          role: 'qa',
          status: 'running',
          task: 'Audit session stability',
          branch: 'fixture/worker-a',
          started_at: now,
        },
      ]),
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);

  var loopRow = page.getByText('turn-scope-fixture', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();

  await page.getByRole('button', { name: /Loop:/ }).first().click();

  await expect(page.getByTestId('session-graph-view')).toBeVisible();
  await expect(page.getByTestId('graph-child-turn-901')).toBeVisible();

  await page.getByTestId('graph-child-turn-901').click();
  await expect(page.getByTestId('graph-child-spawn-100')).toBeVisible();
  await expect(page.getByTestId('graph-child-spawn-101')).toHaveCount(0);

  await page.waitForTimeout(1200);
  var newSpawnNode = page.getByTestId('graph-child-spawn-101');
  await expect(newSpawnNode).toBeVisible();

  await newSpawnNode.click();
  await expect(page.getByText('Validate stream ordering', { exact: false }).first()).toBeVisible();
});

test('loop topology graph renders tool calls even when loop run has no daemon session id', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var now = new Date().toISOString();

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 350,
      envelope: {
        type: 'event',
        data: {
          session_id: WAIT_FIXTURE_TURN_ID,
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [
                {
                  type: 'tool_use',
                  name: 'Bash',
                  input: { command: 'go test ./...' },
                },
              ],
            },
          },
        },
      },
    },
  ]);

  await routeSessionsAsRunning(page, state, fixture);
  await routeLoopRunAsRunningWithoutDaemonSession(page, state, fixture);

  var spawnsURL = `${projectBaseURL(state, fixture.id)}/spawns`;
  await page.route(spawnsURL, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([
        {
          id: 100,
          parent_turn_id: WAIT_FIXTURE_TURN_ID,
          parent_daemon_session_id: WAIT_FIXTURE_SESSION_ID,
          profile: 'worker-a',
          role: 'qa',
          status: 'running',
          task: 'Audit session stability',
          branch: 'fixture/worker-a',
          started_at: now,
        },
      ]),
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops`);

  var loopRow = page.getByText('turn-scope-fixture', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();
  await page.getByText('#4301', { exact: true }).first().click();
  await page.getByRole('button', { name: /Loop:/ }).first().click();

  await expect(page.getByTestId('session-graph-view')).toBeVisible();
  await page.waitForTimeout(900);

  await expect(page.getByTestId('session-graph-event-log')).toContainText(/Bash\s*[→-].*go test \.\/\.\.\./);
});
