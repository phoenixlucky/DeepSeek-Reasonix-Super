import { useCallback, useEffect, useState, type ReactNode } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import { HttpError } from "../port/http_error";
import type { AgentPort, NotifyPrefs, PhoneNotifyPrefs } from "../port/port";
import type { HubPort } from "../port/hub";
import type { PhoneView } from "../port/share";
import { Group } from "./Group";
import { Switch } from "./Switch";

const POLL_MS = 10_000;

const when = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
};

/** What this computer tells the phones that have signed in to the same account.
 *  Only the kind of occasion and a count leave this machine: no session name,
 *  command or text. A phone chooses on its own page whether to receive them. */
export function PhoneNotifications({ hub, port, onError }: { hub: HubPort; port: AgentPort; onError: (e: unknown) => void }) {
  const [prefs, setPrefs] = useState<NotifyPrefs | null>(null);
  const [phones, setPhones] = useState<PhoneView[] | null>(null);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState("");

  useEffect(() => {
    let live = true;
    port.notifyPrefs().then((p) => live && setPrefs(p)).catch(() => live && setPrefs(null));
    return () => { live = false; };
  }, [port]);

  const readPhones = useCallback(async () => {
    try {
      const list = await hub.sharePhones();
      setPhones(list ? list.phones : null);
      setProblem("");
    } catch (e) {
      setProblem(reason(e));
      if (e instanceof HttpError && e.reason?.code === "phones.unavailable") setPhones(null);
    }
  }, [hub]);

  useEffect(() => {
    void readPhones();
    const timer = setInterval(() => void readPhones(), POLL_MS);
    return () => clearInterval(timer);
  }, [readPhones]);

  const flip = useCallback(
    (patch: Partial<PhoneNotifyPrefs>) => {
      if (!prefs) return;
      port.setNotifyPrefs({ ...prefs, phone: { ...prefs.phone, ...patch } }).then((got) => got && setPrefs(got)).catch(onError);
    },
    [port, prefs, onError],
  );

  const act = async (id: string, fn: () => Promise<{ phones: PhoneView[] }>) => {
    setBusy(id);
    try {
      setPhones((await fn()).phones);
      setProblem("");
      setConfirm("");
    } catch (e) {
      setProblem(reason(e));
    } finally {
      setBusy("");
    }
  };

  // A kernel with no window answers no preferences at all, and a settings page
  // does not grow a heading for a switch it cannot show.
  if (!prefs) return null;
  const on = prefs.phone.enabled;
  const row = (label: string, hint: string, sw: ReactNode) => (
    <div className="lrow subrow" data-off={on ? undefined : ""}>
      <span className="tx">
        <span className="lb">{label}</span>
        <span className="ds">{hint}</span>
      </span>
      {sw}
    </div>
  );
  return (
    <Group id="phone-notify" title={t("手机通知")}
      hint={t("有事等你时，电脑可以提醒登录了同一个账号的手机。发出去的只有“哪一类”和数量，不含会话名称、命令或任何对话内容；手机要在它自己的页面里选择接收。默认全部关闭。")}>
      <div className="lrow">
        <span className="tx">
          <span className="lb">{t("提醒我的手机")}</span>
          <span className="ds">{t("关闭时电脑不会向账号服务报告任何事。立即生效")}</span>
        </span>
        <Switch data-action="phone-notify.enabled" on={on} label={t("提醒我的手机")} onClick={() => flip({ enabled: !on })} />
      </div>
      {row(t("有操作等待批准时"), t("模型要动的东西超出了已经给出的授权，正停在那里等你"),
        <Switch data-action="phone-notify.approval" on={prefs.phone.approval} busy={!on} label={t("有操作等待批准时")} onClick={() => flip({ approval: !prefs.phone.approval })} />)}
      {row(t("模型提问时"), t("模型停下来问你一个问题，没有答案它就不会往下走"),
        <Switch data-action="phone-notify.input" on={prefs.phone.input} busy={!on} label={t("模型提问时")} onClick={() => flip({ input: !prefs.phone.input })} />)}
      {row(t("一轮失败时"), t("这一轮因为出错而结束；你自己取消的不算"),
        <Switch data-action="phone-notify.error" on={prefs.phone.error} busy={!on} label={t("一轮失败时")} onClick={() => flip({ error: !prefs.phone.error })} />)}

      {phones && (
        <>
          <div className="peernote"><b>{t("已登记的手机")}</b></div>
          {phones.length === 0 && <div className="peernote">{t("还没有手机登记。在手机上打开这台电脑的远程页面，并选择接收提醒。")}</div>}
          <ul className="wa-list" aria-label={t("已登记的手机")}>
            {phones.map((p) => (
              <li key={p.id}>
                <span className="tx">
                  <span className="lb">{p.name}{p.ordinal ? ` · ${t("设备 {n}", { n: p.ordinal })}` : ""}</span>
                  <span className="ds">
                    {p.state === "pending" ? t("等你批准 · 标识 {id}", { id: p.keyThumbprint }) : t("最近使用 {at}", { at: when(p.lastSeenAt) })}
                  </span>
                </span>
                <span className="acts">
                  {p.state === "pending" ? (
                    <>
                      <button className="act" data-action="phone.add" data-target={p.id} disabled={busy === p.id} onClick={() => void act(p.id, () => hub.resolvePhone(p.id, "add"))}>{t("添加")}</button>
                      {p.replaceCandidate && (
                        <button className="act" data-action="phone.replace" data-target={p.id} disabled={busy === p.id} onClick={() => void act(p.id, () => hub.resolvePhone(p.id, "replace", p.replaceCandidate))}>{t("替换旧设备")}</button>
                      )}
                      <button className="act ghost" data-action="phone.reject" data-target={p.id} disabled={busy === p.id} onClick={() => void act(p.id, () => hub.resolvePhone(p.id, "reject"))}>{t("拒绝")}</button>
                    </>
                  ) : confirm === p.id ? (
                    <button className="act danger" data-action="phone.revoke" data-target={p.id} disabled={busy === p.id} onClick={() => void act(p.id, () => hub.revokePhone(p.id))}>{t("确认移除")}</button>
                  ) : (
                    <button className="act ghost" data-action="phone.revoke-ask" data-target={p.id} onClick={() => setConfirm(p.id)}>{t("移除")}</button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
      {problem && <div className="why" role="alert">{problem}</div>}
    </Group>
  );
}
