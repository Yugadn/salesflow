// One thin fetch wrapper for all four services. Every response is either
// {"data": ...} or {"error": {"code", "message"}}; this unwraps the first and
// turns the second into an ApiError the UI can show or branch on.

export type Service = "master" | "order" | "warehouse" | "billing";

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

async function request<T>(service: Service, method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/${service}${path}`, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  let payload: { data?: T; error?: { code: string; message: string } } = {};
  try {
    payload = await res.json();
  } catch {
    // A proxy error page or an empty body: fall through to the generic error.
  }

  if (!res.ok) {
    const err = payload.error;
    throw new ApiError(res.status, err?.code ?? "http_" + res.status, err?.message ?? `${service}-service returned ${res.status}`);
  }
  return payload.data as T;
}

export const api = {
  get: <T>(service: Service, path: string) => request<T>(service, "GET", path),
  post: <T>(service: Service, path: string, body?: unknown) => request<T>(service, "POST", path, body),
  put: <T>(service: Service, path: string, body: unknown) => request<T>(service, "PUT", path, body),
};

/** Human-readable message for any error thrown by a query or mutation. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "Something went wrong";
}
