import { useEffect, useId, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { JobEntry } from "../../port/port";
import { listenAction } from "../listen";

const LONG_PRESS_MS = 450;

/** What a job is called in the list: the name its caller gave it for a person,
 *  else the short preview of what it runs, else its id. */
export const jobName = (j: JobEntry) => j.description?.trim() || j.label || j.id;

/** The name of a job with its full text on hover, focus, Escape to close and a
 *  long press on a touch screen. The tip is real text, so a pointer can move onto
 *  it and select or copy the command (React sees the portal as a child, so
 *  leaving the name for the tip is not leaving); it is placed in the window
 *  because the list sits in a panel that clips what overflows it. */
export function JobName({ job, children }: { job: JobEntry; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const [place, setPlace] = useState<CSSProperties>({});
  const box = useRef<HTMLSpanElement>(null);
  const press = useRef<ReturnType<typeof setTimeout> | null>(null);
  const id = useId();
  const name = jobName(job);
  const command = job.detail?.trim() || (job.description?.trim() ? job.label : "");

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const stop = () => { if (press.current) { clearTimeout(press.current); press.current = null; } };
    const off = [
      listenAction(el, "keydown", { action: "job.tip", value: job.id, listener: (e) => { if ((e as KeyboardEvent).key === "Escape") { e.stopPropagation(); setOpen(false); } } }),
      listenAction(el, "pointerdown", { action: "job.tip", value: job.id, listener: (e) => { if ((e as PointerEvent).pointerType === "touch") { stop(); press.current = setTimeout(() => setOpen(true), LONG_PRESS_MS); } } }),
      listenAction(el, "pointerup", { action: "job.tip", value: job.id, listener: stop }),
      listenAction(el, "pointercancel", { action: "job.tip", value: job.id, listener: stop }),
    ];
    return () => { stop(); off.forEach((f) => f()); };
  }, [job.id]);

  // Under the name, or above it when there is no room below.
  useEffect(() => {
    if (!open || !box.current) return;
    const r = box.current.getBoundingClientRect();
    const width = Math.min(420, window.innerWidth * 0.8);
    const left = Math.max(8, Math.min(r.left, window.innerWidth - width - 8));
    setPlace(r.bottom + 120 > window.innerHeight ? { left, bottom: window.innerHeight - r.top + 4 } : { left, top: r.bottom + 4 });
  }, [open]);

  return (
    <span
      className="job-name"
      ref={box}
      aria-describedby={open ? id : undefined}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOpen(false); }}
    >
      {children}
      {open && createPortal(
        <span className="job-tip" role="tooltip" id={id} style={place}>
          <span className="jt-name">{name}</span>
          {command && command !== name && <code className="jt-cmd">{command}</code>}
        </span>,
        document.body,
      )}
    </span>
  );
}
