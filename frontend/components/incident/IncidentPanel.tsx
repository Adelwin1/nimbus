"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import {
  IncidentSeverityBadge,
  IncidentStatusBadge,
} from "@/components/incident/IncidentBadges";
import { getApplicationIncidents } from "@/lib/api";
import { subscribeToApplicationEvents } from "@/lib/live-events";
import type { IncidentListItem } from "@/types/incident";

export function IncidentPanel({ applicationId }: { applicationId: string }) {
  const [incidents, setIncidents] = useState<IncidentListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [live, setLive] = useState(false);
  const [error, setError] = useState("");

  const loadIncidents = useCallback(async () => {
    try {
      const response = await getApplicationIncidents(applicationId);

      setIncidents(response.incidents);
      setError("");
    } catch (loadError) {
      setError(
        loadError instanceof Error
          ? loadError.message
          : "Incidents could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [applicationId]);

  useEffect(() => {
    const initialLoad = window.setTimeout(() => {
      void loadIncidents();
    }, 0);

    const unsubscribe = subscribeToApplicationEvents(applicationId, {
      onConnected: () => setLive(true),

      onIncident: () => {
        setLive(true);
        void loadIncidents();
      },

      onConnectionError: () => {
        setLive(false);
      },
    });

    return () => {
      window.clearTimeout(initialLoad);
      unsubscribe();
    };
  }, [applicationId, loadIncidents]);

  const activeCount = incidents.filter(
    (incident) => incident.status !== "resolved",
  ).length;

  return (
    <section className="mt-8 overflow-hidden rounded-2xl border border-slate-800 bg-slate-900">
      <div className="flex flex-col justify-between gap-5 border-b border-slate-800 p-6 md:flex-row md:items-center">
        <div>
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="text-xl font-semibold text-white">Incidents</h2>

            <span className="rounded-full border border-red-800 bg-red-950 px-3 py-1 text-xs font-semibold text-red-300">
              {activeCount} active
            </span>

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

          <p className="mt-2 text-sm text-slate-400">
            Failures, latency problems, deployment failures, and recovery
            progress.
          </p>
        </div>

        <Link
          href="/incidents"
          className="rounded-xl border border-slate-700 px-4 py-2.5 text-center text-sm font-medium text-slate-200 transition hover:bg-slate-800"
        >
          View all incidents
        </Link>
      </div>

      {error ? (
        <div className="border-b border-red-900 bg-red-950/40 px-6 py-4 text-sm text-red-300">
          {error}
        </div>
      ) : null}

      {loading ? (
        <div className="px-6 py-14 text-center text-sm text-slate-400">
          Loading incidents...
        </div>
      ) : incidents.length === 0 ? (
        <div className="px-6 py-14 text-center">
          <p className="font-medium text-slate-200">No incidents recorded</p>

          <p className="mt-2 text-sm text-slate-400">
            Nimbus will open one automatically when a real failure threshold is
            reached.
          </p>
        </div>
      ) : (
        <div className="divide-y divide-slate-800">
          {incidents.slice(0, 3).map((incident) => (
            <Link
              key={incident.id}
              href={`/incidents/${incident.id}`}
              className="flex flex-col justify-between gap-4 p-6 transition hover:bg-slate-800/50 md:flex-row md:items-center"
            >
              <div>
                <div className="flex flex-wrap items-center gap-3">
                  <p className="font-semibold text-white">{incident.title}</p>

                  <IncidentStatusBadge status={incident.status} />

                  <IncidentSeverityBadge severity={incident.severity} />
                </div>

                <p className="mt-2 line-clamp-2 text-sm text-slate-400">
                  {incident.summary}
                </p>
              </div>

              <time className="shrink-0 text-sm text-slate-500">
                {formatDate(incident.created_at)}
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
