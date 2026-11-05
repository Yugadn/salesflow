import { useState, type FormEvent } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { useDeliver, useDeliveries, useDelivery, useShip, useWarehouses } from "../../api/hooks";
import type { Delivery, DeliveryStatus } from "../../api/types";
import { DeliveryStatusBadge } from "../../components/StatusBadge";
import { Button, Card, EmptyState, ErrorBanner, Input, PageHeader, QueryState, Table, Td } from "../../components/ui";
import { formatDate } from "../../lib/format";
import { can } from "../../lib/permissions";
import { useSession } from "../../lib/session";

const filters: { label: string; status?: DeliveryStatus }[] = [
  { label: "All" },
  { label: "To ship", status: "PENDING" },
  { label: "In transit", status: "SHIPPED" },
  { label: "Delivered", status: "DELIVERED" },
];

export function DeliveriesPage() {
  const [params, setParams] = useSearchParams();
  const status = (params.get("status") ?? undefined) as DeliveryStatus | undefined;
  const deliveries = useDeliveries({ status });

  return (
    <>
      <PageHeader title="Deliveries" description="Delivery orders are created automatically once stock is reserved for a confirmed order." />
      <div className="mb-4 flex flex-wrap gap-1" role="tablist" aria-label="Filter by status">
        {filters.map((f) => {
          const active = f.status === status;
          return (
            <button
              key={f.label}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => setParams(f.status ? { status: f.status } : {})}
              className={
                "rounded-full px-3 py-1 text-sm font-medium " +
                (active ? "bg-slate-900 text-white" : "bg-white text-slate-600 ring-1 ring-slate-200 hover:bg-slate-100")
              }
            >
              {f.label}
            </button>
          );
        })}
      </div>
      <Card>
        <QueryState query={deliveries}>
          <Table
            head={["Delivery", "Order", "Customer", "Status", "Created"]}
            empty={(deliveries.data ?? []).length === 0 && <EmptyState title="No deliveries" hint="Confirm an order to create one." />}
          >
            {(deliveries.data ?? []).map((d) => (
              <tr key={d.id} className="hover:bg-slate-50">
                <Td>
                  <Link to={`/deliveries/${d.id}`} className="font-mono text-xs font-medium text-indigo-700 hover:underline">
                    {d.number}
                  </Link>
                </Td>
                <Td>
                  <Link to={`/orders/${d.order_id}`} className="font-mono text-xs text-slate-600 hover:underline">
                    {d.order_number}
                  </Link>
                </Td>
                <Td>{d.customer_name}</Td>
                <Td>
                  <DeliveryStatusBadge status={d.status} />
                </Td>
                <Td>{formatDate(d.created_at, true)}</Td>
              </tr>
            ))}
          </Table>
        </QueryState>
      </Card>
    </>
  );
}

export function DeliveryDetailPage() {
  const id = Number(useParams().id);
  const delivery = useDelivery(id);
  return <QueryState query={delivery}>{delivery.data && <DeliveryView delivery={delivery.data} />}</QueryState>;
}

function DeliveryView({ delivery }: { delivery: Delivery }) {
  const warehouses = useWarehouses();
  const ship = useShip(delivery.id);
  const deliver = useDeliver(delivery.id);
  const [qty, setQty] = useState<Record<number, string>>(() =>
    Object.fromEntries(delivery.lines.map((l) => [l.line_no, String(l.ordered_qty)])),
  );
  const warehouse = warehouses.data?.find((w) => w.id === delivery.warehouse_id);
  const canWork = can.workStock(useSession()?.user);
  const canDeliver = canWork && delivery.status === "SHIPPED";

  const submit = (e: FormEvent) => {
    e.preventDefault();
    // Only send lines that differ from the ordered quantity; the rest are delivered in full.
    const overrides = delivery.lines
      .filter((l) => Number(qty[l.line_no]) !== l.ordered_qty)
      .map((l) => ({ line_no: l.line_no, delivered_qty: Number(qty[l.line_no]) }));
    deliver.mutate(overrides);
  };

  return (
    <>
      <PageHeader
        back={{ to: "/deliveries", label: "Deliveries" }}
        title={
          <span className="flex items-center gap-3">
            <span className="font-mono">{delivery.number}</span> <DeliveryStatusBadge status={delivery.status} />
          </span>
        }
        description={
          <>
            For order{" "}
            <Link to={`/orders/${delivery.order_id}`} className="font-mono text-indigo-700 hover:underline">
              {delivery.order_number}
            </Link>{" "}
            · {delivery.customer_name} · from {warehouse?.name ?? `warehouse #${delivery.warehouse_id}`}
          </>
        }
        actions={
          delivery.status === "PENDING" &&
          canWork && (
            <Button loading={ship.isPending} onClick={() => ship.mutate()}>
              Mark as shipped
            </Button>
          )
        }
      />
      <div className="mb-4">
        <ErrorBanner error={ship.error} />
      </div>

      <form onSubmit={submit}>
        <Card
          title={canDeliver ? "Record proof of delivery" : "Lines"}
          actions={
            canDeliver && (
              <Button type="submit" loading={deliver.isPending}>
                Confirm delivery
              </Button>
            )
          }
        >
          {canDeliver && (
            <p className="border-b border-slate-200 px-4 py-2.5 text-sm text-slate-600">
              Lower a quantity for goods that were damaged or refused. Only delivered goods are invoiced; the rest returns to stock.
            </p>
          )}
          <Table head={["Product", "Ordered", "Delivered"]}>
            {delivery.lines.map((l) => (
              <tr key={l.line_no}>
                <Td>
                  <div className="font-medium text-slate-900">{l.product_name}</div>
                  <div className="font-mono text-xs text-slate-500">{l.sku}</div>
                </Td>
                <Td>
                  {l.ordered_qty} {l.uom}
                </Td>
                <Td>
                  {canDeliver ? (
                    <div className="flex items-center gap-2">
                      <Input
                        aria-label={`Delivered quantity for ${l.product_name}`}
                        type="number"
                        min={0}
                        max={l.ordered_qty}
                        className="w-24"
                        value={qty[l.line_no] ?? ""}
                        onChange={(e) => setQty({ ...qty, [l.line_no]: e.target.value })}
                        required
                      />
                      <span className="text-slate-500">{l.uom}</span>
                    </div>
                  ) : l.delivered_qty === null ? (
                    <span className="text-slate-400">—</span>
                  ) : (
                    <span className={l.delivered_qty < l.ordered_qty ? "font-medium text-amber-700" : ""}>
                      {l.delivered_qty} {l.uom}
                      {l.delivered_qty < l.ordered_qty && ` (${l.ordered_qty - l.delivered_qty} short)`}
                    </span>
                  )}
                </Td>
              </tr>
            ))}
          </Table>
          {deliver.error != null && (
            <div className="p-4">
              <ErrorBanner error={deliver.error} />
            </div>
          )}
        </Card>
      </form>

      <dl className="mt-6 grid gap-4 text-sm sm:grid-cols-3">
        {[
          ["Created", formatDate(delivery.created_at, true)],
          ["Shipped", formatDate(delivery.shipped_at, true)],
          ["Delivered", formatDate(delivery.delivered_at, true)],
        ].map(([k, v]) => (
          <div key={k} className="rounded-lg bg-white p-3 ring-1 ring-slate-200">
            <dt className="text-xs text-slate-500">{k}</dt>
            <dd className="mt-0.5 font-medium text-slate-900">{v}</dd>
          </div>
        ))}
      </dl>
    </>
  );
}
