const { expect, test } = require('@playwright/test');
const {
  fixtureProject,
  loadState,
  normalizeExpectedToken,
  projectAppURL,
  projectBaseURL,
} = require('./helpers.js');

async function gotoFixture(page, request) {
  const state = loadState();
  const fixture = await fixtureProject(request, state);
  await page.goto(projectAppURL(state, fixture.id));
  return { state, fixture };
}

async function openReplayLoop(page) {
  const loopRow = page.getByText('fixture-replay', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();
}

test('renders ADAF shell with replay fixtures', async ({ page, request }) => {
  await gotoFixture(page, request);
  await expect(page).toHaveTitle(/running/i);
  await expect(page.locator('#root')).toBeVisible();
  await expect(page.getByText('fixture-replay')).toBeVisible();
});

test('serves web API metadata', async ({ request }) => {
  const state = loadState();
  const response = await request.get(`${state.baseURL}/api/config`);
  expect(response.status()).toBe(200);
  const payload = await response.json();
  expect(payload).toHaveProperty('default_role');
  expect(payload).toHaveProperty('roles');
  expect(payload).toHaveProperty('recent_projects');
});

test('returns JSON 404 for unknown API route', async ({ request }) => {
  const state = loadState();
  const response = await request.get(`${state.baseURL}/api/not-a-real-route`);
  expect(response.status()).toBe(404);
  const payload = await response.json();
  expect(payload).toEqual({ error: 'not found' });
});

test('serves seeded replay sessions through project-scoped API', async ({ request }) => {
  const state = loadState();
  const fixture = await fixtureProject(request, state);
  const response = await request.get(`${projectBaseURL(state, fixture.id)}/sessions`);
  expect(response.status()).toBe(200);
  const sessions = await response.json();
  expect(Array.isArray(sessions)).toBe(true);
  expect(sessions.length).toBe(state.fixtures.length);

  const byProfile = new Map();
  sessions.forEach((session) => {
    byProfile.set(String(session.profile_name || ''), session);
  });

  state.fixtures.forEach((seeded) => {
    const session = byProfile.get(String(seeded.profile));
    expect(session).toBeTruthy();
    expect(session.agent_name).toBe(seeded.provider);
    expect(session.status).toBe('done');
  });
});

test('serves compiled web assets', async ({ request }) => {
  const state = loadState();
  const response = await request.get(`${state.baseURL}/static/app.js`);
  expect(response.status()).toBe(200);
  const contentType = response.headers()['content-type'] || '';
  expect(contentType).toMatch(/(javascript|text\/javascript|application\/javascript|text\/plain)/);
});

test('serves global stylesheet', async ({ request }) => {
  const state = loadState();
  const response = await request.get(`${state.baseURL}/static/style.css`);
  expect(response.status()).toBe(200);
  expect(response.headers()['content-type'] || '').toContain('text/css');
});

test('serves usage endpoint payload shape', async ({ request }) => {
  const state = loadState();
  const response = await request.get(`${state.baseURL}/api/usage`);
  expect(response.status()).toBe(200);
  const payload = await response.json();
  expect(payload).toBeTruthy();
  if (Array.isArray(payload)) {
    return;
  }
  if (payload.snapshots != null) {
    expect(Array.isArray(payload.snapshots)).toBe(true);
  }
  if (payload.errors != null) {
    expect(Array.isArray(payload.errors)).toBe(true);
  }
});

test('usage limits dropdown opens from top bar', async ({ page, request }) => {
  await gotoFixture(page, request);
  await page.getByRole('button', { name: 'Limits' }).click();
  await expect(page.getByText('Usage Limits', { exact: true })).toBeVisible();
});

test('replays captured fixture outputs in the UI', async ({ page, request }) => {
  const { state } = await gotoFixture(page, request);
  await openReplayLoop(page);

  for (const fixture of state.fixtures) {
    await page.getByText(String(fixture.profile), { exact: true }).first().click();
    const expectedOutput = String(fixture.expected_output || '').trim();
    const expectedToken = normalizeExpectedToken(expectedOutput);
    await expect(page.getByText(expectedToken, { exact: false }).first()).toBeVisible({ timeout: 15_000 });
  }
});

test('replay renders prompt, thinking, and tool blocks for codex fixture', async ({ page, request }) => {
  const { state } = await gotoFixture(page, request);
  await openReplayLoop(page);

  const codexFixture = state.fixtures.find((fixture) => fixture.provider === 'codex');
  expect(codexFixture).toBeTruthy();

  await page.getByText(String(codexFixture.profile), { exact: true }).first().click();
  await expect(page.getByText('In the current directory, create a file named fixture_note.txt', { exact: false }).first()).toBeVisible();

  const thinkingToggle = page.getByText('THINKING', { exact: true }).first();
  await expect(thinkingToggle).toBeVisible();
  await thinkingToggle.click();
  await expect(page.getByText('Creating file with exact content', { exact: false }).first()).toBeVisible();

  await expect(page.getByText('Bash', { exact: false }).first()).toBeVisible();
});

test('shows continuation marker when a prompt resumes from a prior turn', async ({ page, request }) => {
  const { state } = await gotoFixture(page, request);
  await openReplayLoop(page);

  const codexFixture = state.fixtures.find((fixture) => fixture.provider === 'codex');
  expect(codexFixture).toBeTruthy();

  await page.getByText(String(codexFixture.profile), { exact: true }).first().click();
  await expect(page.getByText('Continues from turn 1', { exact: false }).first()).toBeVisible();
  await expect(page.getByText('Continue with the next steps after the previous response.', { exact: false }).first()).toBeVisible();
});

test('assistant inspect modal opens for replayed messages', async ({ page, request }) => {
  const { state } = await gotoFixture(page, request);
  await openReplayLoop(page);

  const codexFixture = state.fixtures.find((fixture) => fixture.provider === 'codex');
  expect(codexFixture).toBeTruthy();
  await page.getByText(String(codexFixture.profile), { exact: true }).first().click();

  const inspectButton = page.getByRole('button', { name: 'inspect' }).first();
  await expect(inspectButton).toBeVisible();
  await inspectButton.click();

  const inspector = page.getByRole('dialog', { name: 'Prompt Inspector' });
  await expect(inspector).toBeVisible();
  await expect(inspector.getByText('Structured Events', { exact: false })).toBeVisible();
  await inspector.getByRole('button', { name: 'Close' }).click();
});

test('navigation updates hash and survives reload', async ({ page, request }) => {
  await gotoFixture(page, request);

  await page.getByRole('button', { name: 'Docs' }).click();
  await expect(page).toHaveURL(/#\/docs/);
  await expect(page.getByText('Seed Doc', { exact: true })).toBeVisible();

  await page.reload();
  await expect(page).toHaveURL(/#\/docs/);
  await expect(page.getByText('Seed Doc', { exact: true })).toBeVisible();

  await page.getByRole('button', { name: 'Logs' }).click();
  await expect(page).toHaveURL(/#\/logs/);
  await expect(page.getByText('Seed objective', { exact: false })).toBeVisible();
});

test('boot path has no failed HTTP requests', async ({ page, request }) => {
  const failed = [];
  page.on('requestfailed', (req) => {
    const failure = req.failure();
    failed.push({ url: req.url(), errorText: failure ? failure.errorText : '' });
  });

  await gotoFixture(page, request);
  await page.waitForTimeout(500);
  expect(failed).toEqual([]);
});

test('boot and initial interactions have no uncaught browser exceptions', async ({ page, request }) => {
  const pageErrors = [];
  page.on('pageerror', (err) => {
    pageErrors.push(String(err || ''));
  });

  await gotoFixture(page, request);
  await page.getByRole('button', { name: 'Docs' }).click();
  await page.getByRole('button', { name: 'Plan' }).click();
  await page.getByRole('button', { name: 'Loops' }).click();
  await page.waitForTimeout(300);

  expect(pageErrors).toEqual([]);
});
