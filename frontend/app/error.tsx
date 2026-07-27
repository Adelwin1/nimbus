"use client";

export default function GlobalError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <main
      id="main-content"
      className="flex min-h-screen items-center justify-center bg-slate-950 px-6 text-slate-100"
    >
      <section className="w-full max-w-lg rounded-2xl border border-red-500/30 bg-slate-900 p-8 text-center shadow-2xl">
        <p className="text-sm font-semibold uppercase tracking-wider text-red-300">
          Something went wrong
        </p>

        <h1 className="mt-3 text-3xl font-bold">
          Nimbus could not load this page.
        </h1>

        <p className="mt-4 text-slate-300">
          The error was handled safely. Try loading the page again.
        </p>

        <button
          type="button"
          onClick={reset}
          className="mt-7 rounded-lg bg-sky-500 px-5 py-3 font-semibold text-slate-950 transition hover:bg-sky-400 focus:outline-none focus:ring-2 focus:ring-sky-300"
        >
          Try again
        </button>
      </section>
    </main>
  );
}
