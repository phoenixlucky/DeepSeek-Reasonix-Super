import { useCallback, useEffect, useState } from "react";
import { current, t } from "../i18n";
import type { FeedbackLinkedInstall } from "../port/feedback";
import type { AgentPort } from "../port/port";
import { reason } from "../i18n/kernel";
import { boardNameProblem, boardPrefill, markBoardOfferSeen } from "./boardname";
import { FeedbackBoardJoin } from "./FeedbackBoardJoin";
import { linkFailure } from "./feedbackfailure";

interface Props {
  port: AgentPort;
  // Linking names this install, which only exists once a report has been sent.
  hasReport: boolean;
  // The board changed under the page: the link step may have listed the person.
  onBoardChanged?: () => void;
}

const day = (iso: string): string => {
  const at = new Date(iso);
  return Number.isNaN(at.getTime()) ? "" : at.toLocaleDateString(current() === "zh" ? "zh-CN" : "en", { year: "numeric", month: "short", day: "numeric" });
};

// The account link is the person's own call: nothing here runs at sign-in, and
// every control is inert until an account is signed in.
export function FeedbackLink({ port, hasReport, onBoardChanged }: Props) {
  const [signedIn, setSignedIn] = useState<boolean | null>(null);
  const [links, setLinks] = useState<FeedbackLinkedInstall[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const [asking, setAsking] = useState(false);
  const [join, setJoin] = useState(true);
  const [name, setName] = useState("");

  const load = useCallback(async () => {
    try {
      const state = await port.account();
      setSignedIn(state.signedIn && !state.expired);
      if (!state.signedIn || state.expired) {
        setLinks(null);
        return;
      }
      setLinks(await port.feedbackLinks());
      setProblem(null);
    } catch (e) {
      setProblem(linkFailure(e).message);
    }
  }, [port]);

  useEffect(() => {
    void load();
  }, [load]);

  const run = async (act: () => Promise<void>) => {
    setBusy(true);
    setProblem(null);
    try {
      await act();
      setLinks(await port.feedbackLinks());
    } catch (e) {
      setProblem(linkFailure(e).message);
    } finally {
      setBusy(false);
    }
  };

  const ask = () => {
    setAsking(true);
    setJoin(true);
    setProblem(null);
    port.feedbackEnv(document.documentElement.lang).then((e) => setName((prev) => prev || boardPrefill(e.displayName)), () => {});
  };

  // Linking happens on confirm; the board name rides along only when it is
  // ticked and valid, and a refusal of the name leaves the link in place.
  const confirm = async () => {
    setBusy(true);
    setProblem(null);
    try {
      await port.linkFeedbackAccount();
    } catch (e) {
      setProblem(linkFailure(e).message);
      setBusy(false);
      return;
    }
    markBoardOfferSeen();
    setAsking(false);
    try {
      setLinks(await port.feedbackLinks());
      if (join && boardNameProblem(name) === null) await port.joinFeedbackBoard(name.trim());
    } catch (e) {
      setProblem(join ? t("已关联，但没能加入贡献榜：{why}。可以在下面的贡献榜里改个名字再加入。", { why: reason(e) }) : linkFailure(e).message);
    } finally {
      setBusy(false);
      onBoardChanged?.();
    }
  };

  if (signedIn === null) return null;
  const here = links?.find((l) => l.current) ?? null;
  const others = (links ?? []).filter((l) => !l.current);
  const canLink = signedIn && hasReport && !busy;

  return (
    <section className="fbk-link" aria-label={t("账号关联")}>
      <div className="fbk-link-hd">
        <span className="fbk-link-name">{t("账号关联")}</span>
        <span className="fbk-hint">{t("关联后，这台电脑的反馈等级会显示在网页账号页上。关联不会改变额度，随时可以解除。")}</span>
      </div>
      {!signedIn && <p className="fbk-hint">{t("先在设置里登录账号，才能关联。")}</p>}
      {signedIn && !hasReport && !here && <p className="fbk-hint">{t("发出第一条反馈之后才能关联这台电脑。")}</p>}
      <div className="fbk-link-row">
        {here ? (
          <>
            <span className="fbk-chip" data-status="recorded">{t("这台电脑已关联")} · {day(here.linkedAt)}</span>
            <button type="button" className="btn sm" data-action="feedback.account-unlink" disabled={busy} onClick={() => void run(() => port.unlinkFeedbackAccount())}>
              {t("解除关联")}
            </button>
          </>
        ) : (
          <button type="button" className="btn sm" data-action="feedback.account-link" disabled={!canLink || asking} onClick={ask}>
            {t("把这台电脑关联到我的账号")}
          </button>
        )}
      </div>
      {asking && !here && (
        <div className="fbk-link-confirm" role="group" aria-label={t("确认关联")}>
          <FeedbackBoardJoin checked={join} name={name} disabled={busy} onChecked={setJoin} onName={setName} />
          <div className="fbk-link-row">
            <button type="button" className="btn sm" data-primary data-action="feedback.account-link-confirm" disabled={busy || (join && boardNameProblem(name) !== null)} onClick={() => void confirm()}>
              {t("确认")}
            </button>
            <button type="button" className="btn sm" data-action="feedback.account-link-cancel" disabled={busy} onClick={() => setAsking(false)}>
              {t("取消")}
            </button>
          </div>
        </div>
      )}
      {others.length > 0 && (
        <ul className="fbk-link-list" aria-label={t("账号下的其他电脑")}>
          {others.map((l) => (
            <li key={l.id}>
              <code className="fbk-code">{l.id}</code>
              <span className="fbk-meta">{day(l.linkedAt)}</span>
              <button type="button" className="btn sm" data-action="feedback.account-unlink-other" data-target={l.id} disabled={busy} onClick={() => void run(() => port.unlinkFeedbackInstall(l.id))}>
                {t("解除")}
              </button>
            </li>
          ))}
        </ul>
      )}
      {problem && <p className="fbk-hint" role="alert" data-tone="error">{problem}</p>}
    </section>
  );
}
