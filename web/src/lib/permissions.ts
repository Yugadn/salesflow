// What each role may do, mirroring the gateway and service rules. The UI uses
// it only to hide actions that would be refused; the backend still enforces
// every rule, so a hidden button is a courtesy, not the protection.

import type { Role, SessionUser } from "./session";

const is = (user: SessionUser | undefined, ...roles: Role[]) => !!user && roles.includes(user.role);

export const can = {
  editMasterData: (u?: SessionUser) => is(u, "admin"),
  manageUsers: (u?: SessionUser) => is(u, "admin"),
  seeInvoices: (u?: SessionUser) => is(u, "admin"),
  sell: (u?: SessionUser) => is(u, "admin", "sales"),
  workStock: (u?: SessionUser) => is(u, "admin", "warehouse"),
};

export const roleLabel: Record<Role, string> = {
  admin: "Admin",
  sales: "Sales",
  warehouse: "Warehouse",
};
