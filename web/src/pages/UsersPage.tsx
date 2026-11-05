import { useSalespersons, useSaveUser, useUsers } from "../api/hooks";
import type { User } from "../api/types";
import { CrudPage } from "../components/CrudPage";
import { Badge } from "../components/ui";
import { byId } from "../lib/format";
import { roleLabel } from "../lib/permissions";

export function UsersPage() {
  const salespersons = useSalespersons();
  const spById = byId(salespersons.data);

  return (
    <CrudPage<User>
      title="Users"
      description="Who can sign in, and as what. A sales login acts as one salesperson and only sees that salesperson's orders. Users are deactivated, never deleted."
      noun="user"
      query={useUsers()}
      save={useSaveUser()}
      rowKey={(u) => u.id}
      columns={[
        { header: "Name", cell: (u) => <span className={u.active ? "" : "text-slate-400 line-through"}>{u.name}</span> },
        { header: "Email", cell: (u) => <span className="font-mono text-xs">{u.email}</span> },
        { header: "Role", cell: (u) => <Badge tone={u.role === "admin" ? "indigo" : u.role === "sales" ? "blue" : "amber"}>{roleLabel[u.role]}</Badge> },
        { header: "Acts as", cell: (u) => (u.salesperson_id ? (spById.get(u.salesperson_id)?.name ?? `#${u.salesperson_id}`) : "—") },
        { header: "Status", cell: (u) => (u.active ? <Badge tone="green">Active</Badge> : <Badge>Inactive</Badge>) },
      ]}
      fields={[
        { name: "email", label: "Email", lockedOnEdit: true, hint: "Used to sign in." },
        { name: "name", label: "Name" },
        {
          name: "role",
          label: "Role",
          type: "select",
          options: [
            { value: "admin", label: "Admin — everything" },
            { value: "sales", label: "Sales — own orders" },
            { value: "warehouse", label: "Warehouse — stock and deliveries" },
          ],
        },
        {
          name: "salesperson_id",
          label: "Acts as salesperson",
          type: "select",
          options: (salespersons.data ?? []).map((s) => ({ value: s.id, label: `${s.code} — ${s.name}` })),
          showWhen: (f) => f.role === "sales",
          hint: "Orders created by this login belong to this salesperson.",
        },
        { name: "password", label: "Password", type: "password", optionalOnEdit: true, hint: "8 to 72 characters. Leave empty to keep the current one." },
        { name: "active", label: "Status", type: "boolean", editOnly: true, hint: "Inactive users cannot sign in." },
      ]}
    />
  );
}
