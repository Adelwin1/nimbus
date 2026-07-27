"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { listApplications } from "@/lib/api";
import type { Application, ApplicationStatus } from "@/types/application";

export default function ApplicationsPage() {
  return (
    <ProtectedRoute>
      <ApplicationsContent />
    </ProtectedRoute>
  );
}

function ApplicationsContent() {
  const [applications, setApplications] = useState<Application[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;

    async function loadApplications() {
      try {
        const response = await listApplications();

        if (!cancelled) {
          setApplications(response.applications);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(
            loadError instanceof Error
              ? loadError.message
              : "Applications could not be loaded.",
          );
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void loadApplications();

    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <main id="main-content" className="min-h-screen bg-slate-950 text-white">
      <header className="border-b border-slate-800 bg-slate-900">
        <div className="mx-auto flex max-w-7xl items-center justify-between px-6 py-5">
          <div>
            <Link
              href="/dashboard"
              className="text-xl font-semibold text-white"
            >
              Nimbus
            </Link>

            <p className="text-sm text-slate-400">Application management</p>
          </div>

          <div className="flex items-center gap-3">
            <Link
              href="/dashboard"
              className="rounded-lg border border-slate-700 px-4 py-2 text-sm text-slate-200 transition hover:border-slate-500 hover:text-white"
            >
              Dashboard
            </Link>

            <Link
              href="/apps/new"
              className="rounded-lg bg-sky-500 px-4 py-2 text-sm font-medium text-slate-950 transition hover:bg-sky-400"
            >
              Add application
            </Link>
          </div>
        </div>
      </header>

      <div className="mx-auto max-w-7xl px-6 py-10">
        <div className="mb-8">
          <p className="text-sm font-medium uppercase tracking-wider text-sky-400">
            Your services
          </p>

          <h1 className="mt-2 text-3xl font-bold">Applications</h1>

          <p className="mt-3 text-slate-400">
            Manage every application connected to your Nimbus account.
          </p>
        </div>

        {loading ? (
          <LoadingState />
        ) : error ? (
          <ErrorState message={error} />
        ) : applications.length === 0 ? (
          <EmptyState />
        ) : (
          <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
            {applications.map((application) => (
              <ApplicationCard key={application.id} application={application} />
            ))}
          </div>
        )}
      </div>
    </main>
  );
}

function ApplicationCard({ application }: { application: Application }) {
  return (
    <Link
      href={`/apps/${application.id}`}
      className="group rounded-2xl border border-slate-800 bg-slate-900 p-6 transition hover:-translate-y-1 hover:border-sky-500/60"
    >
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold text-white group-hover:text-sky-300">
            {application.name}
          </h2>

          <p className="mt-1 text-sm capitalize text-slate-400">
            {application.environment}
          </p>
        </div>

        <StatusBadge status={application.status} />
      </div>

      <p className="mt-5 line-clamp-3 min-h-15 text-sm leading-6 text-slate-400">
        {application.description || "No description has been added."}
      </p>

      <div className="mt-6 space-y-3 border-t border-slate-800 pt-5 text-sm">
        <DetailRow
          label="Version"
          value={application.current_version ?? "Not set"}
        />

        <DetailRow
          label="Check interval"
          value={`${application.monitoring_interval_seconds}s`}
        />

        <DetailRow
          label="Last checked"
          value={formatDate(application.last_checked_at)}
        />
      </div>

      <p className="mt-5 text-sm font-medium text-sky-400">
        View application →
      </p>
    </Link>
  );
}

function StatusBadge({ status }: { status: ApplicationStatus }) {
  const styles: Record<ApplicationStatus, string> = {
    healthy: "border-emerald-800 bg-emerald-950 text-emerald-300",
    degraded: "border-amber-800 bg-amber-950 text-amber-300",
    down: "border-red-800 bg-red-950 text-red-300",
    deploying: "border-blue-800 bg-blue-950 text-blue-300",
    unknown: "border-slate-700 bg-slate-800 text-slate-300",
  };

  return (
    <span
      className={`rounded-full border px-3 py-1 text-xs font-medium capitalize ${styles[status]}`}
    >
      {status}
    </span>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-slate-500">{label}</span>
      <span className="truncate text-slate-200">{value}</span>
    </div>
  );
}

function EmptyState() {
  return (
    <div className="rounded-2xl border border-dashed border-slate-700 bg-slate-900 px-6 py-16 text-center">
      <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-sky-500/10 text-2xl">
        ☁
      </div>

      <h2 className="mt-5 text-xl font-semibold">No applications yet</h2>

      <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-slate-400">
        Register your first deployed application to begin monitoring its health
        and reliability.
      </p>

      <Link
        href="/apps/new"
        className="mt-6 inline-block rounded-xl bg-sky-500 px-5 py-3 font-medium text-slate-950 transition hover:bg-sky-400"
      >
        Add your first application
      </Link>
    </div>
  );
}

function LoadingState() {
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900 px-6 py-16 text-center text-slate-400">
      Loading applications...
    </div>
  );
}

function ErrorState({ message }: { message: string }) {
  return (
    <div className="rounded-2xl border border-red-900 bg-red-950/40 px-6 py-5 text-red-300">
      {message}
    </div>
  );
}

function formatDate(value: string | null): string {
  if (!value) {
    return "Never";
  }

  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
