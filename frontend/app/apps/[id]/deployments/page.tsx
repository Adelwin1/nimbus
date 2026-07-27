"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { DeploymentStatusBadge } from "@/components/deployment/DeploymentStatusBadge";
import { getDeployments } from "@/lib/api";
import { subscribeToApplicationEvents } from "@/lib/live-events";
import type { Deployment } from "@/types/deployment";

export default function DeploymentHistoryPage() {
  return (
    <ProtectedRoute>
      <DeploymentHistory />
    </ProtectedRoute>
  );
}

function DeploymentHistory() {
  const params = useParams<{ id: string }>();
  const applicationId = params.id;

  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [live, setLive] = useState(false);

  const loadDeployments = useCallback(async () => {
    try {
      const response = await getDeployments(applicationId);

      setDeployments(response.deployments);
      setError("");
    } catch (loadError) {
      setError(
        loadError instanceof Error
          ? loadError.message
          : "Deployment history could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [applicationId]);

  useEffect(() => {
    const initialLoad = window.setTimeout(() => {
      void loadDeployments();
    }, 0);

    const unsubscribe = subscribeToApplicationEvents(applicationId, {
      onConnected: () => setLive(true),
      onDeployment: () => {
        setLive(true);
        void loadDeployments();
      },
      onConnectionError: () => setLive(false),
    });

    return () => {
      window.clearTimeout(initialLoad);
      unsubscribe();
    };
  }, [applicationId, loadDeployments]);

  return (
    <main
      id="main-content"
      className="min-h-screen bg-slate-950 px-5 py-10 text-slate-100"
    >
      <div className="mx-auto max-w-6xl">
        <Link
          href={`/apps/${applicationId}`}
          className="text-sm font-medium text-sky-400 hover:text-sky-300"
        >
          ← Back to application
        </Link>

        <div className="mt-6 flex flex-col justify-between gap-5 md:flex-row md:items-end">
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-3xl font-bold text-white">
                Deployment history
              </h1>

              <span
                className={`rounded-full border px-3 py-1 text-xs ${
                  live
                    ? "border-emerald-800 bg-emerald-950 text-emerald-300"
                    : "border-slate-700 bg-slate-900 text-slate-400"
                }`}
              >
                {live ? "Live updates" : "Connecting"}
              </span>
            </div>

            <p className="mt-3 text-slate-400">
              Persistent records for every release attempt.
            </p>
          </div>

          <Link
            href={`/apps/${applicationId}/deployments/new`}
            className="rounded-xl bg-sky-500 px-5 py-3 text-center font-semibold text-slate-950 transition hover:bg-sky-400"
          >
            New deployment
          </Link>
        </div>

        {error ? (
          <div className="mt-8 rounded-xl border border-red-900 bg-red-950/40 px-5 py-4 text-sm text-red-300">
            {error}
          </div>
        ) : null}

        <div className="mt-8 overflow-hidden rounded-2xl border border-slate-800 bg-slate-900">
          {loading ? (
            <div className="px-6 py-16 text-center text-slate-400">
              Loading deployment history...
            </div>
          ) : deployments.length === 0 ? (
            <div className="px-6 py-16 text-center">
              <h2 className="text-lg font-semibold text-white">
                No deployments yet
              </h2>

              <p className="mt-2 text-sm text-slate-400">
                Start the application’s first release.
              </p>
            </div>
          ) : (
            <div className="divide-y divide-slate-800">
              {deployments.map((deployment) => (
                <Link
                  key={deployment.id}
                  href={`/deployments/${deployment.id}`}
                  className="grid gap-5 p-6 transition hover:bg-slate-800/50 md:grid-cols-[1.2fr_1fr_1fr_auto] md:items-center"
                >
                  <div>
                    <p className="font-semibold text-white">
                      {deployment.version}
                    </p>

                    <p className="mt-1 text-sm text-slate-500">
                      {deployment.previous_version
                        ? `From ${deployment.previous_version}`
                        : "Initial tracked version"}
                    </p>
                  </div>

                  <div>
                    <p className="text-xs uppercase tracking-wider text-slate-500">
                      Commit
                    </p>

                    <p className="mt-1 font-mono text-sm text-slate-300">
                      {deployment.commit_sha ?? "—"}
                    </p>
                  </div>

                  <div>
                    <p className="text-xs uppercase tracking-wider text-slate-500">
                      Created
                    </p>

                    <p className="mt-1 text-sm text-slate-300">
                      {formatDate(deployment.created_at)}
                    </p>
                  </div>

                  <DeploymentStatusBadge status={deployment.status} />
                </Link>
              ))}
            </div>
          )}
        </div>
      </div>
    </main>
  );
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
