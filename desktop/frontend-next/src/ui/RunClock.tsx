import { useEffect, useState } from "react";
import { elapsed } from "../i18n/format";
import { usePageVisible, useShown } from "./shown";

export function RunClock({ since }: { since: number }) {
  const shown = useShown();
  const visible = usePageVisible();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    setNow(Date.now());
    if (!shown || !visible) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [since, shown, visible]);
  return <span className="studio-runclock" aria-hidden="true">{elapsed(Math.max(0, now - since))}</span>;
}
