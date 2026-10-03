"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { ConsoleShell } from "@/components/console/ConsoleShell";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import {
  IncidentSeverityBadge,
  IncidentStatusBadge,
} from "@/components/incident/IncidentBadges";
import { IncidentNotes } from "@/components/incident/IncidentNotes";
import { IncidentTimeline } from "@/components/incident/IncidentTimeline";
import { RollbackDialog } from "@/components/incident/RollbackDialog";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { useToast } from "@/components/ui/ToastProvider";
import {
  acknowledgeIncident,
  getIncidentDetail,
  resolveIncident,
} from "@/lib/api";
import { subscribeToApplicationEvents } from "@/lib/live-events";
import type { Incident, IncidentEvent } from "@/types/incident";

type DetailState = {
  incident: Incident;
  events: IncidentEvent[];
};

export default function IncidentDetailPage() {
  return (
    <ProtectedRoute>
      <IncidentDetail />
    </ProtectedRoute>
  );
}

function IncidentDetail() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const toast = useToast();
  const incidentId = params.id;

  const [detail, setDetail] = useState<DetailState | null>(null);
  const [loading, setLoading] = useState(true);
  const [action, setAction] = useState<"acknowledge" | "resolve" | null>(null);
  const [showRollback, setShowRollback] = useState(false);
  const [showResolveConfirmation, setShowResolveConfirmation] = useState(false);
  const [live, setLive] = useState(false);
  const [error, setError] = useState("");

  const loadDetail = useCallback(async () => {
    try {
      const response = await getIncidentDetail(incidentId);

      setDetail(response);
      setError("");
    } catch (loadError) {
      setError(safeErrorMessage(loadError, "Incident could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [incidentId]);

  useEffect(() => {
    const initialLoad = window.setTimeout(() => {
      void loadDetail();
    }, 0);

    return () => {
      window.clearTimeout(initialLoad);
    };
  }, [loadDetail]);

  useEffect(() => {
    const applicationId = detail?.incident.application_id;

    if (!applicationId) {
      return;
    }

    const unsubscribe = subscribeToApplicationEvents(applicationId, {
      onConnected: () => setLive(true),

      onIncident: (event) => {
        setLive(true);

        if (event.incident_id === incidentId) {
          void loadDetail();
        }
      },

      onDeployment: (event) => {
        if (event.deployment_id === detail.incident.rollback_deployment_id) {
          void loadDetail();
        }
      },

      onConnectionError: () => {
        setLive(false);
      },
    });

    return unsubscribe;
  }, [
    detail?.incident.application_id,
    detail?.incident.rollback_deployment_id,
    incidentId,
    loadDetail,
  ]);

  async function handleAcknowledge() {
    setAction("acknowledge");
    setError("");

    try {
      await acknowledgeIncident(incidentId);
      await loadDetail();

      toast.success(
        "Incident acknowledged",
        "The incident is now marked as acknowledged.",
      );
    } catch (actionError) {
      const message = safeErrorMessage(
        actionError,
        "Incident could not be acknowledged.",
      );

      setError(message);
      toast.error("Incident not acknowledged", message);
    } finally {
      setAction(null);
    }
  }

  async function handleResolve() {
    setAction("resolve");
    setError("");

    try {
      await resolveIncident(incidentId);
      await loadDetail();

      toast.success(
        "Incident resolved",
        "The incident is now marked as resolved.",
      );
    } catch (actionError) {
      const message = safeErrorMessage(
        actionError,
        "Incident could not be resolved.",
      );

      setError(message);
      toast.error("Incident not resolved", message);
    } finally {
      setShowResolveConfirmation(false);
      setAction(null);
    }
  }

  if (loading) {
    return (
      <ConsoleShell>
        Loading incident...
      </ConsoleShell>
    );
  }

  if (!detail) {
    return (
      <ConsoleShell>
        <p className="text-red-300">{error || "Incident not found."}</p>
      </ConsoleShell>
    );
  }

  const { incident, events } = detail;
  const active = incident.status !== "resolved";

  return (
    <ConsoleShell>
      <div className="mx-auto max-w-6xl">
        <Link
          href="/incidents"
          className="text-sm font-medium text-teal-300 hover:text-teal-200"
        >
          ← All incidents
        </Link>

        <section className="mt-6 rounded-md border border-white/10 bg-[#0d1117] p-6">
          <div className="flex flex-col justify-between gap-6 lg:flex-row lg:items-start">
            <div className="max-w-3xl">
              <div className="flex flex-wrap items-center gap-3">
                <IncidentStatusBadge status={incident.status} />

                <IncidentSeverityBadge severity={incident.severity} />

                <span
                  className={`rounded-full border px-3 py-1 text-xs ${
                    live
                      ? "border-emerald-800 bg-emerald-950 text-emerald-300"
                      : "border-slate-700 bg-slate-800 text-slate-400"
                  }`}
                >
                  {live ? "Live" : "Connecting"}
                </span>
              </div>

              <h1 className="mt-4 text-2xl font-semibold tracking-tight text-white">
                {incident.title}
              </h1>

              <p className="mt-4 leading-7 text-slate-400">
                {incident.summary}
              </p>
            </div>

            <div className="flex flex-wrap gap-3">
              {incident.status === "open" ? (
                <button
                  type="button"
                  onClick={handleAcknowledge}
                  disabled={action !== null}
                  aria-busy={action !== null}
                  className="rounded-md border border-amber-700 px-4 py-2.5 text-sm font-semibold text-amber-300 transition hover:bg-amber-950 disabled:opacity-60"
                >
                  {action === "acknowledge"
                    ? "Acknowledging..."
                    : "Acknowledge"}
                </button>
              ) : null}

              {active ? (
                <button
                  type="button"
                  onClick={() => setShowResolveConfirmation(true)}
                  disabled={action !== null}
                  aria-busy={action !== null}
                  className="rounded-md border border-emerald-700 px-4 py-2.5 text-sm font-semibold text-emerald-300 transition hover:bg-emerald-950 disabled:opacity-60"
                >
                  {action === "resolve" ? "Resolving..." : "Resolve"}
                </button>
              ) : null}

              {active && !incident.rollback_deployment_id ? (
                <button
                  type="button"
                  onClick={() => setShowRollback(true)}
                  className="rounded-md bg-amber-400 px-4 py-2.5 text-sm font-semibold text-slate-950 transition hover:bg-amber-300"
                >
                  Start rollback
                </button>
              ) : null}
            </div>
          </div>

          {error ? (
            <div
              role="alert"
              aria-live="polite"
              className="mt-6 rounded-md border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-300"
            >
              {error}
            </div>
          ) : null}

          <div className="mt-8 grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
            <InfoCard
              label="Incident type"
              value={formatValue(incident.incident_type)}
            />

            <InfoCard label="Opened" value={formatDate(incident.created_at)} />

            <InfoCard
              label="Acknowledged"
              value={
                incident.acknowledged_at
                  ? formatDate(incident.acknowledged_at)
                  : "Not acknowledged"
              }
            />

            <InfoCard
              label="Resolved"
              value={
                incident.resolved_at
                  ? formatDate(incident.resolved_at)
                  : "Not resolved"
              }
            />
          </div>

          <div className="mt-6 flex flex-wrap gap-3">
            <Link
              href={`/apps/${incident.application_id}`}
              className="text-sm font-medium text-teal-300 hover:text-teal-200"
            >
              View application
            </Link>

            {incident.source_deployment_id ? (
              <Link
                href={`/deployments/${incident.source_deployment_id}`}
                className="text-sm font-medium text-teal-300 hover:text-teal-200"
              >
                Source deployment
              </Link>
            ) : null}

            {incident.rollback_deployment_id ? (
              <Link
                href={`/deployments/${incident.rollback_deployment_id}`}
                className="text-sm font-medium text-amber-400 hover:text-amber-300"
              >
                Rollback deployment
              </Link>
            ) : null}
          </div>
        </section>

        <IncidentNotes key={incident.id} incidentId={incident.id} onSaved={loadDetail} />

        <div className="mt-8">
          <IncidentTimeline events={events} />
        </div>
      </div>

      <ConfirmDialog
        open={showResolveConfirmation}
        title="Resolve this incident?"
        description="Resolve this incident only after confirming the underlying problem has been addressed. This action updates the incident timeline."
        confirmLabel="Resolve incident"
        loading={action === "resolve"}
        onConfirm={handleResolve}
        onCancel={() => setShowResolveConfirmation(false)}
      />

      {showRollback ? (
        <RollbackDialog
          incident={incident}
          onClose={() => setShowRollback(false)}
          onStarted={(deploymentId) => {
            toast.success(
              "Rollback started",
              "Nimbus is triggering the rollback and verifying application health.",
            );

            setShowRollback(false);
            router.push(`/deployments/${deploymentId}`);
          }}
        />
      ) : null}
    </ConsoleShell>
  );
}

function safeErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.name === "APIError"
    ? error.message
    : fallback;
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-white/10 bg-[#090c10] p-4">
      <p className="text-xs uppercase tracking-wider text-slate-500">{label}</p>

      <p className="mt-2 text-sm text-slate-200">{value}</p>
    </div>
  );
}

function formatValue(value: string): string {
  return value
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}
