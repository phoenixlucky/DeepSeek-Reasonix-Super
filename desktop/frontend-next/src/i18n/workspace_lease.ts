import type { WorkspaceLease } from "../port/wire";
import { t } from "./index";

export function workspaceLeaseDetail(claim: WorkspaceLease, showHolder = true): string {
  return [showHolder && (claim.holder || claim.holderSessionId) ? t("会话 {holder}（{session}）持有写入范围：{paths}", {
    holder: claim.holder ?? "", session: claim.holderSessionId ?? "", paths: JSON.stringify(claim.paths ?? []),
  }) : "", claim.requestedPaths?.length ? t("所需写入范围：{paths}", { paths: JSON.stringify(claim.requestedPaths) }) : "",
  showHolder && claim.repository ? t("这些工作区在同一个 Git 仓库 {repo} 内，同一仓库共用一个写入域；把项目移出该仓库，或在用户配置里设 write_lease = \"optimistic\"，才能并行写入", { repo: claim.repository }) : ""].filter(Boolean).join(" · ");
}
