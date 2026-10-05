const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const { spawnSync } = require("node:child_process");

function command(root, executable, args) {
  const result = spawnSync(executable, args, {
    cwd: root,
    encoding: "utf8",
    timeout: 180000
  });
  assert.equal(result.status, 0, result.stderr || result.stdout);
  return result;
}

test("verifies a proposed repair in an isolated Go checkout",
  { timeout: 240000 }, async () => {
    const root = await fs.mkdtemp(path.join(os.tmpdir(), "nimbus-verify-test-"));
    const sourcePath = path.join(root, "backend/internal/middleware/cors.go");
    const original = `package middleware
const methods = "GET, POST, PATCH, DELETE, OPTIONS"
`;
    try {
      await fs.mkdir(path.dirname(sourcePath), { recursive: true });
      await fs.mkdir(path.join(root, "backend/cmd/api"), { recursive: true });

      await fs.writeFile(path.join(root, "backend/go.mod"),
        "module example.com/nimbusfixture\n\ngo 1.22\n");
      await fs.writeFile(sourcePath, original);

      await fs.writeFile(
        path.join(root, "backend/internal/middleware/cors_test.go"),
        `package middleware
import (
    "strings"
    "testing"
)
func TestPutAllowed(t *testing.T) {
    for _, method := range strings.Split(methods, ", ") {
        if method == "PUT" { return }
    }
    t.Fatal("PUT must be allowed")
}
`);

      await fs.writeFile(path.join(root, "backend/cmd/api/main.go"),
        `package main
type testRouter struct{}
func (testRouter) Put(path string, handler func()) {}
var router testRouter
func handler() {}
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
        "commit", "-qm", "Broken Go fixture"
      ]);

      // Establish that the original fixture actually fails its regression test.
      const baseline = spawnSync("go", ["test", "./internal/middleware"], {
        cwd: path.join(root, "backend"),
        encoding: "utf8",
        timeout: 120000
      });
      assert.equal(baseline.status, 1, baseline.stderr || baseline.stdout);
      assert.ok(baseline.stdout.includes("PUT must be allowed"));

      command(root, process.execPath, [
        path.join(__dirname, "propose-cors-fix.cjs"), root
      ]);
      const directories = await fs.readdir(path.join(root, "repair-reviews"));
      assert.equal(directories.length, 1);
      const directory = path.join(root, "repair-reviews", directories[0]);

      command(root, process.execPath, [
        path.join(__dirname, "verify-cors-fix.cjs"), root, directory
      ]);

      const review = JSON.parse(
        await fs.readFile(path.join(directory, "review.json"), "utf8")
      );
      assert.equal(review.validation.checks_passed, true);
      assert.equal(review.validation.live_source_modified, false);
      assert.equal(review.validation.checks.length, 2);
      assert.equal(await fs.readFile(sourcePath, "utf8"), original);

      const worktrees = command(root, "git", ["worktree", "list", "--porcelain"]);
      assert.equal(
        worktrees.stdout.split("\n").filter(line => line.startsWith("worktree ")).length,
        1
      );
    } finally {
      await fs.rm(root, { recursive: true, force: true });
    }
  });
