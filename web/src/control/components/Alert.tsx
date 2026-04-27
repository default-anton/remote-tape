import { type ReactNode } from "react";

export function Alert({ children }: { children: ReactNode }) {
  return <div className="panel error">{children}</div>;
}
