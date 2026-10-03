"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import {
  ConsoleShell,
  Status,
  consoleButton,
} from "@/components/console/ConsoleShell";
import { ReliabilityView } from "@/components/console/ReliabilityView";
import type { ReliabilityOverview } from "@/types/insights";

type App = {
  id: string;
  name: string;
  description: string;
  version: string;
  status: "healthy" | "down";
  latency: number;
  interval: number;
  threshold: number;
};
type Release = ReliabilityOverview["releases"][number];
type Incident = ReliabilityOverview["incidents"][number] & {
  appId: string;
  resolvedAt?: number;
};
const sampleTime = "2026-10-03T12:00:00.000Z";
const initialApps: App[] = [
  {
    id: "store",
    name: "Storefront",
    description: "Customer web application",
    version: "v2.4.0",
    status: "healthy",
    latency: 84,
    interval: 30,
    threshold: 3,
  },
  {
    id: "api",
    name: "Payments API",
    description: "Payment processing service",
    version: "v1.8.2",
    status: "healthy",
    latency: 126,
    interval: 60,
    threshold: 3,
  },
  {
    id: "worker",
    name: "Background worker",
    description: "Asynchronous order processing",
    version: "v3.1.0",
    status: "healthy",
    latency: 42,
    interval: 60,
    threshold: 2,
  },
];
const initialReleases: Release[] = initialApps.map((app, index) => ({
  id: `seed-${index}`,
  application_name: app.name,
  version: app.version,
  previous_version: null,
  status: "successful",
  kind: "deployment",
  created_at: sampleTime,
  duration_seconds: 8 + index * 3,
}));
const initialHistory = Array.from({ length: 25 }, (_, index) => ({
  time: new Date(Date.parse(sampleTime) - (24 - index) * 3600000).toISOString(),
  checks: 120,
  successful: index === 8 ? 116 : 120,
  latency_ms: Math.round(
    85 + Math.sin(index * 0.7) * 18 + (index === 8 ? 60 : 0),
  ),
}));

