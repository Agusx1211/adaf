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

test('standalone chat can be created with profile, team, and skills', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const primaryProfile = uniqueName('team-chat-profile');
  const delegatedProfile = uniqueName('delegated-profile');
  const teamName = uniqueName('chat-team');
  const skillID = uniqueName('chat-skill').replace(/-/g, '_');

  const createPrimary = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: primaryProfile, agent: 'generic' },
  });
  expect(createPrimary.status()).toBe(201);

  const createDelegated = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: delegatedProfile, agent: 'generic' },
  });
  expect(createDelegated.status()).toBe(201);

  const createTeam = await request.post(`${state.baseURL}/api/config/teams`, {
    data: {
      name: teamName,
      description: 'e2e standalone team',
      delegation: {
        profiles: [
          { name: delegatedProfile, role: 'developer', max_instances: 1 },
        ],
        max_parallel: 1,
      },
    },
  });
  expect(createTeam.status()).toBe(201);

  const createSkill = await request.post(`${state.baseURL}/api/config/skills`, {
    data: { id: skillID, short: 'Skill for standalone creation flow' },
  });
  expect(createSkill.status()).toBe(201);

  const beforeResponse = await request.get(`${projectBaseURL(state, fixture.id)}/chat-instances`);
  expect(beforeResponse.status()).toBe(200);
  const beforeChats = await beforeResponse.json();
  const beforeCount = Array.isArray(beforeChats) ? beforeChats.length : 0;

  await page.getByRole('button', { name: 'Standalone' }).click();
  await page.getByRole('button', { name: '+ New Chat' }).first().click();

  const modal = page.getByRole('dialog', { name: 'New Chat' });
  await expect(modal).toBeVisible();

  const selects = modal.locator('select');
  await selects.nth(0).selectOption(primaryProfile);
  await selects.nth(1).selectOption(teamName);
  await modal.getByText(skillID, { exact: true }).first().click();
  await modal.getByRole('button', { name: 'Create' }).click();

  const afterResponse = await request.get(`${projectBaseURL(state, fixture.id)}/chat-instances`);
  expect(afterResponse.status()).toBe(200);
  const afterChats = await afterResponse.json();
  expect(Array.isArray(afterChats)).toBe(true);
  expect(afterChats.length).toBe(beforeCount + 1);

  const created = afterChats.find((chat) => (
    String(chat.profile || '') === primaryProfile
    && String(chat.team || '') === teamName
    && Array.isArray(chat.skills)
    && chat.skills.includes(skillID)
  ));
  expect(created).toBeTruthy();

  const deleteChat = await request.delete(`${projectBaseURL(state, fixture.id)}/chat-instances/${encodeURIComponent(created.id)}`);
  expect(deleteChat.status()).toBe(200);

  await request.delete(`${state.baseURL}/api/config/skills/${encodeURIComponent(skillID)}`);
  await request.delete(`${state.baseURL}/api/config/teams/${encodeURIComponent(teamName)}`);
  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(primaryProfile)}`);
  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(delegatedProfile)}`);
});

test('issues board modal edit and comments persist to backend store', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const nextTitle = uniqueName('issue-title');
  const nextDescription = `Updated description ${nextTitle}`;
  const nextComment = `Comment ${nextTitle}`;

  await page.getByRole('button', { name: 'Issues' }).click();
  await page.getByRole('button', { name: /Seed issue/ }).first().click();

  const modal = page.getByRole('dialog', { name: 'Issue #1' });
  await expect(modal).toBeVisible();

  await modal.locator('input').nth(0).fill(nextTitle);
  await modal.locator('select').nth(0).selectOption('closed');
  await modal.locator('select').nth(1).selectOption('high');
  await modal.locator('textarea').first().fill(nextDescription);

  await modal.getByRole('button', { name: 'Save Changes' }).click();
  await expect(page.getByText('Issue updated')).toBeVisible();

  await modal.locator('textarea').nth(1).fill(nextComment);
  await modal.getByRole('button', { name: 'Post Comment' }).click();
  await expect(page.getByText('Comment added')).toBeVisible();

  const issueResponse = await request.get(`${projectBaseURL(state, fixture.id)}/issues/1`);
  expect(issueResponse.status()).toBe(200);
  const issue = await issueResponse.json();
  expect(issue.title).toBe(nextTitle);
  expect(issue.status).toBe('closed');
  expect(issue.priority).toBe('high');
  expect(issue.description).toBe(nextDescription);
  expect(Array.isArray(issue.comments)).toBe(true);
  expect(issue.comments.length).toBeGreaterThan(0);
  expect(issue.comments[issue.comments.length - 1].body).toBe(nextComment);
  expect(Array.isArray(issue.history)).toBe(true);
  expect(issue.history.some((entry) => String(entry.type || '') === 'commented')).toBe(true);
});

