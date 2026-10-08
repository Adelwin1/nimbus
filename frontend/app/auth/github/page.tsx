"use client";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/contexts/AuthContext";
import { saveTokens } from "@/lib/token-storage";
import { GITHUB_VERIFIER_KEY } from "@/lib/github-signin";
import type { AuthResponse } from "@/types/auth";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api/v1";
export default function GitHubReturnPage() {
 const router = useRouter();
 const { reloadUser } = useAuth();
 const started = useRef(false);
 const [error, setError] = useState("");
 useEffect(() => {
  if (started.current) return;
  started.current = true;
  async function complete() {
   try {
    const code = new URL(window.location.href).searchParams.get("code");
    window.history.replaceState(null, "", "/auth/github");
    const verifier = sessionStorage.getItem(GITHUB_VERIFIER_KEY);
    if (!code || !verifier) throw new Error("missing exchange");
    const response = await fetch(`${API_URL}/github/login/exchange`, {
     method: "POST", headers: {"Content-Type": "application/json"},
     body: JSON.stringify({code, verifier}), cache: "no-store",
    });
    if (!response.ok) throw new Error("exchange rejected");
    const session: AuthResponse = await response.json();
    if (!session.access_token || !session.refresh_token) throw new Error("invalid response");
    saveTokens(session.access_token, session.refresh_token);
    sessionStorage.removeItem(GITHUB_VERIFIER_KEY);
    await reloadUser();
    router.replace("/integrations");
   } catch {
    sessionStorage.removeItem(GITHUB_VERIFIER_KEY);
    setError("GitHub sign-in could not be completed. Start again from Nimbus in this browser.");
   }
  }
  void complete();
 }, [reloadUser, router]);
 return <main id="main-content" className="flex min-h-screen items-center justify-center bg-[#090c10] px-6 text-slate-200">
  <section className="max-w-md rounded-md border border-white/10 bg-[#0d1117] p-8">
   <h1 className="text-xl font-semibold">{error ? "Sign-in needs another try" : "Opening your workspace"}</h1>
   {error ? <><p role="alert" className="mt-3 text-sm text-rose-300">{error}</p><Link href="/login" className="mt-5 inline-block text-teal-300">Return to sign-in</Link></> : <p role="status" className="mt-3 text-sm text-slate-400">Verifying your GitHub identity…</p>}
  </section>
 </main>;
}
