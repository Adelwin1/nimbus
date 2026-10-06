"use client";

import Image from "next/image";

import { useEffect, useRef, useState } from "react";
import { apiRequest } from "@/lib/api";

type Journey = {
  id: string;
  name: string;
  base_url: string;
  enabled: boolean;
};
type SourceInvestigation = {
 repository: string; commit_sha: string; baseline_commit?: string;
 candidates: {path: string; url: string; changed_since_baseline: boolean; matching_lines: number[]; reasons: string[]}[];
 facts: string[]; actions: string[]; limitations: string[];
};
type Run = {
  id: string;
  status: string;
  source_investigation?: SourceInvestigation | null;
  queued_at: string;
  release_context?: { repository?: string; commit_sha?: string; preview_url?: string; preview_commit_binding?: string };
  error_message?: string | null;
  result?: {

    evidence?: {
      console_errors: number;
      page_errors: number;
      failed_requests: {
        method: string;
        endpoint: string;
        status: number | null;
        kind: string;
        resource_type: string;
      }[];
      screenshot: string | null;
    };

    analysis?: {
      facts: string[];
      recommendations: { hypothesis: string; action: string }[];
      conclusion: string;
    };
    steps: {
      number: number;
      action: string;
      passed: boolean;
      duration_ms: number;
      reason?: string;
    }[];
  } | null;
};

const initialSteps = JSON.stringify([
  { action: "navigate", path: "/login" },
  { action: "expect_visible", selector: "input[type='email']" },
  { action: "expect_visible", selector: "input[type='password']" },
  { action: "expect_visible", selector: "button[type='submit']" },
], null, 2);

const inputClass =
  "mt-2 w-full rounded-md border border-white/15 bg-[#090c10] p-3 text-sm text-white";
const buttonClass =
  "rounded-md border border-white/15 px-4 py-2 text-sm disabled:opacity-40";

