import { useQueryClient } from "@tanstack/react-query";
import { Check, CircleAlert, Loader2 } from "lucide-react";
import { useEffect } from "react";
import { Link, useParams } from "react-router";

import { awaitingEvent, keys, useDeliveries, useInvoices, useOrder, useOrderAction, useRegions } from "../../api/hooks";
import type { Order, OrderStatus } from "../../api/types";
import { DeliveryStatusBadge, OrderStatusBadge } from "../../components/StatusBadge";
import { Button, Card, ErrorBanner, Money, PageHeader, QueryState, Table, Td } from "../../components/ui";
import { num } from "../../components/head";
import { formatDate } from "../../lib/format";
import { can } from "../../lib/permissions";
import { useSession } from "../../lib/session";

const steps: { status: OrderStatus; label: string; waiting?: string }[] = [
  { status: "DRAFT", label: "Draft" },
  { status: "CONFIRMED", label: "Confirmed", waiting: "Warehouse is reserving stock…" },
  { status: "FULFILLING", label: "Stock reserved" },
  { status: "DELIVERED", label: "Delivered", waiting: "Billing is issuing the invoice…" },
  { status: "INVOICED", label: "Invoiced" },
];

export function OrderDetailPage() {
  const id = Number(useParams().id);
  const order = useOrder(id);
  const qc = useQueryClient();
  const status = order.data?.status;

  // When an event moves the order on, refresh its delivery and invoice too.
  useEffect(() => {
    if (!status) return;
    qc.invalidateQueries({ queryKey: keys.deliveries });
    qc.invalidateQueries({ queryKey: keys.invoices });
  }, [status, qc]);

  return (
    <QueryState query={order}>
      {order.data && <OrderView order={order.data} />}
    </QueryState>
  );
}

function OrderView({ order }: { order: Order }) {
  const confirm = useOrderAction(order.id, "confirm");
  const cancel = useOrderAction(order.id, "cancel");
  const regions = useRegions();
  const user = useSession()!.user;
  const deliveries = useDeliveries({ order_id: order.id });
  // Invoices are for admins; others see the invoice number the order carries.
  const invoices = useInvoices({ order_id: order.id }, can.seeInvoices(user));
  const delivery = deliveries.data?.[0];
  const invoice = invoices.data?.[0];
  const region = regions.data?.find((r) => r.id === order.region_id);

  return (
    <>
      <PageHeader
        back={{ to: "/orders", label: "Orders" }}
        title={
          <span className="flex items-center gap-3">
            <span className="font-mono">{order.number}</span> <OrderStatusBadge status={order.status} />
          </span>
        }
        description={`${order.customer_name} · ${order.salesperson_name}`}
        actions={
          order.status === "DRAFT" &&
          can.sell(user) && (
            <>
              <Button variant="danger" loading={cancel.isPending} onClick={() => cancel.mutate()}>
                Cancel order
              </Button>
              <Button loading={confirm.isPending} onClick={() => confirm.mutate()}>
                Confirm order
              </Button>
            </>
          )
        }
      />
      <div className="mb-4 space-y-2">
        <ErrorBanner error={confirm.error ?? cancel.error} />
      </div>

      <Timeline order={order} />

      <div className="mt-6 grid items-start gap-6 lg:grid-cols-[2fr_1fr]">
        <Card title="Lines">
          <Table head={["Product", "Qty", num("Base qty"), num("Unit price"), num("Subtotal")]}>
            {order.lines.map((l) => (
              <tr key={l.line_no}>
                <Td>
                  <div className="font-medium text-slate-900">{l.product_name}</div>
                  <div className="font-mono text-xs text-slate-500">{l.sku}</div>
                </Td>
                <Td>
                  {l.qty} {l.uom}
                </Td>
                <Td numeric className="text-slate-500">
                  {l.base_qty.toLocaleString("id-ID")}
                </Td>
                <Td numeric>
                  <Money value={l.unit_price} />
                </Td>
                <Td numeric className="font-medium">
                  <Money value={l.subtotal} />
                </Td>
              </tr>
            ))}
            <tr className="bg-slate-50">
              <Td className="font-medium">Total</Td>
              <Td>{""}</Td>
              <Td>{""}</Td>
              <Td>{""}</Td>
              <Td numeric className="text-base font-semibold">
                <Money value={order.total} />
              </Td>
            </tr>
          </Table>
        </Card>

        <div className="space-y-6">
          <Card title="Details">
            <dl className="divide-y divide-slate-100 text-sm">
              {[
                ["Customer", `${order.customer_code} — ${order.customer_name}`],
                ["Region", region?.name ?? `#${order.region_id}`],
                ["Payment term", order.payment_term_days === 0 ? "Cash on delivery" : `${order.payment_term_days} days`],
                ["Created", formatDate(order.created_at, true)],
                ["Confirmed", formatDate(order.confirmed_at, true)],
              ].map(([k, v]) => (
                <div key={k} className="flex justify-between gap-4 px-4 py-2">
                  <dt className="text-slate-500">{k}</dt>
                  <dd className="text-right text-slate-900">{v}</dd>
                </div>
              ))}
            </dl>
          </Card>
          <Card title="Fulfilment">
            <div className="space-y-3 p-4 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-slate-500">Delivery</span>
                {delivery ? (
                  <Link to={`/deliveries/${delivery.id}`} className="flex items-center gap-2 font-mono text-xs text-indigo-700 hover:underline">
                    {delivery.number} <DeliveryStatusBadge status={delivery.status} />
                  </Link>
                ) : (
                  <span className="text-slate-400">—</span>
                )}
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-500">Invoice</span>
                {invoice ? (
                  <Link to={`/invoices/${invoice.id}`} className="font-mono text-xs text-indigo-700 hover:underline">
                    {invoice.number} · <Money value={invoice.total} />
                  </Link>
                ) : order.invoice_number ? (
                  <span className="font-mono text-xs text-slate-700">{order.invoice_number}</span>
                ) : (
                  <span className="text-slate-400">—</span>
                )}
              </div>
              {invoice && invoice.total !== order.total && (
                <p className="text-xs text-slate-500">The invoice covers only what was actually delivered.</p>
              )}
            </div>
          </Card>
        </div>
      </div>
    </>
  );
}

