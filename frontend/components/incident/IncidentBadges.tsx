import type { IncidentSeverity, IncidentStatus } from "@/types/incident";

const statusStyles: Record<IncidentStatus, string> = {
  open: "border-red-800 bg-red-950 text-red-300",
  acknowledged: "border-amber-800 bg-amber-950 text-amber-300",
  resolved: "border-emerald-800 bg-emerald-950 text-emerald-300",
};

const severityStyles: Record<IncidentSeverity, string> = {
  warning: "border-amber-800 bg-amber-950 text-amber-300",
  critical: "border-red-800 bg-red-950 text-red-300",
};

export function IncidentStatusBadge({ status }: { status: IncidentStatus }) {
  return (
    <span
      className={`inline-flex rounded-full border px-3 py-1 text-xs font-semibold capitalize ${statusStyles[status]}`}
    >
      {status}
    </span>
  );
}

export function IncidentSeverityBadge({
  severity,
}: {
  severity: IncidentSeverity;
}) {
  return (
    <span
      className={`inline-flex rounded-full border px-3 py-1 text-xs font-semibold capitalize ${severityStyles[severity]}`}
    >
      {severity}
    </span>
  );
}
