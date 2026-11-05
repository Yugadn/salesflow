// The signed-in session, shared by the API client and React. A tiny external
// store: React reads it with useSyncExternalStore, the API client reads it
// directly, and both see sign-out immediately.
//
// The token lives in localStorage so a refresh keeps you signed in. That is
// readable by any script on the page, so it relies on the app having no XSS;
// an httpOnly cookie would remove that risk and is on the roadmap.

import { useSyncExternalStore } from "react";

export type Role = "admin" | "sales" | "warehouse";

export interface SessionUser {
  id: number;
  email: string;
  name: string;
  role: Role;
  salesperson_id?: number;
}

export interface Session {
  token: string;
  expires_at: string;
  user: SessionUser;
}

const KEY = "salesflow.session";
const listeners = new Set<() => void>();
let current: Session | null = load();
let expiryTimer: ReturnType<typeof setTimeout> | undefined;
scheduleExpiry();

function expired(s: Session): boolean {
  return new Date(s.expires_at).getTime() <= Date.now();
}

function load(): Session | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const s = JSON.parse(raw) as Session;
    return expired(s) ? null : s;
  } catch {
    return null;
  }
}

// Signs out the moment the token expires, so the app never keeps showing a
// session the backend no longer accepts.
function scheduleExpiry() {
  clearTimeout(expiryTimer);
  if (!current) return;
  const ms = Math.min(new Date(current.expires_at).getTime() - Date.now(), 2 ** 31 - 1);
  expiryTimer = setTimeout(() => session.clear(), Math.max(ms, 0));
}

function emit() {
  listeners.forEach((fn) => fn());
}

export const session = {
  /** Pure read: React calls this during render. */
  get: (): Session | null => current,
  set(s: Session) {
    current = s;
    try {
      localStorage.setItem(KEY, JSON.stringify(s));
    } catch {
      // Private mode or storage disabled: the session lasts until the tab closes.
    }
    scheduleExpiry();
    emit();
  },
  clear() {
    if (!current) return;
    current = null;
    clearTimeout(expiryTimer);
    try {
      localStorage.removeItem(KEY);
    } catch {
      // ignore
    }
    emit();
  },
  subscribe(fn: () => void) {
    listeners.add(fn);
    return () => {
      listeners.delete(fn);
    };
  },
};

/** The current session, re-rendering when it changes. */
export function useSession(): Session | null {
  return useSyncExternalStore(session.subscribe, session.get);
}
