// Server-side only. Requires a Node runtime with fetch.
async function reportNimbusError(event) {
  const endpoint = process.env.NIMBUS_ERROR_ENDPOINT;
  const token = process.env.NIMBUS_ERROR_TOKEN;
  if (!endpoint || !token) return false;

  try {
    const url = new URL(endpoint);
    if (!["http:", "https:"].includes(url.protocol) ||
        url.username || url.password) return false;

    const response = await fetch(url, {
      method: "POST",
      redirect: "error",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json"
      },
      body: JSON.stringify({
        error_type: event.error_type,
        error_code: event.error_code,
        release_version: event.release_version || "",
        frames: event.frames || []
      }),
      signal: AbortSignal.timeout(3000)
    });
    return response.status === 202;
  } catch {
    // Reporting failures must not crash the application.
    return false;
  }
}
module.exports = { reportNimbusError };

if (require.main === module) {
  reportNimbusError({
    error_type: "DemoDatabaseError",
    error_code: "DEMO_DB_UNAVAILABLE",
    release_version: "demo-v1",
    frames: [{
      file: "src/demo/database.js",
      function: "connectDatabase",
      line: 42
    }]
  }).then(recorded => {
    console.log(recorded ? "Demo error recorded." : "Report failed. Check endpoint, token, and API availability.");
    if (!recorded) process.exitCode = 1;
  });
}
