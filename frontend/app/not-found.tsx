import Link from "next/link";

export default function NotFound() {
  return (
    <main
      id="main-content"
      className="flex min-h-screen items-center justify-center bg-slate-950 px-6 text-slate-100"
    >
      <section className="w-full max-w-lg rounded-2xl border border-slate-700 bg-slate-900 p-8 text-center shadow-2xl">
        <p className="text-sm font-semibold uppercase tracking-wider text-sky-300">
          404
        </p>

        <h1 className="mt-3 text-3xl font-bold">Page not found</h1>

        <p className="mt-4 text-slate-300">
          The page may have moved, been deleted, or never existed.
        </p>

        <Link
          href="/dashboard"
          className="mt-7 inline-flex rounded-lg bg-sky-500 px-5 py-3 font-semibold text-slate-950 transition hover:bg-sky-400 focus:outline-none focus:ring-2 focus:ring-sky-300"
        >
          Return to dashboard
        </Link>
      </section>
    </main>
  );
}
