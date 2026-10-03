"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import {
  ConsoleShell,
  Status,
  consoleButton,
} from "@/components/console/ConsoleShell";
import { ReliabilityView } from "@/components/console/ReliabilityView";
import { useAuth } from "@/contexts/AuthContext";
import { getReliabilityOverview, listApplications } from "@/lib/api";
import type { Application } from "@/types/application";
import type { ReliabilityOverview } from "@/types/insights";

export default function DashboardPage() {
  return (
    <ProtectedRoute>
      <DashboardContent />
    </ProtectedRoute>
  );
}
function DashboardContent() {
  const { logout } = useAuth();
  const router = useRouter();
  const [apps, setApps] = useState<Application[]>([]);
  const [analytics, setAnalytics] = useState<ReliabilityOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [loggingOut, setLoggingOut] = useState(false);
  const inFlight = useRef(false);
  const load = useCallback(async (signal?: AbortSignal) => {
    if (inFlight.current) return;
    inFlight.current = true;
    try {
      const [applications, overview] = await Promise.all([
        listApplications(),
        getReliabilityOverview(),
      ]);
      if (!signal?.aborted) {
        setApps(applications.applications);
        setAnalytics(overview);
        setError("");
      }
    } catch (err) {
      if (!signal?.aborted)
        setError(
          err instanceof Error ? err.message : "Dashboard could not be loaded.",
        );
    } finally {
      inFlight.current = false;
      if (!signal?.aborted) setLoading(false);
    }
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    const initial = window.setTimeout(() => void load(controller.signal), 0);
    const interval = window.setInterval(() => {
      if (!document.hidden) void load(controller.signal);
    }, 15000);
    return () => {
      controller.abort();
      window.clearTimeout(initial);
      window.clearInterval(interval);
    };
  }, [load]);
  async function signOut() {
    setLoggingOut(true);
    try {
      await logout();
      router.replace("/login");
    } finally {
      setLoggingOut(false);
    }
  }
  return (
    <ConsoleShell
      actions={
        <>
          <button onClick={() => void load()} className={consoleButton}>
            Refresh
          </button>
          <button
            onClick={() => void signOut()}
            disabled={loggingOut}
            className={consoleButton}
          >
            {loggingOut ? "Signing out…" : "Log out"}
          </button>
        </>
      }
    >
      <div className="mb-7 flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="font-mono text-[10px] tracking-widest text-slate-500">
            OPERATIONS / OVERVIEW
          </p>
          <h1 className="mt-2 text-2xl font-semibold tracking-tight text-white">
            Reliability overview
          </h1>
          <p className="mt-2 text-xs text-slate-500">
            {analytics
              ? `Last refreshed ${new Date(analytics.generated_at).toLocaleTimeString()} · Polls every 15s while visible`
              : "Your monitored services, releases, and incidents."}
          </p>
        </div>
        <Link
          href="/apps/new"
          className="rounded-md bg-teal-400 px-4 py-2.5 text-xs font-semibold text-slate-950 hover:bg-teal-300"
        >
          + Add application
        </Link>
      </div>
      {error && (
        <p
          role="alert"
          className="mb-5 rounded-md border border-rose-900 bg-rose-950/30 p-4 text-sm text-rose-300"
        >
          {error} Previous data, if available, may be stale.
        </p>
      )}
      {loading ? (
        <p className="py-16 text-sm text-slate-500">Loading workspace…</p>
      ) : (
        <>
          <section className="mb-5 overflow-hidden rounded-lg border border-white/[0.07] bg-[#0d1117]">
            <div className="flex justify-between border-b border-white/[0.07] p-5">
              <h2 className="text-sm font-medium">
                Applications{" "}
                <span className="ml-2 text-slate-500">{apps.length}</span>
              </h2>
              <Link
                href="/apps"
                className="text-xs text-slate-400 hover:text-teal-300"
              >
                Manage applications →
              </Link>
            </div>
            {apps.length === 0 ? (
              <div className="p-8 text-sm text-slate-500">
                No applications yet. Add a deployed service to start collecting
                real checks.
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs">
                  <thead className="text-slate-500">
                    <tr>
                      {[
                        "Service",
                        "Status",
                        "Version",
                        "Interval",
                        "Last check",
                      ].map((label) => (
                        <th key={label} className="px-5 py-3 font-normal">
                          {label}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {apps.map((app) => (
                      <tr
                        key={app.id}
                        className="border-t border-white/[0.05] hover:bg-white/[0.02]"
                      >
                        <td className="px-5 py-4">
                          <Link
                            href={`/apps/${app.id}`}
                            className="text-sm font-medium hover:text-teal-300"
                          >
                            {app.name}
                          </Link>
                          <p className="mt-1 text-[10px] text-slate-500">
                            {app.environment}
                          </p>
                        </td>
                        <td className="px-5 py-4">
                          <Status value={app.status} />
                        </td>
                        <td className="px-5 py-4 font-mono">
                          {app.current_version ?? "—"}
                        </td>
                        <td className="px-5 py-4 font-mono text-slate-400">
                          {app.monitoring_interval_seconds}s
                        </td>
                        <td className="px-5 py-4 text-slate-400">
                          {app.last_checked_at
                            ? new Date(app.last_checked_at).toLocaleString()
                            : "Never"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
          {analytics && <ReliabilityView data={analytics} />}
        </>
      )}
    </ConsoleShell>
  );
}
