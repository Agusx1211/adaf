// @ts-check
const { defineConfig, devices } = require('@playwright/test');
const fs = require('node:fs');
const path = require('node:path');

const stateFile = path.join(__dirname, '.state', 'web-server.json');

function getBaseURL() {
  try {
    const state = JSON.parse(fs.readFileSync(stateFile, 'utf8'));
    if (typeof state.baseURL === 'string' && state.baseURL !== '') {
      return state.baseURL;
    }
  } catch {
    // Will be initialized by globalSetup during test run.
  }
  return 'http://127.0.0.1:4173';
}

module.exports = defineConfig({
  testDir: './tests',
  timeout: 60 * 1000,
  workers: 1,
  globalSetup: require.resolve('./global-setup.js'),
  globalTeardown: require.resolve('./global-teardown.js'),
  outputDir: 'test-results',
  reporter: [['list']],
  use: {
    baseURL: getBaseURL(),
    ignoreHTTPSErrors: true,
    trace: 'on-first-retry',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],
});
