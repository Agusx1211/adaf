const fs = require('node:fs');
const path = require('node:path');
const { expect, test } = require('@playwright/test');

const STATE_FILE = path.join(__dirname, '..', '.state', 'web-server.json');

function loadState() {
  const raw = fs.readFileSync(STATE_FILE, 'utf8');
  return JSON.parse(raw);
}

test('renders ADAF shell', async ({ page }) => {
  const state = loadState();
  await page.goto(state.baseURL);
  await expect(page).toHaveTitle(/adaf/i);
  await expect(page.locator('#root')).toBeVisible();
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
