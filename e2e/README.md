# ADAF Playwright Harness

This folder contains an end-to-end style Playwright harness for the web server.

## What it starts

- Launches `adaf web --daemon=false` with an ephemeral port.
- Writes a temporary isolated ADAF config with the current repo path as a recent project.
- Waits for API readiness before tests execute.
- Stops the background server and cleans temporary state on teardown.

## Run locally

```bash
make e2e
```

This runs:

1. `make web` (rebuilds frontend bundle into `internal/webserver/static`)
2. `cd e2e && npm install`
3. `cd e2e && npm run install:browsers`
4. `cd e2e && npm test`

## Useful commands

```bash
cd e2e
npm run test:headed
npm run test:ui
```
