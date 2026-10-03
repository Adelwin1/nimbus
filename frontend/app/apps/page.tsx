"use client";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import {
  ConsoleShell,
  Status,
  consoleButton,
} from "@/components/console/ConsoleShell";
import { listApplications } from "@/lib/api";
import type { Application } from "@/types/application";
export default function ApplicationsPage() {
  return (
    <ProtectedRoute>
      <Inventory />
    </ProtectedRoute>
  );
}
function Inventory() {
  const [apps, setApps] = useState<Application[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("all");
  const load = useCallback(async () => {
    try {
      setApps((await listApplications()).applications);
      setError("");
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Applications could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(initial);
  }, [load]);
  const filtered = apps.filter(
    (app) =>
      (filter === "all" || app.status === filter) &&
      `${app.name} ${app.description} ${app.environment}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  return (
    <ConsoleShell
      actions={
        <button className={consoleButton} onClick={() => void load()}>
          Refresh
        </button>
      }
    >
      <div className="mb-7 flex flex-wrap justify-between gap-4">
        <div>
          <p className="font-mono text-[10px] tracking-widest text-slate-500">
            OPERATIONS / INVENTORY
          </p>
          <h1 className="mt-2 text-2xl font-semibold text-white">
            Applications
          </h1>
          <p className="mt-2 text-xs text-slate-500">
            Configure probes, failure thresholds, and deployment hooks.
          </p>
        </div>
        <Link
          href="/apps/new"
          className="self-start rounded-md bg-teal-400 px-4 py-2.5 text-xs font-semibold text-slate-950"
        >
          + Add application
        </Link>
      </div>
      <div className="mb-5 flex flex-wrap gap-3">
        <label className="sr-only" htmlFor="app-search">
          Search applications
        </label>
        <input
          id="app-search"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search applications…"
          className="min-w-0 flex-1 rounded-md border border-white/10 bg-[#0d1117] px-4 py-2.5 text-sm"
        />
        <label className="sr-only" htmlFor="app-filter">
          Filter status
        </label>
        <select
          id="app-filter"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          className="rounded-md border border-white/10 bg-[#0d1117] px-3 py-2 text-xs"
        >
          {["all", "healthy", "degraded", "down", "deploying", "unknown"].map(
            (status) => (
              <option key={status} value={status}>
                {status === "all" ? "All statuses" : status}
              </option>
            ),
          )}
        </select>
      </div>
      {error && (
        <p role="alert" className="mb-4 text-sm text-rose-300">
          {error}
        </p>
      )}
      <section className="overflow-hidden rounded-lg border border-white/10 bg-[#0d1117]">
        {loading ? (
          <p className="p-8 text-slate-500">Loading inventory…</p>
        ) : filtered.length === 0 ? (
          <p className="p-8 text-sm text-slate-500">
            {apps.length
              ? "No applications match your filters."
              : "No applications registered. Add your first service to begin monitoring."}
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="text-slate-500">
                <tr>
                  {[
                    "Service",
                    "Status",
                    "Version",
                    "Probe / threshold",
                    "Actions",
                  ].map((label) => (
                    <th key={label} className="px-5 py-4 font-normal">
                      {label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filtered.map((app) => (
                  <tr key={app.id} className="border-t border-white/[0.06]">
                    <td className="px-5 py-5">
                      <Link
                        href={`/apps/${app.id}`}
                        className="text-sm font-medium hover:text-teal-300"
                      >
                        {app.name}
                      </Link>
                      <p className="mt-1 text-xs text-slate-500">
                        {app.environment}
                      </p>
                    </td>
                    <td className="px-5 py-5">
                      <Status value={app.status} />
                    </td>
                    <td className="px-5 py-5 font-mono text-slate-400">
                      {app.current_version ?? "—"}
                    </td>
                    <td className="px-5 py-5 font-mono text-slate-400">
                      {app.monitoring_interval_seconds}s /{" "}
                      {app.failure_threshold} failures
                    </td>
                    <td className="px-5 py-5">
                      <div className="flex gap-4">
                        <Link
                          href={`/apps/${app.id}`}
                          className="text-teal-300"
                        >
                          Inspect
                        </Link>
                        <Link
                          href={`/apps/${app.id}/settings`}
                          className="text-slate-400"
                        >
                          Settings
                        </Link>
                        <Link
                          href={`/apps/${app.id}/deployments`}
                          className="text-slate-400"
                        >
                          Releases
                        </Link>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </ConsoleShell>
  );
}
