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

test('loop-mode started frames do not create ghost running sessions from turn-like IDs', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 1800,
      envelope: {
        type: 'started',
        data: {
          session_id: WAIT_FIXTURE_TURN_ID,
        },
      },
    },
  ]);

  await routeSessionsAsRunning(page, state, fixture);

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText('1 running', { exact: true })).toBeVisible();
  await page.waitForTimeout(2400);

  await expect(page.getByText('1 running', { exact: true })).toBeVisible();
  await expect(page.getByText('2 running', { exact: true })).toHaveCount(0);
});

test('duplicate prompt frames from snapshot recent and live stream render once', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var promptText = 'DUPLICATE_PROMPT_MARKER_3d87b6b0';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 250,
      envelope: {
        type: 'prompt',
        data: {
          session_id: WAIT_FIXTURE_TURN_ID,
          turn_id: WAIT_FIXTURE_TURN_ID,
          prompt: promptText,
        },
      },
    },
    {
      delay_ms: 350,
      envelope: {
        type: 'event',
        data: {
          event: {
            type: 'assistant',
            message: {
              content: [
                { type: 'text', text: 'intermediate assistant output' },
              ],
            },
          },
        },
      },
    },
    {
      delay_ms: 450,
      envelope: {
        type: 'prompt',
        data: {
          session_id: WAIT_FIXTURE_TURN_ID,
          turn_id: WAIT_FIXTURE_TURN_ID,
          prompt: promptText,
        },
      },
    },
  ]);

  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events`;
  await page.route(historyURL, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '' });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops`);

  var loopRow = page.getByText('turn-scope-fixture', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();

  var turnID = page.getByText('#901', { exact: true }).first();
  await expect(turnID).toBeVisible();
  await turnID.click();

  await page.waitForTimeout(900);
  await expect(page.getByText(promptText, { exact: false })).toHaveCount(1);
});
