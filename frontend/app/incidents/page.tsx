"use client";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import {
  ConsoleShell,
  Status,
  consoleButton,
} from "@/components/console/ConsoleShell";
import { duration } from "@/components/console/ReliabilityView";
import { getIncidents } from "@/lib/api";
import type { IncidentListItem } from "@/types/incident";
export default function IncidentsPage() {
  return (
    <ProtectedRoute>
      <IncidentList />
    </ProtectedRoute>
  );
}
function IncidentList() {
  const [incidents, setIncidents] = useState<IncidentListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState("all");
  const [now, setNow] = useState(0);
  const load = useCallback(async () => {
    try {
      setIncidents((await getIncidents()).incidents);
      setNow(Date.now());
      setError("");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Incidents could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    const interval = window.setInterval(() => {
      if (!document.hidden) void load();
    }, 15000);
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(interval);
    };
  }, [load]);
  const filtered = incidents.filter(
    (item) =>
      filter === "all" ||
      (filter === "active"
        ? item.status !== "resolved"
        : item.status === "resolved"),
  );
  return (
    <ConsoleShell
      actions={
        <button className={consoleButton} onClick={() => void load()}>
          Refresh
        </button>
      }
    >
      <p className="font-mono text-[10px] tracking-widest text-slate-500">
        OPERATIONS / RESPONSE
      </p>
      <h1 className="mt-2 text-2xl font-semibold text-white">Incidents</h1>
      <p className="mt-2 text-xs text-slate-500">
        Threshold breaches, deployment failures, and recovery timelines.
      </p>
      <div className="my-6 flex gap-2">
        {["all", "active", "resolved"].map((value) => (
          <button
            key={value}
            aria-pressed={filter === value}
            onClick={() => setFilter(value)}
            className={consoleButton}
          >
            {value}
          </button>
        ))}
      </div>
      {error && (
        <p role="alert" className="mb-4 text-sm text-rose-300">
          {error}
        </p>
      )}
      <section className="overflow-hidden rounded-lg border border-white/10 bg-[#0d1117]">
        {loading ? (
          <p className="p-8 text-sm text-slate-500">Loading incidents…</p>
        ) : filtered.length === 0 ? (
          <p className="p-8 text-sm text-slate-500">
            No incidents in this view.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="text-slate-500">
                <tr>
                  {["Incident", "Severity", "Status", "Duration", "Opened"].map(
                    (label) => (
                      <th key={label} className="px-5 py-4 font-normal">
                        {label}
                      </th>
                    ),
                  )}
                </tr>
              </thead>
              <tbody>
                {filtered.map((item) => (
                  <tr key={item.id} className="border-t border-white/[0.06]">
                    <td className="px-5 py-5">
                      <Link
                        href={`/incidents/${item.id}`}
                        className="text-sm hover:text-teal-300"
                      >
                        {item.title}
                      </Link>
                      <p className="mt-2 text-slate-500">
                        {item.application_name}
                      </p>
                    </td>
                    <td className="px-5 py-5">
                      <Status value={item.severity} />
                    </td>
                    <td className="px-5 py-5">
                      <Status value={item.status} />
                    </td>
                    <td className="px-5 py-5 font-mono text-slate-400">
                      {duration(
                        Math.max(
                          0,
                          ((item.resolved_at
                            ? Date.parse(item.resolved_at)
                            : now) -
                            Date.parse(item.created_at)) /
                            1000,
                        ),
                      )}
                    </td>
                    <td className="px-5 py-5 text-slate-500">
                      {new Date(item.created_at).toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      <p className="mt-4 text-xs text-slate-600">
        Duration is calculated at the last refresh. Open an incident to
        acknowledge, resolve, or initiate recovery.
      </p>
    </ConsoleShell>
  );
}
