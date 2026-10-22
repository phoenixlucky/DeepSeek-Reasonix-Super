import { useCallback, useEffect, useState } from "react";
import { pressedChord } from "./keys";
import { listenAction } from "./listen";
import "../styles/browserwide.css";

export type BrowserSize = "split" | "wide" | "full";

export function useBrowserWide(docked: boolean, active: boolean, blocked = false) {
  const [size, setSize] = useState<BrowserSize>("split");
  const toggle = useCallback(() => setSize((size) => size === "wide" ? "split" : "wide"), []);
  const toggleFull = useCallback(() => setSize((size) => size === "full" ? "split" : "full"), []);
  useEffect(() => {
    if (!docked) setSize("split");
  }, [docked]);
  useEffect(() => {
    if (blocked) setSize((size) => size === "full" ? "split" : size);
  }, [blocked]);
  useEffect(() => {
    if (!docked || !active) return;
    const key = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.isComposing || e.repeat) return;
      if (pressedChord(e, "b", true)) {
        e.preventDefault();
        toggle();
      } else if (pressedChord(e, "f", true)) {
        e.preventDefault();
        toggleFull();
      } else if (e.key === "Escape" && size !== "split") {
        e.preventDefault();
        e.stopPropagation();
        setSize("split");
      }
    };
    // Document bubbling follows controls and dismiss layers, but precedes the
    // window's Escape handler that closes the browser or stops the turn.
    return listenAction(document, "keydown", { action: "browser.wide", listener: key as EventListener });
  }, [docked, active, size, toggle, toggleFull]);
  return { size: docked ? size : "split", toggle, toggleFull };
}
