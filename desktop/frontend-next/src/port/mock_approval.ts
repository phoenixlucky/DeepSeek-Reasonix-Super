import type { WireEvent } from "./wire";

// Which owner answers a pending approval. The kernel decides it from the
// approval itself — a recovery gate, the plan tool, otherwise the tool gate —
// and the fixture reads the same fields the card does rather than inventing a
// third rule.
export function approvalKind(ev: WireEvent): "plan_approval" | "recovery_approval" | "tool_approval" {
  const a = ev.approval;
  if (a?.kind === "plan" || a?.tool === "exit_plan_mode") return "plan_approval";
  if (a?.kind === "recovery") return "recovery_approval";
  return "tool_approval";
}
