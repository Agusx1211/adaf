const { expect, test } = require('@playwright/test');
const {
  fixtureProject,
  loadState,
  projectAppURL,
  projectBaseURL,
  uniqueName,
} = require('./helpers.js');

async function gotoFixture(page, request) {
  const state = loadState();
  const fixture = await fixtureProject(request, state);
  await page.goto(projectAppURL(state, fixture.id));
  return { state, fixture };
}

test('standalone chat can be created and deleted with real backend data', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const profileName = uniqueName('standalone-profile');

  const createProfile = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: profileName, agent: 'generic' },
  });
  expect(createProfile.status()).toBe(201);

  const listBefore = await request.get(`${projectBaseURL(state, fixture.id)}/chat-instances`);
  expect(listBefore.status()).toBe(200);
  const beforeChats = await listBefore.json();
  const beforeCount = Array.isArray(beforeChats) ? beforeChats.length : 0;

  await page.getByRole('button', { name: 'Standalone' }).click();
  const newChatButton = page.getByRole('button', { name: '+ New Chat' }).first();
  await expect(newChatButton).toBeVisible();
  await newChatButton.click();

  const newChatModal = page.getByRole('dialog', { name: 'New Chat' });
  await expect(newChatModal).toBeVisible();
  await newChatModal.locator('select').first().selectOption(profileName);
  await newChatModal.getByRole('button', { name: 'Create' }).click();

  await expect(page.getByText('New Chat').first()).toBeVisible();

  const listAfterCreate = await request.get(`${projectBaseURL(state, fixture.id)}/chat-instances`);
  expect(listAfterCreate.status()).toBe(200);
  const afterCreateChats = await listAfterCreate.json();
  expect(Array.isArray(afterCreateChats)).toBe(true);
  expect(afterCreateChats.length).toBe(beforeCount + 1);

  page.once('dialog', (dialog) => dialog.accept());
  await page.locator('button[title="Delete chat"]').first().click();
  await expect(page.getByText('Chat deleted')).toBeVisible();

  const listAfterDelete = await request.get(`${projectBaseURL(state, fixture.id)}/chat-instances`);
  expect(listAfterDelete.status()).toBe(200);
  const afterDeleteChats = await listAfterDelete.json();
  expect(Array.isArray(afterDeleteChats)).toBe(true);
  expect(afterDeleteChats.length).toBe(beforeCount);

  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(profileName)}`);
});

test('issues detail edit persists to backend store', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const nextTitle = uniqueName('issue-title');
  const nextDescription = `Updated description ${nextTitle}`;

  await page.getByRole('button', { name: 'Issues' }).click();
  await page.getByText('Seed issue', { exact: true }).click();

  const panel = page.locator('div').filter({ hasText: 'Issue #1' }).first();
  await panel.getByRole('button', { name: 'Edit' }).click();

  await panel.locator('input').nth(0).fill(nextTitle);
  await panel.locator('select').nth(0).selectOption('resolved');
  await panel.locator('select').nth(1).selectOption('high');
  await panel.locator('textarea').first().fill(nextDescription);

  await panel.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Issue updated')).toBeVisible();

  const issueResponse = await request.get(`${projectBaseURL(state, fixture.id)}/issues/1`);
  expect(issueResponse.status()).toBe(200);
  const issue = await issueResponse.json();
  expect(issue.title).toBe(nextTitle);
  expect(issue.status).toBe('resolved');
  expect(issue.priority).toBe('high');
  expect(issue.description).toBe(nextDescription);
});

test('docs detail edit persists to backend store', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const nextTitle = uniqueName('doc-title');
  const nextContent = `# ${nextTitle}\n\nUpdated by real e2e doc test.`;

  await page.getByRole('button', { name: 'Docs' }).click();
  await page.getByText('Seed Doc', { exact: true }).click();

  const panel = page.locator('div').filter({ hasText: 'Documentation' }).first();
  await panel.getByRole('button', { name: 'Edit' }).click();
  await panel.getByPlaceholder('Doc title').fill(nextTitle);
  await panel.locator('textarea').first().fill(nextContent);
  await panel.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Doc updated')).toBeVisible();

  const docResponse = await request.get(`${projectBaseURL(state, fixture.id)}/docs/seed-doc`);
  expect(docResponse.status()).toBe(200);
  const doc = await docResponse.json();
  expect(doc.title).toBe(nextTitle);
  expect(doc.content).toBe(nextContent);
});

test('plan detail edit persists to backend store', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const nextTitle = uniqueName('plan-title');
  const nextDescription = `Plan description for ${nextTitle}`;

  await page.getByRole('button', { name: 'Plan' }).click();
  await page.getByText('Seed Delivery Plan', { exact: true }).first().click();

  const panel = page.locator('div').filter({ hasText: 'Plan:' }).first();
  await panel.getByRole('button', { name: 'Edit' }).click();

  await panel.getByPlaceholder('Plan title').fill(nextTitle);
  await panel.locator('select').first().selectOption('frozen');
  await panel.locator('textarea').first().fill(nextDescription);

  await panel.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Plan updated')).toBeVisible();

  const planResponse = await request.get(`${projectBaseURL(state, fixture.id)}/plans/seed-plan`);
  expect(planResponse.status()).toBe(200);
  const plan = await planResponse.json();
  expect(plan.title).toBe(nextTitle);
  expect(plan.status).toBe('frozen');
  expect(plan.description).toBe(nextDescription);
});

test('logs detail edit persists to backend store', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const nextObjective = `Objective ${uniqueName('turn')}`;

  await page.getByRole('button', { name: 'Logs' }).click();
  await page.getByText('Seed objective', { exact: false }).first().click();

  const panel = page.locator('div').filter({ hasText: 'Turn #1' }).first();
  await panel.getByRole('button', { name: 'Edit' }).click();

  await panel.locator('select').first().selectOption('complete');
  await panel.locator('input[type="number"]').first().fill('77');
  await panel.locator('textarea').first().fill(nextObjective);

  await panel.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Log entry updated')).toBeVisible();

  const turnResponse = await request.get(`${projectBaseURL(state, fixture.id)}/turns/1`);
  expect(turnResponse.status()).toBe(200);
  const turn = await turnResponse.json();
  expect(turn.build_state).toBe('complete');
  expect(Number(turn.duration_secs)).toBe(77);
  expect(turn.objective).toBe(nextObjective);
});
