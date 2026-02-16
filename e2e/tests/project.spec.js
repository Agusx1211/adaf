const path = require('node:path');
const { expect, test } = require('@playwright/test');
const {
  fixtureProject,
  listProjects,
  loadState,
  projectAppURL,
  projectBaseURL,
  uniqueName,
} = require('./helpers.js');

test('project browser creates, initializes, opens, and switches projects', async ({ page, request }) => {
  const state = loadState();
  const fixture = await fixtureProject(request, state);
  const fixtureFolderName = path.basename(state.fixtureProjectDir);
  const newProjectName = uniqueName('ui-project');
  const newProjectPath = path.join(state.workspaceRoot, newProjectName);

  await page.goto(projectAppURL(state, fixture.id));

  const trigger = page.getByTitle('Browse projects').first();
  await expect(trigger).toContainText('E2E Replay Fixtures');
  await trigger.click();

  const dialog = page.getByRole('dialog', { name: 'File Explorer' });
  await expect(dialog).toBeVisible();

  await dialog.getByRole('button', { name: '+ New Folder' }).click();
  await dialog.getByPlaceholder('Folder name').fill(newProjectName);
  await dialog.getByRole('button', { name: 'Create' }).click();

  await expect(dialog.getByText(newProjectName, { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: 'Init' }).click();
  await expect(dialog.getByRole('button', { name: 'Open' })).toHaveCount(2);
  await dialog.getByRole('button', { name: 'Open' }).nth(1).click();

  await expect(trigger).toContainText(newProjectName);

  const projects = await listProjects(request, state);
  const openedProject = projects.find((project) => path.resolve(String(project.path || '')) === path.resolve(newProjectPath));
  expect(openedProject).toBeTruthy();

  const currentURL = new URL(page.url());
  expect(currentURL.searchParams.get('project')).toBe(String(openedProject.id));
  await expect.poll(async () => page.evaluate(() => localStorage.getItem('adaf_project_id'))).toBe(String(openedProject.id));

  const dashboardResponse = await request.get(`${state.baseURL}/api/projects/dashboard`);
  expect(dashboardResponse.status()).toBe(200);
  const dashboard = await dashboardResponse.json();
  const listed = (dashboard.projects || []).find((project) => String(project.id || '') === String(openedProject.id));
  expect(listed).toBeTruthy();

  await trigger.click();
  const dialogAgain = page.getByRole('dialog', { name: 'File Explorer' });
  await expect(dialogAgain.getByText(fixtureFolderName, { exact: true })).toBeVisible();
  await expect(dialogAgain.getByRole('button', { name: 'Open' })).toHaveCount(2);
  await dialogAgain.getByRole('button', { name: 'Open' }).nth(0).click();

  await expect(trigger).toContainText('E2E Replay Fixtures');
  await expect.poll(async () => page.evaluate(() => localStorage.getItem('adaf_project_id'))).toBe(String(fixture.id));
});

test('filesystem browse and mkdir endpoints operate inside allowed root', async ({ request }) => {
  const state = loadState();
  const folderName = uniqueName('mkdir');
  const folderPath = path.join(state.workspaceRoot, folderName);

  const browseRoot = await request.get(`${state.baseURL}/api/fs/browse`);
  expect(browseRoot.status()).toBe(200);
  const rootPayload = await browseRoot.json();
  expect(path.resolve(rootPayload.path)).toBe(path.resolve(state.workspaceRoot));

  const createFolder = await request.post(`${state.baseURL}/api/fs/mkdir`, {
    data: { path: folderPath },
  });
  expect(createFolder.status()).toBe(200);

  const browseAfter = await request.get(`${state.baseURL}/api/fs/browse?path=${encodeURIComponent(state.workspaceRoot)}`);
  expect(browseAfter.status()).toBe(200);
  const afterPayload = await browseAfter.json();
  const names = (afterPayload.entries || []).map((entry) => String(entry.name || ''));
  expect(names).toContain(folderName);
});

test('project browser supports nested navigation and parent up action', async ({ page, request }) => {
  const state = loadState();
  const fixture = await fixtureProject(request, state);
  const parentDir = uniqueName('nav-parent');
  const childDir = uniqueName('nav-child');
  const nestedPath = path.join(state.workspaceRoot, parentDir, childDir);

  const mkdirResponse = await request.post(`${state.baseURL}/api/fs/mkdir`, {
    data: { path: nestedPath },
  });
  expect(mkdirResponse.status()).toBe(200);

  await page.goto(projectAppURL(state, fixture.id));
  await page.getByTitle('Browse projects').first().click();

  const dialog = page.getByRole('dialog', { name: 'File Explorer' });
  await expect(dialog).toBeVisible();

  await dialog.getByText(parentDir, { exact: true }).first().click();
  await expect(dialog.getByText(childDir, { exact: true }).first()).toBeVisible();

  const upButton = dialog.getByRole('button', { name: '.. Up' });
  await expect(upButton).toBeVisible();
  await upButton.click();

  await expect(dialog.getByText(parentDir, { exact: true }).first()).toBeVisible();
});

test('project-scoped routes isolate data per project', async ({ request }) => {
  const state = loadState();
  const fixture = await fixtureProject(request, state);
  const isolatedProjectName = uniqueName('scope-project');
  const isolatedPath = path.join(state.workspaceRoot, isolatedProjectName);

  const initResponse = await request.post(`${state.baseURL}/api/projects/init`, {
    data: { path: isolatedPath },
  });
  expect(initResponse.status()).toBe(201);

  const openResponse = await request.post(`${state.baseURL}/api/projects/open`, {
    data: { path: isolatedPath },
  });
  expect(openResponse.status()).toBe(200);
  const opened = await openResponse.json();
  const isolatedID = String(opened.id || '');
  expect(isolatedID).not.toBe('');

  const fixtureIssuesResponse = await request.get(`${projectBaseURL(state, fixture.id)}/issues`);
  expect(fixtureIssuesResponse.status()).toBe(200);
  const fixtureIssues = await fixtureIssuesResponse.json();
  expect(Array.isArray(fixtureIssues)).toBe(true);
  expect(fixtureIssues.length).toBeGreaterThan(0);

  const isolatedIssuesResponse = await request.get(`${projectBaseURL(state, isolatedID)}/issues`);
  expect(isolatedIssuesResponse.status()).toBe(200);
  const isolatedIssues = await isolatedIssuesResponse.json();
  expect(Array.isArray(isolatedIssues)).toBe(true);
  expect(isolatedIssues.length).toBe(0);
});