function Timeline({ order }: { order: Order }) {
  if (order.status === "REJECTED" || order.status === "CANCELLED") {
    return (
      <div className="flex items-start gap-2 rounded-lg bg-red-50 p-4 text-sm text-red-800 ring-1 ring-red-200">
        <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
        <div>
          <p className="font-medium">{order.status === "REJECTED" ? "Rejected by the warehouse" : "Cancelled"}</p>
          {order.reject_reason && <p className="mt-0.5">{order.reject_reason}. Nothing was reserved.</p>}
        </div>
      </div>
    );
  }

  const current = steps.findIndex((s) => s.status === order.status);
  return (
    <Card>
      <ol className="flex flex-wrap items-center gap-x-2 gap-y-3 p-4">
        {steps.map((step, i) => {
          const done = i < current || (i === current && order.status === "INVOICED");
          const active = i === current && order.status !== "INVOICED";
          return (
            <li key={step.status} className="flex items-center gap-2">
              <span
                className={
                  "flex size-6 items-center justify-center rounded-full text-xs font-semibold " +
                  (done ? "bg-emerald-600 text-white" : active ? "bg-indigo-600 text-white" : "bg-slate-200 text-slate-500")
                }
              >
                {done ? <Check className="size-3.5" aria-hidden /> : i + 1}
              </span>
              <span className={"text-sm " + (active ? "font-semibold text-slate-900" : done ? "text-slate-700" : "text-slate-400")}>
                {step.label}
              </span>
              {i < steps.length - 1 && <span className="mx-1 hidden h-px w-8 bg-slate-300 sm:block" aria-hidden />}
            </li>
          );
        })}
      </ol>
      {awaitingEvent(order.status) && (
        <p className="flex items-center gap-2 border-t border-slate-200 px-4 py-2.5 text-sm text-slate-600" role="status">
          <Loader2 className="size-4 animate-spin" aria-hidden />
          {steps[current]?.waiting} This page updates by itself.
        </p>
      )}
      {order.status === "FULFILLING" && (
        <p className="border-t border-slate-200 px-4 py-2.5 text-sm text-slate-600">
          Stock is reserved. Ship and deliver it from the delivery order; this page follows along.
        </p>
      )}
    </Card>
  );
}
