import { CircleAlert, Loader2, X } from "lucide-react";
import { useEffect, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";
import { Link } from "react-router";

import { errorMessage } from "../lib/api";
import { formatIDR } from "../lib/money";
import { isNumericHead, type HeadCell } from "./head";

const cx = (...classes: (string | false | undefined)[]) => classes.filter(Boolean).join(" ");

// ---------------------------------------------------------------- buttons

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "danger" | "ghost";
  loading?: boolean;
};

const buttonStyles = {
  primary: "bg-indigo-600 text-white hover:bg-indigo-700 disabled:bg-indigo-300",
  secondary: "bg-white text-slate-700 ring-1 ring-slate-300 hover:bg-slate-50 disabled:text-slate-400",
  danger: "bg-white text-red-700 ring-1 ring-red-200 hover:bg-red-50 disabled:text-red-300",
  ghost: "text-slate-600 hover:bg-slate-100 hover:text-slate-900",
};

export function Button({ variant = "primary", loading, disabled, className, children, ...props }: ButtonProps) {
  return (
    <button
      type="button"
      disabled={disabled || loading}
      className={cx(
        "inline-flex items-center justify-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
        "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:cursor-not-allowed",
        buttonStyles[variant],
        className,
      )}
      {...props}
    >
      {loading && <Loader2 className="size-4 animate-spin" aria-hidden />}
      {children}
    </button>
  );
}

// ---------------------------------------------------------------- form controls

const controlStyles =
  "block w-full rounded-md border-0 bg-white px-2.5 py-1.5 text-sm text-slate-900 ring-1 ring-slate-300 ring-inset " +
  "placeholder:text-slate-400 focus:ring-2 focus:ring-indigo-500 disabled:bg-slate-50 disabled:text-slate-500";

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cx(controlStyles, className)} {...props} />;
}

export function Select({ className, children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={cx(controlStyles, "pr-8", className)} {...props}>
      {children}
    </select>
  );
}

export function Field({ label, hint, htmlFor, children }: { label: string; hint?: ReactNode; htmlFor?: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <label htmlFor={htmlFor} className="block text-sm font-medium text-slate-700">
        {label}
      </label>
      {children}
      {hint && <p className="text-xs text-slate-500">{hint}</p>}
    </div>
  );
}

// ---------------------------------------------------------------- layout blocks

export function PageHeader({
  title,
  description,
  actions,
  back,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  back?: { to: string; label: string };
}) {
  return (
    <div className="mb-6">
      {back && (
        <Link to={back.to} className="mb-2 inline-block text-sm text-slate-500 hover:text-slate-800">
          ← {back.label}
        </Link>
      )}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900">{title}</h1>
          {description && <p className="mt-1 text-sm text-slate-500">{description}</p>}
        </div>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </div>
    </div>
  );
}

export function Card({ title, actions, children, className }: { title?: ReactNode; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cx("min-w-0 rounded-lg bg-white ring-1 ring-slate-200", className)}>
      {(title || actions) && (
        <header className="flex items-center justify-between gap-2 border-b border-slate-200 px-4 py-3">
          <h2 className="text-sm font-semibold text-slate-800">{title}</h2>
          {actions}
        </header>
      )}
      {children}
    </section>
  );
}

// ---------------------------------------------------------------- tables

export function Table({ head, children, empty }: { head: HeadCell[]; children: ReactNode; empty?: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full divide-y divide-slate-200 text-sm">
        <thead className="bg-slate-50">
          <tr>
            {head.map((h, i) => (
              <th
                key={i}
                scope="col"
                className={cx(
                  "px-4 py-2.5 text-xs font-semibold tracking-wide whitespace-nowrap text-slate-500 uppercase",
                  isNumericHead(h) ? "text-right" : "text-left",
                )}
              >
                {isNumericHead(h) ? h.label : h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100">{children}</tbody>
      </table>
      {empty}
    </div>
  );
}

export function Td({ children, className, numeric }: { children: ReactNode; className?: string; numeric?: boolean }) {
  return <td className={cx("px-4 py-2.5 whitespace-nowrap text-slate-700", numeric && "tabular text-right", className)}>{children}</td>;
}

// ---------------------------------------------------------------- states

export function Spinner({ label = "Loading…" }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 p-6 text-sm text-slate-500" role="status">
      <Loader2 className="size-4 animate-spin" aria-hidden /> {label}
    </div>
  );
}

export function ErrorBanner({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <div className="flex items-start gap-2 rounded-md bg-red-50 p-3 text-sm text-red-800 ring-1 ring-red-200" role="alert">
      <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
      <span>{errorMessage(error)}</span>
    </div>
  );
}

export function EmptyState({ title, hint, action }: { title: string; hint?: ReactNode; action?: ReactNode }) {
  return (
    <div className="px-6 py-10 text-center">
      <p className="text-sm font-medium text-slate-700">{title}</p>
      {hint && <p className="mt-1 text-sm text-slate-500">{hint}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

/** Renders loading and error states for a query, then its content. */
export function QueryState({ query, children }: { query: { isPending: boolean; error: unknown }; children: ReactNode }) {
  if (query.isPending) return <Spinner />;
  if (query.error)
    return (
      <div className="p-4">
        <ErrorBanner error={query.error} />
      </div>
    );
  return <>{children}</>;
}

// ---------------------------------------------------------------- misc

export function Money({ value, className }: { value: string; className?: string }) {
  return <span className={cx("tabular", className)}>{formatIDR(value)}</span>;
}

const badgeTones = {
  gray: "bg-slate-100 text-slate-700 ring-slate-200",
  blue: "bg-blue-50 text-blue-700 ring-blue-200",
  amber: "bg-amber-50 text-amber-800 ring-amber-200",
  indigo: "bg-indigo-50 text-indigo-700 ring-indigo-200",
  green: "bg-emerald-50 text-emerald-700 ring-emerald-200",
  red: "bg-red-50 text-red-700 ring-red-200",
};

export type Tone = keyof typeof badgeTones;

export function Badge({ tone = "gray", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={cx("inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset", badgeTones[tone])}>
      {children}
    </span>
  );
}

export function Modal({
  open,
  title,
  onClose,
  children,
  footer,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-slate-900/40 p-4 pt-[10vh]" onMouseDown={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        className="w-full max-w-md rounded-lg bg-white shadow-xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <h2 id="modal-title" className="text-sm font-semibold text-slate-900">
            {title}
          </h2>
          <button type="button" onClick={onClose} className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700" aria-label="Close">
            <X className="size-4" />
          </button>
        </div>
        <div className="space-y-4 p-4">{children}</div>
        {footer && <div className="flex justify-end gap-2 border-t border-slate-200 px-4 py-3">{footer}</div>}
      </div>
    </div>
  );
}
