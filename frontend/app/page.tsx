import Link from "next/link";

export default function HomePage() {
  return (
    <main id="main-content" className="min-h-screen bg-slate-950 text-white">
      <nav className="mx-auto flex max-w-7xl items-center justify-between px-6 py-6">
        <p className="text-xl font-semibold">Nimbus</p>

        <Link href="/login" className="rounded-lg px-4 py-2 text-sm font-medium text-slate-300 hover:text-white">
          Log in
        </Link>
      </nav>

      <section className="mx-auto flex max-w-5xl flex-col items-center px-6 py-28 text-center">
        <p className="rounded-full border border-sky-900 bg-sky-950/50 px-4 py-2 text-sm text-sky-300">
          Personal cloud reliability
        </p>

        <h1 className="mt-8 max-w-4xl text-5xl font-bold tracking-tight sm:text-7xl">
          Monitor deployments and recover from failures.
        </h1>

        <p className="mt-7 max-w-2xl text-lg leading-8 text-slate-400">
          Nimbus monitors deployed applications, verifies releases, detects
          outages, and helps restore the last healthy version.
        </p>

        <Link
          href="/demo"
          className="mt-10 rounded-xl bg-sky-500 px-10 py-5 text-xl font-semibold text-slate-950 shadow-lg shadow-sky-950 transition hover:bg-sky-400"
        >
          Get started
        </Link>
        <p className="mt-4 text-sm text-slate-400">No login required · Explore the interactive demo</p>
      </section>
    </main>
  );
}
