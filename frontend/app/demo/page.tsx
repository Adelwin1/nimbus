"use client";

import Link from "next/link";
import { useState } from "react";

type Status = "healthy" | "down";
type DemoApp = { id: string; name: string; description: string; version: string; status: Status; latency: number };
type Release = { id: number; appId: string; version: string; status: "successful" | "failed"; kind: string };
type Incident = { id: number; appId: string; status: "open" | "acknowledged" | "resolved" };
const initialApps: DemoApp[] = [
  { id: "store", name: "Storefront", description: "Customer web application", version: "v2.4.0", status: "healthy", latency: 84 },
  { id: "api", name: "Payments API", description: "Payment processing service", version: "v1.8.2", status: "healthy", latency: 126 },
  { id: "worker", name: "Background worker", description: "Asynchronous order processing", version: "v3.1.0", status: "healthy", latency: 42 },
];
const initialReleases: Release[] = initialApps.map((app, index) => ({ id: index, appId: app.id, version: app.version, status: "successful", kind: "Deployment" }));
const button = "rounded-lg border border-slate-700 px-4 py-2 text-sm font-medium transition hover:border-sky-400 disabled:opacity-40";

export default function DemoPage() {
  const [apps, setApps] = useState(initialApps);
  const [releases, setReleases] = useState(initialReleases);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [selected, setSelected] = useState("store");
  const [view, setView] = useState("Applications");
  const [version, setVersion] = useState("v2.5.0");
  const [failRelease, setFailRelease] = useState(false);
  const [message, setMessage] = useState("Choose an application to check its health or try a release.");
  const [events, setEvents] = useState(["Monitoring started for three sample applications."]);
  const app = apps.find((item) => item.id === selected)!;
  const nameFor = (id: string) => apps.find((item) => item.id === id)?.name;

  function record(text: string) {
    setMessage(text);
    setEvents((previous) => [text, ...previous].slice(0, 12));
  }
  function outage() {
    if (app.status === "down") return;
    setApps((previous) => previous.map((item) => item.id === app.id ? { ...item, status: "down" } : item));
    setIncidents((previous) => [{ id: Date.now(), appId: app.id, status: "open" }, ...previous]);
    record(`${app.name}: simulated health check failed. An incident was opened.`);
  }
  function recover(appId: string) {
    setApps((previous) => previous.map((item) => item.id === appId ? { ...item, status: "healthy" } : item));
    setIncidents((previous) => previous.map((item) => item.appId === appId && item.status !== "resolved" ? { ...item, status: "resolved" } : item));
    record(`${nameFor(appId)}: recovered to the last healthy version. Incident resolved.`);
  }
  function deploy() {
    const nextVersion = version.trim();
    if (!nextVersion) return;
    setReleases((previous) => [{ id: Date.now(), appId: app.id, version: nextVersion, status: failRelease ? "failed" : "successful", kind: "Deployment" }, ...previous]);
    if (failRelease) {
      outage();
      record(`${app.name}: release ${nextVersion} failed verification. Last healthy version ${app.version} retained.`);
    } else {
      setApps((previous) => previous.map((item) => item.id === app.id ? { ...item, version: nextVersion } : item));
      record(`${app.name}: release ${nextVersion} passed health verification.`);
    }
  }
  function reset() {
    setApps(initialApps); setReleases(initialReleases); setIncidents([]);
    setSelected("store"); setView("Applications"); setVersion("v2.5.0"); setFailRelease(false);
    setEvents(["Monitoring started for three sample applications."]);
    setMessage("Demo reset. Try a health check, deployment, or outage.");
  }

  return (
    <main id="main-content" className="min-h-screen bg-slate-950 text-white">
      <header className="border-b border-slate-800">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-6 py-5">
          <Link href="/" className="text-xl font-semibold">Nimbus <span className="ml-2 text-xs font-normal text-sky-300">Interactive demo</span></Link>
          <div className="flex items-center gap-4"><button className={button} onClick={reset}>Reset demo</button><Link href="/login" className="text-sm text-slate-300 hover:text-white">Log in</Link></div>
        </div>
      </header>
      <div className="mx-auto max-w-7xl px-6 py-8">
        <p className="rounded-xl border border-sky-900 bg-sky-950/40 px-5 py-3 text-sm text-sky-200">Sample workspace · No account needed. All checks, releases, and recovery actions are simulated. Refresh to reset.</p>
        <div className="my-8"><p className="text-sm text-sky-400">Cloud reliability</p><h1 className="mt-2 text-3xl font-bold sm:text-4xl">Your applications, at a glance.</h1><p className="mt-3 text-slate-400">Explore monitoring, release verification, and incident recovery.</p></div>
        <div className="grid gap-4 sm:grid-cols-3">
          {[["Applications", apps.length], ["Healthy", apps.filter((item) => item.status === "healthy").length], ["Active incidents", incidents.filter((item) => item.status !== "resolved").length]].map(([label, count]) => <div key={label} className="rounded-xl border border-slate-800 bg-slate-900 p-5"><p className="text-sm text-slate-400">{label}</p><p className="mt-2 text-3xl font-semibold">{count}</p></div>)}
        </div>
        <nav aria-label="Demo sections" className="my-6 flex flex-wrap gap-2">{["Applications", "Deployments", "Incidents"].map((tab) => <button key={tab} aria-pressed={view === tab} onClick={() => setView(tab)} className={`${button} ${view === tab ? "bg-sky-950 text-sky-300 border-sky-700" : "text-slate-300"}`}>{tab}</button>)}</nav>
        <p role="status" className="mb-6 rounded-lg bg-slate-900 px-4 py-3 text-sm text-sky-200">{message}</p>
        {view === "Applications" && <div className="grid gap-6 lg:grid-cols-[1fr_1.2fr]">
          <section aria-label="Sample applications" className="space-y-3">{apps.map((item) => <button key={item.id} aria-pressed={selected === item.id} onClick={() => setSelected(item.id)} className={`w-full rounded-xl border p-5 text-left ${selected === item.id ? "border-sky-500 bg-sky-950/30" : "border-slate-800 bg-slate-900"}`}><div className="flex items-center justify-between gap-3"><h2 className="font-semibold">{item.name}</h2><Badge status={item.status}/></div><p className="mt-2 text-sm text-slate-400">{item.description}</p><p className="mt-3 text-xs text-slate-400">Production · {item.version} · {item.status === "healthy" ? `${item.latency} ms` : "HTTP 503"}</p></button>)}</section>
          <section className="rounded-xl border border-slate-800 bg-slate-900 p-6"><h2 className="text-xl font-semibold">{app.name}</h2><p className="mt-2 text-sm text-slate-400">Monitoring every 30 seconds · Failure threshold: 3 checks</p><div className="my-6 flex flex-wrap gap-3"><button className={button} onClick={() => record(`${app.name}: simulated check returned HTTP ${app.status === "healthy" ? `200 in ${app.latency} ms` : "503"}.`)}>Check now</button><button className={button} disabled={app.status === "down"} onClick={outage}>Simulate outage</button>{app.status === "down" && <button className={button} onClick={() => recover(app.id)}>Recover application</button>}</div><div className="border-t border-slate-800 pt-5"><h3 className="font-semibold">Verify a release</h3><form onSubmit={(event) => { event.preventDefault(); deploy(); }} className="mt-4 space-y-4"><label className="block text-sm text-slate-300">Release version<input value={version} onChange={(event) => setVersion(event.target.value)} required maxLength={80} className="mt-2 block w-full rounded-lg border border-slate-700 bg-slate-950 px-4 py-3 text-white" /></label><label className="flex items-center gap-2 text-sm text-slate-400"><input type="checkbox" checked={failRelease} onChange={(event) => setFailRelease(event.target.checked)}/>Simulate failed verification</label><button type="submit" disabled={!version.trim() || app.status === "down"} className="rounded-lg bg-sky-500 px-5 py-3 text-sm font-semibold text-slate-950 hover:bg-sky-400 disabled:opacity-40">Deploy release</button>{app.status === "down" && <p className="text-sm text-amber-300">Recover the application before trying another release.</p>}</form></div></section>
        </div>}
        {view === "Deployments" && <section aria-label="Release history" className="space-y-3"><h2 className="text-xl font-semibold">Release history</h2>{releases.map((release) => <div key={release.id} className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-800 bg-slate-900 p-5"><div><h3 className="font-semibold">{nameFor(release.appId)} · {release.version}</h3><p className="mt-1 text-sm text-slate-400">{release.kind} · {release.status === "successful" ? "Health verification passed" : "Health verification failed; previous version retained"}</p></div><Badge status={release.status}/></div>)}</section>}
        {view === "Incidents" && <section aria-label="Incident list" className="space-y-3"><h2 className="text-xl font-semibold">Incident response</h2>{incidents.length === 0 ? <div className="rounded-xl border border-dashed border-slate-700 p-8 text-slate-400">No incidents yet. Simulate an outage in Applications to try acknowledgment and recovery.</div> : incidents.map((incident) => <div key={incident.id} className="rounded-xl border border-slate-800 bg-slate-900 p-5"><div className="flex flex-wrap justify-between gap-3"><h3 className="font-semibold">{nameFor(incident.appId)} health failure</h3><Badge status={incident.status}/></div><p className="my-3 text-sm text-slate-400">Consecutive failed checks triggered a critical incident.</p>{incident.status !== "resolved" && <div className="flex flex-wrap gap-3"><button className={button} disabled={incident.status === "acknowledged"} onClick={() => { setIncidents((previous) => previous.map((item) => item.id === incident.id ? { ...item, status: "acknowledged" } : item)); record(`${nameFor(incident.appId)}: incident acknowledged.`); }}>Acknowledge</button><button className={button} onClick={() => recover(incident.appId)}>Recover last healthy version</button></div>}</div>)}</section>}
        <section className="mt-8 rounded-xl border border-slate-800 p-6"><h2 className="font-semibold">Activity timeline</h2><ol className="mt-4 space-y-3">{events.map((event, index) => <li key={`${index}-${event}`} className="border-l-2 border-sky-800 pl-4 text-sm text-slate-400">{event}</li>)}</ol></section>
      </div>
    </main>
  );
}
function Badge({ status }: { status: string }) {
  const good = ["healthy", "successful", "resolved"].includes(status);
  return <span className={`rounded-full px-3 py-1 text-xs font-medium ${good ? "bg-emerald-950 text-emerald-300" : "bg-amber-950 text-amber-300"}`}>{status}</span>;
}
