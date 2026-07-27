"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";

import { ApplicationForm } from "@/components/application/ApplicationForm";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { createApplication } from "@/lib/api";
import type { CreateApplicationInput } from "@/types/application";

export default function NewApplicationPage() {
  return (
    <ProtectedRoute>
      <NewApplicationContent />
    </ProtectedRoute>
  );
}

function NewApplicationContent() {
  const router = useRouter();

  async function handleCreate(input: CreateApplicationInput) {
    const response = await createApplication(input);

    router.push(`/apps/${response.application.id}`);
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

            <p className="text-sm text-slate-400">Register a new application</p>
          </div>

          <Link
            href="/apps"
            className="rounded-lg border border-slate-700 px-4 py-2 text-sm text-slate-200 transition hover:border-slate-500 hover:text-white"
          >
            Back to applications
          </Link>
        </div>
      </header>

      <div className="mx-auto max-w-5xl px-6 py-10">
        <div className="mb-8">
          <p className="text-sm font-medium uppercase tracking-wider text-sky-400">
            Application management
          </p>

          <h1 className="mt-2 text-3xl font-bold">Add an application</h1>

          <p className="mt-3 max-w-2xl text-slate-400">
            Register a deployed service and configure how Nimbus should monitor
            it.
          </p>
        </div>

        <ApplicationForm
          submitLabel="Create application"
          onSubmit={handleCreate}
        />
      </div>
    </main>
  );
}
