import { useState } from "react";
import { t } from "../../i18n";
import { reason } from "../../i18n/kernel";
import { count } from "../../i18n/format";
import type { ToolOutputPage } from "../../port/session";
import type { StateTool } from "../../state/outputcap";

export type ReadOutput = (callId: string, offset?: number, limit?: number) => Promise<ToolOutputPage>;

const PAGE = 64 * 1024;
// Four megabytes is more than a person reads in a card; past it the rest stays in the kernel.
const MAX_PAGES = 64;

type Fetch = { phase: "idle" } | { phase: "loading" } | { phase: "failed"; why: string } | { phase: "done"; text: string };

// A call whose middle the page let go of can bring it back whole: the pages are
// read from the kernel in turn and kept only by the card that asked.
export function useFullOutput(tool: StateTool, read?: ReadOutput) {
  const [got, setGot] = useState<Fetch>({ phase: "idle" });
  const hidden = tool.hidden ?? 0;
  const load = async () => {
    if (!read || !tool.id || got.phase === "loading") return;
    setGot({ phase: "loading" });
    try {
      let text = "";
      let offset = 0;
      for (let i = 0; i < MAX_PAGES; i++) {
        const page = await read(tool.id, offset, PAGE);
        text += page.text;
        offset = page.offset + new TextEncoder().encode(page.text).length;
        if (!page.truncated) break;
      }
      setGot({ phase: "done", text });
    } catch (e) {
      setGot({ phase: "failed", why: reason(e) || t("取回失败，请重试") });
    }
  };
  return {
    text: got.phase === "done" ? got.text : tool.output,
    note: hidden > 0 && got.phase !== "done" ? <HiddenNote hidden={hidden} got={got} canLoad={!!read && !!tool.id} onLoad={() => void load()} /> : null,
  };
}

function HiddenNote({ hidden, got, canLoad, onLoad }: { hidden: number; got: Fetch; canLoad: boolean; onLoad: () => void }) {
  return (
    <div className="bound">
      {t("中间 {n} 个字符没有放在页面里，只显示了开头和结尾", { n: count(hidden) })}
      {canLoad && got.phase !== "loading" && (
        <button type="button" className="out-more" data-action="tool.show-full" onClick={onLoad}>
          {got.phase === "failed" ? t("重试") : t("显示完整输出")}
        </button>
      )}
      {got.phase === "loading" && <span role="status"> {t("正在取回…")}</span>}
      {got.phase === "failed" && <span role="alert" className="bad"> {got.why}</span>}
    </div>
  );
}
