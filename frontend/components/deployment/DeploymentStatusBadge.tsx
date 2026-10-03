import type { DeploymentStatus } from "@/types/deployment";

type DeploymentStatusBadgeProps = {
  status: DeploymentStatus;
};

const statusStyles: Record<DeploymentStatus, string> = {
  pending: "border-slate-700 bg-slate-800 text-slate-300",
  triggering: "border-violet-800 bg-violet-950 text-violet-300",
  verifying: "border-amber-800 bg-amber-950 text-amber-300",
  successful: "border-emerald-800 bg-emerald-950 text-emerald-300",
  failed: "border-red-800 bg-red-950 text-red-300",
  cancelled: "border-slate-700 bg-[#0d1117] text-slate-400",
};

export function DeploymentStatusBadge({ status }: DeploymentStatusBadgeProps) {
  return (
    <span
      className={`inline-flex rounded-full border px-3 py-1 text-xs font-semibold capitalize ${statusStyles[status]}`}
    >
      {status}
    </span>
  );
}
