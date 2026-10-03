"use client";

import { useState } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { startIncidentRollback } from "@/lib/api";
import type { Incident } from "@/types/incident";

type RollbackDialogProps = {
  incident: Incident;
  onClose: () => void;
  onStarted: (deploymentId: string) => void;
};

export function RollbackDialog({
  incident,
  onClose,
  onStarted,
}: RollbackDialogProps) {
  const toast = useToast();

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleRollback() {
    setSubmitting(true);
    setError("");

    try {
      const response = await startIncidentRollback(incident.id);

      onStarted(response.deployment.id);
    } catch (rollbackError) {
      const message =
        rollbackError instanceof Error && rollbackError.name === "APIError"
          ? rollbackError.message
          : "Rollback could not be started.";

      setError(message);
      toast.error("Rollback not started", message);
      setSubmitting(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[#090c10]/80 px-5 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
      aria-labelledby="rollback-title"
    >
      <div className="w-full max-w-lg rounded-md border border-slate-700 bg-[#0d1117] p-6 shadow-2xl">
        <p className="text-sm font-semibold uppercase tracking-wider text-amber-400">
          Recovery action
        </p>

        <h2 id="rollback-title" className="mt-2 text-2xl font-bold text-white">
          Start verified rollback?
        </h2>

        <p className="mt-4 text-sm leading-7 text-slate-400">
          Nimbus will find the latest successful deployment, trigger the
          encrypted rollback webhook, and require three consecutive healthy
          checks before resolving this incident.
        </p>

        <div className="mt-5 rounded-md border border-white/10 bg-[#090c10] p-4">
          <p className="text-sm font-medium text-slate-200">{incident.title}</p>

          <p className="mt-2 text-xs text-slate-500">
            The incident stays open if the rollback webhook or health
            verification fails.
          </p>
        </div>

        {error ? (
          <div
            role="alert"
            aria-live="polite"
            className="mt-5 rounded-md border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-300"
          >
            {error}
          </div>
        ) : null}

        <div className="mt-7 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            aria-busy={submitting}
            className="rounded-md border border-slate-700 px-5 py-3 font-medium text-slate-200 transition hover:bg-slate-800 disabled:opacity-60"
          >
            Cancel
          </button>

          <button
            type="button"
            onClick={handleRollback}
            disabled={submitting}
            aria-busy={submitting}
            className="rounded-md bg-amber-400 px-5 py-3 font-semibold text-slate-950 transition hover:bg-amber-300 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? "Starting rollback..." : "Start rollback"}
          </button>
        </div>
      </div>
    </div>
  );
}
