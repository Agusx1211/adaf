const { expect, test } = require('@playwright/test');
const {
  fixtureProject,
  loadState,
  projectAppURL,
  uniqueName,
} = require('./helpers.js');

async function gotoConfig(page, request) {
  const state = loadState();
  const fixture = await fixtureProject(request, state);
  await page.goto(projectAppURL(state, fixture.id));
  await page.getByRole('button', { name: 'Config' }).click();
  await expect(page.getByText(/Profiles \(\d+\)/)).toBeVisible();
  return { state, fixture };
}

async function clickSectionNew(page, sectionRegex) {
  const sectionLabel = page.getByText(sectionRegex).first();
  await expect(sectionLabel).toBeVisible();
  const header = sectionLabel.locator('xpath=..');
  await header.getByRole('button', { name: '+ New' }).click();
}

async function readConfigList(request, state, endpoint) {
  let lastStatus = 0;
  for (let attempt = 0; attempt < 8; attempt += 1) {
    const response = await request.get(`${state.baseURL}${endpoint}`);
    lastStatus = response.status();
    if (lastStatus === 200) {
      const payload = await response.json();
      return Array.isArray(payload) ? payload : [];
    }
    await pageWait(150);
  }
  expect(lastStatus).toBe(200);
  return [];
}

function pageWait(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

test('config profile CRUD persists through API', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const profileName = uniqueName('profile');

  await clickSectionNew(page, /Profiles \(\d+\)/);
  await page.getByPlaceholder('my-profile').fill(profileName);
  await page.getByPlaceholder('Strengths, weaknesses, best use cases...').fill('Profile created by e2e test.');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const createdProfiles = await readConfigList(request, state, '/api/config/profiles');
  const createdProfile = createdProfiles.find((profile) => String(profile.name || '') === profileName);
  expect(createdProfile).toBeTruthy();

  await page.getByPlaceholder('Strengths, weaknesses, best use cases...').fill('Updated by e2e profile CRUD.');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const updatedProfiles = await readConfigList(request, state, '/api/config/profiles');
  const updatedProfile = updatedProfiles.find((profile) => String(profile.name || '') === profileName);
  expect(updatedProfile).toBeTruthy();
  expect(String(updatedProfile.description || '')).toContain('Updated by e2e profile CRUD');

  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page.getByText('Deleted').first()).toBeVisible();

  const remainingProfiles = await readConfigList(request, state, '/api/config/profiles');
  expect(remainingProfiles.find((profile) => String(profile.name || '') === profileName)).toBeFalsy();
});

test('profile editor shows cost tier and performance telemetry for seeded profiles', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const fixtures = Array.isArray(state.fixtures) ? state.fixtures : [];
  expect(fixtures.length).toBeGreaterThan(0);

  const target = fixtures[0];
  const profileName = String(target.profile || '');
  expect(profileName.length).toBeGreaterThan(0);

  await page.getByText(profileName, { exact: true }).first().click();

  const costSelect = page.locator('label', { hasText: 'Cost Tier' }).first().locator('xpath=following-sibling::select[1]');
  await expect(costSelect).toHaveValue('free');

  const panel = page.getByTestId('profile-performance-panel');
  await expect(panel).toBeVisible();
  await expect(page.getByText('Speed', { exact: true })).toHaveCount(0);
  await expect(page.getByText('Intelligence (1-10, 0=unset)', { exact: true })).toHaveCount(0);
  await expect(page.getByText('Max Instances (0=unlimited)', { exact: true })).toHaveCount(0);
  await expect(panel.getByText('Feedback Count')).toBeVisible();
  await expect(panel.getByText('Avg Quality')).toBeVisible();
  await expect(panel.getByText('Avg Difficulty')).toBeVisible();
  await expect(panel.getByText('Avg Duration')).toBeVisible();
  await expect(panel.getByText('Trend Over Time (0-10)')).toBeVisible();
  await expect(panel.getByText('Recent Raw Feedback')).toBeVisible();
  await expect(panel.getByText('spawn #700')).toBeVisible();
  await expect(panel.getByText('quality 8.00')).toBeVisible();
  await expect(panel.getByText('difficulty 4.00')).toBeVisible();
  await expect(panel.getByText('role scout')).toBeVisible();
  await expect(panel.getByText('parent seed-profile')).toBeVisible();
});

