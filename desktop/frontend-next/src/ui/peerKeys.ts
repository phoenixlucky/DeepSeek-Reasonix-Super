import type { PeerHeld, PeerView } from "../port/port";

export const peerLabel = (p: Pick<PeerView, "title" | "project" | "key">) =>
  p.project && p.title ? `${p.project} / ${p.title}` : p.title || p.key;

export const heldLabel = (h: PeerHeld) =>
  peerLabel({ title: h.fromTitle ?? "", project: h.fromProject, key: h.fromSession });
