const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");

function git(root, args) {
  const result = spawnSync("git", args, {
    cwd: root, encoding: "utf8", timeout: 15000
  });
  if (result.error) throw result.error;
  return result;
}

async function main() {
  const root = path.resolve(process.argv[2] || process.cwd());
  const relative = "backend/internal/middleware/cors.go";
  const routeFile = "backend/cmd/api/main.go";
  const head = git(root, ["rev-parse", "HEAD"]);
  if (head.status !== 0) throw new Error("Target must be a Git repository.");

  const source = await fs.readFile(path.join(root, relative), "utf8");
  const routes = await fs.readFile(path.join(root, routeFile), "utf8");
  const matches = [...source.matchAll(/"GET, POST, [A-Z, ]+"/g)];
  if (matches.length !== 1) {
    throw new Error("CORS pattern is ambiguous or unsupported; no patch generated.");
  }

  const routeMethods = [
    ["GET", "Get"], ["POST", "Post"], ["PUT", "Put"],
    ["PATCH", "Patch"], ["DELETE", "Delete"], ["OPTIONS", "Options"]
  ];
  const declared = matches[0][0].slice(1, -1).split(", ");
  const missing = routeMethods
    .filter(([method, handler]) =>
      routes.split("\n").some(line => {
        const match = line.match(/^\s*[A-Za-z_][A-Za-z0-9_]*\.(Get|Post|Put|Patch|Delete|Options)\s*\(/);
        return match && match[1] === handler;
      }) && !declared.includes(method))
    .map(([method]) => method);

  if (!missing.length) {
    console.log("No supported CORS mismatch found. No fix proposed.");
    return;
  }

  const methods = routeMethods
    .map(([method]) => method)
    .filter(method => declared.includes(method) || missing.includes(method));
  const updated = source.replace(matches[0][0], `"${methods.join(", ")}"`);
  const temporary = await fs.mkdtemp(path.join(os.tmpdir(), "nimbus-cors-fix-"));

  try {
    const before = path.join(temporary, "before.go");
    const after = path.join(temporary, "after.go");
    await fs.writeFile(before, source);
    await fs.writeFile(after, updated);
    const diff = git(root, ["diff", "--no-index", "--", before, after]);
    if (diff.status !== 1 || !diff.stdout.includes("@@")) {
      throw new Error("Patch generation failed.");
    }
    const hunks = diff.stdout.slice(diff.stdout.indexOf("@@"));
    const patch = [
      `diff --git a/${relative} b/${relative}`,
      `--- a/${relative}`,
      `+++ b/${relative}`,
      hunks
    ].join("\n");

    const output = path.join(temporary, "proposed.patch");
    await fs.writeFile(output, patch);
    const check = git(root, ["apply", "--check", "--", output]);
    if (check.status !== 0) {
      throw new Error("Generated patch does not apply cleanly.");
    }

    const review = {
      rule: "go-cors-route-method-mismatch",
      base_commit: head.stdout.trim(),
      source_sha256: crypto.createHash("sha256").update(source).digest("hex"),
      observed: {
        declared_methods: declared,
        route_methods_missing_from_cors: missing
      },
      proposed_change: `Add ${missing.join(", ")} to the CORS method list.`,
      validation: {
        patch_applies_to_current_files: true,
        tests_run: false
      },
      limitations: [
        "Static pattern detection; it does not verify the browser's origin.",
        "Only supported methods registered in cmd/api/main.go are inspected.",
        "The patch is proposed and has not been applied."
      ]
    };

    const directory = path.join(root, "repair-reviews", crypto.randomUUID());
    await fs.mkdir(directory, { recursive: true });
    await fs.writeFile(path.join(directory, "proposed.patch"), patch);
    await fs.writeFile(
      path.join(directory, "review.json"),
      JSON.stringify(review, null, 2) + "\n"
    );
    console.log("Fix proposed:", directory);
    console.log("Patch applies cleanly. Application files remain unchanged.");
  } finally {
    await fs.rm(temporary, { recursive: true, force: true });
  }
}

main().catch(error => {
  console.error(error.message);
  process.exitCode = 1;
});
