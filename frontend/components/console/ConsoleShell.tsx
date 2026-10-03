"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

export function ConsoleShell({
  children,
  demo = false,
  actions,
}: {
  children: ReactNode;
  demo?: boolean;
  actions?: ReactNode;
}) {
  const pathname = usePathname();
  const links = demo
    ? [
        ["Overview", "/demo"],
        ["Real workspace", "/login"],
      ]
    : [
        ["Overview", "/dashboard"],
        ["Applications", "/apps"],
        ["Incidents", "/incidents"],
      ];
  return (
    <div className="nimbus-console min-h-screen bg-[#090c10] text-slate-200">
      <aside className="fixed inset-y-0 left-0 hidden w-56 flex-col border-r border-white/[0.07] bg-[#0d1117] lg:flex">
        <Link
          href="/"
          className="flex items-center gap-3 px-6 py-7 text-lg font-semibold tracking-tight"
        >
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-teal-400 font-mono text-sm font-bold text-slate-950">
            N
          </span>
          Nimbus
        </Link>
        <div className="mx-4 rounded-md border border-white/10 px-3 py-3">
          <p className="text-xs text-slate-500">WORKSPACE</p>
          <p className="mt-1 text-sm">
            {demo ? "Sandbox / demo" : "Personal / production"}
          </p>
        </div>
        <nav aria-label="Console navigation" className="mt-7 space-y-1 px-3">
          {links.map(([label, href], index) => (
            <Link
              key={href}
              href={href}
              aria-current={pathname === href ? "page" : undefined}
              className={`flex gap-3 rounded-md px-3 py-2.5 text-sm ${pathname === href ? "bg-white/[0.06] text-white" : "text-slate-400 hover:bg-white/[0.04]"}`}
            >
              <span className="font-mono text-slate-600">0{index + 1}</span>
              {label}
            </Link>
          ))}
        </nav>
        <div className="mt-auto border-t border-white/[0.07] p-5">
          <p className="font-mono text-xs text-slate-500">
            NIMBUS CONTROL PLANE
          </p>
          <p className="mt-2 text-xs text-slate-400">
            {demo
              ? "Simulated data. No external actions."
              : "HTTP monitoring · Release verification"}
          </p>
        </div>
      </aside>
      <div className="lg:ml-56">
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-white/[0.07] px-5 py-4 sm:px-8">
          <div className="flex items-center gap-3 text-sm">
            <Link href="/" className="font-semibold lg:hidden">
              Nimbus
            </Link>
            <span className="text-slate-500">Workspace</span>
            <span className="text-slate-700">/</span>
            <span>{demo ? "Interactive demo" : "Reliability"}</span>
          </div>
          <div className="flex items-center gap-3">{actions}</div>
        </header>
        <nav className="flex gap-5 border-b border-white/[0.07] px-5 py-3 text-xs text-slate-400 lg:hidden">
          {links.map(([label, href]) => (
            <Link key={href} href={href}>
              {label}
            </Link>
          ))}
        </nav>
        <main
          id="main-content"
          className="mx-auto max-w-[1500px] px-5 py-8 sm:px-8"
        >
          {children}
        </main>
      </div>
    </div>
  );
}
export function Status({ value }: { value: string }) {
  const color = ["healthy", "successful", "resolved"].includes(value)
    ? "text-teal-300 bg-teal-400"
    : ["down", "failed", "critical"].includes(value)
      ? "text-rose-300 bg-rose-400"
      : "text-amber-300 bg-amber-400";
  return (
    <span
      className={`inline-flex items-center gap-2 text-xs ${color.split(" ")[0]}`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${color.split(" ")[1]}`} />
      {value}
    </span>
  );
}
export const consoleButton =
  "rounded-md border border-white/15 bg-white/[0.03] px-3 py-2 text-xs font-medium text-slate-200 transition hover:bg-white/[0.08] disabled:cursor-not-allowed disabled:opacity-40";
