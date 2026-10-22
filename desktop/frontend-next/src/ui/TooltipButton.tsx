import { useCallback, useEffect, useId, useImperativeHandle, useRef, useState, type ComponentPropsWithRef } from "react";
import { createPortal } from "react-dom";
import { beside, pinToViewport, useFollow } from "./place";
import "./tooltip.css";
import { listenAction } from "./listen";

type Props = Omit<ComponentPropsWithRef<"button">, "onBlur"> & { shortcut?: string };
let dismissActive: (() => void) | undefined;

export function TooltipButton({
  title, shortcut, ref, children, onPointerEnter, onPointerLeave, onPointerDown,
  onFocus, onClick, "aria-describedby": describedBy, ...props
}: Props) {
  const anchor = useRef<HTMLButtonElement>(null);
  const tip = useRef<HTMLDivElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [open, setOpen] = useState(false);
  const id = useId();
  useImperativeHandle(ref, () => anchor.current!, []);
  const clear = useCallback(() => { clearTimeout(timer.current); }, []);
  const close = useCallback(() => {
    clear(); setOpen(false);
    if (dismissActive === close) dismissActive = undefined;
  }, [clear]);
  const show = () => {
    clear();
    if (!title || props.disabled) return;
    dismissActive?.();
    dismissActive = close;
    setOpen(true);
  };
  useEffect(() => () => {
    clear();
    if (dismissActive === close) dismissActive = undefined;
  }, [clear, close]);
  useEffect(() => {
    if (!open) return;
    const key = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.stopPropagation();
      close();
    };
    const offKey = listenAction(window, "keydown", { action: "layer.dismiss", listener: key as EventListener }, true);
    addEventListener("blur", close);
    return () => { offKey(); removeEventListener("blur", close); };
  }, [open, close]);
  const place = useCallback(() => {
    if (!anchor.current || !tip.current) return;
    const rect = anchor.current.getBoundingClientRect();
    const box = tip.current.getBoundingClientRect();
    pinToViewport(tip.current, rect.left + (rect.width - box.width) / 2, beside(rect, box.height, innerHeight, "above", 6));
  }, []);
  useFollow(open, place, `${title}:${shortcut}`);
  return <>
    <button {...props} ref={anchor} type={props.type ?? "button"}
      aria-describedby={[describedBy, open ? id : undefined].filter(Boolean).join(" ") || undefined}
      onPointerEnter={(event) => {
        onPointerEnter?.(event);
        if (event.pointerType !== "touch") { clear(); timer.current = setTimeout(show, 400); }
      }}
      onPointerLeave={(event) => { close(); onPointerLeave?.(event); }}
      onPointerDown={(event) => { close(); onPointerDown?.(event); }}
      onFocus={(event) => { show(); onFocus?.(event); }}
      onBlur={close}
      onClick={(event) => { close(); onClick?.(event); }}>
      {children}
    </button>
    {open && title && !props.disabled && createPortal(
      <div className="icon-tooltip" id={id} role="tooltip" ref={tip}>
        <span>{title}</span>{shortcut && <kbd>{shortcut}</kbd>}
      </div>, document.body,
    )}
  </>;
}
