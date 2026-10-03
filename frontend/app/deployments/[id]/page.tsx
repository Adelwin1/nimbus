"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { ConsoleShell } from "@/components/console/ConsoleShell";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { DeploymentStatusBadge } from "@/components/deployment/DeploymentStatusBadge";
import { GitHubReleaseLinks } from "@/components/deployment/GitHubReleaseLinks";
import { DeploymentTimeline } from "@/components/deployment/DeploymentTimeline";
import { getDeploymentDetail } from "@/lib/api";
import { subscribeToApplicationEvents } from "@/lib/live-events";
import type { Deployment, DeploymentEvent } from "@/types/deployment";

type DetailState = {
  deployment: Deployment;
  events: DeploymentEvent[];
};

export default function DeploymentDetailPage() {
  return (
    <ProtectedRoute>
      <DeploymentDetail />
    </ProtectedRoute>
  );
}

function DeploymentDetail() {
  const params = useParams<{ id: string }>();
  const deploymentId = params.id;

  const [detail, setDetail] = useState<DetailState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [live, setLive] = useState(false);

  const loadDetail = useCallback(async () => {
    try {
      const response = await getDeploymentDetail(deploymentId);

      setDetail(response);
      setError("");
    } catch (loadError) {
      setError(
        loadError instanceof Error
          ? loadError.message
          : "Deployment could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [deploymentId]);

  useEffect(() => {
    const initialLoad = window.setTimeout(() => {
      void loadDetail();
    }, 0);

    return () => {
      window.clearTimeout(initialLoad);
    };
  }, [loadDetail]);

  useEffect(() => {
    if (!detail?.deployment.application_id) {
      return;
    }

    const unsubscribe = subscribeToApplicationEvents(
      detail.deployment.application_id,
      {
        onConnected: () => setLive(true),

        onDeployment: (event) => {
          setLive(true);

          if (event.deployment_id === deploymentId) {
            void loadDetail();
          }
        },

        onConnectionError: () => {
          setLive(false);
        },
      },
    );

    return unsubscribe;
  }, [deploymentId, detail?.deployment.application_id, loadDetail]);

  if (loading) {
    return (
      <ConsoleShell>
        Loading deployment...
      </ConsoleShell>
    );
  }

  if (!detail) {
    return (
      <ConsoleShell>
        <p className="text-red-300">{error || "Deployment not found."}</p>
      </ConsoleShell>
    );
  }

  const { deployment, events } = detail;

  return (
    <ConsoleShell>
      <div className="mx-auto max-w-6xl">
        <Link
          href={`/apps/${deployment.application_id}/deployments`}
          className="text-sm font-medium text-teal-300 hover:text-teal-200"
        >
          ← Deployment history
        </Link>

        <div className="mt-6 rounded-md border border-white/10 bg-[#0d1117] p-6">
          <div className="flex flex-col justify-between gap-5 md:flex-row md:items-start">
            <div>
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-2xl font-semibold tracking-tight text-white">
                  {deployment.version}
                </h1>

                <DeploymentStatusBadge status={deployment.status} />

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

              <p className="mt-3 text-slate-400">
                {deployment.deployment_type === "rollback"
                  ? "Rollback release"
                  : "Application deployment"}
              </p>
            </div>

            <Link
              href={`/apps/${deployment.application_id}`}
              className="rounded-md border border-slate-700 px-4 py-2.5 text-center text-sm font-medium text-slate-200 transition hover:bg-slate-800"
            >
              View application
            </Link>
          </div>

          {error ? (
            <div className="mt-6 rounded-md border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-300">
              {error}
            </div>
          ) : null}

          <div className="mt-8 grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
            <DetailCard
              label="Previous version"
              value={deployment.previous_version ?? "None"}
            />

            <DetailCard
              label="Commit SHA"
              value={deployment.commit_sha ?? "None"}
              monospace
            />

            <DetailCard
              label="Started"
              value={
                deployment.started_at
                  ? formatDate(deployment.started_at)
                  : "Not started"
              }
            />

            <DetailCard
              label="Completed"
              value={
                deployment.completed_at
                  ? formatDate(deployment.completed_at)
                  : "In progress"
              }
            />
          </div>

          <GitHubReleaseLinks key={deployment.id} deployment={deployment} />

          <div className="mt-8 border-t border-white/10 pt-6">
            <h2 className="font-semibold text-white">Release notes</h2>

            <p className="mt-3 whitespace-pre-wrap text-sm leading-7 text-slate-400">
              {deployment.release_notes || "No release notes were provided."}
            </p>
          </div>
        </div>

        <div className="mt-8">
          <DeploymentTimeline events={events} />
        </div>
      </div>
    </ConsoleShell>
  );
}

function DetailCard({
  label,
  value,
  monospace = false,
}: {
  label: string;
  value: string;
  monospace?: boolean;
}) {
  return (
    <div className="rounded-md border border-white/10 bg-[#090c10] p-4">
      <p className="text-xs uppercase tracking-wider text-slate-500">{label}</p>

      <p
        className={`mt-2 break-words text-sm text-slate-200 ${
          monospace ? "font-mono" : ""
        }`}
      >
        {value}
      </p>
    </div>
  );
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}
