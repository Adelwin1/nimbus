"use client";

import { useEffect, useRef, useState } from "react";
import { apiRequest } from "@/lib/api";

type Check = {
  name: string;
  passed: boolean;
  exit_code: number | null;
  timed_out: boolean;
};
type Review = {
  id: string;
  rule: string;
  base_commit: string;
  patch: string;
  patch_sha256: string;
  review_status: string;
  validation: {
    tests_run: boolean;
    checks_passed: boolean;
    checks: Check[];
  };
};

export function RepairReviews({ applicationId }: { applicationId: string }) {
  const [reviews, setReviews] = useState<Review[]>([]);
  const [metadata, setMetadata] = useState("");
  const [patch, setPatch] = useState("");
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const [error, setError] = useState("");
  const [loadError, setLoadError] = useState("");
  const [message, setMessage] = useState("");
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const data = await apiRequest<{ reviews: Review[] }>(
          `/repair-reviews/apps/${applicationId}`, { authenticated: true },
        );
        if (!cancelled) {
          setReviews(data.reviews);
          setLoadError("");
        }
      } catch {
        if (!cancelled) setLoadError("Repair reviews could not be loaded.");
      }
    }
    void load();
    return () => { cancelled = true; };
  }, [applicationId, revision]);

  async function readFile(file: File | undefined, kind: "metadata" | "patch") {
    if (!file) return;
    if (file.size > (kind === "patch" ? 32768 : 16384)) {
      setError("Selected file is too large.");
      return;
    }
    try {
      const text = await file.text();
      if (kind === "patch") setPatch(text);
      else setMetadata(text);
      setError("");
    } catch {
      setError("Selected file could not be read.");
    }
  }

  async function upload() {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const data = JSON.parse(metadata);
      const validation = data.validation || {};
      if (!data.rule || !data.base_commit || !patch ||
          (validation.checks !== undefined && !Array.isArray(validation.checks))) {
        throw new Error();
      }
      await apiRequest(`/repair-reviews/apps/${applicationId}`, {
        authenticated: true,
        method: "POST",
        body: JSON.stringify({
          rule: data.rule,
          base_commit: data.base_commit,
          patch,
          validation: {
            tests_run: validation.tests_run === true,
            checks_passed: validation.checks_passed === true,
            checks: (validation.checks || []).map((check: Check) => ({
              name: check.name,
              passed: check.passed === true,
              exit_code: check.exit_code ?? null,
              timed_out: check.timed_out === true,
            })),
          },
        }),
      });
      setRevision(value => value + 1);
      setMessage("Proposal imported. No source code was changed.");
    } catch {
      setError("Import failed. Check the review JSON, patch, and validation results.");
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }

  async function decide(id: string, decision: "approved" | "rejected") {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError("");
    try {
      await apiRequest(`/repair-reviews/${id}/decision`, {
        authenticated: true,
        method: "POST",
        body: JSON.stringify({ decision }),
      });
      setRevision(value => value + 1);
      setMessage(`Review ${decision}. The patch has not been applied or deployed.`);
    } catch {
      setError("Decision could not be saved. Refresh the reviews and try again.");
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }

  return (
    <section className="mt-8 rounded-md border border-white/10 bg-[#0d1117] p-6">
      <p className="text-xs uppercase tracking-widest text-teal-300">Change review</p>
      <h2 className="mt-2 text-xl font-semibold">Proposed repairs</h2>
      <p className="mt-2 text-sm text-slate-400">
        Review a generated diff and locally reported checks.
        Approval records a decision; it does not apply or deploy a change.
        Currently supports the Go CORS repair rule.
      </p>

      <details className="mt-5 rounded-md border border-white/10 p-4">
        <summary className="cursor-pointer text-sm">Import a repair proposal</summary>
        <label className="mt-4 block text-sm">
          Review metadata — review.json
          <input type="file" accept=".json" className="mt-2 block text-xs"
            onChange={event => { void readFile(event.target.files?.[0], "metadata"); }} />
        </label>
        <label className="mt-4 block text-sm">
          Proposed diff — proposed.patch
          <input type="file" accept=".patch,.diff" className="mt-2 block text-xs"
            onChange={event => { void readFile(event.target.files?.[0], "patch"); }} />
        </label>
        <button type="button" disabled={busy || !metadata || !patch}
          onClick={() => { void upload(); }}
          className="mt-4 rounded-md border border-white/15 px-4 py-2 text-sm disabled:opacity-40">
          Import proposal
        </button>
      </details>

      {loadError && <p role="alert" className="mt-4 text-sm text-rose-300">{loadError}</p>}
      {error && <p role="alert" className="mt-4 text-sm text-rose-300">{error}</p>}
      {message && <p role="status" className="mt-4 text-sm text-teal-300">{message}</p>}

      {reviews.length === 0 &&
        <p className="mt-5 text-sm text-slate-400">No repair proposals imported.</p>}
      <ul className="mt-5 space-y-5">
        {reviews.map(review => (
          <li key={review.id} className="rounded-md border border-white/10 p-4">
            <div className="flex flex-wrap justify-between gap-3 text-sm">
              <strong>{review.rule}</strong>
              <span className="font-mono text-teal-300">{review.review_status}</span>
            </div>
            <p className="mt-2 break-all font-mono text-xs text-slate-400">
              Source commit: {review.base_commit}
            </p>
            <p className="mt-2 text-xs text-slate-400">
              Checks below were reported by the local verifier.
              Nimbus has not independently executed this patch.
            </p>
            <ul className="mt-3 space-y-1 text-sm">
              {review.validation.checks.map(check => (
                <li key={check.name} className={check.passed ? "text-teal-300" : "text-rose-300"}>
                  {check.name}: {check.passed ? "passed" : "failed"}
                </li>
              ))}
              {!review.validation.tests_run && <li>Tests have not been reported.</li>}
            </ul>
            <details className="mt-4">
              <summary className="cursor-pointer text-sm">Inspect proposed diff</summary>
              <pre className="mt-3 max-h-96 overflow-auto rounded-md bg-[#090c10] p-4 text-xs">
                {review.patch}
              </pre>
              <p className="mt-2 break-all font-mono text-xs text-slate-500">
                SHA-256: {review.patch_sha256}
              </p>
            </details>
            {review.review_status === "pending" && (
              <div className="mt-4 flex gap-3">
                <button type="button"
                  disabled={busy || !review.validation.checks_passed}
                  onClick={() => { void decide(review.id, "approved"); }}
                  className="rounded-md bg-teal-400 px-4 py-2 text-sm text-black disabled:opacity-40">
                  Approve review
                </button>
                <button type="button" disabled={busy}
                  onClick={() => { void decide(review.id, "rejected"); }}
                  className="rounded-md border border-white/15 px-4 py-2 text-sm disabled:opacity-40">
                  Reject
                </button>
              </div>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
