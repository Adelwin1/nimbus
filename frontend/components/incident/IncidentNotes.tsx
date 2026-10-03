"use client";

import { useRef, useState } from "react";
import { apiRequest } from "@/lib/api";
import { consoleButton } from "@/components/console/ConsoleShell";

export function IncidentNotes({ incidentId, onSaved }: {
  incidentId: string;
  onSaved: () => Promise<void>;
}) {
  const [message, setMessage] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const pending = useRef(false);

  async function save(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current || !message.trim()) return;

    pending.current = true;
    setSaving(true);
    setError("");
    setSuccess("");

    try {
      await apiRequest(`/incidents/${incidentId}/notes`, {
        method: "POST",
        authenticated: true,
        body: JSON.stringify({ message: message.trim() }),
      });
      setMessage("");
      setSuccess("Note saved to the incident timeline.");
      await onSaved();
    } catch (err) {
      setError(
        err instanceof Error && err.name === "APIError"
          ? err.message
          : "Could not save the note. Try again."
      );
    } finally {
      pending.current = false;
      setSaving(false);
    }
  }

  return (
    <section className="mt-6 rounded-md border border-white/10 bg-[#0d1117] p-5">
      <h2 className="text-base font-semibold">Investigation notes</h2>
      <p className="mt-1 text-sm text-slate-400">
        Record findings, actions taken, and recovery details.
      </p>

      <form onSubmit={save} className="mt-4">
        <label htmlFor="incident-note" className="text-xs text-slate-400">
          Add a note
        </label>
        <textarea
          id="incident-note"
          value={message}
          onChange={(event) => {
            setMessage(event.target.value);
            setSuccess("");
          }}
          disabled={saving}
          maxLength={5000}
          required
          rows={4}
          placeholder="What did you investigate? What changed?"
          className="mt-2 block w-full rounded-md border border-white/15 bg-[#090c10] p-3 text-sm text-slate-200 outline-none focus:border-teal-400 disabled:opacity-50"
        />

        <div className="mt-3 flex items-center justify-between gap-3">
          <span className="font-mono text-xs text-slate-500">
            {message.length.toLocaleString()} / 5,000
          </span>
          <button
            type="submit"
            className={consoleButton}
            disabled={saving || !message.trim()}
          >
            {saving ? "Saving…" : "Save note"}
          </button>
        </div>

        {error ? (
          <p role="alert" className="mt-3 text-sm text-rose-300">{error}</p>
        ) : null}
        {success ? (
          <p role="status" className="mt-3 text-sm text-teal-300">{success}</p>
        ) : null}
      </form>
    </section>
  );
}
