"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { getHealthHistory, getHealthOverview, runHealthCheck } from "@/lib/api";
import type { HealthCheck, HealthOverview } from "@/types/health";

type HealthPanelProps = {
  applicationId: string;
};

export function HealthPanel({ applicationId }: HealthPanelProps) {
  const [overview, setOverview] = useState<HealthOverview | null>(null);
  const [checks, setChecks] = useState<HealthCheck[]>([]);
  const [loading, setLoading] = useState(true);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");

  const loadHealthData = useCallback(async () => {
    try {
      const [overviewResponse, historyResponse] = await Promise.all([
        getHealthOverview(applicationId),
        getHealthHistory(applicationId, 50, 0),
      ]);

      setOverview(overviewResponse.health);
      setChecks(historyResponse.checks);
      setError("");
    } catch (loadError) {
      setError(
        loadError instanceof Error
          ? loadError.message
          : "Health information could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [applicationId]);

  useEffect(() => {
    const initialLoad = window.setTimeout(() => {
      void loadHealthData();
    }, 0);

    const interval = window.setInterval(() => {
      void loadHealthData();
    }, 15000);

    return () => {
      window.clearTimeout(initialLoad);
      window.clearInterval(interval);
    };
  }, [loadHealthData]);

  async function handleCheckNow() {
    setChecking(true);
    setError("");

    try {
      await runHealthCheck(applicationId);
      await loadHealthData();
    } catch (checkError) {
      setError(
        checkError instanceof Error
          ? checkError.message
          : "The health check could not be completed.",
      );
    } finally {
      setChecking(false);
    }
  }

  const chartData = useMemo(
    () =>
      [...checks].reverse().map((check) => ({
        checkedAt: formatChartDate(check.checked_at),
        latency: check.latency_ms,
        healthy: check.healthy,
      })),
    [checks],
  );

  if (loading) {
    return (
      <section className="mt-8 rounded-md border border-white/10 bg-[#0d1117] px-6 py-16 text-center text-slate-400">
        Loading health information...
      </section>
    );
  }

  return (
    <section className="mt-8 space-y-6">
      <div className="flex flex-col justify-between gap-5 rounded-md border border-white/10 bg-[#0d1117] p-6 md:flex-row md:items-center">
        <div>
          <p className="text-sm font-medium uppercase tracking-wider text-teal-300">
            Real monitoring
          </p>

          <h2 className="mt-2 text-2xl font-semibold text-white">
            Application health
          </h2>

          <p className="mt-2 text-sm text-slate-400">
            Results below come from real HTTP checks stored by Nimbus.
          </p>
        </div>

        <button
          type="button"
          onClick={handleCheckNow}
          disabled={checking}
          className="rounded-md bg-sky-500 px-5 py-3 font-medium text-slate-950 transition hover:bg-sky-400 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {checking ? "Checking..." : "Check now"}
        </button>
      </div>

      {error ? (
        <div className="rounded-md border border-red-900 bg-red-950/40 px-5 py-4 text-sm text-red-300">
          {error}
        </div>
      ) : null}

      {overview ? (
        <>
          <HealthSummaryCards overview={overview} />

          <div className="grid gap-6 xl:grid-cols-3">
            <div className="rounded-md border border-white/10 bg-[#0d1117] p-6 xl:col-span-2">
              <div>
                <h3 className="text-lg font-semibold text-white">
                  Latency history
                </h3>

                <p className="mt-1 text-sm text-slate-400">
                  Response latency from stored health-check results.
                </p>
              </div>

              {chartData.length === 0 ? (
                <ChartEmptyState />
              ) : (
                <div className="mt-6 h-80 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={chartData}>
                      <CartesianGrid strokeDasharray="3 3" stroke="#334155" />

                      <XAxis
                        dataKey="checkedAt"
                        stroke="#94a3b8"
                        tick={{ fontSize: 12 }}
                        minTickGap={30}
                      />

                      <YAxis
                        stroke="#94a3b8"
                        tick={{ fontSize: 12 }}
                        unit="ms"
                        width={70}
                      />

                      <Tooltip
                        contentStyle={{
                          backgroundColor: "#0f172a",
                          border: "1px solid #334155",
                          borderRadius: "12px",
                        }}
                        labelStyle={{
                          color: "#e2e8f0",
                        }}
                        formatter={(value) => [
                          `${Number(value)} ms`,
                          "Latency",
                        ]}
                      />

                      <Line
                        type="monotone"
                        dataKey="latency"
                        stroke="#38bdf8"
                        strokeWidth={3}
                        dot={{
                          r: 3,
                          fill: "#38bdf8",
                        }}
                        activeDot={{
                          r: 6,
                        }}
                      />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              )}
            </div>

            <AvailabilityCard overview={overview} />
          </div>

          <HealthCheckTable checks={checks} />
        </>
      ) : (
        <div className="rounded-md border border-white/10 bg-[#0d1117] px-6 py-14 text-center">
          <h3 className="text-lg font-semibold">
            Health information unavailable
          </h3>

          <p className="mt-2 text-sm text-slate-400">
            Run a manual check to create the first health result.
          </p>
        </div>
      )}
    </section>
  );
}

function HealthSummaryCards({ overview }: { overview: HealthOverview }) {
  const { state, statistics } = overview;

  return (
    <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
      <HealthCard
        label="Current status"
        value={capitalize(state.status)}
        description={
          state.last_checked_at
            ? `Checked ${formatDate(state.last_checked_at)}`
            : "No checks yet"
        }
      />

      <HealthCard
        label="Availability"
        value={`${statistics.availability_percent.toFixed(2)}%`}
        description={`${statistics.successful_checks} successful checks`}
      />

      <HealthCard
        label="Average latency"
        value={`${Math.round(statistics.average_latency_ms)} ms`}
        description={`Maximum ${statistics.maximum_latency_ms} ms`}
      />

      <HealthCard
        label="Total checks"
        value={String(statistics.total_checks)}
        description={`${statistics.failed_checks} failed checks`}
      />
    </div>
  );
}

function HealthCard({
  label,
  value,
  description,
}: {
  label: string;
  value: string;
  description: string;
}) {
  return (
    <div className="rounded-md border border-white/10 bg-[#0d1117] p-5">
      <p className="text-sm text-slate-400">{label}</p>

      <p className="mt-3 text-2xl font-bold text-white">{value}</p>

      <p className="mt-2 text-xs text-slate-500">{description}</p>
    </div>
  );
}

function AvailabilityCard({ overview }: { overview: HealthOverview }) {
  const { statistics, state } = overview;

  return (
    <div className="rounded-md border border-white/10 bg-[#0d1117] p-6">
      <h3 className="text-lg font-semibold text-white">Availability</h3>

      <p className="mt-1 text-sm text-slate-400">
        Percentage of stored checks that returned a successful HTTP response.
      </p>

      <div className="mt-8 text-center">
        <p className="text-5xl font-bold text-teal-300">
          {statistics.availability_percent.toFixed(2)}%
        </p>

        <p className="mt-3 text-sm text-slate-400">
          {statistics.successful_checks} of {statistics.total_checks} checks
          successful
        </p>
      </div>

      <div className="mt-8 space-y-4 border-t border-white/10 pt-6">
        <StatRow
          label="Successful"
          value={String(statistics.successful_checks)}
        />

        <StatRow label="Failed" value={String(statistics.failed_checks)} />

        <StatRow
          label="Consecutive failures"
          value={String(state.consecutive_failures)}
        />

        <StatRow
          label="Minimum latency"
          value={`${statistics.minimum_latency_ms} ms`}
        />

        <StatRow
          label="Maximum latency"
          value={`${statistics.maximum_latency_ms} ms`}
        />
      </div>
    </div>
  );
}

function HealthCheckTable({ checks }: { checks: HealthCheck[] }) {
  return (
    <div className="overflow-hidden rounded-md border border-white/10 bg-[#0d1117]">
      <div className="border-b border-white/10 p-6">
        <h3 className="text-lg font-semibold text-white">
          Health-check history
        </h3>

        <p className="mt-1 text-sm text-slate-400">
          Latest real results recorded by the monitoring engine.
        </p>
      </div>

      {checks.length === 0 ? (
        <div className="px-6 py-14 text-center text-sm text-slate-400">
          No health checks have been stored yet.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-800">
            <thead className="bg-[#090c10]">
              <tr>
                <TableHeader>Result</TableHeader>
                <TableHeader>Status code</TableHeader>
                <TableHeader>Latency</TableHeader>
                <TableHeader>Checked</TableHeader>
                <TableHeader>Error</TableHeader>
              </tr>
            </thead>

            <tbody className="divide-y divide-slate-800">
              {checks.map((check) => (
                <tr key={check.id}>
                  <TableCell>
                    <span
                      className={`inline-flex rounded-full border px-3 py-1 text-xs font-medium ${
                        check.healthy
                          ? "border-emerald-800 bg-emerald-950 text-emerald-300"
                          : "border-red-800 bg-red-950 text-red-300"
                      }`}
                    >
                      {check.healthy ? "Successful" : "Failed"}
                    </span>
                  </TableCell>

                  <TableCell>{check.status_code ?? "No response"}</TableCell>

                  <TableCell>{check.latency_ms} ms</TableCell>

                  <TableCell>{formatDate(check.checked_at)}</TableCell>

                  <TableCell>
                    <span className="block max-w-xs truncate">
                      {check.error_message ?? "—"}
                    </span>
                  </TableCell>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function TableHeader({ children }: { children: React.ReactNode }) {
  return (
    <th className="px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider text-slate-500">
      {children}
    </th>
  );
}

function TableCell({ children }: { children: React.ReactNode }) {
  return (
    <td className="whitespace-nowrap px-6 py-4 text-sm text-slate-300">
      {children}
    </td>
  );
}

function StatRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-5">
      <span className="text-sm text-slate-400">{label}</span>

      <span className="text-sm font-medium text-slate-100">{value}</span>
    </div>
  );
}

function ChartEmptyState() {
  return (
    <div className="mt-6 flex h-80 items-center justify-center rounded-md border border-dashed border-slate-700 bg-[#090c10] text-sm text-slate-400">
      Run a health check to begin the latency chart.
    </div>
  );
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatChartDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