export function JourneyPanel({ applicationId }: { applicationId: string }) {
  const [journeys, setJourneys] = useState<Journey[]>([]);
  const [selected, setSelected] = useState("");
  const [runs, setRuns] = useState<Run[]>([]);
  const [name, setName] = useState("Login page journey");
  const [baseURL, setBaseURL] = useState("http://localhost:3000");
  const [steps, setSteps] = useState(initialSteps);
  const [commitSHA, setCommitSHA] = useState("");
  const [previewURL, setPreviewURL] = useState("");
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [revision, setRevision] = useState(0);
  const [historyError, setHistoryError] = useState("");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const data = await apiRequest<{ journeys: Journey[] }>(
          `/journeys/apps/${applicationId}`, { authenticated: true },
        );
        if (!cancelled) {
          setJourneys(data.journeys);
          setSelected(current =>
            data.journeys.some(journey => journey.id === current)
              ? current : data.journeys[0]?.id || "",
          );
        }
      } catch {
        if (!cancelled) setError("Journeys could not be loaded.");
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [applicationId, revision]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      if (!selected) return;
      try {
        const data = await apiRequest<{ runs: Run[] }>(
          `/journeys/${selected}/runs`, { authenticated: true },
        );
        if (!cancelled) {
          setRuns(data.runs);
          setHistoryError("");
        }
      } catch {
        if (!cancelled) setHistoryError("Run history could not be refreshed.");
      } finally {
        if (!cancelled) timer = setTimeout(() => { void poll(); }, 3000);
      }
    }
    void poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [selected, revision]);

  async function save() {
    if (lock.current) return;
    let parsed: unknown;
    try {
      parsed = JSON.parse(steps);
      if (!Array.isArray(parsed)) throw new Error();
    } catch {
      setError("Steps must be a valid JSON array.");
      return;
    }
    lock.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const data = await apiRequest<{ id: string }>(
        `/journeys/apps/${applicationId}`,
        {
          authenticated: true,
          method: "POST",
          body: JSON.stringify({ name, base_url: baseURL, steps: parsed }),
        },
      );
      setSelected(data.id);
      setRuns([]);
      setRevision(value => value + 1);
      setMessage("Journey saved. Select Run journey to execute it.");
    } catch {
      setError("Journey could not be saved. Check the URL and step definitions.");
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }

  async function run() {
    if (lock.current || !selected) return;
    lock.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await apiRequest(`/journeys/${selected}/runs`, {
        authenticated: true, method: "POST",
        body: JSON.stringify({commit_sha:commitSHA.trim(),preview_url:previewURL.trim()}),
      });
      setRevision(value => value + 1);
      setMessage("Run queued. The browser worker will execute it.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Run could not be queued.");
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }

  async function investigate(runId: string) {
    if (lock.current) return;
    lock.current = true; setBusy(true); setError("");
    try {
      await apiRequest(`/github/runs/${runId}/investigate`, {authenticated:true, method:"POST"});
      setRevision(value => value + 1);
      setMessage("Source investigation saved. Candidates are leads, not confirmed causes.");
    } catch (err) {setError(err instanceof Error ? err.message : "Source investigation unavailable.");}
    finally {lock.current = false; setBusy(false);}
  }

  const latest = selected ? runs[0] : undefined;
  const pending = runs.some(run => ["queued", "running"].includes(run.status));

  return (
    <section className="mt-8 rounded-md border border-white/10 bg-[#0d1117] p-6">
      <p className="text-xs uppercase tracking-widest text-teal-300">Browser monitoring</p>
      <h2 className="mt-2 text-xl font-semibold">User journeys</h2>
      <p className="mt-2 text-sm text-slate-400">
        Run browser actions and verify the result. Journeys currently run on demand,
        separately from HTTP health checks. Use test accounts; input values are stored.
      </p>

      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <div className="space-y-4">
          <label className="block text-sm">
            Journey name
            <input className={inputClass} value={name} maxLength={120}
              onChange={event => setName(event.target.value)} />
          </label>
          <label className="block text-sm">
            Application origin
            <input className={inputClass} value={baseURL}
              onChange={event => setBaseURL(event.target.value)} />
          </label>
          <label className="block text-sm">
            Browser steps (JSON)
            <textarea className={`${inputClass} font-mono`} rows={12}
              value={steps} onChange={event => setSteps(event.target.value)} />
          </label>
          <p className="text-xs text-slate-400">
            Start with navigate. Available actions: navigate, click, fill,
            expect_visible, expect_text. Maximum 20 steps.
          </p>
          <button type="button" className={buttonClass} disabled={busy}
            onClick={() => { void save(); }}>Save new journey</button>
        </div>

        <div>
          <label className="block text-sm">
            Saved journey
            <select className={inputClass} value={selected}
              onChange={event => {
                setSelected(event.target.value);
                setRuns([]);
                setHistoryError("");
              }}>
              <option value="">Select a journey</option>
              {journeys.map(journey =>
                <option key={journey.id} value={journey.id}>{journey.name}</option>,
              )}
            </select>
          </label>
          <div className="mt-4 space-y-3 rounded-md border border-white/10 p-4">
            <h3 className="text-sm font-semibold">Release context (optional)</h3>
            <label className="block text-sm">Full commit SHA
              <input className={inputClass} value={commitSHA} maxLength={40} disabled={busy || pending}
                onChange={event => setCommitSHA(event.target.value)} placeholder="40-character Git commit SHA" />
            </label>
            <label className="block text-sm">Preview origin
              <input className={inputClass} value={previewURL} maxLength={2048} disabled={busy || pending}
                onChange={event => setPreviewURL(event.target.value)} placeholder="https://your-preview.example.com" />
            </label>
            <p className="text-xs text-slate-400">Provide both fields or leave both blank. Nimbus verifies the commit in the linked repository. You are responsible for confirming the preview serves that commit. The worker must allow this origin.</p>
          </div>
          <button type="button"
            className="mt-4 rounded-md bg-teal-400 px-4 py-2 text-sm font-medium text-black disabled:opacity-40"
            disabled={busy || !selected || pending}
            onClick={() => { void run(); }}>
            {pending ? "Test pending" : "Run journey"}
          </button>

          {historyError && <p role="alert" className="mt-4 text-sm text-rose-300">{historyError}</p>}
          {latest ? (
            <div className="mt-6">
              <p className="font-mono text-sm">
                Latest run: <strong>{latest.status}</strong>
              </p>
              {latest.release_context?.commit_sha && <div className="mt-3 break-all rounded-md border border-white/10 p-3 text-xs text-slate-400">
                <p>Repository: {latest.release_context.repository}</p>
                <p>Commit: {latest.release_context.commit_sha}</p>
                <p>Preview: {latest.release_context.preview_url}</p>
                <p className="mt-2">Preview-to-commit association supplied by the operator.</p>
              </div>}
              {latest.status === "failed" && latest.release_context?.commit_sha && <button type="button" className={`mt-4 ${buttonClass}`} disabled={busy}
                onClick={() => {void investigate(latest.id);}}>Investigate source</button>}
              {latest.source_investigation && <div className="mt-4 rounded-md border border-white/10 p-4">
                <h3 className="font-semibold">Source investigation</h3>
                <p className="mt-2 text-xs text-slate-400">Commit: {latest.source_investigation.commit_sha}</p>
                {latest.source_investigation.baseline_commit && <p className="text-xs text-slate-400">Earlier passing commit: {latest.source_investigation.baseline_commit}</p>}
                <ul className="mt-3 space-y-2 text-sm">{latest.source_investigation.facts.map((fact,index)=><li key={index}>{fact}</li>)}</ul>
                <h4 className="mt-4 text-sm font-semibold">Candidate source files</h4>
                {latest.source_investigation.candidates.map(file=><div key={file.path} className="mt-3 text-sm">
                  <a className="break-all text-teal-300" href={file.url} target="_blank" rel="noopener noreferrer">{file.path}</a>
                  <p className="text-xs text-slate-400">Matching lines: {file.matching_lines.join(", ") || "none"}</p>
                  <ul className="mt-1 text-xs text-slate-400">{file.reasons.map((reason,index)=><li key={index}>{reason}</li>)}</ul>
                </div>)}
                <h4 className="mt-4 text-sm font-semibold">Next steps</h4>
                <ul className="mt-2 space-y-2 text-sm">{latest.source_investigation.actions.map((action,index)=><li key={index}>{action}</li>)}</ul>
                <h4 className="mt-4 text-sm font-semibold">Limits of this investigation</h4>
                <ul className="mt-2 space-y-2 text-xs text-slate-400">{latest.source_investigation.limitations.map((limit,index)=><li key={index}>{limit}</li>)}</ul>
              </div>}
              {latest.error_message &&
                <p className="mt-2 text-sm text-rose-300">{latest.error_message}</p>}
              <ol className="mt-4 space-y-3">
                {latest.result?.steps.map(step => (
                  <li key={step.number} className="rounded-md border border-white/10 p-3 text-sm">
                    <span className={step.passed ? "text-teal-300" : "text-rose-300"}>
                      Step {step.number}: {step.action} — {step.passed ? "passed" : "failed"}
                    </span>
                    <span className="ml-2 text-slate-400">{step.duration_ms} ms</span>
                    {step.reason && <p className="mt-2 text-slate-300">{step.reason}</p>}
                  </li>
                ))}
              </ol>

              {latest.result?.evidence && (
                <div className="mt-6">
                  <h3 className="text-sm font-semibold">Failure evidence</h3>
                  <p className="mt-2 text-sm text-slate-400">
                    Console errors: {latest.result.evidence.console_errors}
                    {" · "}Uncaught browser errors: {latest.result.evidence.page_errors}
                  </p>
                  <ul className="mt-3 space-y-2">
                    {latest.result.evidence.failed_requests.map((request, index) => (
                      <li key={index} className="break-all rounded-md border border-white/10 p-3 font-mono text-xs">
                        {request.method} {request.endpoint}
                        {" — "}{request.status ?? request.kind}
                      </li>
                    ))}
                  </ul>
                  {latest.result.evidence.screenshot && (
                    <Image
                      src={latest.result.evidence.screenshot}
                      alt="Browser viewport when the journey step failed"
                      width={1280}
                      height={720}
                      unoptimized
                      className="mt-4 h-auto w-full rounded-md border border-white/10"
                    />
                  )}
                </div>
              )}

              {latest.result?.analysis && (
                <div className="mt-6 rounded-md border border-white/10 p-4">
                  <h3 className="text-sm font-semibold">Investigation guidance</h3>
                  <p className="mt-2 text-sm text-slate-400">
                    {latest.result.analysis.conclusion}
                  </p>
                  <h4 className="mt-4 text-xs uppercase text-teal-300">Observed facts</h4>
                  <ul className="mt-2 space-y-2 text-sm">
                    {latest.result.analysis.facts.map((fact, index) =>
                      <li key={index}>{fact}</li>
                    )}
                  </ul>
                  <h4 className="mt-4 text-xs uppercase text-slate-400">Possible causes and next steps</h4>
                  <ul className="mt-2 space-y-3 text-sm">
                    {latest.result.analysis.recommendations.map((item, index) => (
                      <li key={index}>
                        <p className="text-slate-300">{item.hypothesis}</p>
                        <p className="mt-1 text-teal-200">{item.action}</p>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              <h3 className="mt-6 text-sm font-semibold">Recent runs</h3>
              <ul className="mt-2 space-y-2 text-sm text-slate-400">
                {runs.map(run => (
                  <li key={run.id}>
                    {new Date(run.queued_at).toLocaleString()} — {run.status}{run.release_context?.commit_sha ? ` · ${run.release_context.commit_sha.slice(0, 8)}` : ""}
                  </li>
                ))}
              </ul>
            </div>
          ) : <p className="mt-6 text-sm text-slate-400">No runs to show yet.</p>}
        </div>
      </div>
      {error && <p role="alert" className="mt-4 text-sm text-rose-300">{error}</p>}
      {message && <p role="status" className="mt-4 text-sm text-teal-300">{message}</p>}
    </section>
  );
}
