const test = require("node:test");
const assert = require("node:assert/strict");
const http = require("node:http");
const fs = require("node:fs/promises");
const os = require("node:os");
const path = require("node:path");
const { spawn } = require("node:child_process");

function run(file) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ["run-journey.cjs", file], {
      cwd: __dirname,
      stdio: ["ignore", "pipe", "pipe"]
    });
    let output = "";
    let errors = "";
    child.stdout.on("data", chunk => output += chunk);
    child.stderr.on("data", chunk => errors += chunk);
    child.on("error", reject);
    child.on("close", code => {
      try {
        resolve({ code, report: JSON.parse(output) });
      } catch {
        reject(new Error(errors || "Runner returned invalid JSON."));
      }
    });
  });
}

test("browser journey clicks, verifies results, and stops on failure",
  { timeout: 90000 }, async () => {
    const server = http.createServer((req, res) => {
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(`<!doctype html>
        <html><body>
          <h1>Test shop</h1>
          <button id="add">Add to cart</button>
          <p id="cart">Cart empty</p>
          <script>
            document.querySelector("#add").onclick = () => {
              document.querySelector("#cart").textContent = "1 item in cart";
            };
          </script>
        </body></html>`);
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    const directory = await fs.mkdtemp(path.join(os.tmpdir(), "nimbus-journey-"));

    try {
      const base_url = `http://127.0.0.1:${server.address().port}`;
      const steps = [
        { action: "navigate", path: "/" },
        { action: "click", selector: "#add" },
        { action: "expect_text", selector: "#cart", value: "1 item in cart" }
      ];
      const file = path.join(directory, "journey.json");

      await fs.writeFile(file, JSON.stringify({
        name: "Working cart", base_url, steps
      }));
      const good = await run(file);
      assert.equal(good.code, 0);
      assert.equal(good.report.passed, true);
      assert.equal(good.report.steps.length, 3);

      await fs.writeFile(file, JSON.stringify({
        name: "Broken expectation",
        base_url,
        steps: [
          ...steps.slice(0, 2),
          { action: "expect_text", selector: "#cart", value: "Order completed" },
          { action: "click", selector: "#add" }
        ]
      }));
      const bad = await run(file);
      assert.equal(bad.code, 1);
      assert.equal(bad.report.passed, false);
      assert.equal(bad.report.steps.length, 3);
      assert.equal(bad.report.steps[2].passed, false);

      console.log("Verified: cart interaction passes; incorrect result fails at step 3.");
    } finally {
      await new Promise(resolve => server.close(resolve));
      await fs.rm(directory, { recursive: true, force: true });
    }
  });
