import Link from "next/link";

export function DashboardNav() {
  return (
    <header className="border-b border-slate-800 bg-slate-950/95 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-5 py-4">
        <Link href="/dashboard" className="text-lg font-bold text-white">
          Nimbus
        </Link>

        <nav
          aria-label="Dashboard navigation"
          className="flex items-center gap-2"
        >
          <Link
            href="/dashboard"
            className="rounded-lg px-3 py-2 text-sm font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
          >
            Applications
          </Link>

          <Link
            href="/incidents"
            className="rounded-lg px-3 py-2 text-sm font-medium text-slate-300 transition hover:bg-slate-800 hover:text-white"
          >
            Incidents
          </Link>
        </nav>
      </div>
    </header>
  );
}
