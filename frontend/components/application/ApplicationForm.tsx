"use client";

import { FormEvent, useState } from "react";

import { APIError } from "@/lib/api";
import type {
  ApplicationEnvironment,
  CreateApplicationInput,
} from "@/types/application";

type ApplicationFormProps = {
  initialValues?: Partial<CreateApplicationInput>;
  submitLabel: string;
  updateMode?: boolean;
  onSubmit: (input: CreateApplicationInput) => Promise<void>;
};

export function ApplicationForm({
  initialValues,
  submitLabel,
  updateMode = false,
  onSubmit,
}: ApplicationFormProps) {
  const [name, setName] = useState(initialValues?.name ?? "");
  const [description, setDescription] = useState(
    initialValues?.description ?? "",
  );
  const [applicationURL, setApplicationURL] = useState(
    initialValues?.application_url ?? "",
  );
  const [healthURL, setHealthURL] = useState(initialValues?.health_url ?? "");
  const [repositoryURL, setRepositoryURL] = useState(
    initialValues?.repository_url ?? "",
  );
  const [environment, setEnvironment] = useState<ApplicationEnvironment>(
    initialValues?.environment ?? "production",
  );
  const [currentVersion, setCurrentVersion] = useState(
    initialValues?.current_version ?? "",
  );

  const [monitoringInterval, setMonitoringInterval] = useState(
    initialValues?.monitoring_interval_seconds ?? 60,
  );
  const [failureThreshold, setFailureThreshold] = useState(
    initialValues?.failure_threshold ?? 3,
  );
  const [latencyThreshold, setLatencyThreshold] = useState(
    initialValues?.latency_threshold_ms ?? 2000,
  );

  const [deploymentWebhookURL, setDeploymentWebhookURL] = useState("");
  const [rollbackWebhookURL, setRollbackWebhookURL] = useState("");
  const [webhookToken, setWebhookToken] = useState("");

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");

    const input: CreateApplicationInput = {
      name: name.trim(),
      description: description.trim(),
      application_url: applicationURL.trim(),
      health_url: healthURL.trim(),
      repository_url: emptyToNull(repositoryURL),
      environment,
      current_version: emptyToNull(currentVersion),
      monitoring_interval_seconds: monitoringInterval,
      failure_threshold: failureThreshold,
      latency_threshold_ms: latencyThreshold,
      deployment_webhook_url: emptyToNull(deploymentWebhookURL),
      rollback_webhook_url: emptyToNull(rollbackWebhookURL),
      webhook_token: emptyToNull(webhookToken),
    };

    const validationError = validateApplicationInput(input);

    if (validationError) {
      setError(validationError);
      return;
    }

    setSubmitting(true);

    try {
      await onSubmit(input);
    } catch (submissionError) {
      if (submissionError instanceof APIError) {
        setError(submissionError.message);
      } else {
        setError("The application could not be saved.");
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form noValidate onSubmit={handleSubmit} className="space-y-8">
      {error ? (
        <div
          role="alert"
          aria-live="polite"
          className="rounded-xl border border-red-900 bg-red-950/50 px-4 py-3 text-sm text-red-300"
        >
          {error}
        </div>
      ) : null}

      <section className="rounded-2xl border border-slate-800 bg-slate-900 p-6">
        <div className="mb-6">
          <h2 className="text-lg font-semibold text-white">
            Application details
          </h2>
          <p className="mt-1 text-sm text-slate-400">
            Add the basic information Nimbus uses to identify your application.
          </p>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field label="Application name" required>
            <input
              type="text"
              value={name}
              onChange={(event) => setName(event.target.value)}
              minLength={2}
              maxLength={120}
              required
              placeholder="Nimbus API"
              className={inputClassName}
            />
          </Field>

          <Field label="Environment" required>
            <select
              value={environment}
              onChange={(event) =>
                setEnvironment(event.target.value as ApplicationEnvironment)
              }
              className={inputClassName}
            >
              <option value="development">Development</option>
              <option value="staging">Staging</option>
              <option value="production">Production</option>
            </select>
          </Field>

          <div className="md:col-span-2">
            <Field label="Description">
              <textarea
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                maxLength={5000}
                rows={4}
                placeholder="Describe what this application does."
                className={inputClassName}
              />
            </Field>
          </div>

          <Field label="Current version">
            <input
              type="text"
              value={currentVersion}
              onChange={(event) => setCurrentVersion(event.target.value)}
              maxLength={100}
              placeholder="v1.0.0"
              className={inputClassName}
            />
          </Field>

          <Field label="Repository URL">
            <input
              type="url"
              value={repositoryURL}
              onChange={(event) => setRepositoryURL(event.target.value)}
              placeholder="https://github.com/username/project"
              className={inputClassName}
            />
          </Field>
        </div>
      </section>

      <section className="rounded-2xl border border-slate-800 bg-slate-900 p-6">
        <div className="mb-6">
          <h2 className="text-lg font-semibold text-white">Monitoring URLs</h2>
          <p className="mt-1 text-sm text-slate-400">
            Nimbus will use the health endpoint for real monitoring checks.
          </p>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field label="Application URL" required>
            <input
              type="url"
              value={applicationURL}
              onChange={(event) => setApplicationURL(event.target.value)}
              required
              placeholder="https://example.com"
              className={inputClassName}
            />
          </Field>

          <Field label="Health URL" required>
            <input
              type="url"
              value={healthURL}
              onChange={(event) => setHealthURL(event.target.value)}
              required
              placeholder="https://example.com/health"
              className={inputClassName}
            />
          </Field>
        </div>
      </section>

      <section className="rounded-2xl border border-slate-800 bg-slate-900 p-6">
        <div className="mb-6">
          <h2 className="text-lg font-semibold text-white">
            Monitoring thresholds
          </h2>
          <p className="mt-1 text-sm text-slate-400">
            Configure how often Nimbus checks the service and when failures
            should be reported.
          </p>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <Field label="Check interval (seconds)" required>
            <input
              type="number"
              value={monitoringInterval}
              onChange={(event) =>
                setMonitoringInterval(Number(event.target.value))
              }
              min={30}
              max={86400}
              required
              className={inputClassName}
            />
          </Field>

          <Field label="Failure threshold" required>
            <input
              type="number"
              value={failureThreshold}
              onChange={(event) =>
                setFailureThreshold(Number(event.target.value))
              }
              min={1}
              max={20}
              required
              className={inputClassName}
            />
          </Field>

          <Field label="Latency threshold (ms)" required>
            <input
              type="number"
              value={latencyThreshold}
              onChange={(event) =>
                setLatencyThreshold(Number(event.target.value))
              }
              min={100}
              max={60000}
              required
              className={inputClassName}
            />
          </Field>
        </div>
      </section>

      <section className="rounded-2xl border border-slate-800 bg-slate-900 p-6">
        <div className="mb-6">
          <h2 className="text-lg font-semibold text-white">
            Webhook configuration
          </h2>
          <p className="mt-1 text-sm text-slate-400">
            These values are encrypted before being stored.
            {updateMode
              ? " Leave a field blank to keep its current value."
              : " All webhook fields are optional."}
          </p>
        </div>

        <div className="grid gap-5 md:grid-cols-2">
          <Field label="Deployment webhook URL">
            <input
              type="url"
              value={deploymentWebhookURL}
              onChange={(event) => setDeploymentWebhookURL(event.target.value)}
              placeholder="https://example.com/hooks/deploy"
              className={inputClassName}
            />
          </Field>

          <Field label="Rollback webhook URL">
            <input
              type="url"
              value={rollbackWebhookURL}
              onChange={(event) => setRollbackWebhookURL(event.target.value)}
              placeholder="https://example.com/hooks/rollback"
              className={inputClassName}
            />
          </Field>

          <div className="md:col-span-2">
            <Field label="Webhook token">
              <input
                type="password"
                value={webhookToken}
                onChange={(event) => setWebhookToken(event.target.value)}
                autoComplete="off"
                placeholder={
                  updateMode
                    ? "Leave blank to keep the current token"
                    : "Optional secret token"
                }
                className={inputClassName}
              />
            </Field>
          </div>
        </div>
      </section>

      <div className="flex justify-end">
        <button
          type="submit"
          disabled={submitting}
          aria-busy={submitting}
          className="rounded-xl bg-sky-500 px-6 py-3 font-medium text-slate-950 transition hover:bg-sky-400 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {submitting ? "Saving..." : submitLabel}
        </button>
      </div>
    </form>
  );
}

function Field({
  label,
  required = false,
  children,
}: {
  label: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm font-medium text-slate-200">
        {label}
        {required ? <span className="ml-1 text-sky-400">*</span> : null}
      </span>

      {children}
    </label>
  );
}

function validateApplicationInput(
  input: CreateApplicationInput,
): string | null {
  if (input.name.trim().length < 2) {
    return "Application name must contain at least 2 characters.";
  }

  if (!isHTTPURL(input.application_url)) {
    return "Enter a valid application URL.";
  }

  if (!isHTTPURL(input.health_url)) {
    return "Enter a valid health URL.";
  }

  if (input.repository_url && !isHTTPURL(input.repository_url)) {
    return "Enter a valid repository URL.";
  }

  if (
    input.deployment_webhook_url &&
    !isHTTPURL(input.deployment_webhook_url)
  ) {
    return "Enter a valid deployment webhook URL.";
  }

  if (input.rollback_webhook_url && !isHTTPURL(input.rollback_webhook_url)) {
    return "Enter a valid rollback webhook URL.";
  }

  if (
    input.monitoring_interval_seconds < 30 ||
    input.monitoring_interval_seconds > 86400
  ) {
    return "Check interval must be between 30 and 86400 seconds.";
  }

  if (input.failure_threshold < 1 || input.failure_threshold > 20) {
    return "Failure threshold must be between 1 and 20.";
  }

  if (input.latency_threshold_ms < 100 || input.latency_threshold_ms > 60000) {
    return "Latency threshold must be between 100 and 60000 milliseconds.";
  }

  return null;
}

function isHTTPURL(value: string): boolean {
  try {
    const parsed = new URL(value);

    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.hostname.length > 0
    );
  } catch {
    return false;
  }
}

function emptyToNull(value: string): string | null {
  const normalized = value.trim();
  return normalized === "" ? null : normalized;
}

const inputClassName =
  "w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-white outline-none transition placeholder:text-slate-600 focus:border-sky-500 focus:ring-2 focus:ring-sky-500/20";