test('config skill appears in standalone new chat modal and supports delete', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const skillID = uniqueName('skill').replace(/-/g, '_');
  const profileName = uniqueName('chat-profile');

  const profileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: profileName, agent: 'generic' },
  });
  expect(profileCreate.status()).toBe(201);

  await clickSectionNew(page, /Skills \(\d+\)/);
  await page.getByPlaceholder('my_skill').fill(skillID);
  await page.getByPlaceholder('Concise instruction (1-4 sentences) for prompt embedding...').fill('Skill seeded by e2e.');
  await page.getByPlaceholder('Full documentation in Markdown...').fill('# Skill\n\nUsed in standalone modal.');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const createdSkills = await readConfigList(request, state, '/api/config/skills');
  expect(createdSkills.find((skill) => String(skill.id || '') === skillID)).toBeTruthy();

  await page.getByRole('button', { name: 'Standalone' }).click();
  const newChatButton = page.getByRole('button', { name: '+ New Chat' }).first();
  await expect(newChatButton).toBeVisible();
  await newChatButton.click();

  const newChatModal = page.getByRole('dialog', { name: 'New Chat' });
  await expect(newChatModal).toBeVisible();
  await newChatModal.locator('select').first().selectOption(profileName);
  await expect(newChatModal.getByText(skillID, { exact: true })).toBeVisible();
  await newChatModal.getByRole('button', { name: 'Cancel' }).click();

  await page.getByRole('button', { name: 'Config' }).click();
  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page.getByText('Deleted').first()).toBeVisible();

  const remainingSkills = await readConfigList(request, state, '/api/config/skills');
  expect(remainingSkills.find((skill) => String(skill.id || '') === skillID)).toBeFalsy();

  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(profileName)}`);
});

test('config loop CRUD works with real profile step', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const profileName = uniqueName('loop-profile');
  const loopName = uniqueName('loop');

  const profileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: profileName, agent: 'generic' },
  });
  expect(profileCreate.status()).toBe(201);

  await clickSectionNew(page, /Loops \(\d+\)/);
  await page.getByPlaceholder('my-loop').fill(loopName);

  const profileSelect = page.locator('label', { hasText: 'Profile' }).first().locator('xpath=following-sibling::select[1]');
  await profileSelect.selectOption(profileName);

  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const loops = await readConfigList(request, state, '/api/config/loops');
  const createdLoop = loops.find((loop) => String(loop.name || '') === loopName);
  expect(createdLoop).toBeTruthy();
  expect(createdLoop.steps && createdLoop.steps[0] && createdLoop.steps[0].profile).toBe(profileName);

  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page.getByText('Deleted').first()).toBeVisible();

  const remainingLoops = await readConfigList(request, state, '/api/config/loops');
  expect(remainingLoops.find((loop) => String(loop.name || '') === loopName)).toBeFalsy();

  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(profileName)}`);
});

