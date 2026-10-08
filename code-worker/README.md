# Repository checks

The API queues checks for a journey's captured repository and commit. It verifies current GitHub access at queue time and again before downloading source. A single-use worker grant is returned only through the trusted database queue. Source is size-limited, path-checked, and excludes .env files. Symlinks and oversized repositories are rejected.

Enable `CODE_CHECKS_ENABLED=true` on the API only when a repository worker is configured. Connecting GitHub alone does not execute repository code. The browser worker and this repository worker are separate processes.

On the worker host, install the browser-worker dependencies (the worker reuses its `pg` dependency), start Docker, and pull execution images:

```sh
cd browser-worker
npm ci
cd ..
docker pull node:24-bookworm-slim
docker pull golang:1.26.4-bookworm
node --test code-worker/runner.test.cjs code-worker/runner.integration.test.cjs
```

Start a worker with the same database used by the API:

```sh
DATABASE_URL="$NIMBUS_WORKER_DATABASE" NIMBUS_API_ORIGIN='http://localhost:8080' node code-worker/worker.cjs
```

Use the production HTTPS API origin for production. Keep the worker database URL private. Database access is trusted: deploy this worker on a dedicated execution host rather than giving arbitrary repositories access to your development machine. Docker restrictions reduce exposure but do not make shared-kernel execution risk-free. For a public multi-user launch, use dedicated sandbox infrastructure and quotas.

Profiles:

- `node-test`: Node's built-in test runner, from the chosen repository directory.
- `go-test`: `go test -json ./... -count=1`, with network access and module fetching disabled.

The workload runs as a non-root user with a read-only source mount, no network, no injected GitHub/database credentials, dropped capabilities, a writable temporary filesystem, and limits on CPU, memory, processes, output and time. The worker removes the container and temporary checkout after each attempt. Jobs that lose their lease become errors.

Dependencies must be vendored or available in the execution image. Test suites requiring a database, package downloads, a browser, Python, Java, npm/Vitest or TypeScript compilation need additional profiles; they are not supported by these first two profiles. Missing dependencies and empty test suites are reported as execution errors, not application bugs.

Results store structured file/line evidence and bounded static import relationships, not raw logs. Test output locations do not establish which code caused a browser failure. Static imports do not prove a runtime call chain. Passing tests do not verify that an operator-supplied preview serves the recorded commit. This step does not generate or apply arbitrary code fixes.
