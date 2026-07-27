"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { useAuth } from "@/contexts/AuthContext";
import { getDashboardSummary } from "@/lib/api";
import type { DashboardSummary } from "@/types/application";

export default function DashboardPage() {
  return (
    <ProtectedRoute>
      <DashboardContent />
    </ProtectedRoute>
  );
}

function DashboardContent() {
  const router = useRouter();
  const { user, logout } = useAuth();

  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [loggingOut, setLoggingOut] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;

    async function loadDashboard() {
      try {
        const response = await getDashboardSummary();

        if (!cancelled) {
          setSummary(response.summary);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(
            loadError instanceof Error
              ? loadError.message
              : "Dashboard information could not be loaded.",
          );
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void loadDashboard();

    return () => {
      cancelled = true;
    };
  }, []);

  async function handleLogout() {
    setLoggingOut(true);

    try {
      await logout();
      router.replace("/login");
    } finally {
      setLoggingOut(false);
    }
  }

  return (
    <main id="main-content" className="min-h-screen bg-slate-950 text-white">
      <header className="border-b border-slate-800 bg-slate-900">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-5">
          <div>
            <p className="text-xl font-semibold">Nimbus</p>
            <p className="text-sm text-slate-400">
              Personal Cloud Reliability Dashboard
            </p>
          </div>

          <div className="flex items-center gap-3">
            <Link
              href="/apps"
              className="rounded-lg border border-slate-700 px-4 py-2 text-sm text-slate-200 transition hover:border-slate-500 hover:text-white"
            >
              Applications
            </Link>

            <button
              type="button"
              onClick={handleLogout}
              disabled={loggingOut}
              className="rounded-lg border border-slate-700 px-4 py-2 text-sm text-slate-200 transition hover:border-slate-500 hover:text-white disabled:opacity-60"
            >
              {loggingOut ? "Logging out..." : "Log out"}
            </button>
          </div>
        </div>
      </header>

      <div className="mx-auto max-w-7xl px-6 py-10">
        <div className="flex flex-col justify-between gap-6 md:flex-row md:items-end">
          <div>
            <p className="text-sm font-medium uppercase tracking-wider text-sky-400">
              Personal overview
            </p>

            <h1 className="mt-2 text-3xl font-bold">
              Welcome back
              {user?.name ? `, ${user.name}` : ""}
            </h1>

            <p className="mt-3 max-w-2xl text-slate-400">
              Review the real status totals for applications registered to your
              account.
            </p>
          </div>

          <Link
            href="/apps/new"
            className="rounded-xl bg-sky-500 px-5 py-3 text-center font-medium text-slate-950 transition hover:bg-sky-400"
          >
            Add application
          </Link>
        </div>

        {loading ? (
          <div className="mt-10 rounded-2xl border border-slate-800 bg-slate-900 px-6 py-16 text-center text-slate-400">
            Loading dashboard...
          </div>
        ) : error || !summary ? (
          <div className="mt-10 rounded-2xl border border-red-900 bg-red-950/40 px-6 py-5 text-red-300">
            {error || "Dashboard data is unavailable."}
          </div>
        ) : (
          <>
            <div className="mt-10 grid gap-5 sm:grid-cols-2 xl:grid-cols-5">
              <SummaryCard
                label="Total applications"
                value={summary.total_applications}
                description="All owned applications"
              />

              <SummaryCard
                label="Healthy"
                value={summary.healthy_applications}
                description="Responding normally"
              />

              <SummaryCard
                label="Degraded"
                value={summary.degraded_applications}
                description="Performance concerns"
              />

              <SummaryCard
                label="Down"
                value={summary.down_applications}
                description="Currently unavailable"
              />

              <SummaryCard
                label="Unknown"
                value={summary.unknown_applications}
                description="Not checked yet"
              />
            </div>

            {summary.total_applications === 0 ? (
              <EmptyDashboard />
            ) : (
              <section className="mt-8 rounded-2xl border border-slate-800 bg-slate-900 p-6">
                <div className="flex flex-col justify-between gap-5 md:flex-row md:items-center">
                  <div>
                    <h2 className="text-lg font-semibold">
                      Manage your applications
                    </h2>

                    <p className="mt-2 text-sm text-slate-400">
                      View configuration, monitoring URLs, encrypted webhook
                      status, and application settings.
                    </p>
                  </div>

                  <Link
                    href="/apps"
                    className="rounded-xl border border-slate-700 px-5 py-3 text-center text-sm text-slate-200 transition hover:border-sky-500 hover:text-white"
                  >
                    View all applications
                  </Link>
                </div>
              </section>
            )}
          </>
        )}
      </div>
    </main>
  );
}

function SummaryCard({
  label,
  value,
  description,
}: {
  label: string;
  value: number;
  description: string;
}) {
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900 p-5">
      <p className="text-sm text-slate-400">{label}</p>

      <p className="mt-3 text-3xl font-bold text-white">{value}</p>

      <p className="mt-2 text-xs text-slate-500">{description}</p>
    </div>
  );
}

function EmptyDashboard() {
  return (
    <section className="mt-8 rounded-2xl border border-dashed border-slate-700 bg-slate-900 px-6 py-14 text-center">
      <h2 className="text-xl font-semibold">No applications registered</h2>

      <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-slate-400">
        Add your first application to begin tracking real reliability and health
        information.
      </p>

      <Link
        href="/apps/new"
        className="mt-6 inline-block rounded-xl bg-sky-500 px-5 py-3 font-medium text-slate-950 transition hover:bg-sky-400"
      >
        Add your first application
      </Link>
    </section>
  );
}
