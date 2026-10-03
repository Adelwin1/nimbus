"use client";

import { useEffect, useState } from "react";
import { getApplication, getDeployments } from "@/lib/api";
import type { Deployment } from "@/types/deployment";

export function GitHubReleaseLinks({ deployment }: { deployment: Deployment }) {
  const [repository, setRepository] = useState("");
  const [previous, setPrevious] = useState("");

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const app = await getApplication(deployment.application_id);
        const url = new URL(
          app.application.repository_url || "https://invalid.local"
        );
        const path = url.pathname
          .replace(/\.git\/?$/, "")
          .replace(/\/$/, "");

        if (
          url.protocol !== "https:" ||
          url.hostname !== "github.com" ||
          !/^\/[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(path)
        ) return;

        if (!cancelled) setRepository(`https://github.com${path}`);

        const response = await getDeployments(deployment.application_id);
        const earlier = response.deployments
          .filter((item) =>
            item.status === "successful" &&
            item.id !== deployment.id &&
            Date.parse(item.created_at) < Date.parse(deployment.created_at)
          )
          .sort((a, b) =>
            Date.parse(b.created_at) - Date.parse(a.created_at)
          )[0];

        if (!cancelled) setPrevious(earlier?.commit_sha || "");
      } catch {
        // Optional source links do not block deployment details.
      }
    }

    void load();
    return () => { cancelled = true; };
  }, [deployment.application_id, deployment.id, deployment.created_at]);

  const commit = deployment.commit_sha;
  if (!repository) return null;

  const valid = (value: string) => /^[a-fA-F0-9]{7,64}$/.test(value);
  const style =
    "rounded-md border border-white/15 px-3 py-2 text-xs text-teal-300 hover:bg-white/[0.04]";

  return (
    <section className="mt-6 border-t border-white/10 pt-5">
      <h2 className="text-sm font-semibold">GitHub source</h2>
      <div className="mt-3 flex flex-wrap gap-3">
        <a
          href={repository}
          target="_blank"
          rel="noopener noreferrer"
          className={style}
        >
          Repository ↗
        </a>

        {commit && valid(commit) ? (
          <a
            href={`${repository}/commit/${commit}`}
            target="_blank"
            rel="noopener noreferrer"
            className={style}
          >
            Commit {commit.slice(0, 7)} ↗
          </a>
        ) : null}

        {commit && valid(commit) && valid(previous) && commit !== previous ? (
          <a
            href={`${repository}/compare/${previous}...${commit}`}
            target="_blank"
            rel="noopener noreferrer"
            className={style}
          >
            Changes since previous successful release ↗
          </a>
        ) : null}
      </div>

      {!commit ? (
        <p className="mt-2 text-xs text-slate-500">
          Add a commit SHA when creating a release to link its source.
        </p>
      ) : null}
    </section>
  );
}
