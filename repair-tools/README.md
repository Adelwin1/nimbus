# Tested repair PRs and pre-deployment checks

Nimbus currently proposes one supported fix: missing HTTP methods in the Go CORS configuration. Source investigation lists candidate files; it does not generate arbitrary bug fixes.

From the repository root, with Go installed:

```sh
node repair-tools/propose-cors-fix.cjs .
node repair-tools/verify-cors-fix.cjs . repair-reviews/REVIEW_ID
node repair-tools/publish-cors-pr.cjs . repair-reviews/REVIEW_ID
```

Replace REVIEW_ID with the generated directory name. A healthy CORS configuration produces no proposal. The PR command re-runs tests and the API build, verifies the exact method-list change, and writes a PR plan without publishing by default. It uses a temporary worktree. Repository code executes locally; the worktree is not a security sandbox. Run only on code you trust.

After inspecting `pr-body.md`, explicitly publish using your GitHub CLI account:

```sh
gh auth login
node repair-tools/publish-cors-pr.cjs . repair-reviews/REVIEW_ID --publish
```

The recorded commit must still be the default branch tip. Publishing pushes a new branch and opens a draft PR, without merging or deploying. The GitHub App remains read-only. A failed PR creation may leave the pushed branch; `pr-plan.json` records that state. Existing middleware tests and compilation passing do not prove that every application workflow is correct.

The GitHub Actions workflow checks backend migrations/tests/race detection/build, frontend tests/lint/build, browser journey fixtures, and repair tools on PRs and main pushes. No production credentials are needed. These are repository checks and local browser fixtures, not tests of every connected application's live preview. Configure branch protection to require **Nimbus Quality Gate** before merging. Render/Vercel deployment settings are separate; this workflow does not deploy. Existing provider auto-deploy settings are not changed by this installer.

Local checks:

```sh
cd browser-worker
npm ci
npx playwright install chromium
npm test
cd ..
node --test repair-tools/*.test.cjs
cd frontend
npm test && npm run lint && npm run build
```
