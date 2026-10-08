import Link from "next/link";
import GitHubSignIn from "@/components/auth/GitHubSignIn";

export default function HomePage() {
  return (
    <main
      id="main-content"
      className="nimbus-console min-h-screen bg-[#090c10] text-slate-200"
    >
      <nav className="mx-auto flex max-w-6xl items-center justify-between border-b border-white/[0.07] px-6 py-5">
        <p className="flex items-center gap-3 text-lg font-semibold tracking-tight">
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-teal-400 font-mono text-sm font-bold text-slate-950">
            N
          </span>
          Nimbus
        </p>
        <Link
          href="/login"
          className="rounded-md px-3 py-2 text-xs text-slate-400 hover:text-white"
        >
          Log in
        </Link>
      </nav>
      <section className="mx-auto max-w-6xl px-6 pb-16 pt-16 text-center sm:pt-24">
        <p className="font-mono text-[10px] tracking-[0.2em] text-teal-400">
          APPLICATION RELIABILITY / CONTROL PLANE
        </p>
        <h1 className="mx-auto mt-6 max-w-3xl text-4xl font-semibold leading-tight tracking-tight text-white sm:text-6xl">
          Know what failed.
          <br />
          Get back to healthy.
        </h1>
        <p className="mx-auto mt-6 max-w-xl text-sm leading-7 text-slate-400">
          HTTP monitoring, release verification, and incident recovery in one
          workspace. Follow a deployment from its first probe to its last
          healthy version.
        </p>
        <div className="mx-auto mt-8 max-w-sm"><GitHubSignIn /></div>
        <p className="mt-3 text-xs text-slate-500">Real workspace · No separate Nimbus signup</p>
        <Link href="/demo" className="mt-4 inline-block text-sm text-slate-400 hover:text-white">Explore the sample demo</Link>
        <div className="mx-auto mt-14 max-w-4xl overflow-hidden rounded-lg border border-white/10 bg-[#0d1117] text-left shadow-2xl">
          <div className="flex justify-between border-b border-white/[0.07] px-5 py-4">
            <span className="font-mono text-[10px] text-slate-400">
              NIMBUS / RELIABILITY OVERVIEW
            </span>
            <span className="font-mono text-[10px] text-teal-400">
              SAMPLE WORKSPACE
            </span>
          </div>
          <div className="grid gap-0 sm:grid-cols-3">
            {[
              [
                "MONITOR",
                "Observe HTTP health",
                "Check status, response latency, and failure thresholds.",
              ],
              [
                "VERIFY",
                "Inspect every release",
                "Track deployment progress and verify application health.",
              ],
              [
                "RECOVER",
                "Respond to incidents",
                "Acknowledge failures and restore a healthy release.",
              ],
            ].map(([label, title, description]) => (
              <div
                key={label}
                className="border-b border-white/[0.07] p-6 sm:border-r"
              >
                <p className="font-mono text-[10px] tracking-widest text-teal-400">
                  {label}
                </p>
                <h2 className="mt-4 text-sm font-medium">{title}</h2>
                <p className="mt-2 text-xs leading-6 text-slate-500">
                  {description}
                </p>
              </div>
            ))}
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="text-left text-slate-500">
                <tr>
                  <th className="px-6 py-3 font-normal">Service</th>
                  <th className="px-6 py-3 font-normal">Environment</th>
                  <th className="px-6 py-3 font-normal">Health</th>
                  <th className="px-6 py-3 font-normal">Release</th>
                </tr>
              </thead>
              <tbody>
                {[
                  ["Storefront", "v2.4.0"],
                  ["Payments API", "v1.8.2"],
                  ["Background worker", "v3.1.0"],
                ].map(([name, version]) => (
                  <tr key={name} className="border-t border-white/[0.05]">
                    <td className="px-6 py-4 text-slate-300">{name}</td>
                    <td className="px-6 py-4 text-slate-500">production</td>
                    <td className="px-6 py-4 text-teal-300">● healthy</td>
                    <td className="px-6 py-4 font-mono text-slate-400">
                      {version}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        <p className="mt-5 text-[11px] text-slate-600">
          Demo probes and recovery actions are simulated. Sign in to monitor
          your own applications.
        </p>
      </section>
    </main>
  );
}
