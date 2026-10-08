"use client";
import { useState } from "react";
import { startGitHubSignIn } from "@/lib/github-signin";

export default function GitHubSignIn() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function start() {
    setBusy(true); setError("");
    try { window.location.assign(await startGitHubSignIn()); }
    catch { setBusy(false); setError("Could not start GitHub sign-in. Enable browser storage and try again."); }
  }
  return <div className="mt-6">
    <button type="button" onClick={start} disabled={busy}
      className="inline-flex w-full items-center justify-center rounded-md bg-teal-400 px-6 py-3 font-semibold text-slate-950 hover:bg-teal-300 disabled:opacity-60">
      {busy ? "Opening GitHub…" : "Continue with GitHub"}
    </button>
    {error && <p role="alert" className="mt-2 text-sm text-rose-300">{error}</p>}
  </div>;
}
