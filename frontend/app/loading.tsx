export default function Loading() {
  return (
    <main
      id="main-content"
      aria-busy="true"
      aria-label="Loading Nimbus"
      className="flex min-h-screen items-center justify-center bg-slate-950 px-6 text-slate-100"
    >
      <div className="text-center">
        <div
          aria-hidden="true"
          className="mx-auto h-10 w-10 animate-spin rounded-full border-4 border-slate-700 border-t-sky-400"
        />

        <p className="mt-4 text-sm text-slate-300">Loading Nimbus…</p>
      </div>
    </main>
  );
}
