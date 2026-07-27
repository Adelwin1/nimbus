import type { ReactNode } from "react";

import { DashboardNav } from "@/components/navigation/DashboardNav";

export default function DashboardLayout({ children }: { children: ReactNode }) {
  return (
    <>
      <DashboardNav />
      {children}
    </>
  );
}