test('wiki detail edit persists to backend store', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const nextTitle = uniqueName('wiki-title');
  const nextContent = `# ${nextTitle}\n\nUpdated by real e2e wiki test.`;

  await page.getByRole('button', { name: 'Wiki' }).click();
  await page.getByText('Seed Wiki', { exact: true }).click();

  const panel = page.locator('div').filter({ hasText: 'Wiki' }).first();
  await panel.getByRole('button', { name: 'Edit' }).click();
  await panel.getByPlaceholder('Wiki title').fill(nextTitle);
  await panel.locator('textarea').first().fill(nextContent);
  await panel.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Wiki entry updated')).toBeVisible();

  const wikiResponse = await request.get(`${projectBaseURL(state, fixture.id)}/wiki/seed-wiki`);
  expect(wikiResponse.status()).toBe(200);
  const wiki = await wikiResponse.json();
  expect(wiki.title).toBe(nextTitle);
  expect(wiki.content).toBe(nextContent);
});

test('wiki search API returns matching entries', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const searchTag = uniqueName('wiki-search');
  const base = projectBaseURL(state, fixture.id);

  // Create a wiki entry with a unique tag in both title and content.
  const createRes = await request.post(`${base}/wiki`, {
    data: { title: `Search Test ${searchTag}`, content: `Content about ${searchTag} architecture.` },
  });
  expect(createRes.status()).toBe(201);
  const created = await createRes.json();

  // Search for the unique tag — should find the entry.
  const searchRes = await request.get(`${base}/wiki/search?q=${encodeURIComponent(searchTag)}`);
  expect(searchRes.status()).toBe(200);
  const results = await searchRes.json();
  expect(Array.isArray(results)).toBe(true);

  const found = results.find((entry) => entry.id === created.id);
  expect(found).toBeTruthy();
  expect(found.title).toContain(searchTag);

  // Cleanup.
  await request.delete(`${base}/wiki/${encodeURIComponent(created.id)}`);
});

test('wiki create and delete roundtrip via API', async ({ page, request }) => {
  const { state, fixture } = await gotoFixture(page, request);
  const base = projectBaseURL(state, fixture.id);
  const wikiTitle = uniqueName('wiki-roundtrip');

  // List wiki entries before creation.
  const listBefore = await request.get(`${base}/wiki`);
  expect(listBefore.status()).toBe(200);
  const before = await listBefore.json();
  const beforeCount = Array.isArray(before) ? before.length : 0;

  // Create a new wiki entry.
  const createRes = await request.post(`${base}/wiki`, {
    data: { title: wikiTitle, content: `Content for ${wikiTitle}` },
  });
  expect(createRes.status()).toBe(201);
  const created = await createRes.json();
  expect(created.id).toBeTruthy();
  expect(created.title).toBe(wikiTitle);

  // Verify it appears in the list.
  const listAfterCreate = await request.get(`${base}/wiki`);
  expect(listAfterCreate.status()).toBe(200);
  const afterCreate = await listAfterCreate.json();
  expect(Array.isArray(afterCreate)).toBe(true);
  expect(afterCreate.length).toBe(beforeCount + 1);

  const inList = afterCreate.find((entry) => entry.id === created.id);
  expect(inList).toBeTruthy();

  // Delete the entry.
  const deleteRes = await request.delete(`${base}/wiki/${encodeURIComponent(created.id)}`);
  expect(deleteRes.status()).toBe(200);

  // Verify it's gone.
  const listAfterDelete = await request.get(`${base}/wiki`);
  expect(listAfterDelete.status()).toBe(200);
  const afterDelete = await listAfterDelete.json();
  expect(Array.isArray(afterDelete)).toBe(true);
  expect(afterDelete.length).toBe(beforeCount);
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
