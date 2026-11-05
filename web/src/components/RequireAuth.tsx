import { ShieldX } from "lucide-react";
import type { ReactNode } from "react";
import { Link, Navigate, useLocation } from "react-router";

import { useSession, type SessionUser } from "../lib/session";
import { EmptyState } from "./ui";

/** Sends signed-out visitors to the login page, then back here afterwards. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const s = useSession();
  const location = useLocation();
  if (!s) {
    const next = location.pathname + location.search;
    return <Navigate to={next === "/" ? "/login" : `/login?next=${encodeURIComponent(next)}`} replace />;
  }
  return <>{children}</>;
}

/** Shows a page only to users the rule allows; the API would refuse anyway. */
export function Allow({ when, children }: { when: (u: SessionUser) => boolean; children: ReactNode }) {
  const s = useSession();
  if (s && when(s.user)) return <>{children}</>;
  return (
    <div className="pt-10">
      <ShieldX className="mx-auto size-8 text-slate-300" aria-hidden />
      <EmptyState
        title="Not available for your role"
        hint="Ask an administrator if you need access."
        action={
          <Link to="/" className="text-sm text-indigo-700 hover:underline">
            Back to the dashboard
          </Link>
        }
      />
    </div>
  );
}
