import { Boxes } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Navigate, useSearchParams } from "react-router";

import { useLogin } from "../api/hooks";
import { Button, ErrorBanner, Field, Input } from "../components/ui";
import { useSession } from "../lib/session";

// Accounts created by `go run ./tools/seed`, listed so a visitor can try each role.
const demoAccounts = [
  { email: "admin@salesflow.local", password: "admin12345", role: "Admin", what: "everything, incl. users and invoices" },
  { email: "andi@salesflow.local", password: "sales12345", role: "Sales", what: "own orders, Jakarta customers" },
  { email: "gudang@salesflow.local", password: "gudang12345", role: "Warehouse", what: "stock, shipping, proof of delivery" },
];

export function LoginPage() {
  const current = useSession();
  const [params] = useSearchParams();
  const login = useLogin();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  // Only same-app paths, never an absolute URL from the query string.
  const next = params.get("next")?.startsWith("/") && !params.get("next")?.startsWith("//") ? params.get("next")! : "/";
  if (current) return <Navigate to={next} replace />;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    login.mutate({ email, password });
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-900 px-4 py-10">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center justify-center gap-2 text-white">
          <Boxes className="size-6 text-indigo-400" aria-hidden />
          <span className="text-lg font-semibold tracking-tight">salesflow</span>
        </div>

        <form onSubmit={submit} className="space-y-4 rounded-lg bg-white p-6 shadow-xl">
          <h1 className="text-base font-semibold text-slate-900">Sign in</h1>
          <Field label="Email" htmlFor="email">
            <Input id="email" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
          </Field>
          <Field label="Password" htmlFor="password">
            <Input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </Field>
          <ErrorBanner error={login.error} />
          <Button type="submit" loading={login.isPending} className="w-full">
            Sign in
          </Button>
        </form>

        <div className="mt-6 rounded-lg bg-slate-800 p-4 text-sm text-slate-300">
          <p className="mb-2 font-medium text-white">Demo accounts</p>
          <ul className="space-y-1.5">
            {demoAccounts.map((a) => (
              <li key={a.email}>
                <button
                  type="button"
                  className="w-full rounded px-2 py-1 text-left hover:bg-slate-700"
                  onClick={() => {
                    setEmail(a.email);
                    setPassword(a.password);
                  }}
                >
                  <span className="font-medium text-white">{a.role}</span> <span className="text-slate-400">— {a.what}</span>
                  <span className="block font-mono text-xs text-slate-400">{a.email}</span>
                </button>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-xs text-slate-500">Click one to fill the form. Created by the demo seed.</p>
        </div>
      </div>
    </div>
  );
}
