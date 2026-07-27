"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import {
  IncidentSeverityBadge,
  IncidentStatusBadge,
} from "@/components/incident/IncidentBadges";
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

  const loadIncidents = useCallback(async () => {
    try {
      const response = await getIncidents();

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
  }, []);

  useEffect(() => {
    const initialLoad = window.setTimeout(() => {
      void loadIncidents();
    }, 0);

    const refresh = window.setInterval(() => {
      void loadIncidents();
    }, 15000);

    return () => {
      window.clearTimeout(initialLoad);
      window.clearInterval(refresh);
    };
  }, [loadIncidents]);

  const activeCount = incidents.filter(
    (incident) => incident.status !== "resolved",
  ).length;

  return (
    <main
      id="main-content"
      className="min-h-screen bg-slate-950 px-5 py-10 text-slate-100"
    >
      <div className="mx-auto max-w-6xl">
        <Link
          href="/dashboard"
          className="text-sm font-medium text-sky-400 hover:text-sky-300"
        >
          ← Dashboard
        </Link>

        <div className="mt-6">
          <p className="text-sm font-semibold uppercase tracking-wider text-red-400">
            Reliability response
          </p>

          <h1 className="mt-2 text-3xl font-bold text-white">Incidents</h1>

          <p className="mt-3 text-slate-400">
            {activeCount} active incident
            {activeCount === 1 ? "" : "s"} across your applications.
          </p>
        </div>

        {error ? (
          <div className="mt-8 rounded-xl border border-red-900 bg-red-950/40 px-5 py-4 text-sm text-red-300">
            {error}
          </div>
        ) : null}

        <div className="mt-8 overflow-hidden rounded-2xl border border-slate-800 bg-slate-900">
          {loading ? (
            <div className="px-6 py-16 text-center text-slate-400">
              Loading incidents...
            </div>
          ) : incidents.length === 0 ? (
            <div className="px-6 py-16 text-center">
              <h2 className="text-lg font-semibold text-white">No incidents</h2>

              <p className="mt-2 text-sm text-slate-400">
                Nimbus has not detected any qualifying reliability failures.
              </p>
            </div>
          ) : (
            <div className="divide-y divide-slate-800">
              {incidents.map((incident) => (
                <Link
                  key={incident.id}
                  href={`/incidents/${incident.id}`}
                  className="grid gap-5 p-6 transition hover:bg-slate-800/50 lg:grid-cols-[1.5fr_1fr_auto] lg:items-center"
                >
                  <div>
                    <div className="flex flex-wrap items-center gap-3">
                      <p className="font-semibold text-white">
                        {incident.title}
                      </p>

                      <IncidentStatusBadge status={incident.status} />

                      <IncidentSeverityBadge severity={incident.severity} />
                    </div>

                    <p className="mt-2 line-clamp-2 text-sm text-slate-400">
                      {incident.summary}
                    </p>
                  </div>

                  <div>
                    <p className="text-xs uppercase tracking-wider text-slate-500">
                      Application
                    </p>

                    <p className="mt-1 text-sm text-slate-300">
                      {incident.application_name}
                    </p>
                  </div>

                  <time className="text-sm text-slate-500">
                    {formatDate(incident.created_at)}
                  </time>
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
