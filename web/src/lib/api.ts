// One thin fetch wrapper for every service. Calls go to /api/<service>/...,
// which nginx forwards to the KrakenD gateway. Every response is either
// {"data": ...} or {"error": {"code", "message"}}; this unwraps the first and
// turns the second into an ApiError the UI can show or branch on.

import { session } from "./session";

export type Service = "auth" | "master" | "order" | "warehouse" | "billing";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

// The gateway rejects some calls itself, with an empty body.
const fallbackMessages: Record<number, string> = {
  401: "Your session has ended. Please sign in again.",
  403: "Your role does not allow this.",
  429: "Too many attempts. Please wait a minute and try again.",
  502: "A service is not responding. Please try again shortly.",
  503: "A service is temporarily unavailable. Please try again shortly.",
};

async function request<T>(service: Service, method: string, path: string, body?: unknown, headers?: Record<string, string>): Promise<T> {
  const token = session.get()?.token;
  const res = await fetch(`/api/${service}${path}`, {
    method,
    headers: {
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...headers,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  let payload: { data?: T; error?: { code: string; message: string } } = {};
  try {
    payload = await res.json();
  } catch {
    // Empty body (gateway rejection) or a proxy error page.
  }

  if (!res.ok) {
    // An expired or revoked token: sign out, and the router sends us to the login page.
    if (res.status === 401 && token) session.clear();
    const err = payload.error;
    throw new ApiError(
      res.status,
      err?.code ?? `http_${res.status}`,
      err?.message ?? fallbackMessages[res.status] ?? `${service}-service returned ${res.status}`,
    );
  }
  return payload.data as T;
}

export const api = {
  get: <T>(service: Service, path: string) => request<T>(service, "GET", path),
  post: <T>(service: Service, path: string, body?: unknown, headers?: Record<string, string>) =>
    request<T>(service, "POST", path, body, headers),
  put: <T>(service: Service, path: string, body: unknown) => request<T>(service, "PUT", path, body),
};

/** Human-readable message for any error thrown by a query or mutation. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "Something went wrong";
}
