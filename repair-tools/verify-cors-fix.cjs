const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");

function run(cwd, executable, args, timeout = 15000) {
  const result = spawnSync(executable, args, {
    cwd, encoding: "utf8", timeout, maxBuffer: 1048576
  });
  return {
    passed: !result.error && result.status === 0,
    exit_code: result.status,
    timed_out: result.error?.code === "ETIMEDOUT",
    stdout: result.stdout || ""
  };
}
function requireSuccess(result, message) {
  if (!result.passed) throw new Error(message);
}
function hash(data) {
  return crypto.createHash("sha256").update(data).digest("hex");
}

async function main() {
  if (!process.argv[2] || !process.argv[3]) {
    throw new Error("Usage: node verify-cors-fix.cjs repository review-directory");
  }
  const root = path.resolve(process.argv[2]);
  const directory = path.resolve(process.argv[3]);
  const reviewFile = path.join(directory, "review.json");
  const patchFile = path.join(directory, "proposed.patch");
  const review = JSON.parse(await fs.readFile(reviewFile, "utf8"));
  const patch = await fs.readFile(patchFile);
  if (review.rule !== "go-cors-route-method-mismatch" ||
      !/^[0-9a-f]{40,64}$/.test(review.base_commit) ||
      patch.length > 32768) {
    throw new Error("Unsupported review or patch.");
  }

  const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "nimbus-fix-check-"));
  const checkout = path.join(temporary, "checkout");
  let added = false;
  try {
    requireSuccess(run(root, "git", [
      "worktree", "add", "--detach", checkout, review.base_commit
    ]), "Could not create isolated checkout.");
    added = true;

    const source = await fs.readFile(
      path.join(checkout, "backend/internal/middleware/cors.go")
    );
    if (hash(source) !== review.source_sha256) {
      throw new Error("Source differs from the recorded commit. Generate a fresh proposal from committed source.");
    }
    requireSuccess(run(checkout, "git", ["apply", "--check", "--", patchFile]),
      "Patch does not apply to the recorded commit.");
    requireSuccess(run(checkout, "git", ["apply", "--", patchFile]),
      "Patch application failed.");

    const changed = run(checkout, "git", ["diff", "--name-only"]);
    requireSuccess(changed, "Could not inspect patched files.");
    if (changed.stdout.trim() !== "backend/internal/middleware/cors.go") {
      throw new Error("Patch changes files outside the supported rule.");
    }

    const checks = [];
    for (const [name, args] of [
      ["middleware tests", ["test", "./internal/middleware", "-count=1"]],
      ["API build", ["build", "-o", path.join(temporary, "nimbus-api"), "./cmd/api"]]
    ]) {
      const result = run(path.join(checkout, "backend"), "go", args, 120000);
      checks.push({
        name, passed: result.passed,
        exit_code: result.exit_code, timed_out: result.timed_out
      });
      if (!result.passed) break;
    }

    review.validation = {
      patch_applies_to_recorded_commit: true,
      tests_run: checks.some(check => check.name === "middleware tests"),
      checks,
      checks_passed: checks.length === 2 && checks.every(check => check.passed),
      patch_sha256: hash(patch),
      checked_at: new Date().toISOString(),
      live_source_modified: false
    };
    await fs.writeFile(reviewFile, JSON.stringify(review, null, 2) + "\n");
    console.log(JSON.stringify(review.validation, null, 2));
    if (!review.validation.checks_passed) process.exitCode = 1;
  } finally {
    let cleaned = !added;
    if (added) {
      cleaned = run(root, "git", ["worktree", "remove", "--force", checkout]).passed;
    }
    if (cleaned) await fs.rm(temporary, { recursive: true, force: true });
    else console.error("Temporary worktree cleanup failed:", checkout);
  }
}
main().catch(error => {
  console.error(error.message);
  process.exitCode = 1;
});
