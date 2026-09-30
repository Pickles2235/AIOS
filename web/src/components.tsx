import React, { useEffect, useId, useRef } from "react";
import { motionSx, semanticSx, stylex, sx } from "./styles";
import { buttonVariant, noticeRole, type ButtonVariant, type NoticeTone } from "./component-state";

export const props = (...styles: Parameters<typeof stylex.props>) =>
  stylex.props(...styles);

export const buttonStyle = (variant: ButtonVariant = "secondary") =>
  variant === "primary" ? sx.primary : variant === "quiet" ? sx.legendButton : sx.secondary;

export function Brand() {
  return (
    <a {...props(sx.brand)} href="#" aria-label="AIOS home">
      <span {...props(sx.brandMark)} aria-hidden="true">AI</span>
      <span>AIOS</span>
      <em {...props(sx.brandLocal)}>local</em>
    </a>
  );
}

export function AppChrome({ children, headerActions, ...rest }: React.ComponentPropsWithoutRef<"main"> & { headerActions?: React.ReactNode }) {
  return <main {...props(sx.shell)} {...rest}><header {...props(sx.topbar)}><Brand /><div {...props(sx.chromeActions)}>{headerActions}</div></header>{children}</main>;
}

export const Button = React.forwardRef<HTMLButtonElement, React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; primary?: boolean }>(function Button({
  variant = "secondary",
  primary,
  type = "button",
  children,
  ...rest
}, ref) {
  return <button ref={ref} {...props(sx.button, buttonStyle(buttonVariant(primary ? "primary" : variant)))} type={type} {...rest}>{children}</button>;
});

export function Notice({ children, tone = "info", error }: { children: React.ReactNode; tone?: NoticeTone; error?: boolean }) {
  const resolvedTone = error ? "error" : tone;
  return <div {...props(resolvedTone === "error" ? sx.error : sx.notice)} role={noticeRole(resolvedTone)}>{children}</div>;
}

export function StatusBadge({ children }: { children: React.ReactNode }) {
  return <span {...props(sx.vectorStatus)}>{children}</span>;
}

export function Panel({ children, labelledBy, className, ...rest }: React.ComponentPropsWithoutRef<"section"> & { labelledBy?: string }) {
  return <section {...props(sx.graphPanel)} aria-labelledby={labelledBy} className={className} {...rest}>{children}</section>;
}

export function StepRail({ children, label, live = false }: { children: React.ReactNode; label: string; live?: boolean }) {
  return <ol {...props(live ? sx.liveRail : sx.phaseRail)} aria-label={label}>{children}</ol>;
}

export function Progress({ value, max, label }: { value?: number; max?: number; label: string }) {
  return <progress {...props(semanticSx.progressMeter)} value={value} max={max} aria-label={label} />;
}

const focusableSelector = 'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function Dialog({ title, onClose, opener, children }: {
  title: string;
  onClose: () => void;
  opener?: React.RefObject<HTMLElement | null>;
  children: React.ReactNode;
}) {
  const dialog = useRef<HTMLDivElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  const dismiss = () => { onClose(); opener?.current?.focus(); };
  useEffect(() => {
    closeButton.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); dismiss(); return; }
      if (event.key !== "Tab") return;
      const focusable = Array.from(dialog.current?.querySelectorAll<HTMLElement>(focusableSelector) || []);
      if (focusable.length < 2) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);
  return <div {...props(motionSx.dialogOverlay)} role="presentation"><div ref={dialog} {...props(motionSx.dialog)} role="dialog" aria-modal="true" aria-labelledby={titleId}><div {...props(sx.panelHead)}><h2 id={titleId} {...props(sx.heading2)}>{title}</h2><Button ref={closeButton} variant="quiet" onClick={dismiss}>Close</Button></div>{children}</div></div>;
}
