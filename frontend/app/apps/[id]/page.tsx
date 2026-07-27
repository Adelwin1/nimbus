"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";

import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { HealthPanel } from "@/components/health/HealthPanel";
import { DeploymentPanel } from "@/components/deployment/DeploymentPanel";
import { IncidentPanel } from "@/components/incident/IncidentPanel";
import { getApplication } from "@/lib/api";
import type { Application, ApplicationStatus } from "@/types/application";

export default function ApplicationOverviewPage() {
  return (
    <ProtectedRoute>
      <ApplicationOverviewContent />
    </ProtectedRoute>
  );
}

function ApplicationOverviewContent() {
  const params = useParams<{ id: string }>();
  const applicationID = params.id;

  const [application, setApplication] = useState<Application | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;

    async function loadApplication() {
      try {
        const response = await getApplication(applicationID);

        if (!cancelled) {
          setApplication(response.application);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(
            loadError instanceof Error
              ? loadError.message
              : "Application could not be loaded.",
          );
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void loadApplication();

    return () => {
      cancelled = true;
    };
  }, [applicationID]);

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

            <p className="text-sm text-slate-400">Application overview</p>
          </div>

          <Link
            href="/apps"
            className="rounded-lg border border-slate-700 px-4 py-2 text-sm text-slate-200 transition hover:border-slate-500 hover:text-white"
          >
            All applications
          </Link>
        </div>
      </header>

      <div className="mx-auto max-w-7xl px-6 py-10">
        {loading ? (
          <LoadingState />
        ) : error || !application ? (
          <ErrorState message={error || "Application not found."} />
        ) : (
          <ApplicationOverview application={application} />
        )}
      </div>
    </main>
  );
}

function ApplicationOverview({ application }: { application: Application }) {
  return (
    <>
      <div className="flex flex-col justify-between gap-6 md:flex-row md:items-start">
        <div>
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-3xl font-bold">{application.name}</h1>

            <StatusBadge status={application.status} />
          </div>

          <p className="mt-2 capitalize text-slate-400">
            {application.environment} environment
          </p>

          <p className="mt-4 max-w-3xl leading-7 text-slate-300">
            {application.description || "No description has been added."}
          </p>
        </div>

        <Link
          href={`/apps/${application.id}/settings`}
          className="rounded-xl bg-sky-500 px-5 py-3 text-center font-medium text-slate-950 transition hover:bg-sky-400"
        >
          Application settings
        </Link>
      </div>

      <div className="mt-10 grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
        <SummaryCard
          label="Current status"
          value={capitalize(application.status)}
        />

        <SummaryCard
          label="Current version"
          value={application.current_version ?? "Not set"}
        />

        <SummaryCard
          label="Consecutive failures"
          value={String(application.consecutive_failures)}
        />

        <SummaryCard
          label="Last checked"
          value={formatDate(application.last_checked_at)}
        />
      </div>

      <HealthPanel applicationId={application.id} />

      <DeploymentPanel applicationId={application.id} />

      <IncidentPanel applicationId={application.id} />

      <div className="mt-8 grid gap-6 lg:grid-cols-2">
        <section className="rounded-2xl border border-slate-800 bg-slate-900 p-6">
          <h2 className="text-lg font-semibold">Application endpoints</h2>

          <div className="mt-6 space-y-5">
            <URLRow
              label="Application URL"
              value={application.application_url}
            />

            <URLRow label="Health URL" value={application.health_url} />

            <URLRow label="Repository" value={application.repository_url} />
          </div>
        </section>

        <section className="rounded-2xl border border-slate-800 bg-slate-900 p-6">
          <h2 className="text-lg font-semibold">Monitoring configuration</h2>

          <div className="mt-6 space-y-4">
            <DetailRow
              label="Check interval"
              value={`${application.monitoring_interval_seconds} seconds`}
            />

            <DetailRow
              label="Failure threshold"
              value={`${application.failure_threshold} failures`}
            />

            <DetailRow
              label="Latency threshold"
              value={`${application.latency_threshold_ms} ms`}
            />

            <DetailRow
              label="Last healthy"
              value={formatDate(application.last_healthy_at)}
            />
          </div>
        </section>

        <section className="rounded-2xl border border-slate-800 bg-slate-900 p-6 lg:col-span-2">
          <h2 className="text-lg font-semibold">
            Encrypted webhook configuration
          </h2>

          <p className="mt-1 text-sm text-slate-400">
            Nimbus never returns stored webhook URLs or tokens in API responses.
          </p>

          <div className="mt-6 grid gap-4 md:grid-cols-3">
            <SecretStatus
              label="Deployment webhook"
              configured={application.deployment_webhook_configured}
            />

            <SecretStatus
              label="Rollback webhook"
              configured={application.rollback_webhook_configured}
            />

            <SecretStatus
              label="Webhook token"
              configured={application.webhook_token_configured}
            />
          </div>
        </section>
      </div>
    </>
  );
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900 p-5">
      <p className="text-sm text-slate-400">{label}</p>
      <p className="mt-2 text-xl font-semibold text-white">{value}</p>
    </div>
  );
}

function URLRow({ label, value }: { label: string; value: string | null }) {
  return (
    <div>
      <p className="text-sm text-slate-500">{label}</p>

      {value ? (
        <a
          href={value}
          target="_blank"
          rel="noreferrer"
          className="mt-1 block break-all text-sm text-sky-400 hover:text-sky-300"
        >
          {value}
        </a>
      ) : (
        <p className="mt-1 text-sm text-slate-300">Not configured</p>
      )}
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-6 border-b border-slate-800 pb-4 last:border-0 last:pb-0">
      <span className="text-sm text-slate-400">{label}</span>
      <span className="text-right text-sm text-slate-100">{value}</span>
    </div>
  );
}

function SecretStatus({
  label,
  configured,
}: {
  label: string;
  configured: boolean;
}) {
  return (
    <div className="rounded-xl border border-slate-800 bg-slate-950 p-4">
      <p className="text-sm text-slate-400">{label}</p>

      <p
        className={`mt-2 text-sm font-medium ${
          configured ? "text-emerald-400" : "text-slate-500"
        }`}
      >
        {configured ? "Configured and encrypted" : "Not configured"}
      </p>
    </div>
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

function LoadingState() {
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900 px-6 py-16 text-center text-slate-400">
      Loading application...
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

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
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
