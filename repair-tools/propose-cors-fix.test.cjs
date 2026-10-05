const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const { spawnSync } = require("node:child_process");

function command(root, executable, args) {
  const result = spawnSync(executable, args, {
    cwd: root, encoding: "utf8", timeout: 15000
  });
  assert.equal(result.status, 0, result.stderr || result.stdout);
  return result;
}

test("proposes a clean CORS patch without changing source", async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), "nimbus-repair-test-"));
  const sourcePath = path.join(root, "backend/internal/middleware/cors.go");
  const source = `package middleware
const methods = "GET, POST, PATCH, DELETE, OPTIONS"
`;
  try {
    await fs.mkdir(path.dirname(sourcePath), { recursive: true });
    await fs.mkdir(path.join(root, "backend/cmd/api"), { recursive: true });
    await fs.writeFile(sourcePath, source);
    await fs.writeFile(path.join(root, "backend/cmd/api/main.go"), `package main
func main() {
    router.Put("/rules", handler)
}
`);
    command(root, "git", ["init", "-q"]);
    command(root, "git", ["add", "backend"]);
    command(root, "git", [
      "-c", "user.name=Nimbus Test",
      "-c", "user.email=test@example.com",
      "-c", "commit.gpgsign=false",
      "commit", "-qm", "Broken CORS fixture"
    ]);

    const script = path.join(__dirname, "propose-cors-fix.cjs");
    const output = command(root, process.execPath, [script, root]);
    assert.ok(output.stdout.includes("Fix proposed:"), output.stdout);
    assert.equal(await fs.readFile(sourcePath, "utf8"), source);

    const reviews = await fs.readdir(path.join(root, "repair-reviews"));
    assert.equal(reviews.length, 1);
    const directory = path.join(root, "repair-reviews", reviews[0]);
    const review = JSON.parse(
      await fs.readFile(path.join(directory, "review.json"), "utf8")
    );
    assert.deepEqual(review.observed.route_methods_missing_from_cors, ["PUT"]);
    assert.equal(review.validation.tests_run, false);

    command(root, "git", ["apply", path.join(directory, "proposed.patch")]);
    assert.ok((await fs.readFile(sourcePath, "utf8")).includes(
      '"GET, POST, PUT, PATCH, DELETE, OPTIONS"'
    ));
    command(root, process.execPath, [script, root]);
    assert.equal((await fs.readdir(path.join(root, "repair-reviews"))).length, 1);
  } finally {
    await fs.rm(root, { recursive: true, force: true });
  }
});
