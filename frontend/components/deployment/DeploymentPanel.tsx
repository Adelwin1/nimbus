"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { DeploymentStatusBadge } from "@/components/deployment/DeploymentStatusBadge";
import { getDeployments } from "@/lib/api";
import { subscribeToApplicationEvents } from "@/lib/live-events";
import type { Deployment } from "@/types/deployment";

type DeploymentPanelProps = {
  applicationId: string;
};

export function DeploymentPanel({ applicationId }: DeploymentPanelProps) {
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
          : "Deployments could not be loaded.",
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
      onConnected: () => {
        setLive(true);
      },
      onDeployment: () => {
        setLive(true);
        void loadDeployments();
      },
      onConnectionError: () => {
        setLive(false);
      },
    });

    return () => {
      window.clearTimeout(initialLoad);
      unsubscribe();
    };
  }, [applicationId, loadDeployments]);

  const recentDeployments = deployments.slice(0, 3);

  return (
    <section className="mt-8 rounded-2xl border border-slate-800 bg-slate-900">
      <div className="flex flex-col justify-between gap-5 border-b border-slate-800 p-6 md:flex-row md:items-center">
        <div>
          <div className="flex items-center gap-3">
            <h2 className="text-xl font-semibold text-white">Deployments</h2>

            <span
              className={`inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs ${
                live
                  ? "border-emerald-800 bg-emerald-950 text-emerald-300"
                  : "border-slate-700 bg-slate-800 text-slate-400"
              }`}
            >
              <span
                className={`h-2 w-2 rounded-full ${
                  live ? "bg-emerald-400" : "bg-slate-500"
                }`}
              />

              {live ? "Live" : "Connecting"}
            </span>
          </div>

          <p className="mt-2 text-sm text-slate-400">
            Trigger releases and follow verification progress in real time.
          </p>
        </div>

        <div className="flex flex-wrap gap-3">
          <Link
            href={`/apps/${applicationId}/deployments`}
            className="rounded-xl border border-slate-700 px-4 py-2.5 text-sm font-medium text-slate-200 transition hover:border-slate-500 hover:bg-slate-800"
          >
            View history
          </Link>

          <Link
            href={`/apps/${applicationId}/deployments/new`}
            className="rounded-xl bg-sky-500 px-4 py-2.5 text-sm font-semibold text-slate-950 transition hover:bg-sky-400"
          >
            New deployment
          </Link>
        </div>
      </div>

      {error ? (
        <div className="border-b border-red-900 bg-red-950/40 px-6 py-4 text-sm text-red-300">
          {error}
        </div>
      ) : null}

      {loading ? (
        <div className="px-6 py-12 text-center text-sm text-slate-400">
          Loading deployments...
        </div>
      ) : recentDeployments.length === 0 ? (
        <div className="px-6 py-14 text-center">
          <h3 className="font-medium text-slate-200">No deployments yet</h3>

          <p className="mt-2 text-sm text-slate-400">
            Create the first release after configuring a deployment webhook.
          </p>
        </div>
      ) : (
        <div className="divide-y divide-slate-800">
          {recentDeployments.map((deployment) => (
            <Link
              key={deployment.id}
              href={`/deployments/${deployment.id}`}
              className="flex flex-col justify-between gap-4 p-6 transition hover:bg-slate-800/50 sm:flex-row sm:items-center"
            >
              <div>
                <div className="flex flex-wrap items-center gap-3">
                  <p className="font-semibold text-white">
                    {deployment.version}
                  </p>

                  <DeploymentStatusBadge status={deployment.status} />
                </div>

                <p className="mt-2 text-sm text-slate-400">
                  {deployment.commit_sha
                    ? `Commit ${deployment.commit_sha}`
                    : "No commit SHA provided"}
                </p>
              </div>

              <time className="text-sm text-slate-500">
                {formatDate(deployment.created_at)}
              </time>
            </Link>
          ))}
        </div>
      )}
    </section>
  );
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
