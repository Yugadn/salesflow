import { ChevronRight } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router";

import { useProducts, useReceiveStock, useRegions, useSaveWarehouse, useStocks, useWarehouses } from "../../api/hooks";
import type { Warehouse } from "../../api/types";
import { CrudPage } from "../../components/CrudPage";
import { Button, Card, EmptyState, ErrorBanner, Field, Input, PageHeader, QueryState, Select, Table, Td } from "../../components/ui";
import { num } from "../../components/head";
import { byId } from "../../lib/format";
import { can } from "../../lib/permissions";
import { useSession } from "../../lib/session";

export function WarehousesPage() {
  const canWork = can.workStock(useSession()?.user);
  const regions = useRegions();
  const regionsById = byId(regions.data);
  return (
    <CrudPage<Warehouse>
      title="Warehouses"
      description="One warehouse serves each region and reserves stock for that region's orders."
      noun="warehouse"
      canCreate={canWork}
      editable={canWork}
      query={useWarehouses()}
      save={useSaveWarehouse()}
      rowKey={(w) => w.id}
      columns={[
        { header: "Code", cell: (w) => <span className="font-mono text-xs">{w.code}</span> },
        {
          header: "Name",
          cell: (w) => (
            <Link to={`/warehouses/${w.id}`} className="font-medium text-indigo-700 hover:underline">
              {w.name}
            </Link>
          ),
        },
        { header: "Region", cell: (w) => regionsById.get(w.region_id)?.name ?? `#${w.region_id}` },
      ]}
      fields={[
        { name: "code", label: "Code" },
        { name: "name", label: "Name" },
        {
          name: "region_id",
          label: "Region served",
          type: "select",
          options: (regions.data ?? []).map((r) => ({ value: r.id, label: `${r.code} — ${r.name}` })),
          createOnly: true,
        },
      ]}
      rowActions={(w) => (
        <Link
          to={`/warehouses/${w.id}`}
          className="inline-flex items-center gap-0.5 rounded-md px-2 py-1.5 text-sm font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900"
        >
          Stock <ChevronRight className="size-4" aria-hidden />
        </Link>
      )}
    />
  );
}

export function WarehouseStockPage() {
  const canWork = can.workStock(useSession()?.user);
  const id = Number(useParams().id);
  const warehouses = useWarehouses();
  const stocks = useStocks(id);
  const products = useProducts();
  const productsById = byId(products.data);
  const warehouse = warehouses.data?.find((w) => w.id === id);

  return (
    <>
      <PageHeader
        back={{ to: "/warehouses", label: "Warehouses" }}
        title={warehouse ? `${warehouse.name} stock` : "Stock"}
        description="Quantities are in each product's base unit. Reserved stock is promised to confirmed orders that have not been delivered yet."
      />
      <div className="grid items-start gap-6 lg:grid-cols-[3fr_2fr]">
        <Card title="Stock levels">
          <QueryState query={stocks}>
            <Table
              head={["Product", num("On hand"), num("Reserved"), num("Available")]}
              empty={(stocks.data ?? []).length === 0 && <EmptyState title="No stock yet" hint="Receive goods to start." />}
            >
              {(stocks.data ?? []).map((s) => {
                const p = productsById.get(s.product_id);
                const available = s.on_hand - s.reserved;
                return (
                  <tr key={s.product_id}>
                    <Td>
                      <div className="font-medium text-slate-900">{p?.name ?? `Product #${s.product_id}`}</div>
                      {p && <div className="font-mono text-xs text-slate-500">{p.sku}</div>}
                    </Td>
                    <Td numeric>{s.on_hand.toLocaleString("id-ID")}</Td>
                    <Td numeric>{s.reserved.toLocaleString("id-ID")}</Td>
                    <Td numeric className={available === 0 ? "font-semibold text-red-700" : "font-semibold"}>
                      {available.toLocaleString("id-ID")} {p?.base_uom}
                    </Td>
                  </tr>
                );
              })}
            </Table>
          </QueryState>
        </Card>
        {canWork && <ReceiveForm warehouseId={id} />}
      </div>
    </>
  );
}

function ReceiveForm({ warehouseId }: { warehouseId: number }) {
  const products = useProducts();
  const receive = useReceiveStock(warehouseId);
  const [productId, setProductId] = useState("");
  const [qty, setQty] = useState("");
  const product = products.data?.find((p) => p.id === Number(productId));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    receive.mutate({ product_id: Number(productId), qty: Number(qty) }, { onSuccess: () => setQty("") });
  };

  return (
    <Card title="Receive goods">
      <form onSubmit={submit} className="space-y-3 p-4">
        <Field label="Product" htmlFor="rcv-product">
          <Select id="rcv-product" value={productId} onChange={(e) => setProductId(e.target.value)} required>
            <option value="" disabled>
              Select…
            </option>
            {(products.data ?? []).map((p) => (
              <option key={p.id} value={p.id}>
                {p.sku} — {p.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={`Quantity${product ? ` (${product.base_uom})` : ""}`} htmlFor="rcv-qty" hint="Counted in the base unit.">
          <Input id="rcv-qty" type="number" min={1} value={qty} onChange={(e) => setQty(e.target.value)} required />
        </Field>
        <ErrorBanner error={receive.error} />
        <Button type="submit" loading={receive.isPending}>
          Receive
        </Button>
      </form>
    </Card>
  );
}
