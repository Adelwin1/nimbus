function analyzeJourney(report) {
  const facts = [];
  const recommendations = [];
  const evidence = report.evidence || {};
  const failed = (report.steps || []).find(step => !step.passed);
  const requests = evidence.failed_requests || [];
  const add = (hypothesis, action) => recommendations.push({ hypothesis, action });

  if (failed) {
    facts.push(`Browser step ${failed.number} (${failed.action}) failed.`);
  }
  for (const request of requests.slice(0, 5)) {
    facts.push(`${request.method} ${request.endpoint}: ${request.status ?? "request did not complete"}.`);
  }
  if (evidence.page_errors > 0) {
    facts.push(`${evidence.page_errors} uncaught browser error(s) occurred.`);
    add(
      "A frontend JavaScript exception may have interrupted the flow.",
      "Inspect the browser error stack and application error reports for this run's time window."
    );
  }
  if (evidence.console_errors > 0) {
    facts.push(`${evidence.console_errors} console error(s) occurred.`);
  }
  if (requests.some(r => r.status >= 500)) {
    add(
      "A server or upstream dependency error may have affected the journey.",
      "Inspect server logs for the listed endpoint and run timestamp. Check dependency readiness and recent releases."
    );
  }
  if (requests.some(r => [401, 403].includes(r.status))) {
    add(
      "Authentication or authorization may have blocked a request.",
      "Check the test account's session, permissions, and login steps."
    );
  }
  if (requests.some(r => r.status === 404)) {
    add(
      "A requested route or resource may be missing.",
      "Verify the endpoint and deployment version. Check whether the route changed or the resource exists."
    );
  }
  if (requests.some(r => r.status === 429)) {
    add(
      "Rate limiting may have rejected the test.",
      "Inspect the rate-limit policy and reduce test frequency before retrying."
    );
  }
  if (requests.some(r => r.kind === "network_error")) {
    add(
      "Connectivity, cancellation, or the worker's origin restriction may have blocked a request.",
      "Check endpoint reachability and whether it uses a different origin. This worker blocks cross-origin requests."
    );
  }
  if (failed && ["click", "fill", "expect_visible", "expect_text"].includes(failed.action)) {
    add(
      "The target element or expected content may have changed, or the page may not have reached the intended state.",
      "Compare the failure screenshot with the selector and expected result. Check preceding steps before changing the assertion."
    );
  }
  if (failed && !recommendations.length) {
    add(
      "The available evidence does not establish a cause.",
      "Review the failed step, target URL, worker output, and timeout limits."
    );
  }

  const relatedErrors = evidence.application_errors || [];
  for (const error of relatedErrors.slice(0, 5)) {
    const versions = (error.releases || []).join(", ") || "unknown release";
    facts.push(
      `${error.occurrences} application error report(s) received during this run: ` +
      `${error.error_type} / ${error.error_code}; reported releases: ${versions}.`
    );
  }
  if (failed && relatedErrors.length) {
    add(
      "These application errors overlap the journey's execution window. Timing alone does not confirm they caused the failure.",
      "Compare the reported stack frames, failed request, and server logs. Verify that the reports came from the same environment and request flow."
    );
    if (relatedErrors.some(error =>
      /database|db_|sql|connection/i.test(error.error_type + " " + error.error_code)
    )) {
      add(
        "A reported database or connection error may be relevant to the failed flow.",
        "Check database readiness, connection limits, and server-side database configuration. Confirm the connection error belongs to the failed request."
      );
    }
  }
  if (evidence.error_correlation_available === false) {
    facts.push("Application error correlation was unavailable for this run.");
  }
  return {
    facts,
    recommendations,
    conclusion: report.passed
      ? "All configured journey steps passed. Untested behavior may still fail."
      : "The journey failed. These observations do not establish a confirmed root cause."
  };
}
module.exports = { analyzeJourney };
