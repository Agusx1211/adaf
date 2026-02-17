const { expect, test } = require('@playwright/test');
const {
  fixtureProject,
  loadState,
  projectAppURL,
} = require('./helpers.js');

test('sidebar role label and control cards reflect supervisor turns', async ({ page, request }) => {
  const state = loadState();
  const fixture = await fixtureProject(request, state);

  await page.goto(`${projectAppURL(state, fixture.id)}#/loops`);

  const loopRow = page.getByText('role-control-fixture', { exact: true }).first();
  await expect(loopRow).toBeVisible();
  await loopRow.click();

  const supervisorTurnID = page.getByText('#912', { exact: true }).first();
  await expect(supervisorTurnID).toBeVisible();
  const supervisorTurnRow = supervisorTurnID.locator('xpath=ancestor::div[contains(@style,"padding: 5px 12px 5px 28px")][1]');
  await expect(supervisorTurnRow).toContainText('supervisor');

  await supervisorTurnRow.click();
  await expect(page.getByText('CALL SUPERVISOR', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('STOP LOOP', { exact: true }).first()).toBeVisible();
});
