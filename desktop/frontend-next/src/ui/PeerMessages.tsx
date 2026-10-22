import { useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort } from "../port/port";
import { HeldPeerMessage } from "./HeldPeerMessage";
import { peerLabel } from "./peerKeys";
import { PeerSettings } from "./PeerSettings";
import { usePeers } from "./usePeers";
import type { PeersSettingsStore } from "./usePeersSettings";

const POLL_MS = 2000;

export function PeerMessages({ port, settings }: { port: AgentPort; settings: PeersSettingsStore }) {
  const { state, failed, refresh } = usePeers(port, POLL_MS);
  const [to, setTo] = useState("");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [sent, setSent] = useState("");

  const switches = <PeerSettings port={port} settings={settings} onSaved={() => void refresh()} />;
  if (failed && !state.enabled) return <>{switches}<div className="empty">{t("无法读取会话间消息的状态。")}</div></>;
  if (!state.enabled) return switches;
  const target = to || state.peers[0]?.key || "";
  const send = async () => {
    setBusy(true);
    setError("");
    setSent("");
    try {
      const receipt = await port.sendPeerMessage(target, text);
      setSent(t("已送到「{to}」，对方的用户批准后才会读到。", { to: peerLabel(receipt.to) }));
      setText("");
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
    {switches}
    <div className="box">
      {state.held.map((h) => (
        <HeldPeerMessage key={h.id} port={port} held={h} onDone={() => void refresh()} />
      ))}
      {state.peers.length === 0 ? (
        <div className="peerhead">{t("本机没有别的会话在监听。")}</div>
      ) : (
        <div className="peerform">
          <label className="peerhead" htmlFor="peer-to">{t("发给")}</label>
          <select id="peer-to" className="peerfield" data-action-change="peers.target" aria-label={t("发给")} value={target} onChange={(e) => setTo(e.target.value)}>
            {state.peers.map((p) => (
              <option key={p.key} value={p.key}>{peerLabel(p)}</option>
            ))}
          </select>
          <textarea
            className="peerfield"
            data-action-change="peers.text"
            aria-label={t("要发的内容")}
            rows={3}
            value={text}
            disabled={busy}
            placeholder={t("写下要告诉另一个会话的话。对方的用户批准后，它才会交给那边的模型。")}
            onChange={(e) => setText(e.target.value)}
          />
          {error && <div className="why">{error}</div>}
          {sent && <div className="peerhead">{sent}</div>}
          <div className="acts">
            <button className="act" data-action="peers.send" data-primary disabled={busy || !text.trim() || !target} onClick={() => void send()}>
              {t(busy ? "发送中…" : "发送")}
            </button>
          </div>
        </div>
      )}
    </div>
    </>
  );
}
