import { useSyncExternalStore } from "react";

const KEY = "rx-show-agent-composition";
const listeners = new Set<() => void>();

function read(): boolean {
  try { return localStorage.getItem(KEY) === "on"; }
  catch { return false; }
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  const refresh = (event: StorageEvent) => {
    if (event.key === KEY || event.key === null) fn();
  };
  window.addEventListener("storage", refresh);
  return () => { listeners.delete(fn); window.removeEventListener("storage", refresh); };
}

export function setShowsCompositionPicker(on: boolean): boolean {
  try { localStorage.setItem(KEY, on ? "on" : "off"); }
  catch { return false; }
  listeners.forEach(fn => fn());
  return true;
}

export const useShowsCompositionPicker = () => useSyncExternalStore(subscribe, read, read);
