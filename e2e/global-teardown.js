const fs = require('node:fs');
const path = require('node:path');

const STATE_FILE = path.join(__dirname, '.state', 'web-server.json');

function isProcessAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function killProcess(pid) {
  if (!pid || typeof pid !== 'number') {
    return;
  }

  if (!isProcessAlive(pid)) {
    return;
  }

  try {
    process.kill(pid, 'SIGTERM');
  } catch {
    return;
  }
}

module.exports = async function globalTeardown() {
  if (!fs.existsSync(STATE_FILE)) {
    return;
  }

  let state;
  try {
    state = JSON.parse(fs.readFileSync(STATE_FILE, 'utf8'));
  } catch {
    fs.rmSync(STATE_FILE, { force: true });
    return;
  }

  killProcess(state.pid);

  if (state?.homeDir) {
    try {
      fs.rmSync(state.homeDir, { recursive: true, force: true });
    } catch {
      // Best-effort cleanup only; avoid failing test teardown on transient FS issues.
    }
  }

  if (state?.fixtureProjectDir) {
    try {
      fs.rmSync(state.fixtureProjectDir, { recursive: true, force: true });
    } catch {
      // Best-effort cleanup only.
    }
  }

  if (state?.workspaceRoot) {
    try {
      fs.rmSync(state.workspaceRoot, { recursive: true, force: true });
    } catch {
      // Best-effort cleanup only.
    }
  }

  fs.rmSync(STATE_FILE, { force: true });
};