test('loop editor renders runtime prompt preview scenarios', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const profileName = uniqueName('preview-profile');
  const loopName = uniqueName('preview-loop');

  const profileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: profileName, agent: 'generic' },
  });
  expect(profileCreate.status()).toBe(201);

  await clickSectionNew(page, /Loops \(\d+\)/);
  await page.getByPlaceholder('my-loop').fill(loopName);
  const profileSelect = page.locator('label', { hasText: 'Profile' }).first().locator('xpath=following-sibling::select[1]');
  await profileSelect.selectOption(profileName);

  const previewPanel = page.getByTestId('loop-prompt-preview-panel');
  await expect(previewPanel).toBeVisible();
  const previewRail = page.getByTestId('loop-preview-rail');
  const previewRailPosition = await previewRail.evaluate((el) => getComputedStyle(el).position);
  expect(previewRailPosition).toBe('sticky');
  const stepLabelText = await page.getByLabel('Prompt preview step').evaluate((el) =>
    Array.from(el.options).map((opt) => String(opt.textContent || '')).join(' || ')
  );
  expect(stepLabelText).toContain(`Step 1: ${profileName} (lead)`);
  const previewBody = page.getByTestId('loop-prompt-preview-body');

  await expect.poll(async () => {
    const text = await previewBody.textContent();
    return String(text || '');
  }).toContain(`"${loopName}"`);

  await page.getByRole('button', { name: /Turn 2\+ \(resume continuation\)/ }).click();
  await expect(previewBody).toContainText('Continue from where you left off.');

  await page.getByText('Edit skills…', { exact: true }).first().click();
  const loopSkillOption = page.getByTestId('skills-option-delegation').first();
  await expect(loopSkillOption).toBeVisible();
  await loopSkillOption.hover();
  const loopHoverPreview = page.getByTestId('loop-hover-preview-card');
  await expect(loopHoverPreview).toBeVisible();
  await expect(loopHoverPreview).toContainText('delegation');
  const loopHoverBox = await loopHoverPreview.boundingBox();
  const loopPromptPanelBox = await previewPanel.boundingBox();
  expect(loopHoverBox && loopPromptPanelBox && loopHoverBox.y < loopPromptPanelBox.y).toBeTruthy();

  const hasHorizontalOverflow = await previewBody.evaluate((el) => el.scrollWidth > el.clientWidth + 1);
  expect(hasHorizontalOverflow).toBe(false);

  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(profileName)}`);
});

test('loop editor manual prompt overrides runtime builder output', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const profileName = uniqueName('manual-profile');
  const loopName = uniqueName('manual-loop');
  const manualPrompt = 'manual override prompt for this step';

  const profileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: profileName, agent: 'generic' },
  });
  expect(profileCreate.status()).toBe(201);

  await clickSectionNew(page, /Loops \(\d+\)/);
  await page.getByPlaceholder('my-loop').fill(loopName);
  const profileSelect = page.locator('label', { hasText: 'Profile' }).first().locator('xpath=following-sibling::select[1]');
  await profileSelect.selectOption(profileName);

  const manualPromptInput = page.locator('label', { hasText: 'Manual Prompt (optional override)' }).first().locator('xpath=following-sibling::textarea[1]');
  await manualPromptInput.fill(manualPrompt);

  const previewBody = page.getByTestId('loop-prompt-preview-body');
  await expect.poll(async () => {
    const text = await previewBody.textContent();
    return String(text || '');
  }).toContain(manualPrompt);
  await expect(previewBody).not.toContainText('Project: test-project');

  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const loops = await readConfigList(request, state, '/api/config/loops');
  const persisted = loops.find((loop) => String(loop.name || '') === loopName);
  expect(persisted).toBeTruthy();
  const persistedStep = persisted.steps && persisted.steps[0] ? persisted.steps[0] : null;
  expect(String((persistedStep && persistedStep.manual_prompt) || '')).toBe(manualPrompt);

  await request.delete(`${state.baseURL}/api/config/loops/${encodeURIComponent(loopName)}`);
  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(profileName)}`);
});

