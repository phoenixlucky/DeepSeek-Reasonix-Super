import { createContext, useCallback, useContext, useEffect, useRef } from "react";

type Guard = (leave: () => void) => void;
type Register = (guard: Guard) => () => void;

export const SettingsLeaveContext = createContext<Register | null>(null);

export function useSettingsLeave() {
  const guard = useRef<Guard | null>(null);
  const register = useCallback<Register>((next) => {
    guard.current = next;
    return () => { if (guard.current === next) guard.current = null; };
  }, []);
  const leave = useCallback((next: () => void) => {
    if (guard.current) guard.current(next);
    else next();
  }, []);
  return { register, leave };
}

export function useSettingsLeaveGuard(guard: Guard) {
  const register = useContext(SettingsLeaveContext);
  useEffect(() => register?.(guard), [register, guard]);
}
