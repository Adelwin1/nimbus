"use client";

import { useEffect, useRef, useState } from "react";
import { apiRequest } from "@/lib/api";

type ErrorGroup = {
  id: string;
  title: string;
  occurrences: number;
  first_seen_at: string;
  last_seen_at: string;
  latest_release: string | null;
  frames: { file: string; function: string; line: number }[];
};

export function ApplicationErrors({ applicationId }: { applicationId: string }) {
  const [groups, setGroups] = useState<ErrorGroup[]>([]);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const endpoint = `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1"}/error-reports/${applicationId}`;

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const data = await apiRequest<{ errors: ErrorGroup[] }>(
          `/application-errors/apps/${applicationId}`, { authenticated: true },
        );
        if (!cancelled) {
          setGroups(data.errors);
          setError("");
        }
      } catch {
        if (!cancelled) setError("Application errors could not be refreshed.");
      } finally {
        if (!cancelled) timer = setTimeout(() => { void load(); }, 5000);
      }
    }
    void load();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [applicationId]);

  async function createToken() {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setToken("");
    setMessage("");
    try {
      const data = await apiRequest<{ token: string }>(
        `/application-errors/apps/${applicationId}/token`,
        { authenticated: true, method: "POST" },
      );
      setToken(data.token);
      setMessage("New token created. Any previous reporting token is now invalid.");
    } catch {
      setMessage("Reporting token could not be created.");
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }

  async function copyToken() {
    try {
      await navigator.clipboard.writeText(token);
      setMessage("Token copied. Store it in your application server environment.");
    } catch {
      setMessage("Copy failed. Select the token field and copy it manually.");
    }
  }

  return (
    <section className="mt-8 rounded-md border border-white/10 bg-[#0d1117] p-6">
      <p className="text-xs uppercase tracking-widest text-teal-300">Runtime reporting</p>
      <h2 className="mt-2 text-xl font-semibold">Application errors</h2>
      <p className="mt-2 text-sm text-slate-400">
        Errors reported by your application server, grouped by type, code, and stack frames.
        They are separate from HTTP checks and browser journeys.
      </p>

      <details className="mt-5 rounded-md border border-white/10 p-4">
        <summary className="cursor-pointer text-sm">Set up error reporting</summary>
        <p className="mt-3 text-sm text-slate-400">
          Generating a token replaces the existing one. Update your server configuration afterward.
          Never put the token in frontend code.
        </p>
        <label className="mt-4 block text-sm">
          Reporting endpoint
          <input readOnly value={endpoint}
            className="mt-2 w-full rounded-md border border-white/15 bg-[#090c10] p-3 font-mono text-xs" />
        </label>
        <button type="button" disabled={busy} onClick={() => { void createToken(); }}
          className="mt-4 rounded-md border border-white/15 px-4 py-2 text-sm disabled:opacity-40">
          Generate / replace reporting token
        </button>
        {token && (
          <div className="mt-4" data-sensitive>
            <label className="block text-sm">
              Token — available only in this page session
              <input type="password" autoComplete="off" readOnly value={token}
                className="mt-2 w-full rounded-md border border-white/15 bg-[#090c10] p-3" />
            </label>
            <button type="button" onClick={() => { void copyToken(); }}
              className="mt-3 rounded-md border border-white/15 px-4 py-2 text-sm">
              Copy token
            </button>
            <button type="button" onClick={() => setToken("")}
              className="ml-3 text-sm text-slate-400">Dismiss</button>
          </div>
        )}
        {message && <p role="status" className="mt-3 text-sm text-slate-300">{message}</p>}
      </details>

      {error && <p role="alert" className="mt-4 text-sm text-rose-300">{error}</p>}
      {groups.length === 0 ? (
        <p className="mt-5 text-sm text-slate-400">
          No errors reported yet. This does not prove the application is error-free.
        </p>
      ) : (
        <ul className="mt-5 space-y-4">
          {groups.map(group => (
            <li key={group.id} className="rounded-md border border-white/10 p-4">
              <h3 className="font-mono text-sm text-rose-300">{group.title}</h3>
              <p className="mt-2 text-sm text-slate-400">
                {group.occurrences} occurrences · latest reported release: {group.latest_release || "unknown"}
              </p>
              <p className="mt-1 text-xs text-slate-500">
                First: {new Date(group.first_seen_at).toLocaleString()}
                {" · "}Latest: {new Date(group.last_seen_at).toLocaleString()}
              </p>
              <ol className="mt-3 space-y-1 font-mono text-xs text-slate-300">
                {group.frames.map((frame, index) => (
                  <li key={index}>{frame.function} — {frame.file}:{frame.line}</li>
                ))}
              </ol>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
