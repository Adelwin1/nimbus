const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");
const hash = value => crypto.createHash("sha256").update(value).digest("hex");
function command(cwd, executable, args, timeout = 30000) {
  const result = spawnSync(executable, args, {cwd, encoding: "utf8", timeout, maxBuffer: 1048576});
  if (result.error || result.status !== 0) throw new Error(`${executable} ${args[0]} failed. Check authentication, dependencies and command output locally.`);
  return result.stdout.trim();
}
function expectedRepair(source, routes) {
  const matches = [...source.matchAll(/"GET, POST, [A-Z, ]+"/g)];
  if (matches.length !== 1) throw new Error("Ambiguous CORS method list.");
  const declared = matches[0][0].slice(1,-1).split(", ");
  const pairs = [["GET","Get"],["POST","Post"],["PUT","Put"],["PATCH","Patch"],["DELETE","Delete"],["OPTIONS","Options"]];
  const missing = pairs.filter(([method, handler]) => routes.split("\n").some(line => {
    const match = /^\s*[A-Za-z_][A-Za-z0-9_]*\.(Get|Post|Put|Patch|Delete|Options)\s*\(/.exec(line);
    return match?.[1] === handler;
  }) && !declared.includes(method)).map(([method]) => method);
  if (!missing.length) throw new Error("No supported CORS mismatch exists at this commit.");
  return source.replace(matches[0][0], JSON.stringify(pairs.map(([method]) => method).filter(method => declared.includes(method) || missing.includes(method)).join(", ")));
}
async function main() {
  const [repository, reviewDirectory, ...flags] = process.argv.slice(2);
  if (!repository || !reviewDirectory || flags.some(flag => flag !== "--publish")) throw new Error("Usage: node publish-cors-pr.cjs repository review-directory [--publish]");
  const root = path.resolve(repository), directory = path.resolve(reviewDirectory);
  const review = JSON.parse(await fs.readFile(path.join(directory,"review.json"),"utf8"));
  const patchPath = path.join(directory,"proposed.patch");
  const patch = await fs.readFile(patchPath);
  if (review.rule !== "go-cors-route-method-mismatch" || !/^[0-9a-f]{40,64}$/.test(review.base_commit) || patch.length > 32768) throw new Error("Unsupported repair.");
  if (command(root,"git",["rev-parse","HEAD"]) !== review.base_commit) throw new Error("Stale repair: generate a proposal for the current commit.");
  if (command(root,"git",["status","--porcelain","--untracked-files=no"])) throw new Error("Commit or stash tracked changes before preparing a repair PR.");
  const temporary = await fs.mkdtemp(path.join(os.tmpdir(),"nimbus-pr-"));
  const checkout = path.join(temporary,"checkout");
  let added = false;
  try {
    command(root,"git",["worktree","add","--detach",checkout,review.base_commit]); added = true;
    const relative = "backend/internal/middleware/cors.go";
    const source = await fs.readFile(path.join(checkout,relative),"utf8");
    if (hash(source) !== review.source_sha256) throw new Error("Recorded source hash does not match the commit.");
    const routes = await fs.readFile(path.join(checkout,"backend/cmd/api/main.go"),"utf8");
    const expected = expectedRepair(source,routes);
    const regressionRelative = "backend/internal/middleware/nimbus_repair_regression_test.go";
    const regressionPath = path.join(checkout, regressionRelative);
    try { await fs.access(regressionPath); throw new Error("Regression file already exists. Refresh the repair workflow."); }
    catch (error) { if (error.code !== "ENOENT") throw error; }
    const methodList = expected.match(/"GET, POST, [A-Z, ]+"/)[0];
    await fs.writeFile(regressionPath, `package middleware
import ("net/http"; "net/http/httptest"; "strings"; "testing")
func TestNimbusRepairAllowedMethods(t *testing.T) {
 for _, method := range strings.Split(${methodList}, ", ") {
  request := httptest.NewRequest(http.MethodOptions, "/", nil)
  request.Header.Set("Origin", "http://localhost:3000")
  request.Header.Set("Access-Control-Request-Method", method)
  response := httptest.NewRecorder()
  CORS("http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(response, request)
  found := false
  for _, allowed := range strings.Split(response.Header().Get("Access-Control-Allow-Methods"), ",") {
   if strings.TrimSpace(allowed) == method { found = true }
  }
  if !found { t.Errorf("Nimbus regression: missing allowed method %s", method) }
 }
}
`);
    command(checkout,"gofmt",["-w",regressionRelative]);
    const before = spawnSync("go",["test","./internal/middleware","-run","^TestNimbusRepairAllowedMethods$","-count=1"], {cwd:path.join(checkout,"backend"),encoding:"utf8",timeout:120000,maxBuffer:1048576});
    if (before.error || before.status !== 1 || !before.stdout?.includes("Nimbus regression: missing allowed method")) throw new Error("Could not reproduce the missing CORS method with the regression test.");
    command(checkout,"git",["apply","--check","--",patchPath]);
    command(checkout,"git",["apply","--",patchPath]);
    if (command(checkout,"git",["diff","--name-only"]) !== relative || await fs.readFile(path.join(checkout,relative),"utf8") !== expected) throw new Error("Patch is not exactly the supported CORS method repair.");
    // Re-run checks now. Imported validation metadata is never treated as proof.
    command(path.join(checkout,"backend"),"go",["test","./internal/middleware","-count=1"],120000);
    command(path.join(checkout,"backend"),"go",["build","-o",path.join(temporary,"nimbus-api"),"./cmd/api"],120000);
    const branch = `nimbus/cors-${review.base_commit.slice(0,8)}-${hash(patch).slice(0,8)}`;
    const body = `Nimbus proposes a bounded CORS repair.\n\nBase commit: ${review.base_commit}\nPatch SHA-256: ${hash(patch)}\n\nValidation re-run in a temporary checkout:\n- New CORS regression test failed before the repair and passed afterward.\n- Middleware tests passed.\n- API build passed.\n- Only the CORS method list changed.\n\nThis is static route detection, not a confirmed diagnosis of a browser failure. Review and merge manually. No deployment is triggered by this tool.\n`;
    await fs.writeFile(path.join(directory,"pr-body.md"),body);
    const plan = {base_commit: review.base_commit, patch_sha256: hash(patch), branch, checked_at: new Date().toISOString(), checks_passed: true, regression_reproduced: true, published: false};
    if (flags.includes("--publish")) {
      const repo = JSON.parse(command(root,"gh",["repo","view","--json","nameWithOwner,defaultBranchRef"]));
      if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repo.nameWithOwner) || !repo.defaultBranchRef?.name) throw new Error("Could not identify the GitHub repository and default branch.");
      const base = repo.defaultBranchRef.name;
      const remote = command(root,"git",["remote","get-url","origin"]);
      if (![ `https://github.com/${repo.nameWithOwner}`, `https://github.com/${repo.nameWithOwner}.git`, `git@github.com:${repo.nameWithOwner}.git` ].includes(remote)) throw new Error("Origin does not match the authenticated GitHub repository.");
      const remoteHead = command(root,"git",["ls-remote","origin",`refs/heads/${base}`]).split(/\s/)[0];
      if (remoteHead !== review.base_commit) throw new Error("Default branch has advanced. Refresh the repair before publishing.");
      command(checkout,"git",["switch","-c",branch]);
      command(checkout,"git",["add","--",relative,regressionRelative]);
      command(checkout,"git",["-c","commit.gpgsign=false","commit","-m","Fix missing CORS route methods"]);
      command(checkout,"git",["push","origin",`HEAD:refs/heads/${branch}`],60000);
      plan.branch_pushed = true;
      await fs.writeFile(path.join(directory,"pr-plan.json"),JSON.stringify(plan,null,2)+"\n");
      plan.url = command(root,"gh",["pr","create","--repo",repo.nameWithOwner,"--base",base,"--head",branch,"--draft","--title","Fix missing CORS route methods","--body-file",path.join(directory,"pr-body.md")],60000);
      plan.published = true;
    }
    await fs.writeFile(path.join(directory,"pr-plan.json"),JSON.stringify(plan,null,2)+"\n");
    console.log(plan.url || `Checks passed. Review ${path.join(directory,"pr-body.md")}. Re-run with --publish to push a branch and create a draft PR.`);
  } finally {
    if (added) command(root,"git",["worktree","remove","--force",checkout]);
    await fs.rm(temporary,{recursive:true,force:true});
  }
}
module.exports = {expectedRepair};
if (require.main === module) main().catch(error => {console.error(error.message); process.exitCode=1;});
