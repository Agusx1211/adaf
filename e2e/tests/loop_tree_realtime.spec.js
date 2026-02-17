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

async function routeSessionsAsWaiting(page, state, fixture) {
  var sessionsURL = `${projectBaseURL(state, fixture.id)}/sessions`;
  await page.route(sessionsURL, async (route) => {
    var response = await route.fetch();
    var payload = await response.json();
    var sessions = Array.isArray(payload) ? payload : [];
    sessions.forEach(function (session) {
      if (Number(session && session.id) === WAIT_FIXTURE_SESSION_ID) {
        session.status = 'waiting_for_spawns';
        session.ended_at = '';
      }
    });
    await route.fulfill({ response, json: sessions });
  });
}

test('loop tree keeps sibling spawns visible across partial websocket spawn updates', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 700,
      envelope: {
        type: 'spawn',
        data: {
          spawns: [
            {
              id: 100,
              parent_turn_id: WAIT_FIXTURE_TURN_ID,
              child_turn_id: 2001,
              profile: 'worker-a',
              role: 'qa',
              status: 'running',
            },
          ],
        },
      },
    },
    {
      delay_ms: 950,
      envelope: {
        type: 'spawn',
        data: {
          spawns: [
            {
              id: 101,
              parent_turn_id: WAIT_FIXTURE_TURN_ID,
              child_turn_id: 2002,
              profile: 'worker-b',
              role: 'developer',
              status: 'running',
            },
          ],
        },
      },
    },
  ]);

  await routeSessionsAsRunning(page, state, fixture);

  var spawnsURL = `${projectBaseURL(state, fixture.id)}/spawns`;
  await page.route(spawnsURL, async (route) => {
    var now = new Date().toISOString();
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
        {
          id: 101,
          parent_turn_id: WAIT_FIXTURE_TURN_ID,
          parent_daemon_session_id: WAIT_FIXTURE_SESSION_ID,
          profile: 'worker-b',
          role: 'developer',
          status: 'running',
          task: 'Validate stream ordering',
          branch: 'fixture/worker-b',
          started_at: now,
        },
      ]),
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops`);

  var loopRow = page.getByText('turn-scope-fixture', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();

  var turnID = page.getByText('#901', { exact: true }).first();
  await expect(turnID).toBeVisible();
  var turnRow = turnID.locator('xpath=ancestor::div[contains(@style,"padding: 5px 12px 5px 28px")][1]');

  await expect(turnRow).toContainText('2 spawns');
  await page.waitForTimeout(1400);
  await expect(turnRow).toContainText('2 spawns');
});

test('loop tree clears WAITING badge when waiting session receives a WS started event', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);

  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 700,
      envelope: {
        type: 'started',
        data: {
          session_id: WAIT_FIXTURE_SESSION_ID,
        },
      },
    },
  ]);
  await routeSessionsAsWaiting(page, state, fixture);

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops`);

  var loopRow = page.getByText('turn-scope-fixture', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();

  var turnID = page.getByText('#901', { exact: true }).first();
  await expect(turnID).toBeVisible();
  var turnRow = turnID.locator('xpath=ancestor::div[contains(@style,"padding: 5px 12px 5px 28px")][1]');

  await expect(turnRow).toContainText('WAITING');
  await page.waitForTimeout(1100);
  await expect(turnRow).not.toContainText('WAITING');
});
