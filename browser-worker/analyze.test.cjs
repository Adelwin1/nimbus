const test = require("node:test");
const assert = require("node:assert/strict");
const { analyzeJourney } = require("./analyze.cjs");

test("server failure gives qualified investigation guidance", () => {
  const result = analyzeJourney({
    passed: false,
    steps: [{ number: 3, action: "expect_text", passed: false }],
    evidence: {
      failed_requests: [{
        method: "POST",
        endpoint: "https://example.com/api/cart",
        status: 500,
        kind: "http_error"
      }]
    }
  });
  assert.ok(result.facts.some(fact => fact.includes("/api/cart")));
  assert.ok(result.recommendations.some(item => item.action.includes("server logs")));
  assert.ok(result.conclusion.includes("do not establish"));
});

test("missing element does not invent a server failure", () => {
  const result = analyzeJourney({
    passed: false,
    steps: [{ number: 2, action: "expect_visible", passed: false }]
  });
  assert.ok(result.recommendations.some(item => item.action.includes("screenshot")));
  assert.equal(result.facts.some(fact => fact.includes("500")), false);
});

test("passing run does not claim the application is bug-free", () => {
  const result = analyzeJourney({ passed: true, steps: [] });
  assert.equal(result.recommendations.length, 0);
  assert.ok(result.conclusion.includes("Untested behavior"));
});

test("overlapping application errors remain correlation, not a diagnosis", () => {
  const result = analyzeJourney({
    passed: false,
    steps: [{ number: 2, action: "expect_visible", passed: false }],
    evidence: {
      error_correlation_available: true,
      application_errors: [{
        error_type: "DemoDatabaseError",
        error_code: "DEMO_DB_UNAVAILABLE",
        occurrences: 2,
        releases: ["demo-v1"]
      }]
    }
  });
  assert.ok(result.facts.some(fact => fact.includes("demo-v1")));
  assert.ok(result.recommendations.some(item =>
    item.hypothesis.includes("Timing alone does not confirm")));
  assert.ok(result.recommendations.some(item =>
    item.action.includes("database readiness")));
});
