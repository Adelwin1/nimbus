"use client";

import { ConsoleShell } from "@/components/console/ConsoleShell";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { APIError, createDeployment } from "@/lib/api";

export default function NewDeploymentPage() {
  return (
    <ProtectedRoute>
      <NewDeploymentForm />
    </ProtectedRoute>
  );
}

function NewDeploymentForm() {
  const params = useParams<{ id: string }>();
  const router = useRouter();

  const applicationId = params.id;

  const [version, setVersion] = useState("");
  const [commitSHA, setCommitSHA] = useState("");
  const [releaseNotes, setReleaseNotes] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");

    const normalizedVersion = version.trim();
    const normalizedCommitSHA = commitSHA.trim();
    const normalizedReleaseNotes = releaseNotes.trim();

    if (!normalizedVersion) {
      setError("Version is required.");
      return;
    }

    if (normalizedVersion.length > 100) {
      setError("Version must contain 100 characters or fewer.");
      return;
    }

    if (
      normalizedCommitSHA &&
      !/^[a-fA-F0-9]{7,64}$/.test(normalizedCommitSHA)
    ) {
      setError("Commit SHA must contain 7 to 64 hexadecimal characters.");
      return;
    }

    setSubmitting(true);

    try {
      const response = await createDeployment(applicationId, {
        version: normalizedVersion,
        commit_sha: normalizedCommitSHA || null,
        release_notes: normalizedReleaseNotes,
      });

      router.push(`/deployments/${response.deployment.id}`);
    } catch (submitError) {
      if (submitError instanceof APIError) {
        setError(submitError.message);
      } else {
        setError("Deployment could not be created.");
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <ConsoleShell>
      <div className="mx-auto max-w-3xl">
        <Link
          href={`/apps/${applicationId}`}
          className="text-sm font-medium text-teal-300 hover:text-teal-200"
        >
          ← Back to application
        </Link>

        <div className="mt-6">
          <p className="text-sm font-semibold uppercase tracking-wider text-teal-300">
            Release
          </p>

          <h1 className="mt-2 text-2xl font-semibold tracking-tight text-white">
            Create deployment
          </h1>

          <p className="mt-3 text-slate-400">
            Nimbus will call the encrypted deployment webhook and judge the
            release using repeated real health checks.
          </p>
        </div>

        <form
          noValidate
          onSubmit={handleSubmit}
          className="mt-8 space-y-6 rounded-md border border-white/10 bg-[#0d1117] p-6"
        >
          {error ? (
            <div
              role="alert"
              aria-live="polite"
              className="rounded-md border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-300"
            >
              {error}
            </div>
          ) : null}

          <label className="block">
            <span className="text-sm font-medium text-slate-200">Version</span>

            <input
              value={version}
              onChange={(event) => setVersion(event.target.value)}
              placeholder="v1.4.0"
              maxLength={100}
              required
              className="mt-2 w-full rounded-md border border-slate-700 bg-[#090c10] px-4 py-3 text-white outline-none transition placeholder:text-slate-600 focus:border-teal-400"
            />
          </label>

          <div className="block">
            <label
              htmlFor="deployment-commit-sha"
              className="text-sm font-medium text-slate-200"
            >
              Commit SHA
            </label>

            <input
              id="deployment-commit-sha"
              value={commitSHA}
              onChange={(event) => setCommitSHA(event.target.value)}
              aria-describedby="deployment-commit-sha-help"
              placeholder="a3c9f24"
              maxLength={100}
              className="mt-2 w-full rounded-md border border-slate-700 bg-[#090c10] px-4 py-3 font-mono text-white outline-none transition placeholder:text-slate-600 focus:border-teal-400"
            />

            <span
              id="deployment-commit-sha-help"
              className="mt-2 block text-xs text-slate-500"
            >
              Optional. Hexadecimal characters only.
            </span>
          </div>

          <label className="block">
            <span className="text-sm font-medium text-slate-200">
              Release notes
            </span>

            <textarea
              value={releaseNotes}
              onChange={(event) => setReleaseNotes(event.target.value)}
              placeholder="Describe the changes in this release..."
              rows={8}
              maxLength={10000}
              className="mt-2 w-full resize-y rounded-md border border-slate-700 bg-[#090c10] px-4 py-3 text-white outline-none transition placeholder:text-slate-600 focus:border-teal-400"
            />
          </label>

          <div className="flex flex-col-reverse gap-3 border-t border-white/10 pt-6 sm:flex-row sm:justify-end">
            <Link
              href={`/apps/${applicationId}`}
              className="rounded-md border border-slate-700 px-5 py-3 text-center font-medium text-slate-200 transition hover:bg-slate-800"
            >
              Cancel
            </Link>

            <button
              type="submit"
              disabled={submitting}
              aria-busy={submitting}
              className="rounded-md bg-teal-400 px-5 py-3 font-semibold text-slate-950 transition hover:bg-teal-300 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submitting ? "Starting deployment..." : "Deploy release"}
            </button>
          </div>
        </form>
      </div>
    </ConsoleShell>
  );
}
