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

test('replays captured fixture outputs in the UI', async ({ page, request }) => {
  const { state } = await gotoFixture(page, request);
  const loopRow = page.getByText('fixture-replay', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();

  for (const fixture of state.fixtures) {
    await page.getByText(String(fixture.profile), { exact: true }).first().click();
    const expectedOutput = String(fixture.expected_output || '').trim();
    const expectedToken = normalizeExpectedToken(expectedOutput);
    await expect(page.getByText(expectedToken, { exact: false }).first()).toBeVisible({ timeout: 15_000 });
  }
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
