"use client";

import Link from "next/link";
import { useState } from "react";
import { Status } from "@/components/console/ConsoleShell";
import type { Deployment } from "@/types/deployment";

function elapsed(release: Deployment): string {
  if (!release.started_at || !release.completed_at) return "—";
  const seconds =
    (Date.parse(release.completed_at) - Date.parse(release.started_at)) / 1000;
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  return `${seconds.toFixed(1)} s`;
}

function date(value: string | null): string {
  return value ? new Date(value).toLocaleString() : "—";
}

export function DeploymentComparison({
  deployments,
}: {
  deployments: Deployment[];
}) {
  const [leftId, setLeftId] = useState("");
  const [rightId, setRightId] = useState("");

  if (deployments.length < 2) {
    return (
      <p className="border-t border-white/10 p-5 text-sm text-slate-500">
        Release comparisons become available after two deployments.
      </p>
    );
  }

  const left =
    deployments.find((item) => item.id === leftId) ?? deployments[0];
  const right =
    deployments.find((item) => item.id === rightId && item.id !== left.id) ??
    deployments.find((item) => item.id !== left.id)!;

  const rows: [string, string, string][] = [
    ["Previous version", left.previous_version ?? "—", right.previous_version ?? "—"],
    ["Release type", left.deployment_type, right.deployment_type],
    ["Commit", left.commit_sha ?? "—", right.commit_sha ?? "—"],
    ["Created", date(left.created_at), date(right.created_at)],
    ["Started", date(left.started_at), date(right.started_at)],
    ["Completed", date(left.completed_at), date(right.completed_at)],
    ["Duration", elapsed(left), elapsed(right)],
    ["Release notes", left.release_notes || "—", right.release_notes || "—"],
  ];

  return (
    <section className="border-t border-white/10 p-5">
      <h3 className="text-base font-semibold text-slate-200">Compare releases</h3>
      <p className="mt-1 text-xs text-slate-500">
        Saved release records. Open a release for its verification timeline.
      </p>

      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        {[left, right].map((release, index) => (
          <label key={index} className="text-xs text-slate-400">
            {index === 0 ? "Release A" : "Release B"}
            <select
              value={release.id}
              onChange={(event) =>
                index === 0
                  ? setLeftId(event.target.value)
                  : setRightId(event.target.value)
              }
              className="mt-2 block w-full rounded-md border border-white/15 bg-[#090c10] p-2 text-sm text-slate-200"
            >
              {deployments
                .filter((item) => index === 0 || item.id !== left.id)
                .map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.version} · {date(item.created_at)} · {item.id.slice(0, 8)}
                  </option>
                ))}
            </select>
          </label>
        ))}
      </div>

      <div className="mt-4 overflow-x-auto rounded-md border border-white/10">
        <table className="w-full min-w-[560px] text-left text-sm">
          <thead className="bg-white/[0.03] text-xs text-slate-400">
            <tr>
              <th className="p-3">Field</th>
              {[left, right].map((release) => (
                <th key={release.id} className="p-3">
                  <Link
                    className="text-teal-300 hover:underline"
                    href={`/deployments/${release.id}`}
                  >
                    {release.version} ↗
                  </Link>
                </th>
              ))}
            </tr>
          </thead>

          <tbody className="divide-y divide-white/10">
            <tr>
              <th className="p-3 font-normal text-slate-400">Status</th>
              <td className="p-3"><Status value={left.status} /></td>
              <td className="p-3"><Status value={right.status} /></td>
            </tr>
            {rows.map(([label, a, b]) => (
              <tr key={label}>
                <th className="p-3 align-top font-normal text-slate-400">
                  {label}
                </th>
                <td className="max-w-sm whitespace-pre-wrap break-words p-3 align-top text-slate-300">
                  {a}
                </td>
                <td
                  className={`max-w-sm whitespace-pre-wrap break-words p-3 align-top ${
                    a !== b
                      ? "bg-teal-400/[0.04] text-teal-200"
                      : "text-slate-300"
                  }`}
                >
                  {b}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="mt-2 text-xs text-slate-500">
        Highlighted cells differ. A dash means no recorded value.
      </p>
    </section>
  );
}
