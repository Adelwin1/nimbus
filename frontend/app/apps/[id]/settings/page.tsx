"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { ApplicationForm } from "@/components/application/ApplicationForm";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { useToast } from "@/components/ui/ToastProvider";
import {
  deleteApplication,
  getApplication,
  updateApplication,
} from "@/lib/api";
import type { Application, CreateApplicationInput } from "@/types/application";

export default function ApplicationSettingsPage() {
  return (
    <ProtectedRoute>
      <ApplicationSettingsContent />
    </ProtectedRoute>
  );
}

function ApplicationSettingsContent() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const applicationID = params.id;

  const [application, setApplication] = useState<Application | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;

    async function loadApplication() {
      try {
        const response = await getApplication(applicationID);

        if (!cancelled) {
          setApplication(response.application);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(
            loadError instanceof Error
              ? loadError.message
              : "Application settings could not be loaded.",
          );
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void loadApplication();

    return () => {
      cancelled = true;
    };
  }, [applicationID]);

  async function handleUpdate(input: CreateApplicationInput) {
    await updateApplication(applicationID, input);
    router.push(`/apps/${applicationID}`);
  }

  return (
    <main id="main-content" className="min-h-screen bg-slate-950 text-white">
      <header className="border-b border-slate-800 bg-slate-900">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-5">
          <div>
            <Link
              href="/dashboard"
              className="text-xl font-semibold text-white"
            >
              Nimbus
            </Link>

            <p className="text-sm text-slate-400">Application settings</p>
          </div>

          <Link
            href={`/apps/${applicationID}`}
            className="rounded-lg border border-slate-700 px-4 py-2 text-sm text-slate-200 transition hover:border-slate-500 hover:text-white"
          >
            Back to overview
          </Link>
        </div>
      </header>

      <div className="mx-auto max-w-5xl px-6 py-10">
        {loading ? (
          <LoadingState />
        ) : error || !application ? (
          <ErrorState message={error || "Application not found."} />
        ) : (
          <>
            <div className="mb-8">
              <p className="text-sm font-medium uppercase tracking-wider text-sky-400">
                Configuration
              </p>

              <h1 className="mt-2 text-3xl font-bold">{application.name}</h1>

              <p className="mt-3 text-slate-400">
                Update application details, monitoring thresholds, and encrypted
                webhook settings.
              </p>
            </div>

            <ApplicationForm
              updateMode
              submitLabel="Save changes"
              initialValues={{
                name: application.name,
                description: application.description,
                application_url: application.application_url,
                health_url: application.health_url,
                repository_url: application.repository_url,
                environment: application.environment,
                current_version: application.current_version,
                monitoring_interval_seconds:
                  application.monitoring_interval_seconds,
                failure_threshold: application.failure_threshold,
                latency_threshold_ms: application.latency_threshold_ms,
              }}
              onSubmit={handleUpdate}
            />

            <DeleteApplicationSection
              application={application}
              onDeleted={() => router.replace("/apps")}
            />
          </>
        )}
      </div>
    </main>
  );
}

function DeleteApplicationSection({
  application,
  onDeleted,
}: {
  application: Application;
  onDeleted: () => void;
}) {
  const toast = useToast();

  const [confirmation, setConfirmation] = useState("");
  const [showConfirmation, setShowConfirmation] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");

  const confirmed = confirmation === application.name;

  async function handleDelete() {
    if (!confirmed || deleting) {
      return;
    }

    setDeleting(true);
    setError("");

    try {
      await deleteApplication(application.id);

      toast.success(
        "Application deleted",
        `${application.name} was permanently removed.`,
      );

      onDeleted();
    } catch (deleteError) {
      const message =
        deleteError instanceof Error && deleteError.name === "APIError"
          ? deleteError.message
          : "The application could not be deleted.";

      setError(message);
      setShowDeleteDialog(false);

      toast.error("Application not deleted", message);
    } finally {
      setDeleting(false);
    }
  }

  function cancelDeletion() {
    if (deleting) {
      return;
    }

    setShowDeleteDialog(false);
  }

  return (
    <section className="mt-10 rounded-2xl border border-red-900/80 bg-red-950/20 p-6">
      <h2 className="text-lg font-semibold text-red-300">Delete application</h2>

      <p
        id="delete-application-description"
        className="mt-2 max-w-2xl text-sm leading-6 text-slate-400"
      >
        Deleting this application permanently removes it from your Nimbus
        account. This action cannot be undone.
      </p>

      {!showConfirmation ? (
        <button
          type="button"
          onClick={() => setShowConfirmation(true)}
          className="mt-5 rounded-xl border border-red-800 px-5 py-3 text-sm font-medium text-red-300 transition hover:bg-red-950 focus:outline-none focus:ring-2 focus:ring-red-500"
        >
          Delete application
        </button>
      ) : (
        <div className="mt-6 rounded-xl border border-red-900 bg-slate-950 p-5">
          <label
            htmlFor="delete-application-confirmation"
            className="text-sm text-slate-300"
          >
            Type <strong className="text-white">{application.name}</strong> to
            confirm deletion.
          </label>

          <input
            id="delete-application-confirmation"
            type="text"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            placeholder={application.name}
            aria-invalid={Boolean(error)}
            aria-describedby={
              error
                ? "delete-application-error"
                : "delete-application-description"
            }
            className="mt-4 w-full rounded-xl border border-slate-700 bg-slate-900 px-4 py-3 text-white outline-none placeholder:text-slate-600 focus:border-red-500 focus:ring-2 focus:ring-red-500/20"
          />

          {error ? (
            <p
              id="delete-application-error"
              role="alert"
              className="mt-3 text-sm text-red-300"
            >
              {error}
            </p>
          ) : null}

          <div className="mt-5 flex flex-wrap gap-3">
            <button
              type="button"
              onClick={() => setShowDeleteDialog(true)}
              disabled={!confirmed || deleting}
              className="rounded-xl bg-red-600 px-5 py-3 text-sm font-medium text-white transition hover:bg-red-500 focus:outline-none focus:ring-2 focus:ring-red-400 disabled:cursor-not-allowed disabled:opacity-40"
            >
              Permanently delete
            </button>

            <button
              type="button"
              onClick={() => {
                setShowConfirmation(false);
                setShowDeleteDialog(false);
                setConfirmation("");
                setError("");
              }}
              disabled={deleting}
              className="rounded-xl border border-slate-700 px-5 py-3 text-sm text-slate-300 transition hover:border-slate-500 hover:text-white focus:outline-none focus:ring-2 focus:ring-slate-400"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={showDeleteDialog}
        title="Permanently delete application?"
        description={`This will permanently delete ${application.name}, including its monitoring history, deployments, and incidents. This cannot be undone.`}
        confirmLabel="Delete permanently"
        variant="danger"
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={cancelDeletion}
      />
    </section>
  );
}

function LoadingState() {
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900 px-6 py-16 text-center text-slate-400">
      Loading settings...
    </div>
  );
}

function ErrorState({ message }: { message: string }) {
  return (
    <div className="rounded-2xl border border-red-900 bg-red-950/40 px-6 py-5 text-red-300">
      {message}
    </div>
  );
}
