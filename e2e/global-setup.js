const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const { spawn } = require('node:child_process');
const { prepareFixtureReplayData } = require('./fixture-replay.js');

const STATE_FILE = path.join(__dirname, '.state', 'web-server.json');
const WAIT_MS = 30_000;
const POLL_MS = 200;

function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.unref();

    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'string' ? null : address?.port;
      server.close(() => {
        if (!port || typeof port !== 'number') {
          reject(new Error('Failed to acquire ephemeral port'));
          return;
        }
        resolve(port);
      });
    });
  });
}

function httpGetStatus(url) {
  return new Promise((resolve, reject) => {
    const req = http.get(url, (res) => {
      resolve(res.statusCode || 0);
      res.resume();
    });
    req.on('error', (err) => {
      reject(err);
    });
  });
}

async function waitForReady(url) {
  const deadline = Date.now() + WAIT_MS;
  while (Date.now() < deadline) {
    try {
      const status = await httpGetStatus(url);
      if (status >= 200 && status < 500) {
        return;
      }
    } catch {
      // keep waiting
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_MS));
  }
  throw new Error(`Server did not become ready at ${url}`);
}

function writeAdafConfig(homeDir, repositoryRoot, fixtureProjectDir, fixtures) {
  const adafConfigDir = path.join(homeDir, '.adaf');
  fs.mkdirSync(adafConfigDir, { recursive: true });

  const costs = ['free', 'cheap', 'normal', 'expensive'];
  const profiles = Array.isArray(fixtures)
    ? fixtures.map((fixture, idx) => ({
      name: fixture.profile,
      agent: fixture.provider,
      cost: costs[idx % costs.length],
    }))
    : [];

  const config = {
    recent_projects: [
      {
        id: 'fixture-replay-project',
        path: fixtureProjectDir,
        name: path.basename(fixtureProjectDir),
        root_dir: repositoryRoot,
      },
    ],
    profiles,
  };

  fs.writeFileSync(
    path.join(adafConfigDir, 'config.json'),
    JSON.stringify(config, null, 2) + '\n',
    'utf8',
  );
}

module.exports = async function globalSetup() {
  fs.mkdirSync(path.join(__dirname, '.state'), { recursive: true });

  const repositoryRoot = path.resolve(__dirname, '..');
  const workspaceRoot = path.join(__dirname, '.state', 'workspace');
  const fixtureProjectDir = path.join(workspaceRoot, 'fixture-replay-project');
  const fixtureProjectID = path.relative(repositoryRoot, fixtureProjectDir).split(path.sep).join('/');
  const port = await getFreePort();
  const baseURL = `http://127.0.0.1:${port}`;

  fs.rmSync(workspaceRoot, { recursive: true, force: true });
  fs.mkdirSync(workspaceRoot, { recursive: true });

  const homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'adaf-e2e-'));
  const goModCache = path.join(homeDir, 'go', 'pkg', 'mod');
  const goBuildCache = path.join(homeDir, 'go', 'cache');
  fs.mkdirSync(goModCache, { recursive: true });
  fs.mkdirSync(goBuildCache, { recursive: true });
  const fixtures = prepareFixtureReplayData(repositoryRoot, homeDir, fixtureProjectDir);
  writeAdafConfig(homeDir, repositoryRoot, fixtureProjectDir, fixtures);

  const env = {
    ...process.env,
    HOME: homeDir,
    USERPROFILE: homeDir,
    GOMODCACHE: goModCache,
    GOCACHE: goBuildCache,
  };

  const child = spawn(
    'go',
    [
      'run',
      './cmd/adaf',
      'web',
      '--daemon=false',
      '--host',
      '127.0.0.1',
      '--port',
      String(port),
      '--allowed-root',
      workspaceRoot,
      '--rate-limit',
      '0',
    ],
    {
      cwd: repositoryRoot,
      env,
      stdio: ['ignore', 'pipe', 'pipe'],
      detached: false,
    },
  );

  let runningError;
  child.once('error', (err) => {
    runningError = err;
  });

  const startup = waitForReady(`${baseURL}/api/config`);
  const failure = new Promise((_, reject) => {
    child.once('exit', (code, signal) => {
      if (runningError) {
        reject(runningError);
        return;
      }
      if (code !== 0) {
        reject(new Error(`adaf web exited during startup (code=${code}, signal=${signal || 'none'})`));
      } else {
        reject(new Error('adaf web exited during startup'));
      }
    });
  });

  try {
    await Promise.race([startup, failure]);
  } catch (err) {
    child.removeAllListeners('exit');
    child.removeAllListeners('error');
    try {
      child.kill('SIGKILL');
    } catch {
      // best effort only
    }
    try {
      fs.rmSync(workspaceRoot, { recursive: true, force: true });
    } catch {
      // best effort only
    }
    try {
      fs.rmSync(homeDir, { recursive: true, force: true });
    } catch {
      // best effort only
    }
    throw err;
  }

  fs.writeFileSync(
    STATE_FILE,
    JSON.stringify(
      {
        baseURL,
        pid: child.pid,
        port,
        homeDir,
        repositoryRoot,
        workspaceRoot,
        fixtureProjectDir,
        fixtureProjectID,
        fixtures,
        command: 'go run ./cmd/adaf web --daemon=false',
      },
      null,
      2,
    ),
    'utf8',
  );

  return {
    serverPID: child.pid,
    baseURL,
  };
};
