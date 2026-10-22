import { reason } from "../i18n/kernel";
import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentPort, WorkspaceGit } from "../port/port";
import type { WorktreePreparation, WorktreeSelection, WorktreeSources } from "../port/workspace";

export class WorktreeSetupFailure extends Error {}

interface Attempt {
  id: string;
  port: AgentPort;
  selection: WorktreeSelection;
  session: string;
  status: WorktreePreparation;
  valid: boolean;
  cancelRequested: boolean;
}

const terminal = (phase: string) => ["ready", "failed", "cancelled", "cleanup_failed"].includes(phase);
const pause = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

export function useWorktreeSetup(port: AgentPort, fresh: boolean, root: string, onChanged: () => void, onTreeChanged?: (git: WorkspaceGit) => void, session = "") {
  const [sources, setSources] = useState<WorktreeSources | null>(null);
  const [selection, setSelection] = useState<WorktreeSelection | null>(null);
  const [stage, setStage] = useState<WorktreePreparation | null>(null);
  const attempt = useRef<Attempt | null>(null);
  const prepared = useRef("");
  const mounted = useRef(true);
  const sourceRead = useRef(0);
  const refresh = useCallback(async () => {
    const read = ++sourceRead.current;
    const value = await port.worktreeSources();
    if (mounted.current && read === sourceRead.current) setSources(value);
    return value;
  }, [port]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const current = attempt.current;
      if (current) {
        current.valid = false;
        if (!terminal(current.status.phase)) void current.port.cancelWorktreePreparation(current.id).catch(() => {});
      }
    };
  }, [port]);
  useEffect(() => {
    sourceRead.current++;
    const current = attempt.current;
    const ownMove = current && root !== current.selection.sourceRoot && root === current.status.workspaceRoot;
    const same = current?.port === port && ((root === current.selection.sourceRoot && session === current.session) || ownMove);
    if (!same) {
      if (current) {
        current.valid = false;
        if (!terminal(current.status.phase)) void current.port.cancelWorktreePreparation(current.id).catch(() => {});
      }
      attempt.current = null;
      setStage(null);
      prepared.current = "";
      setSelection(null);
      setSources(null);
    }
    if (fresh && !same) void refresh().catch(() => {});
    return () => { sourceRead.current++; };
  }, [port, fresh, root, session, refresh]);
  const show = (current: Attempt, value: WorktreePreparation) => {
    current.status = value;
    if (mounted.current && current.valid && attempt.current === current) setStage(value);
  };
  const prepare = async (attachments: string[]) => {
    if (prepared.current === root && root) return true;
    if (!selection && !attempt.current) return true;
    let current = attempt.current;
    const reconcile = current && !["failed", "cancelled"].includes(current.status.phase);
    if (!reconcile) {
      if (!selection) throw new WorktreeSetupFailure("worktree source is unavailable");
      const id = crypto.randomUUID();
      current = { id, port, selection: attachments.length ? { ...selection, attachments } : selection, session, valid: true, cancelRequested: false,
        status: { id, phase: "starting", sourceRoot: selection.sourceRoot, ref: selection.ref, startedAt: Date.now() } };
      attempt.current = current;
    }
    if (!current) return false;
    const active = current;
    show(active, active.status);
    try {
      let value: WorktreePreparation;
      try {
        value = reconcile ? await active.port.worktreePreparation(active.id) : await active.port.prepareWorktree(active.id, active.selection);
      } catch {
        value = await active.port.worktreePreparation(active.id);
      }
      while (active.valid && mounted.current) {
        show(active, value);
        if (terminal(value.phase)) break;
        await pause(400);
        if (!active.valid || !mounted.current) return false;
        value = await active.port.worktreePreparation(active.id);
      }
      if (!active.valid || !mounted.current) return false;
      if (value.phase === "ready" && value.workspaceRoot) {
        prepared.current = value.workspaceRoot;
        setSelection(null);
        setSources(null);
        onChanged();
        void active.port.workspaceGit().then(onTreeChanged).catch(() => {});
        if (active.cancelRequested) { show(active, { ...value, phase: "retained" }); return false; }
        await pause(650);
        return active.valid && mounted.current && !active.cancelRequested;
      }
      if (value.phase === "cancelled") return false;
      throw new WorktreeSetupFailure(value.detail || value.code || "worktree creation failed");
    } catch (error) {
      if (active.valid && mounted.current && !(error instanceof WorktreeSetupFailure)) {
        show(active, { ...active.status, phase: "unreachable", detail: reason(error) });
      }
      throw error instanceof WorktreeSetupFailure ? error : new WorktreeSetupFailure("worktree status could not be reconciled");
    }
  };
  const cancel = async () => {
    const current = attempt.current;
    if (!current) return;
    current.cancelRequested = true;
    show(current, { ...current.status, phase: "cancelling" });
    try {
      const value = await current.port.cancelWorktreePreparation(current.id);
      show(current, value.phase === "ready" ? { ...value, phase: "retained" } : value);
    } catch (error) {
      show(current, { ...current.status, phase: "unreachable", detail: reason(error) });
    }
  };
  const dismiss = () => {
    const current = attempt.current;
    if (current && !["ready", "retained", "failed", "cancelled"].includes(current.status.phase)) return;
    setStage(null);
    if (current?.status.phase === "failed" || current?.status.phase === "cancelled") attempt.current = null;
    if (fresh && prepared.current !== root) void refresh().catch(() => {});
  };
  return { sources, selection, setSelection, refresh, prepare, stage, cancel, dismiss };
}