test('config can copy loop and team definitions', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const profileName = uniqueName('copy-profile');
  const teamName = uniqueName('copy-team');
  const loopName = uniqueName('copy-loop');
  const sourceInstructions = 'Source loop instructions for copy test.';
  const sourceManualPrompt = 'source manual prompt for copy test';
  const teamDescription = 'Team copy source created in e2e.';

  const profileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: profileName, agent: 'generic' },
  });
  expect(profileCreate.status()).toBe(201);

  const teamCreate = await request.post(`${state.baseURL}/api/config/teams`, {
    data: {
      name: teamName,
      description: teamDescription,
      delegation: {
        max_parallel: 2,
        profiles: [{ name: profileName, timeout_minutes: 9 }],
      },
    },
  });
  expect(teamCreate.status()).toBe(201);

  const loopCreate = await request.post(`${state.baseURL}/api/config/loops`, {
    data: {
      name: loopName,
      steps: [{
        profile: profileName,
        turns: 2,
        instructions: sourceInstructions,
        manual_prompt: sourceManualPrompt,
      }],
    },
  });
  expect(loopCreate.status()).toBe(201);

  await page.evaluate(async () => {
    if (window.__configReload) await window.__configReload();
  });

  const copyLoopButton = page.getByRole('button', { name: `Copy loop ${loopName}` });
  await expect(copyLoopButton).toBeVisible();
  await copyLoopButton.click();

  const copiedLoopName = `${loopName}-copy`;
  const loopNameInput = page.getByPlaceholder('my-loop');
  await expect(loopNameInput).toHaveValue(copiedLoopName);

  const loopInstructionsInput = page.locator('label', { hasText: 'Instructions (optional)' }).first().locator('xpath=following-sibling::textarea[1]');
  await expect(loopInstructionsInput).toHaveValue(sourceInstructions);

  const loopManualPromptInput = page.locator('label', { hasText: 'Manual Prompt (optional override)' }).first().locator('xpath=following-sibling::textarea[1]');
  await expect(loopManualPromptInput).toHaveValue(sourceManualPrompt);

  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const loops = await readConfigList(request, state, '/api/config/loops');
  const copiedLoop = loops.find((loop) => String(loop.name || '') === copiedLoopName);
  expect(copiedLoop).toBeTruthy();
  const copiedStep = copiedLoop.steps && copiedLoop.steps[0] ? copiedLoop.steps[0] : null;
  expect(String((copiedStep && copiedStep.profile) || '')).toBe(profileName);
  expect(String((copiedStep && copiedStep.instructions) || '')).toBe(sourceInstructions);
  expect(String((copiedStep && copiedStep.manual_prompt) || '')).toBe(sourceManualPrompt);

  const copyTeamButton = page.getByRole('button', { name: `Copy team ${teamName}` });
  await expect(copyTeamButton).toBeVisible();
  await copyTeamButton.click();

  const copiedTeamName = `${teamName}-copy`;
  const teamNameInput = page.getByPlaceholder('my-team');
  await expect(teamNameInput).toHaveValue(copiedTeamName);
  await expect(page.getByPlaceholder('What this team is good at...')).toHaveValue(teamDescription);

  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const teams = await readConfigList(request, state, '/api/config/teams');
  const copiedTeam = teams.find((team) => String(team.name || '') === copiedTeamName);
  expect(copiedTeam).toBeTruthy();
  expect(String(copiedTeam.description || '')).toBe(teamDescription);
  const copiedTeamProfiles = copiedTeam.delegation && Array.isArray(copiedTeam.delegation.profiles)
    ? copiedTeam.delegation.profiles
    : [];
  expect(copiedTeamProfiles.length).toBe(1);
  expect(String(copiedTeamProfiles[0].name || '')).toBe(profileName);
  expect(Number(copiedTeamProfiles[0].timeout_minutes || 0)).toBe(9);

  await request.delete(`${state.baseURL}/api/config/loops/${encodeURIComponent(copiedLoopName)}`);
  await request.delete(`${state.baseURL}/api/config/loops/${encodeURIComponent(loopName)}`);
  await request.delete(`${state.baseURL}/api/config/teams/${encodeURIComponent(copiedTeamName)}`);
  await request.delete(`${state.baseURL}/api/config/teams/${encodeURIComponent(teamName)}`);
  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(profileName)}`);
});

test('team editor shows runtime prompt preview and keeps skill preview alongside', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const workerProfile = uniqueName('team-worker-profile');
  const teamName = uniqueName('team-preview');
  const skillID = uniqueName('team_skill').replace(/-/g, '_');

  const workerProfileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: workerProfile, agent: 'generic' },
  });
  expect(workerProfileCreate.status()).toBe(201);
  const skillCreate = await request.post(`${state.baseURL}/api/config/skills`, {
    data: { id: skillID, short: 'Skill used by team preview e2e.' },
  });
  expect(skillCreate.status()).toBe(201);

  await clickSectionNew(page, /Teams \(\d+\)/);
  await page.getByPlaceholder('my-team').fill(teamName);
  await page.getByPlaceholder('What this team is good at...').fill('Team prompt preview coverage.');
  await page.getByRole('button', { name: 'Enable' }).click();

  const firstProfileSelect = page.locator('label', { hasText: 'Profile' }).first().locator('xpath=following-sibling::select[1]');
  await firstProfileSelect.selectOption(workerProfile);

  const promptPreviewPanel = page.getByTestId('team-prompt-preview-panel');
  await expect(promptPreviewPanel).toBeVisible();
  await page.getByLabel('Team sub-agent preview profile').selectOption(workerProfile);

  const promptPreviewBody = page.getByTestId('team-prompt-preview-body');
  await expect.poll(async () => {
    const text = await promptPreviewBody.textContent();
    return String(text || '');
  }).toContain('You are a sub-agent working as a');
  await expect(promptPreviewBody).toContainText('adaf parent-ask');

  await page.getByText('Edit skills…', { exact: true }).first().click();
  const skillOption = page.locator('label', { hasText: skillID }).first();
  await expect(skillOption).toBeVisible();
  await skillOption.hover();

  const hoverPreviewCard = page.getByTestId('team-hover-preview-card');
  await expect(hoverPreviewCard).toBeVisible();
  await expect(hoverPreviewCard).toContainText(skillID);
  await expect(promptPreviewBody).toBeVisible();
  const hoverBox = await hoverPreviewCard.boundingBox();
  const promptBox = await promptPreviewPanel.boundingBox();
  expect(hoverBox && promptBox && hoverBox.y < promptBox.y).toBeTruthy();

  await request.delete(`${state.baseURL}/api/config/skills/${encodeURIComponent(skillID)}`);
  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(workerProfile)}`);
});

