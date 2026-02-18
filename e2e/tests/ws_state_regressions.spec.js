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

function installReconnectDropSocket(page, targetSessionID, firstEnvelope, secondEnvelope) {
  return page.addInitScript(({ targetID, first, second }) => {
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

    var firstDelivered = false;
    var firstClosed = false;
    var secondDelivered = false;
    var activeSessionSocket = null;
    window.__reconnectConnectCount = 0;
    var targetPath = '/ws/sessions/' + encodeURIComponent(String(targetID || ''));
    function FakeWebSocket(url) {
      this.url = String(url || '');
      this.readyState = FakeWebSocket.CONNECTING;
      this._listeners = { open: [], message: [], error: [], close: [] };
      this._isTargetSession = this.url.indexOf(targetPath) >= 0;
      if (this._isTargetSession) {
        window.__reconnectConnectCount += 1;
      }

      var self = this;
      setTimeout(function () {
        if (self.readyState === FakeWebSocket.CLOSED) return;
        self.readyState = FakeWebSocket.OPEN;
        emit(self, 'open', {});
        if (!self._isTargetSession) return;
        activeSessionSocket = self;

        if (!firstDelivered) {
          firstDelivered = true;
          if (first) {
            emit(self, 'message', { data: JSON.stringify(first) });
          }
          setTimeout(function () {
            var target = activeSessionSocket || self;
            if (!target || target.readyState !== FakeWebSocket.OPEN) return;
            firstClosed = true;
            target.readyState = FakeWebSocket.CLOSED;
            emit(target, 'close', {});
            if (activeSessionSocket === target) activeSessionSocket = null;
          }, 1200);
          return;
        }

        if (firstClosed && !secondDelivered && second) {
          secondDelivered = true;
          setTimeout(function () {
            if (self.readyState !== FakeWebSocket.OPEN) return;
            emit(self, 'message', { data: JSON.stringify(second) });
          }, 40);
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
      if (this._isTargetSession && activeSessionSocket === this) activeSessionSocket = null;
      emit(this, 'close', {});
    };

    window.WebSocket = FakeWebSocket;
  }, { targetID: targetSessionID, first: firstEnvelope || null, second: secondEnvelope || null });
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
    try {
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
    } catch (_) {
      // Ignore route fetch failures triggered by page/context teardown races.
    }
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

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
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

test('mixed historical/live prompt frames with missing turn metadata render once', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var promptToken = 'DUPLICATE_PROMPT_MIXED_TURN_MARKER_59f3c2';
  var historicalPrompt = promptToken + '   \n';
  var livePrompt = promptToken;

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 260,
      envelope: {
        type: 'prompt',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          prompt: livePrompt,
        },
      },
    },
  ]);

  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'prompt',
      data: {
        prompt: historicalPrompt,
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);

  await page.waitForTimeout(900);
  await expect(page.getByText(promptToken, { exact: false })).toHaveCount(1);
});

test('tool_result ANSI color codes render as styled text instead of raw escapes', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var ansiRedToken = 'ANSI_RED_TOKEN_4a6f';
  var ansiBlueToken = 'ANSI_BLUE_TOKEN_9c2d';
  var ansiPayload = '\u001b[31m' + ansiRedToken + '\u001b[0m and \u001b[34m' + ansiBlueToken + '\u001b[0m';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 250,
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
                  type: 'tool_result',
                  name: 'tool_result',
                  content: ansiPayload,
                },
              ],
            },
          },
        },
      },
    },
  ]);

  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
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

  var resultBlock = page.locator('div').filter({ hasText: ansiRedToken }).first();
  await expect(resultBlock).toBeVisible();

  var hasRawEscape = await resultBlock.evaluate(function (el) {
    return String(el.textContent || '').indexOf('\u001b') >= 0;
  });
  expect(hasRawEscape).toBe(false);

  var redSpan = resultBlock.locator('span', { hasText: ansiRedToken }).first();
  await expect(redSpan).toBeVisible();
  var redColor = await redSpan.evaluate(function (el) {
    return window.getComputedStyle(el).color;
  });
  expect(redColor).toBe('rgb(255, 75, 75)');

  var blueSpan = resultBlock.locator('span', { hasText: ansiBlueToken }).first();
  await expect(blueSpan).toBeVisible();
  var blueColor = await blueSpan.evaluate(function (el) {
    return window.getComputedStyle(el).color;
  });
  expect(blueColor).toBe('rgb(91, 206, 252)');
});

