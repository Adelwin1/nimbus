# On-demand hosted checks

The API dispatches `.github/workflows/nimbus-hosted-check.yml` on the trusted public Nimbus repository's main branch. Standard GitHub-hosted Ubuntu runners execute one browser or repository job and exit; a worker server is not required. Jobs can take several minutes to provision. No schedule, keep-alive requests, uploaded artifacts, larger runners, or new cache storage are used.

## Account configuration

Keep the Nimbus execution repository public. Standard hosted runner minutes are free for public repositories. Private execution repositories are rejected by the backend and workflow. Configure a zero-spend Actions budget with usage stopping enabled as a further account-level guard. Do not create paid runners or add artifact uploads. Free API/database hosting still has provider quotas and cold starts; this configuration cannot guarantee continuous uptime or unlimited usage.

GitHub repository Settings > Secrets and variables > Actions:

- Secret `NIMBUS_WORKER_DATABASE_URL`: production Neon connection string, with verified TLS (`sslmode=verify-full`). This is the same database the deployed API uses. It is used only by the trusted worker process, never injected into repository test containers or the browser child.
- Variable `NIMBUS_API_ORIGIN`: `https://nimbus-api-vjw7.onrender.com` (no `/api/v1` suffix).
- Variable `NIMBUS_JOURNEY_ALLOWED_ORIGINS`: comma-separated approved public application origins; initially `https://nimbus-alpha-livid.vercel.app`. Browser journeys for other applications require their origins to be added. Do not allow localhost/internal services for hosted jobs.

Create a fine-grained GitHub personal access token for **only Adelwin1/nimbus**, with **Actions: Read and write** (metadata read is implicit). This is the server's workflow dispatch credential, separate from users' read-only GitHub connections. Give it an expiration and renew before expiration; failure is reported if it expires. Do not paste it into chats or commit it.

Render API Environment:

- `NIMBUS_WORKER_MODE=github-actions`
- `NIMBUS_ACTIONS_REPOSITORY=Adelwin1/nimbus`
- `NIMBUS_ACTIONS_TOKEN=<fine-grained token>`
- `CODE_CHECKS_ENABLED=true`

Keep existing production GitHub callback, frontend origin, encryption and authentication settings. `GITHUB_ALLOW_LOCAL_DEVELOPMENT` must not be enabled in production. Migrations 19, 20 and 21 must apply before enabling hosted mode. The production Docker startup script runs migrations.

## Deployment order

1. Apply this installer locally; format Go; run migration 21 on a fresh test database and run the affected packages, worker tests, frontend tests/lint/build.
2. Configure GitHub Actions secret/variables and restricted token.
3. Commit all pending sign-in/repository-check changes plus hosted execution; push main; wait for CI.
4. Configure Render hosted mode and token, deploy the tested main commit; check `/health`.
5. Deploy the frontend from repository root via `npx vercel --prod --scope adelwin1s-projects`.
6. Stop Mac worker terminals. In production run a browser journey to an allowlisted origin and a `node-test` repository check in `github-service`. Both must finish. Check a controlled failing test before claiming failure localization is verified in production.

The API enforces 10 hosted checks per user per rolling 24 hours and 100 across the installation. Quota reservations share the queue transaction and a database advisory lock. Dispatch failures become job errors rather than indefinite queues. Interrupted setup/runs become errors after expiry when results are refreshed. A dispatched workflow uses only a UUID/kind; credentials are secrets, never workflow inputs.

Database access is trusted. Untrusted repository code is executed by the existing Docker runner with no network, host sockets, injected credentials, writable source, or elevated capabilities. It has time/resource/output limits. This is restricted shared-kernel execution, not a VM sandbox. The supported profiles remain Node built-in tests and offline Go tests; dependencies must already be available or vendored. This change does not expand language coverage or generate arbitrary fixes.
