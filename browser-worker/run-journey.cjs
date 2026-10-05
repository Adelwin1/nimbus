const fs = require("node:fs/promises");
const { chromium } = require("playwright");

async function main() {
  const file = process.argv[2];
  if (!file) throw new Error("Usage: node run-journey.cjs journey.json");

  const journey = JSON.parse(await fs.readFile(file, "utf8"));
  const base = new URL(journey.base_url);
  if (!["http:", "https:"].includes(base.protocol) ||
      base.username || base.password) {
    throw new Error("Use an HTTP(S) URL without embedded credentials.");
  }
  if (!Array.isArray(journey.steps) ||
      journey.steps.length < 1 || journey.steps.length > 20) {
    throw new Error("A journey needs between 1 and 20 steps.");
  }

  const supported = new Set([
    "navigate", "click", "fill", "expect_text", "expect_visible"
  ]);
  for (const step of journey.steps) {
    if (!supported.has(step.action)) {
      throw new Error(`Unsupported action: ${step.action}`);
    }
    const field = step.action === "navigate" ? "path" : "selector";
    if (typeof step[field] !== "string" || !step[field].trim()) {
      throw new Error(`Every ${step.action} step needs ${field}.`);
    }
    if (["fill", "expect_text"].includes(step.action) &&
        typeof step.value !== "string") {
      throw new Error(`${step.action} needs a string value.`);
    }
    if (step.action === "navigate" &&
        new URL(step.path, base).origin !== base.origin) {
      throw new Error("Navigation steps must use the application's origin.");
    }
  }

  const browser = await chromium.launch({ headless: true });
  const report = {
    name: journey.name || "Browser journey",
    started_at: new Date().toISOString(),
    passed: false,
    steps: []
  };


  const evidence = {
    console_errors: 0,
    page_errors: 0,
    failed_requests: [],
    screenshot: null
  };
  report.evidence = evidence;
  let context;
  let timedOut = false;
  const deadline = setTimeout(() => {
    timedOut = true;
    void browser.close().catch(() => {});
  }, 60000);

  try {
    context = await browser.newContext({
      viewport: { width: 1280, height: 720 },
      permissions: [],
      serviceWorkers: "block",
      acceptDownloads: false
    });

    await context.route("**/*", async route => {
      try {
        const target = new URL(route.request().url());
        if (target.origin === base.origin &&
            ["http:", "https:"].includes(target.protocol) &&
            !target.username && !target.password) {
          return await route.continue();
        }
      } catch {}
      await route.abort("blockedbyclient");
    });
    const page = await context.newPage();

    function recordRequest(request, status, kind) {
      if (evidence.failed_requests.length >= 20) return;
      try {
        const url = new URL(request.url());
        // Exclude query parameters, fragments, headers and request bodies.
        const pathname = url.pathname
          .replace(/[0-9a-f]{8}-[0-9a-f-]{27,}/gi, "[id]")
          .replace(/[^/]{40,}/g, "[redacted]")
          .slice(0, 160);
        evidence.failed_requests.push({
          method: request.method(),
          endpoint: url.origin + pathname,
          status,
          kind,
          resource_type: request.resourceType()
        });
      } catch {}
    }
    page.on("console", message => {
      if (message.type() === "error") evidence.console_errors++;
    });
    page.on("pageerror", () => { evidence.page_errors++; });
    page.on("requestfailed", request => {
      recordRequest(request, null, "network_error");
    });
    page.on("response", response => {
      if (response.status() >= 400) {
        recordRequest(response.request(), response.status(), "http_error");
      }
    });
    page.setDefaultTimeout(8000);
    page.setDefaultNavigationTimeout(15000);

    for (const [index, step] of journey.steps.entries()) {
      const started = Date.now();
      const result = {
        number: index + 1,
        action: step.action,
        passed: false
      };
      try {
        switch (step.action) {
          case "navigate": {
            const response = await page.goto(
              new URL(step.path, base).href,
              { waitUntil: "domcontentloaded" }
            );
            if (!response || response.status() >= 400) {
              throw new Error("Navigation returned an unsuccessful response.");
            }
            break;
          }
          case "click":
            await page.locator(step.selector).click();
            break;
          case "fill":
            await page.locator(step.selector).fill(step.value);
            break;
          case "expect_visible":
            await page.locator(step.selector).waitFor({ state: "visible" });
            break;
          case "expect_text": {
            const locator = page.locator(step.selector);
            await locator.waitFor({ state: "visible" });
            const end = Date.now() + 8000;
            let matches = false;
            while (Date.now() < end) {
              if ((await locator.innerText()).includes(step.value)) {
                matches = true;
                break;
              }
              await page.waitForTimeout(100);
            }
            if (!matches) throw new Error("Expected text did not appear.");
            break;
          }
        }
        result.passed = true;

      } catch {
        if (!timedOut) {
          try {
            const screenshot = await page.screenshot({
              type: "jpeg",
              quality: 35,
              fullPage: false,
              timeout: 3000,
              mask: [
                page.locator("input"),
                page.locator("textarea"),
                page.locator("[contenteditable]"),
                page.locator("[data-sensitive]")
              ]
            });
            if (screenshot.length <= 131072) {
              evidence.screenshot = "data:image/jpeg;base64," +
                screenshot.toString("base64");
            }
          } catch {}
        }
        result.reason = timedOut
          ? "Journey exceeded its 60-second limit."
          : `Step failed: ${step.action}. Check the target element, page response, and expected result.`;
      }
      result.duration_ms = Date.now() - started;
      report.steps.push(result);
      if (!result.passed) break;
    }

    report.passed = !timedOut &&
      report.steps.length === journey.steps.length &&
      report.steps.every(step => step.passed);
  } finally {
    clearTimeout(deadline);
    if (context) await context.close().catch(() => {});
    await browser.close().catch(() => {});
  }

  report.finished_at = new Date().toISOString();
  console.log(JSON.stringify(report, null, 2));
  if (!report.passed) process.exitCode = 1;
}

main().catch(() => {
  console.error("Journey could not run. Check the configuration and browser installation.");
  process.exitCode = 1;
});
