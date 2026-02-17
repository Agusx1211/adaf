const fs = require('node:fs');
const path = require('node:path');

const STATE_FILE = path.join(__dirname, '..', '.state', 'web-server.json');

function loadState() {
  const raw = fs.readFileSync(STATE_FILE, 'utf8');
  return JSON.parse(raw);
}

function normalizePath(value) {
  return path.resolve(String(value || ''));
}

function uniqueName(prefix) {
  return [
    String(prefix || 'e2e'),
    Date.now().toString(36),
    Math.random().toString(36).slice(2, 8),
  ].join('-');
}

async function listProjects(request, state) {
  const response = await request.get(`${state.baseURL}/api/projects`);
  if (!response.ok()) {
    throw new Error(`failed to list projects: ${response.status()}`);
  }
  const payload = await response.json();
  return Array.isArray(payload) ? payload : [];
}

async function fixtureProject(request, state) {
  const projects = await listProjects(request, state);
  const fixtureByPath = projects.find(function (project) {
    return normalizePath(project.path) === normalizePath(state.fixtureProjectDir);
  });
  if (fixtureByPath) {
    return fixtureByPath;
  }

  const fixtureByID = projects.find(function (project) {
    return String(project.id || '') === String(state.fixtureProjectID || '');
  });
  if (fixtureByID) {
    return fixtureByID;
  }

  throw new Error('fixture project is not registered in /api/projects');
}

function projectBaseURL(state, projectID) {
  return `${state.baseURL}/api/projects/${encodeURIComponent(String(projectID || ''))}`;
}

function projectAppURL(state, projectID) {
  return `${state.baseURL}/?project=${encodeURIComponent(String(projectID || ''))}`;
}

function normalizeExpectedToken(text) {
  return String(text || '').trim().replace(/[.!?]+$/, '');
}

module.exports = {
  fixtureProject,
  listProjects,
  loadState,
  normalizeExpectedToken,
  projectAppURL,
  projectBaseURL,
  uniqueName,
};
