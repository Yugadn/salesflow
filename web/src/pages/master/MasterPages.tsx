import { useSaveCustomer, useSaveRegion, useSaveSalesperson, useCreateUoM, useCustomers, useRegions, useSalespersons, useUoMs } from "../../api/hooks";
import type { Customer, Region, Salesperson, UoM } from "../../api/types";
import { CrudPage } from "../../components/CrudPage";
import { byId } from "../../lib/format";
import { can } from "../../lib/permissions";
import { useSession } from "../../lib/session";

function useCanEdit() {
  return can.editMasterData(useSession()?.user);
}

function useRegionOptions() {
  const regions = useRegions();
  const options = (regions.data ?? []).map((r) => ({ value: r.id, label: `${r.code} — ${r.name}` }));
  return { regionsById: byId(regions.data), options };
}

export function RegionsPage() {
  const canEdit = useCanEdit();
  return (
    <CrudPage<Region>
      title="Regions"
      description="Sales territories. Each has its own price list and is served by one warehouse."
      noun="region"
      canCreate={canEdit}
      editable={canEdit}
      query={useRegions()}
      save={useSaveRegion()}
      rowKey={(r) => r.id}
      columns={[
        { header: "Code", cell: (r) => <span className="font-mono text-xs">{r.code}</span> },
        { header: "Name", cell: (r) => r.name },
      ]}
      fields={[
        { name: "code", label: "Code", hint: "Short code, e.g. JKT" },
        { name: "name", label: "Name" },
      ]}
    />
  );
}

export function CustomersPage() {
  const canEdit = useCanEdit();
  const { regionsById, options } = useRegionOptions();
  return (
    <CrudPage<Customer>
      title="Customers"
      description="Stores and distributors that buy from you. Their region decides which prices and salespeople apply."
      noun="customer"
      canCreate={canEdit}
      editable={canEdit}
      query={useCustomers()}
      save={useSaveCustomer()}
      rowKey={(c) => c.id}
      columns={[
        { header: "Code", cell: (c) => <span className="font-mono text-xs">{c.code}</span> },
        { header: "Name", cell: (c) => c.name },
        { header: "Region", cell: (c) => regionsById.get(c.region_id)?.name ?? `#${c.region_id}` },
        { header: "Payment term", cell: (c) => (c.payment_term_days === 0 ? "Cash on delivery" : `${c.payment_term_days} days`) },
      ]}
      fields={[
        { name: "code", label: "Code" },
        { name: "name", label: "Name" },
        { name: "region_id", label: "Region", type: "select", options },
        { name: "payment_term_days", label: "Payment term (days)", type: "number", hint: "0 = cash on delivery. Invoices fall due this many days after delivery." },
      ]}
    />
  );
}

export function SalespersonsPage() {
  const canEdit = useCanEdit();
  const { regionsById, options } = useRegionOptions();
  return (
    <CrudPage<Salesperson>
      title="Salespersons"
      description="A salesperson can only take orders from customers in their own region."
      noun="salesperson"
      canCreate={canEdit}
      editable={canEdit}
      query={useSalespersons()}
      save={useSaveSalesperson()}
      rowKey={(s) => s.id}
      columns={[
        { header: "Code", cell: (s) => <span className="font-mono text-xs">{s.code}</span> },
        { header: "Name", cell: (s) => s.name },
        { header: "Region", cell: (s) => regionsById.get(s.region_id)?.name ?? `#${s.region_id}` },
      ]}
      fields={[
        { name: "code", label: "Code" },
        { name: "name", label: "Name" },
        { name: "region_id", label: "Region", type: "select", options },
      ]}
    />
  );
}

export function UoMsPage() {
  const canEdit = useCanEdit();
  return (
    <CrudPage<UoM>
      title="Units of measure"
      description="Units products are counted and sold in. How many base units each holds is set per product."
      noun="unit"
      canCreate={canEdit}
      query={useUoMs()}
      save={useCreateUoM()}
      editable={false}
      rowKey={(u) => u.code}
      columns={[
        { header: "Code", cell: (u) => <span className="font-mono text-xs">{u.code}</span> },
        { header: "Name", cell: (u) => u.name },
      ]}
      fields={[
        { name: "code", label: "Code", hint: "e.g. PCS, DUS, PAK" },
        { name: "name", label: "Name" },
      ]}
    />
  );
}
