import { useCallback, useState } from "react";

/** What the window asks of the focused pane's find bar. The chord and the palette
 *  ask it to be open; the title bar button switches it. Under the settings sheet
 *  nothing is in view to close, so there the button only opens, like the rest. */
export function useFindAsk(settings: boolean | string, leaveSettings: (to: false) => void) {
  const [pulse, setPulse] = useState(0);
  const [togglePulse, setTogglePulse] = useState(0);
  const open = useCallback(() => {
    leaveSettings(false);
    setPulse((n) => n + 1);
  }, [leaveSettings]);
  const toggle = useCallback(() => {
    if (settings) open();
    else setTogglePulse((n) => n + 1);
  }, [settings, open]);
  return { pulse, togglePulse, open, toggle };
}