test('session websocket reconnects after unexpected close and resumes streaming', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var firstPrompt = 'WS_RECONNECT_FIRST_PROMPT_71d4';
  var secondPrompt = 'WS_RECONNECT_SECOND_PROMPT_c239';

  await disableMainPollingInterval(page);
  await installReconnectDropSocket(
    page,
    WAIT_FIXTURE_SESSION_ID,
    {
      type: 'prompt',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        prompt: firstPrompt,
      },
    },
    {
      type: 'prompt',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        prompt: secondPrompt,
      },
    }
  );
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '' });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);

  await expect(page.getByText(firstPrompt, { exact: false })).toHaveCount(1);
  await page.waitForTimeout(3000);
  var connectCount = await page.evaluate(function () {
    return Number(window.__reconnectConnectCount || 0);
  });
  expect(connectCount).toBeGreaterThanOrEqual(3);
  await expect(page.getByText(secondPrompt, { exact: false })).toHaveCount(1, { timeout: 4000 });
});

test('historical and live assistant text frames with equivalent content render once', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var token = 'DUPLICATE_ASSISTANT_TEXT_2aa5';
  var historicalText = token + '\n';
  var liveText = token;

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 260,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{ type: 'text', text: liveText }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'event',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        event: {
          type: 'assistant',
          message: {
            content: [{ type: 'text', text: historicalText }],
          },
        },
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);

  await page.waitForTimeout(900);
  await expect(page.getByText(token, { exact: false })).toHaveCount(1);
});

test('session history fetch requests a bounded tail window', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var historyPattern = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  var seenURLResolve;
  var seenURLPromise = new Promise(function (resolve) { seenURLResolve = resolve; });

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, []);
  await routeSessionsAsRunning(page, state, fixture);

  await page.route(historyPattern, async (route) => {
    seenURLResolve(route.request().url());
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '' });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);

  var requestedURL = await seenURLPromise;
  expect(requestedURL).toContain('tail=');
});

test('live stream keeps a bounded in-memory window of events', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var totalEvents = 650;
  var firstToken = 'STREAM_CAP_TOKEN_0000';
  var lastToken = 'STREAM_CAP_TOKEN_' + String(totalEvents - 1).padStart(4, '0');
  var wsMessages = [];

  for (var i = 0; i < totalEvents; i++) {
    wsMessages.push({
      delay_ms: 0,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{ type: 'text', text: 'STREAM_CAP_TOKEN_' + String(i).padStart(4, '0') }],
            },
          },
        },
      },
    });
  }

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, wsMessages);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '' });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(lastToken, { exact: false })).toHaveCount(1, { timeout: 5000 });
  await expect(page.getByText(firstToken, { exact: false })).toHaveCount(0);
});

test('historical and live tool_result frames with JSON-equivalent payload render once', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var marker = 'DUP_TOOL_RESULT_JSON_MARKER_91ab';
  var historicalResult = '{\n  "marker": "' + marker + '",\n  "ok": true\n}';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 260,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                name: 'bash',
                content: { marker: marker, ok: true },
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'event',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        event: {
          type: 'user',
          message: {
            content: [{
              type: 'tool_result',
              name: 'bash',
              content: historicalResult,
            }],
          },
        },
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await page.waitForTimeout(900);
  await expect(page.getByText(marker, { exact: false })).toHaveCount(1);
});

