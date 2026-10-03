import type { DeploymentEvent } from "@/types/deployment";

type DeploymentTimelineProps = {
  events: DeploymentEvent[];
};

export function DeploymentTimeline({ events }: DeploymentTimelineProps) {
  if (events.length === 0) {
    return (
      <div className="rounded-md border border-white/10 bg-[#0d1117] px-6 py-12 text-center text-sm text-slate-400">
        No deployment events have been recorded.
      </div>
    );
  }

  return (
    <div className="rounded-md border border-white/10 bg-[#0d1117] p-6">
      <h2 className="text-xl font-semibold text-white">Deployment timeline</h2>

      <p className="mt-1 text-sm text-slate-400">
        Every step is stored permanently by Nimbus.
      </p>

      <div className="mt-8 space-y-0">
        {events.map((event, index) => (
          <div key={event.id} className="relative flex gap-4 pb-8 last:pb-0">
            {index < events.length - 1 ? (
              <div className="absolute left-[7px] top-5 h-full w-px bg-slate-700" />
            ) : null}

            <div className="relative mt-1.5 h-4 w-4 shrink-0 rounded-full border-4 border-slate-900 bg-sky-400" />

            <div className="min-w-0 flex-1">
              <div className="flex flex-col justify-between gap-2 sm:flex-row sm:items-start">
                <div>
                  <p className="font-medium text-slate-100">
                    {formatEventType(event.event_type)}
                  </p>

                  <p className="mt-1 text-sm text-slate-400">{event.message}</p>
                </div>

                <time className="shrink-0 text-xs text-slate-500">
                  {formatDate(event.created_at)}
                </time>
              </div>

              {hasMetadata(event.metadata) ? (
                <details className="mt-3">
                  <summary className="cursor-pointer text-xs font-medium text-teal-300">
                    Event details
                  </summary>

                  <pre className="mt-3 overflow-x-auto rounded-md border border-white/10 bg-[#090c10] p-4 text-xs text-slate-400">
                    {JSON.stringify(event.metadata, null, 2)}
                  </pre>
                </details>
              ) : null}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function hasMetadata(metadata: Record<string, unknown>): boolean {
  return Object.keys(metadata ?? {}).length > 0;
}

function formatEventType(value: string): string {
  return value
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}
