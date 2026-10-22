import { useEffect, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { TurnNarrationSettings } from "../port/boundary";
import type { AgentPort } from "../port/port";
import { Group } from "./Group";

type Level = TurnNarrationSettings["level"];

export function TurnNarrationSetting({ port }: { port: AgentPort }) {
  const [level, setLevel] = useState<Level | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let alive = true;
    port.turnNarrationSettings().then((s) => { if (alive) setLevel(s.level); }).catch((e) => { if (alive) setError(reason(e)); });
    return () => { alive = false; };
  }, [port]);
  const choose = (next: Level) => {
    setError("");
    port.saveTurnNarrationSettings(next).then((s) => setLevel(s.level)).catch((e) => setError(reason(e)));
  };
  return (
    <Group id="turn-narration" title={t("回合间叙述")} hint={t("模型在两批工具调用之间说多少。简短是现在的样子；详细会让它每批用三四行说清这一批拿到了什么、下一步打算做什么，方便你跟着它的思路，每轮会多几十个输出词。从下一轮开始生效。")}>
      <div className="seg" data-text role="group" aria-label={t("回合间叙述")}>
        <button data-action="chrome.turn-narration" data-value="brief" aria-pressed={level === "brief"} disabled={level === null} onClick={() => choose("brief")}>{t("简短")}</button>
        <button data-action="chrome.turn-narration" data-value="detailed" aria-pressed={level === "detailed"} disabled={level === null} onClick={() => choose("detailed")}>{t("详细")}</button>
      </div>
      {error && <div className="why">{error}</div>}
    </Group>
  );
}
