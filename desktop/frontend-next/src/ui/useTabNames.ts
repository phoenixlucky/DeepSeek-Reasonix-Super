import { useState } from "react";
import { keyOf, type Surface } from "./workbench_tabs";

// The names a person gave tabs: held by the tab's stable key for the window's session, and
// stored only when they differ from the page's own label, so leaving the field untouched never
// turns a title that is still changing into a name the person chose.
export function useTabNames(ownLabel: (surface: Surface) => string) {
  const [names, setNames] = useState<Record<string, string>>({});
  const [renaming, setRenaming] = useState("");
  const labelFor = (surface: Surface) => names[keyOf(surface)] || ownLabel(surface);
  const nameTab = (surface: Surface, value: string) => {
    const key = keyOf(surface);
    const next = value.trim();
    const keep = next !== "" && next !== ownLabel(surface);
    setNames((cur) => { const { [key]: _gone, ...rest } = cur; return keep ? { ...rest, [key]: next } : rest; });
    setRenaming("");
  };
  return { renaming, setRenaming, labelFor, nameTab };
}