export default function DemoPage() {
  const [apps, setApps] = useState(initialApps);
  const [releases, setReleases] = useState(initialReleases);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [selected, setSelected] = useState("store");
  const [view, setView] = useState("Overview");
  const [version, setVersion] = useState("v2.5.0");
  const [failRelease, setFailRelease] = useState(false);
  const [busy, setBusy] = useState(false);
  const [stage, setStage] = useState("");
  const [message, setMessage] = useState(
    "Choose an application to check its health or try a release.",
  );
  const [events, setEvents] = useState([
    "Monitoring started for three sample applications.",
  ]);
  const [history, setHistory] = useState(initialHistory);
  const [clock, setClock] = useState(Date.parse(sampleTime));
  const generation = useRef(0);
  const running = useRef(false);
  const timer = useRef<{ id: number; resolve: () => void } | null>(null);
  const app = apps.find((item) => item.id === selected)!;
  useEffect(
    () => () => {
      generation.current++;
      if (timer.current) {
        window.clearTimeout(timer.current.id);
        timer.current.resolve();
      }
    },
    [],
  );
  function pause() {
    return new Promise<void>((resolve) => {
      const id = window.setTimeout(() => {
        timer.current = null;
        resolve();
      }, 650);
      timer.current = { id, resolve };
    });
  }
  function record(text: string) {
    setMessage(text);
    setEvents((previous) => [text, ...previous].slice(0, 12));
  }
  function tick() {
    const now = Date.now();
    setClock(now);
    return now;
  }
  function addProbe(healthy: boolean) {
    setHistory((previous) =>
      previous.map((bucket, index) =>
        index === previous.length - 1
          ? {
              ...bucket,
              checks: bucket.checks + 1,
              successful: bucket.successful + (healthy ? 1 : 0),
            }
          : bucket,
      ),
    );
  }
  function openIncident(target: App, title: string) {
    setIncidents((previous) =>
      previous.some(
        (item) => item.appId === target.id && item.status !== "resolved",
      )
        ? previous
        : [
            {
              id: crypto.randomUUID(),
              appId: target.id,
              application_name: target.name,
              title,
              status: "open",
              severity: "critical",
              created_at: new Date(tick()).toISOString(),
              duration_seconds: 0,
            },
            ...previous,
          ],
    );
  }
  function outage() {
    if (running.current || app.status === "down") return;
    setApps((previous) =>
      previous.map((item) =>
        item.id === app.id ? { ...item, status: "down" } : item,
      ),
    );
    addProbe(false);
    openIncident(app, `${app.name} health failure`);
    record(
      `${app.name}: simulated health check failed. An incident was opened.`,
    );
  }
  async function recover(appId: string) {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    const run = generation.current;
    const target = apps.find((item) => item.id === appId)!;
    setStage("Recovery / verifying");
    record(`${target.name}: verifying the last healthy version.`);
    await pause();
    if (run !== generation.current) return;
    const now = tick();
    setApps((previous) =>
      previous.map((item) =>
        item.id === appId ? { ...item, status: "healthy" } : item,
      ),
    );
    setIncidents((previous) =>
      previous.map((item) =>
        item.appId === appId && item.status !== "resolved"
          ? { ...item, status: "resolved", resolvedAt: now }
          : item,
      ),
    );
    addProbe(true);
    setReleases((previous) => [
      {
        id: crypto.randomUUID(),
        application_name: target.name,
        version: target.version,
        previous_version: target.version,
        status: "successful",
        kind: "rollback",
        created_at: new Date(now).toISOString(),
        duration_seconds: 0.65,
      },
      ...previous,
    ]);
    record(
      `${target.name}: recovered to the last healthy version. Incident resolved.`,
    );
    setStage("Recovery / successful");
    setBusy(false);
    running.current = false;
  }
  async function deploy() {
    const next = version.trim();
    if (!next || running.current || app.status === "down") return;
    running.current = true;
    setBusy(true);
    const run = generation.current;
    const id = crypto.randomUUID();
    const started = tick();
    const target = app;
    const fail = failRelease;
    setReleases((previous) => [
      {
        id,
        application_name: target.name,
        version: next,
        previous_version: target.version,
        status: "pending",
        kind: "deployment",
        created_at: new Date(started).toISOString(),
        duration_seconds: null,
      },
      ...previous,
    ]);
    for (const status of ["pending", "triggering", "verifying"]) {
      setStage(`Release / ${status}`);
      setReleases((previous) =>
        previous.map((item) => (item.id === id ? { ...item, status } : item)),
      );
      record(`${target.name}: release ${next} ${status}.`);
      await pause();
      if (run !== generation.current) return;
    }
    const now = tick();
    setReleases((previous) =>
      previous.map((item) =>
        item.id === id
          ? {
              ...item,
              status: fail ? "failed" : "successful",
              duration_seconds: (now - started) / 1000,
            }
          : item,
      ),
    );
    if (fail) {
      setApps((previous) =>
        previous.map((item) =>
          item.id === target.id ? { ...item, status: "down" } : item,
        ),
      );
      addProbe(false);
      openIncident(target, `${target.name} release verification failed`);
      record(
        `${target.name}: release ${next} failed verification. Last healthy version ${target.version} retained.`,
      );
    } else {
      setApps((previous) =>
        previous.map((item) =>
          item.id === target.id ? { ...item, version: next } : item,
        ),
      );
      addProbe(true);
      record(`${target.name}: release ${next} passed health verification.`);
    }
    setStage(`Release / ${fail ? "failed" : "successful"}`);
    setBusy(false);
    running.current = false;
  }
  function reset() {
    generation.current++;
    if (timer.current) {
      window.clearTimeout(timer.current.id);
      timer.current.resolve();
      timer.current = null;
    }
    running.current = false;
    setBusy(false);
    setStage("");
    setApps(initialApps);
    setReleases(initialReleases);
    setIncidents([]);
    setSelected("store");
    setVersion("v2.5.0");
    setFailRelease(false);
    setView("Overview");
    setHistory(initialHistory);
    setClock(Date.parse(sampleTime));
    setEvents(["Monitoring started for three sample applications."]);
    setMessage("Demo reset. Try a health check, deployment, or outage.");
  }
  const checks = history.reduce((sum, bucket) => sum + bucket.checks, 0);
  const successful = history.reduce(
    (sum, bucket) => sum + bucket.successful,
    0,
  );
  const data: ReliabilityOverview = {
    window_start: initialHistory[0].time,
    generated_at: sampleTime,
    metrics: {
      checks,
      successful,
      success_percent: (100 * successful) / checks,
      average_latency_ms:
        history.reduce((sum, bucket) => sum + (bucket.latency_ms ?? 0), 0) /
        history.length,
      p95_latency_ms: 146,
    },
    history,
    releases,
    incidents: incidents.map((item) => ({
      ...item,
      duration_seconds: Math.max(
        0,
        ((item.resolvedAt ?? clock) - Date.parse(item.created_at)) / 1000,
      ),
    })),
    active_incidents: incidents.filter((item) => item.status !== "resolved")
      .length,
  };
  return (
    <ConsoleShell
      demo
      actions={
        <>
          <button className={consoleButton} onClick={reset}>
            Reset demo
          </button>
          <Link href="/login" className={consoleButton}>
            Log in
          </Link>
        </>
      }
    >
      <div className="mb-6 flex flex-wrap justify-between gap-4">
        <div>
          <p className="font-mono text-[10px] tracking-widest text-teal-400">
            SANDBOX / OPERATIONS
          </p>
          <h1 className="mt-2 text-2xl font-semibold text-white">
            Reliability overview
          </h1>
          <p className="mt-2 text-xs text-slate-500">
            Three sample services. One place to monitor, release, and recover.
          </p>
        </div>
        <span className="self-start rounded-md border border-teal-900 px-3 py-2 font-mono text-[10px] text-teal-300">
          NO ACCOUNT REQUIRED
        </span>
      </div>
      <p className="mb-5 border-l-2 border-teal-600 pl-3 text-xs leading-5 text-slate-500">
        Interactive demo · Sample history and simulated probes. Actions stay in
        this tab and reset on refresh.
      </p>
      <nav aria-label="Demo sections" className="mb-5 flex flex-wrap gap-2">
        {["Overview", "Applications", "Deployments", "Incidents"].map((tab) => (
          <button
            key={tab}
            aria-pressed={view === tab}
            onClick={() => setView(tab)}
            className={`${consoleButton} ${view === tab ? "!border-teal-800 !text-teal-300" : ""}`}
          >
            {tab}
          </button>
        ))}
      </nav>
      <p
        role="status"
        className="mb-5 rounded-md border border-white/[0.07] bg-[#0d1117] px-4 py-3 font-mono text-xs text-slate-400"
      >
        {stage && <span className="mr-3 text-teal-300">{stage}</span>}
        {message}
      </p>
      {view === "Overview" && <ReliabilityView data={data} demo />}
      {(view === "Overview" || view === "Applications") && (
        <section className="mt-5 grid gap-5 xl:grid-cols-[1.2fr_1fr]">
          <div className="overflow-hidden rounded-lg border border-white/[0.07] bg-[#0d1117]">
            <div className="border-b border-white/[0.07] p-5">
              <h2 className="text-sm font-medium">Service inventory</h2>
              <p className="mt-1 text-xs text-slate-500">
                Select a service to try monitoring and releases
              </p>
            </div>
            {apps.map((item) => (
              <button
                key={item.id}
                disabled={busy}
                aria-pressed={selected === item.id}
                onClick={() => setSelected(item.id)}
                className={`flex w-full flex-wrap items-center justify-between gap-3 border-b border-white/[0.05] p-5 text-left disabled:opacity-60 ${selected === item.id ? "bg-white/[0.04]" : "hover:bg-white/[0.02]"}`}
              >
                <div>
                  <p className="text-sm">{item.name}</p>
                  <p className="mt-2 font-mono text-[10px] text-slate-500">
                    production / {item.version} /{" "}
                    {item.status === "healthy"
                      ? `${item.latency}ms`
                      : "HTTP 503"}
                  </p>
                </div>
                <Status value={item.status} />
              </button>
            ))}
          </div>
          <div className="rounded-lg border border-white/[0.07] bg-[#0d1117] p-5">
            <h2 className="text-sm font-medium">{app.name}</h2>
            <p className="mt-2 text-xs text-slate-500">
              Probe every {app.interval}s · Alert after {app.threshold} failed
              probes
            </p>
            <div className="my-5 flex flex-wrap gap-2">
              <button
                className={consoleButton}
                disabled={busy}
                onClick={() => {
                  addProbe(app.status === "healthy");
                  record(
                    `${app.name}: simulated check returned HTTP ${app.status === "healthy" ? `200 in ${app.latency} ms` : "503"}.`,
                  );
                }}
              >
                Check now
              </button>
              <button
                className={consoleButton}
                disabled={busy || app.status === "down"}
                onClick={outage}
              >
                Simulate outage
              </button>
              {app.status === "down" && (
                <button
                  className={consoleButton}
                  disabled={busy}
                  onClick={() => void recover(app.id)}
                >
                  Recover application
                </button>
              )}
            </div>
            <form
              onSubmit={(event) => {
                event.preventDefault();
                void deploy();
              }}
              className="border-t border-white/[0.07] pt-5"
            >
              <h3 className="text-sm font-medium">Release verification</h3>
              <label className="mt-4 block text-xs text-slate-400">
                Release version
                <input
                  required
                  maxLength={80}
                  disabled={busy}
                  value={version}
                  onChange={(event) => setVersion(event.target.value)}
                  className="mt-2 w-full rounded-md border border-white/15 bg-[#090c10] px-3 py-2 font-mono text-sm text-white"
                />
              </label>
              <label className="my-4 flex items-center gap-2 text-xs text-slate-400">
                <input
                  type="checkbox"
                  disabled={busy}
                  checked={failRelease}
                  onChange={(event) => setFailRelease(event.target.checked)}
                />
                Simulate failed verification
              </label>
              <button
                disabled={busy || !version.trim() || app.status === "down"}
                type="submit"
                className="rounded-md bg-teal-400 px-4 py-2.5 text-xs font-semibold text-slate-950 disabled:opacity-40"
              >
                Deploy release
              </button>
              {busy && <p className="mt-3 text-xs text-teal-300">{stage}…</p>}
              {app.status === "down" && (
                <p className="mt-3 text-xs text-amber-300">
                  Recover the application before trying another release.
                </p>
              )}
            </form>
          </div>
        </section>
      )}
      {view === "Deployments" && (
        <ReliabilityView data={{ ...data, incidents: [] }} demo />
      )}
      {view === "Incidents" && (
        <section className="space-y-3">
          {incidents.length === 0 ? (
            <p className="rounded-lg border border-white/10 p-8 text-sm text-slate-500">
              No incidents yet. Simulate an outage in Applications to try
              acknowledgment and recovery.
            </p>
          ) : (
            incidents.map((incident) => (
              <div
                key={incident.id}
                className="rounded-lg border border-white/10 bg-[#0d1117] p-5"
              >
                <div className="flex justify-between gap-3">
                  <h2 className="text-sm">{incident.title}</h2>
                  <Status value={incident.status} />
                </div>
                <p className="my-3 text-xs text-slate-500">
                  {incident.application_name} · critical
                </p>
                {incident.status !== "resolved" && (
                  <div className="flex flex-wrap gap-2">
                    <button
                      className={consoleButton}
                      disabled={busy || incident.status === "acknowledged"}
                      onClick={() => {
                        setIncidents((previous) =>
                          previous.map((item) =>
                            item.id === incident.id
                              ? { ...item, status: "acknowledged" }
                              : item,
                          ),
                        );
                        record(
                          `${incident.application_name}: incident acknowledged.`,
                        );
                      }}
                    >
                      Acknowledge
                    </button>
                    <button
                      className={consoleButton}
                      disabled={busy}
                      onClick={() => void recover(incident.appId)}
                    >
                      Recover last healthy version
                    </button>
                  </div>
                )}
              </div>
            ))
          )}
        </section>
      )}
      <section className="mt-5 rounded-lg border border-white/[0.07] bg-[#0d1117] p-5">
        <h2 className="text-sm font-medium">Event log</h2>
        <ol className="mt-4 divide-y divide-white/[0.05]">
          {events.map((event, index) => (
            <li
              key={`${index}-${event}`}
              className="flex gap-4 py-3 font-mono text-[11px] text-slate-400"
            >
              <span className="text-slate-600">
                {String(events.length - index).padStart(3, "0")}
              </span>
              {event}
            </li>
          ))}
        </ol>
      </section>
    </ConsoleShell>
  );
}