test('distinct consecutive tool_result frames are both preserved', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var firstToken = 'TOOL_RESULT_FIRST_TOKEN_5d3c';
  var secondToken = 'TOOL_RESULT_SECOND_TOKEN_6f8a';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 240,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                name: 'bash',
                content: firstToken,
              }],
            },
          },
        },
      },
    },
    {
      delay_ms: 300,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                name: 'bash',
                content: secondToken,
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '' });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);

  await expect(page.getByText(firstToken, { exact: false })).toHaveCount(1, { timeout: 4000 });
  await expect(page.getByText(secondToken, { exact: false })).toHaveCount(1, { timeout: 4000 });
});

test('distinct consecutive tool_use frames are both preserved', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var firstCommand = 'echo TOOL_USE_FIRST_TOKEN_71ad';
  var secondCommand = 'echo TOOL_USE_SECOND_TOKEN_8c2f';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 220,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{
                type: 'tool_use',
                name: 'bash',
                input: { command: firstCommand },
              }],
            },
          },
        },
      },
    },
    {
      delay_ms: 290,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{
                type: 'tool_use',
                name: 'bash',
                input: { command: secondCommand },
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '' });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);

  await expect(page.getByText(firstCommand, { exact: false })).toHaveCount(1, { timeout: 4000 });
  await expect(page.getByText(secondCommand, { exact: false })).toHaveCount(1, { timeout: 4000 });
});

test('identical tool_result frames with different event IDs are both preserved', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var token = 'TOOL_RESULT_IDENTICAL_TOKEN_24af';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 230,
      envelope: {
        type: 'event',
        data: {
          event_id: 'evt-live-result-1',
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                name: 'bash',
                content: token,
              }],
            },
          },
        },
      },
    },
    {
      delay_ms: 300,
      envelope: {
        type: 'event',
        data: {
          event_id: 'evt-live-result-2',
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                name: 'bash',
                content: token,
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'prompt',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        prompt: 'history-anchor-prompt',
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(token, { exact: false })).toHaveCount(2, { timeout: 5000 });
});

test('identical tool_use frames with different event IDs are both preserved', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var commandToken = 'echo TOOL_USE_IDENTICAL_TOKEN_47d1';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 220,
      envelope: {
        type: 'event',
        data: {
          event_id: 'evt-live-tool-1',
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{
                type: 'tool_use',
                name: 'bash',
                input: { command: commandToken },
              }],
            },
          },
        },
      },
    },
    {
      delay_ms: 280,
      envelope: {
        type: 'event',
        data: {
          event_id: 'evt-live-tool-2',
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{
                type: 'tool_use',
                name: 'bash',
                input: { command: commandToken },
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'prompt',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        prompt: 'history-anchor-prompt',
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(commandToken, { exact: false })).toHaveCount(2, { timeout: 5000 });
});

test('historical and live frames with same event ID render once', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var token = 'SAME_EVENT_ID_DEDUPE_TOKEN_5ac3';
  var sharedEventID = 'evt-shared-42';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 260,
      envelope: {
        type: 'event',
        data: {
          event_id: sharedEventID,
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                name: 'bash',
                content: token,
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'event',
      data: {
        event_id: sharedEventID,
        turn_id: WAIT_FIXTURE_TURN_ID,
        event: {
          type: 'user',
          message: {
            content: [{
              type: 'tool_result',
              name: 'bash',
              content: token,
            }],
          },
        },
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(token, { exact: false })).toHaveCount(1, { timeout: 5000 });
});

test('identical tool_use text with different tool call IDs are both preserved', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var commandToken = 'echo TOOL_USE_CALL_ID_TOKEN_9b3e';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 260,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{
                type: 'tool_use',
                id: 'tool-live-call-2',
                name: 'bash',
                input: { command: commandToken },
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'event',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        event: {
          type: 'assistant',
          message: {
            content: [{
              type: 'tool_use',
              id: 'tool-hist-call-1',
              name: 'bash',
              input: { command: commandToken },
            }],
          },
        },
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(commandToken, { exact: false })).toHaveCount(2, { timeout: 5000 });
});

test('identical tool_result text with different tool call IDs are both preserved', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var token = 'TOOL_RESULT_CALL_ID_TOKEN_6c4d';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 260,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                tool_use_id: 'tool-live-call-2',
                name: 'bash',
                content: token,
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'event',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        event: {
          type: 'user',
          message: {
            content: [{
              type: 'tool_result',
              tool_use_id: 'tool-hist-call-1',
              name: 'bash',
              content: token,
            }],
          },
        },
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(token, { exact: false })).toHaveCount(2, { timeout: 5000 });
});

