const { analyzeJourney } = require("./analyze.cjs");
const { Pool } = require("pg");
const { randomUUID } = require("node:crypto");
const { spawn } = require("node:child_process");
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");

const { jobOptions } = require("../hosted-workers/job.cjs");
const hosted = jobOptions();
const database = process.env.DATABASE_URL;
const allowed = new Set(
  (process.env.JOURNEY_ALLOWED_ORIGINS || "")
    .split(",").map(value => value.trim()).filter(Boolean)
);
if (!database || !allowed.size) {
  console.error("Set DATABASE_URL and JOURNEY_ALLOWED_ORIGINS.");
  process.exit(1);
}
const pool = new Pool({
  connectionString: database,
  max: 2,
  connectionTimeoutMillis: 5000,
  query_timeout: 10000
});
let stopping = false;
process.on("SIGINT", () => { stopping = true; });
process.on("SIGTERM", () => { stopping = true; });

function execute(file) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ["run-journey.cjs", file], {
      cwd: __dirname,
      env: Object.fromEntries(
        ["PATH", "HOME", "TMPDIR", "TEMP", "TMP", "LANG",
         "PLAYWRIGHT_BROWSERS_PATH", "SystemRoot"]
          .filter(key => process.env[key] !== undefined)
          .map(key => [key, process.env[key]])
      ),
      stdio: ["ignore", "pipe", "pipe"]
    });
    let output = "";
    let overflow = false;
    const timer = setTimeout(() => child.kill("SIGKILL"), 75000);
    child.stdout.on("data", chunk => {
      output += chunk;
      if (Buffer.byteLength(output) > 393216) {
        overflow = true;
        child.kill("SIGKILL");
      }
    });
    // Do not store raw stderr: it can contain sensitive input.
    child.stderr.resume();
    child.on("error", error => {
      clearTimeout(timer);
      reject(error);
    });
    child.on("close", code => {
      clearTimeout(timer);
      try {
        if (overflow) throw new Error();
        const report = JSON.parse(output);
        if (typeof report.passed !== "boolean" ||
            !Array.isArray(report.steps) ||
            ![0, 1].includes(code) ||
            report.passed !== (code === 0)) {
          throw new Error();
        }
        resolve(report);
      } catch {
        reject(new Error("Browser runner did not return a valid result."));
      }
    });
  });
}

async function cycle() {
  // An interrupted run is recorded as an error, not silently repeated.
  await pool.query(`
    UPDATE browser_journey_runs
    SET status='error', finished_at=now(),
        error_message='Worker stopped or exceeded its execution lease.',
        lease_token=NULL, lease_expires_at=NULL
    WHERE status='running' AND lease_expires_at < now()
  `);

  const token = randomUUID();
  const { rows } = await pool.query(`
    WITH next_run AS (
      SELECT id FROM browser_journey_runs
      WHERE status='queued' AND ($2::uuid IS NULL OR id=$2::uuid)
      ORDER BY queued_at
      FOR UPDATE SKIP LOCKED LIMIT 1
    )
    UPDATE browser_journey_runs r
    SET status='running', started_at=now(),
        lease_token=$1, lease_expires_at=now()+interval '120 seconds'
    FROM next_run n WHERE r.id=n.id
    RETURNING r.id,r.definition,r.release_context
  `, [token, hosted.id]);

  if (!rows.length) return;
  const job = rows[0];
  let directory;
  let report = null;
  let status = "error";
  let message = "Browser journey could not execute.";

  try {
    const origin = new URL(job.definition.base_url).origin;
    if (!allowed.has(origin)) {
      message = "Application origin is not permitted by this worker.";
      throw new Error(message);
    }
    directory = await fs.mkdtemp(path.join(os.tmpdir(), "nimbus-run-"));
    const file = path.join(directory, "journey.json");
    await fs.writeFile(file, JSON.stringify(job.definition), { mode: 0o600 });
    report = await execute(file);
    if (report.steps.length > job.definition.steps.length ||
        (report.passed &&
         (report.steps.length !== job.definition.steps.length ||
          !report.steps.every(step => step.passed === true)))) {
      throw new Error("Incomplete report.");
    }

    // Correlate only reports received while this run was executing.
    try {
      const related = await pool.query(`
        SELECT g.error_type,g.error_code,
               COUNT(*)::int AS occurrences,
               array_agg(DISTINCT o.release_version)
                 FILTER (WHERE o.release_version IS NOT NULL) AS releases
        FROM browser_journey_runs r
        JOIN browser_journeys j ON j.id=r.journey_id
        JOIN application_error_groups g ON g.application_id=j.application_id
        JOIN application_error_occurrences o ON o.group_id=g.id
        WHERE r.id=$1 AND r.lease_token=$2 AND r.status='running'
          AND o.received_at >= r.started_at
          AND o.received_at <= now()
        GROUP BY g.id,g.error_type,g.error_code
        ORDER BY COUNT(*) DESC,g.id
        LIMIT 5
      `, [job.id, token]);
      report.evidence = report.evidence || {};
      report.evidence.application_errors = related.rows;
      report.evidence.error_correlation_available = true;
    } catch {
      report.evidence = report.evidence || {};
      report.evidence.error_correlation_available = false;
    }
    report.release_context = job.release_context || {};
    report.analysis = analyzeJourney(report);
    status = report.passed ? "passed" : "failed";
    message = null;
  } catch {
    // Persist only controlled messages, never credentials or raw exceptions.
  } finally {
    if (directory) await fs.rm(directory, { recursive: true, force: true });
  }

  await pool.query(`
    UPDATE browser_journey_runs
    SET status=$3,result=$4::jsonb,error_message=$5,
        finished_at=now(),lease_token=NULL,lease_expires_at=NULL
    WHERE id=$1 AND status='running' AND lease_token=$2
  `, [job.id, token, status, report ? JSON.stringify(report) : null, message]);

  console.log(`Journey run ${job.id}: ${status}`);
}

(async () => {
  console.log("Nimbus browser worker started.");
  try {
    while (!stopping) {
      try {
        await cycle();
      } catch {
        console.error("Queue unavailable; retrying.");
        if (hosted.once) throw new Error("Hosted queue unavailable.");
      }
      if (hosted.once) break;
      if (!stopping) await new Promise(resolve => setTimeout(resolve, 2000));
    }
  } finally {
    await pool.end();
  }
})().catch(() => {
  console.error("Worker stopped unexpectedly.");
  process.exitCode = 1;
});