test('team editor persists per-sub-agent timeout minutes', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const workerProfile = uniqueName('team-timeout-profile');
  const teamName = uniqueName('team-timeout');

  const workerProfileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: workerProfile, agent: 'generic' },
  });
  expect(workerProfileCreate.status()).toBe(201);

  await clickSectionNew(page, /Teams \(\d+\)/);
  await page.getByPlaceholder('my-team').fill(teamName);
  await page.getByPlaceholder('What this team is good at...').fill('Team timeout persistence coverage.');
  await page.getByRole('button', { name: 'Enable' }).click();

  const firstProfileSelect = page.locator('label', { hasText: 'Profile' }).first().locator('xpath=following-sibling::select[1]');
  await firstProfileSelect.selectOption(workerProfile);

  await page.getByText('Advanced', { exact: true }).first().click();
  const timeoutInput = page.locator('label', { hasText: 'Timeout (min)' }).first().locator('xpath=following-sibling::input[1]');
  await timeoutInput.fill('7');

  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  await expect.poll(async () => {
    const teams = await readConfigList(request, state, '/api/config/teams');
    const persisted = teams.find((team) => String(team.name || '') === teamName);
    if (!persisted || !persisted.delegation || !Array.isArray(persisted.delegation.profiles) || !persisted.delegation.profiles.length) {
      return -1;
    }
    return Number(persisted.delegation.profiles[0].timeout_minutes || 0);
  }).toBe(7);

  await request.delete(`${state.baseURL}/api/config/teams/${encodeURIComponent(teamName)}`);
  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(workerProfile)}`);
});

test('loop editor explicit empty skills does not fall back to defaults', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const profileName = uniqueName('noskills-profile');
  const loopName = uniqueName('noskills-loop');

  const profileCreate = await request.post(`${state.baseURL}/api/config/profiles`, {
    data: { name: profileName, agent: 'generic' },
  });
  expect(profileCreate.status()).toBe(201);

  await clickSectionNew(page, /Loops \(\d+\)/);
  await page.getByPlaceholder('my-loop').fill(loopName);
  const profileSelect = page.locator('label', { hasText: 'Profile' }).first().locator('xpath=following-sibling::select[1]');
  await profileSelect.selectOption(profileName);

  await page.getByText('Edit skills…', { exact: true }).first().click();
  await page.getByRole('button', { name: 'Select All' }).first().click();
  await page.getByRole('button', { name: 'Clear' }).first().click();

  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('Saved').first()).toBeVisible();

  const loops = await readConfigList(request, state, '/api/config/loops');
  const persisted = loops.find((loop) => String(loop.name || '') === loopName);
  expect(persisted).toBeTruthy();
  const persistedStep = persisted.steps && persisted.steps[0] ? persisted.steps[0] : null;
  expect(Boolean(persistedStep && persistedStep.skills_explicit)).toBe(true);

  const previewBody = page.getByTestId('loop-prompt-preview-body');
  await expect(previewBody).not.toContainText('# Skills');
  await expect(previewBody).not.toContainText('## delegation');

  await request.delete(`${state.baseURL}/api/config/profiles/${encodeURIComponent(profileName)}`);
});

test('config team and role CRUD persists', async ({ page, request }) => {
  const { state } = await gotoConfig(page, request);
  const teamName = uniqueName('team');
  const roleName = uniqueName('role');

  await clickSectionNew(page, /Teams \(\d+\)/);
  await expect(page.getByText('New Team', { exact: false })).toBeVisible();
  await page.getByPlaceholder('my-team').fill(teamName);
  await page.getByPlaceholder('What this team is good at...').fill('Team created in e2e tests.');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect.poll(async () => {
    const teams = await readConfigList(request, state, '/api/config/teams');
    return teams.some((team) => String(team.name || '') === teamName);
  }).toBe(true);

  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect.poll(async () => {
    const teams = await readConfigList(request, state, '/api/config/teams');
    return teams.some((team) => String(team.name || '') === teamName);
  }).toBe(false);

  await clickSectionNew(page, /Roles \(\d+\)/);
  await expect(page.getByText('New Role', { exact: false })).toBeVisible();
  await page.getByPlaceholder('my-role').fill(roleName);
  await page.getByPlaceholder('ROLE TITLE (uppercase)').fill('E2E ROLE');
  await page.getByPlaceholder('What this role does...').fill('Role created by e2e.');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect.poll(async () => {
    const roles = await readConfigList(request, state, '/api/config/roles');
    return roles.some((role) => String(role.name || '') === roleName);
  }).toBe(true);

  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect.poll(async () => {
    const roles = await readConfigList(request, state, '/api/config/roles');
    return roles.some((role) => String(role.name || '') === roleName);
  }).toBe(false);
});
