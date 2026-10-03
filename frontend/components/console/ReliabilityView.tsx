"use client";
import Link from "next/link";
import {
  LineChart,
  Line,
  ResponsiveContainer,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import type { ReliabilityOverview } from "@/types/insights";
import { Status } from "./ConsoleShell";

export function duration(seconds: number | null) {
  if (seconds === null) return "—";
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600)
    return `${Math.floor(seconds / 60)}m ${Math.floor(seconds % 60)}s`;
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}
export function ReliabilityView({
  data,
  demo = false,
}: {
  data: ReliabilityOverview;
  demo?: boolean;
}) {
  const metrics = data.metrics;
  const cards = [
    [
      "CHECK SUCCESS",
      metrics.success_percent === null
        ? "—"
        : `${metrics.success_percent.toFixed(2)}%`,
      `${metrics.successful} / ${metrics.checks} probes · past 24h`,
    ],
    [
      "P95 LATENCY",
      metrics.p95_latency_ms === null
        ? "—"
        : `${Math.round(metrics.p95_latency_ms)} ms`,
      "Successful responses · past 24h",
    ],
    [
      "AVERAGE LATENCY",
      metrics.average_latency_ms === null
        ? "—"
        : `${Math.round(metrics.average_latency_ms)} ms`,
      "Successful responses · past 24h",
    ],
    [
      "ACTIVE INCIDENTS",
      String(data.active_incidents),
      "Open and acknowledged",
    ],
  ];
  return (
    <>
      <div className="grid divide-y divide-white/[0.07] overflow-hidden rounded-lg border border-white/[0.07] bg-[#0d1117] sm:grid-cols-2 sm:divide-y-0 xl:grid-cols-4">
        {cards.map(([label, value, note]) => (
          <div key={label} className="border-white/[0.07] p-5 sm:border-r">
            <p className="font-mono text-[10px] tracking-widest text-slate-500">
              {label}
            </p>
            <p className="mt-3 font-mono text-3xl tracking-tight text-white">
              {value}
            </p>
            <p className="mt-3 text-xs text-slate-500">{note}</p>
          </div>
        ))}
      </div>
      <section className="mt-5 rounded-lg border border-white/[0.07] bg-[#0d1117] p-5">
        <div className="flex flex-wrap justify-between gap-3">
          <div>
            <h2 className="text-sm font-medium">Response latency</h2>
            <p className="mt-1 text-xs text-slate-500">
              Hourly average · past 24 hours · gaps mean no successful probes
            </p>
          </div>
          <span className="font-mono text-xs text-teal-300">
            {demo ? "SAMPLE HISTORY" : "STORED HTTP CHECKS"}
          </span>
        </div>
        {metrics.checks === 0 ? (
          <div className="flex h-56 items-center justify-center text-sm text-slate-500">
            No checks in this window. Add an application or run a health check.
          </div>
        ) : (
          <div className="mt-6 h-56 min-w-0">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={data.history}>
                <CartesianGrid
                  vertical={false}
                  stroke="#202830"
                  strokeDasharray="3 5"
                />
                <XAxis
                  dataKey="time"
                  tickFormatter={(time: string) =>
                    new Date(time).toLocaleTimeString([], {
                      hour: "2-digit",
                      minute: "2-digit",
                    })
                  }
                  stroke="#56616d"
                  tick={{ fontSize: 10 }}
                  minTickGap={50}
                />
                <YAxis
                  stroke="#56616d"
                  tick={{ fontSize: 10 }}
                  width={48}
                  unit="ms"
                />
                <Tooltip
                  labelFormatter={(time) =>
                    new Date(String(time)).toLocaleString()
                  }
                  contentStyle={{
                    background: "#111820",
                    border: "1px solid #28313a",
                    borderRadius: 6,
                    color: "#cbd5e1",
                  }}
                />
                <Line
                  dataKey="latency_ms"
                  name="Average latency (ms)"
                  stroke="#2dd4bf"
                  strokeWidth={2}
                  dot={false}
                  connectNulls={false}
                  isAnimationActive={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
        <div className="mt-3 flex h-6 gap-1" aria-label="Hourly check results">
          {data.history.map((bucket) => (
            <div
              key={bucket.time}
              title={`${new Date(bucket.time).toLocaleString()}: ${bucket.successful}/${bucket.checks} successful probes`}
              className={`flex-1 rounded-sm ${bucket.checks === 0 ? "bg-slate-800" : bucket.successful < bucket.checks ? "bg-rose-400/60" : "bg-teal-400/50"}`}
            />
          ))}
        </div>
        <p className="mt-2 text-[10px] text-slate-500">
          Teal: all probes succeeded · Rose: failed probes · Gray: no data.
          Check success is sampled availability, not continuous uptime.
        </p>
      </section>
      <div className="mt-5 grid gap-5 xl:grid-cols-2">
        <section className="overflow-hidden rounded-lg border border-white/[0.07] bg-[#0d1117]">
          <div className="border-b border-white/[0.07] p-5">
            <h2 className="text-sm font-medium">Recent releases</h2>
            <p className="mt-1 text-xs text-slate-500">
              Compare previous and target versions
            </p>
          </div>
          {data.releases.length === 0 ? (
            <p className="p-6 text-sm text-slate-500">No releases recorded.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="text-slate-500">
                  <tr>
                    <th className="p-4 font-normal">Application / version</th>
                    <th className="p-4 font-normal">Result</th>
                    <th className="p-4 font-normal">Duration</th>
                  </tr>
                </thead>
                <tbody>
                  {data.releases.map((release) => (
                    <tr
                      key={release.id}
                      className="border-t border-white/[0.05]"
                    >
                      <td className="p-4">
                        <p>
                          {demo ? (
                            release.application_name
                          ) : (
                            <Link
                              href={`/deployments/${release.id}`}
                              className="hover:text-teal-300"
                            >
                              {release.application_name}
                            </Link>
                          )}
                        </p>
                        <p className="mt-2 font-mono text-slate-500">
                          {release.previous_version ?? "initial"} →{" "}
                          {release.version}
                        </p>
                        <p className="mt-1 text-[10px] text-slate-600">
                          {release.kind}
                        </p>
                      </td>
                      <td className="p-4">
                        <Status value={release.status} />
                      </td>
                      <td className="p-4 font-mono text-slate-400">
                        {duration(release.duration_seconds)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
        <section className="rounded-lg border border-white/[0.07] bg-[#0d1117]">
          <div className="border-b border-white/[0.07] p-5">
            <h2 className="text-sm font-medium">Incident response</h2>
            <p className="mt-1 text-xs text-slate-500">
              Active incidents first · duration at last refresh
            </p>
          </div>
          {data.incidents.length === 0 ? (
            <p className="p-6 text-sm text-slate-500">No incidents recorded.</p>
          ) : (
            data.incidents.map((incident) => (
              <div
                key={incident.id}
                className="border-b border-white/[0.05] p-5"
              >
                <div className="flex justify-between gap-3">
                  <p className="text-sm">
                    {demo ? (
                      incident.title
                    ) : (
                      <Link
                        href={`/incidents/${incident.id}`}
                        className="hover:text-teal-300"
                      >
                        {incident.title}
                      </Link>
                    )}
                  </p>
                  <Status value={incident.status} />
                </div>
                <p className="mt-2 text-xs text-slate-500">
                  {incident.application_name} · {incident.severity} ·{" "}
                  {duration(incident.duration_seconds)}
                </p>
              </div>
            ))
          )}
        </section>
      </div>
    </>
  );
}
