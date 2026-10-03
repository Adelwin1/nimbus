"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import {
  ConsoleShell, Status, consoleButton,
} from "@/components/console/ConsoleShell";
import { apiRequest } from "@/lib/api";

type Application = {
  id: string;
  name: string;
  environment: string;
  status: string;
  project: string;
};

export default function ProjectsPage() {
  return <ProtectedRoute><Projects /></ProtectedRoute>;
}

function Projects() {
  const [applications, setApplications] = useState<Application[]>([]);
  const [environment, setEnvironment] = useState("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const result = await apiRequest<{ applications: Application[] }>(
        "/projects/", { authenticated: true },
      );
      setApplications(result.applications);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Projects unavailable.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => void load(), 0);
    return () => clearTimeout(timer);
  }, [load]);

  const visible = applications.filter(
    app => environment === "all" || app.environment === environment,
  );
  const groups = [...new Set(visible.map(app => app.project))];

  return (
    <ConsoleShell actions={
      <button className={consoleButton} onClick={() => void load()}>
        Refresh
      </button>
    }>
      <h1 className="text-2xl font-semibold">Projects</h1>
      <p className="mt-2 text-sm text-slate-400">
        Use the same project name to group services together.
      </p>

      <label className="mt-5 block max-w-xs text-xs text-slate-400">
        Environment
        <select
          value={environment}
          onChange={event => setEnvironment(event.target.value)}
          className="mt-2 block w-full rounded-md border border-white/15 bg-[#090c10] p-2 text-sm text-slate-200"
        >
          {["all", "development", "staging", "production"].map(value => (
            <option key={value} value={value}>{value}</option>
          ))}
        </select>
      </label>

      {error ? <p role="alert" className="mt-4 text-rose-300">{error}</p> : null}
      {loading ? <p className="mt-6 text-slate-500">Loading projects…</p> : null}
      {!loading && !visible.length ? (
        <p className="mt-6 text-slate-500">No applications in this view.</p>
      ) : null}

      {groups.map(group => (
        <section key={group} className="mt-7">
          <h2 className="font-mono text-sm text-slate-400">{group}</h2>
          <div className="mt-3 grid gap-4 xl:grid-cols-2">
            {visible.filter(app => app.project === group).map(app => (
              <ProjectCard
                key={`${app.id}:${app.project}`}
                app={app}
                onSaved={load}
              />
            ))}
          </div>
        </section>
      ))}
    </ConsoleShell>
  );
}

function ProjectCard({ app, onSaved }: {
  app: Application;
  onSaved: () => Promise<void>;
}) {
  const [project, setProject] = useState(app.project);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function save(event: React.FormEvent) {
    event.preventDefault();
    if (saving || !project.trim()) return;
    setSaving(true);
    setError("");

    try {
      await apiRequest(`/projects/apps/${app.id}`, {
        method: "PUT",
        authenticated: true,
        body: JSON.stringify({ project: project.trim() }),
      });
      await onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Save failed.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <article className="rounded-md border border-white/10 bg-[#0d1117] p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Link href={`/apps/${app.id}`} className="font-semibold text-teal-300">
          {app.name}
        </Link>
        <Status value={app.status} />
      </div>
      <p className="mt-2 text-xs text-slate-500">{app.environment}</p>

      <form onSubmit={save} className="mt-4">
        <label className="text-xs text-slate-400">
          Project name
          <input
            value={project}
            onChange={event => setProject(event.target.value)}
            required
            maxLength={80}
            disabled={saving}
            className="mt-2 block w-full rounded-md border border-white/15 bg-[#090c10] p-2 text-sm text-slate-200"
          />
        </label>
        <button
          className={`mt-3 ${consoleButton}`}
          disabled={saving || !project.trim()}
        >
          {saving ? "Saving…" : "Save project"}
        </button>
        {error ? (
          <p role="alert" className="mt-3 text-sm text-rose-300">{error}</p>
        ) : null}
      </form>
    </article>
  );
}