test('identical tool_result text with same tool call ID render once', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var token = 'TOOL_RESULT_CALL_ID_DEDUPE_TOKEN_3f1a';
  var sharedToolCallID = 'tool-shared-call-88';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 260,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'user',
            message: {
              content: [{
                type: 'tool_result',
                tool_use_id: sharedToolCallID,
                name: 'bash',
                content: token,
              }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    var historyEvent = JSON.stringify({
      type: 'event',
      data: {
        turn_id: WAIT_FIXTURE_TURN_ID,
        event: {
          type: 'user',
          message: {
            content: [{
              type: 'tool_result',
              tool_use_id: sharedToolCallID,
              name: 'bash',
              content: token,
            }],
          },
        },
      },
    });
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: historyEvent + '\n',
    });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(token, { exact: false })).toHaveCount(1, { timeout: 5000 });
});

test('late prior-turn event does not render after next-turn prompt', async ({ page, request }) => {
  var state = loadState();
  var fixture = await fixtureProject(request, state);
  var promptA = 'ORDER_PROMPT_A_6d2f';
  var promptB = 'ORDER_PROMPT_B_7e31';
  var textA1 = 'ORDER_A1_91bc';
  var textA2Late = 'ORDER_A2_LATE_4fa8';
  var textB1 = 'ORDER_B1_52de';

  await disableMainPollingInterval(page);
  await installFakeSessionWebSocket(page, [
    {
      delay_ms: 120,
      envelope: {
        type: 'prompt',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          prompt: promptA,
        },
      },
    },
    {
      delay_ms: 200,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{ type: 'text', text: textA1 }],
            },
          },
        },
      },
    },
    {
      delay_ms: 260,
      envelope: {
        type: 'prompt',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID + 1,
          prompt: promptB,
        },
      },
    },
    {
      delay_ms: 320,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID,
          event: {
            type: 'assistant',
            message: {
              content: [{ type: 'text', text: textA2Late }],
            },
          },
        },
      },
    },
    {
      delay_ms: 380,
      envelope: {
        type: 'event',
        data: {
          turn_id: WAIT_FIXTURE_TURN_ID + 1,
          event: {
            type: 'assistant',
            message: {
              content: [{ type: 'text', text: textB1 }],
            },
          },
        },
      },
    },
  ]);
  await routeSessionsAsRunning(page, state, fixture);

  var historyURL = `${projectBaseURL(state, fixture.id)}/sessions/${WAIT_FIXTURE_SESSION_ID}/events*`;
  await page.route(historyURL, async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: '' });
  });

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops/session-${WAIT_FIXTURE_SESSION_ID}`);
  await expect(page.getByText(promptA, { exact: false })).toHaveCount(1, { timeout: 5000 });
  await expect(page.getByText(promptB, { exact: false })).toHaveCount(1, { timeout: 5000 });
  await expect(page.getByText(textA1, { exact: false })).toHaveCount(1, { timeout: 5000 });
  await expect(page.getByText(textA2Late, { exact: false })).toHaveCount(1, { timeout: 5000 });
  await expect(page.getByText(textB1, { exact: false })).toHaveCount(1, { timeout: 5000 });

  var promptBBox = await page.getByText(promptB, { exact: false }).first().boundingBox();
  var lateBBox = await page.getByText(textA2Late, { exact: false }).first().boundingBox();
  var b1BBox = await page.getByText(textB1, { exact: false }).first().boundingBox();

  expect(promptBBox).not.toBeNull();
  expect(lateBBox).not.toBeNull();
  expect(b1BBox).not.toBeNull();
  expect(lateBBox.y).toBeLessThan(promptBBox.y);
  expect(promptBBox.y).toBeLessThan(b1BBox.y);
});
