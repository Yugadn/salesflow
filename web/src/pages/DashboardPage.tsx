import { ArrowRight } from "lucide-react";
import { Link } from "react-router";

import { useDeliveries, useInvoices, useOrders } from "../api/hooks";
import type { OrderStatus } from "../api/types";
import { OrderStatusBadge } from "../components/StatusBadge";
import { Card, EmptyState, Money, PageHeader, QueryState, Table, Td } from "../components/ui";
import { num } from "../components/head";
import { formatDate } from "../lib/format";
import { formatIDR, toCents } from "../lib/money";

const pipeline: { status: OrderStatus; label: string; hint: string }[] = [
  { status: "DRAFT", label: "Drafts", hint: "awaiting confirmation" },
  { status: "FULFILLING", label: "Fulfilling", hint: "stock reserved, not yet delivered" },
  { status: "DELIVERED", label: "Delivered", hint: "awaiting invoice" },
  { status: "INVOICED", label: "Invoiced", hint: "order-to-cash complete" },
  { status: "REJECTED", label: "Rejected", hint: "not enough stock" },
];

export function DashboardPage() {
  const orders = useOrders();
  const toShip = useDeliveries({ status: "PENDING" });
  const inTransit = useDeliveries({ status: "SHIPPED" });
  const invoices = useInvoices();

  const count = (s: OrderStatus) => (orders.data ?? []).filter((o) => o.status === s).length;
  const billed = (invoices.data ?? []).reduce((sum, i) => sum + toCents(i.total), 0n);

  return (
    <>
      <PageHeader title="Dashboard" description="Most recent 200 orders, deliveries and invoices." />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
        {pipeline.map((p) => (
          <Link
            key={p.status}
            to={`/orders?status=${p.status}`}
            className="rounded-lg bg-white p-4 ring-1 ring-slate-200 transition-shadow hover:shadow-sm hover:ring-slate-300"
          >
            <p className="text-sm text-slate-500">{p.label}</p>
            <p className="tabular mt-1 text-2xl font-semibold text-slate-900">{orders.isPending ? "…" : count(p.status)}</p>
            <p className="mt-1 text-xs text-slate-400">{p.hint}</p>
          </Link>
        ))}
      </div>

      <div className="mt-4 grid gap-4 sm:grid-cols-3">
        <Stat label="To ship" value={toShip.data?.length} to="/deliveries?status=PENDING" />
        <Stat label="In transit" value={inTransit.data?.length} to="/deliveries?status=SHIPPED" />
        <Stat label="Billed" value={invoices.data ? formatIDR(billed) : undefined} to="/invoices" />
      </div>

      <div className="mt-6 grid items-start gap-6 lg:grid-cols-[3fr_2fr]">
        <Card
          title="Recent orders"
          actions={
            <Link to="/orders" className="text-sm text-indigo-700 hover:underline">
              All orders
            </Link>
          }
        >
          <QueryState query={orders}>
            <Table
              head={["Order", "Customer", "Status", num("Total")]}
              empty={(orders.data ?? []).length === 0 && <EmptyState title="No orders yet" hint="Set up master data, then create an order." />}
            >
              {(orders.data ?? []).slice(0, 8).map((o) => (
                <tr key={o.id} className="hover:bg-slate-50">
                  <Td>
                    <Link to={`/orders/${o.id}`} className="font-mono text-xs font-medium text-indigo-700 hover:underline">
                      {o.number}
                    </Link>
                    <div className="text-xs text-slate-400">{formatDate(o.created_at)}</div>
                  </Td>
                  <Td>{o.customer_name}</Td>
                  <Td>
                    <OrderStatusBadge status={o.status} />
                  </Td>
                  <Td numeric>
                    <Money value={o.total} />
                  </Td>
                </tr>
              ))}
            </Table>
          </QueryState>
        </Card>

        <Card title="How an order flows">
          <ol className="space-y-3 p-4 text-sm text-slate-600">
            {[
              ["Order", "A salesperson drafts an order. Prices come from the customer's region; cartons are converted to pieces."],
              ["Confirm", "Confirming publishes order.confirmed. The region's warehouse reserves every line, or rejects the order."],
              ["Deliver", "The warehouse ships and records what arrived. Damaged or refused goods return to stock."],
              ["Invoice", "Billing invoices only the delivered quantities, due after the customer's payment term."],
            ].map(([title, text], i) => (
              <li key={title} className="flex gap-3">
                <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-indigo-50 text-xs font-semibold text-indigo-700">
                  {i + 1}
                </span>
                <span>
                  <strong className="text-slate-900">{title}.</strong> {text}
                </span>
              </li>
            ))}
          </ol>
          <div className="border-t border-slate-200 px-4 py-3">
            <Link to="/orders/new" className="inline-flex items-center gap-1 text-sm font-medium text-indigo-700 hover:underline">
              Create an order <ArrowRight className="size-4" aria-hidden />
            </Link>
          </div>
        </Card>
      </div>
    </>
  );
}

function Stat({ label, value, to }: { label: string; value: number | string | undefined; to: string }) {
  return (
    <Link to={to} className="flex items-center justify-between rounded-lg bg-white p-4 ring-1 ring-slate-200 hover:ring-slate-300">
      <span className="text-sm text-slate-500">{label}</span>
      <span className="tabular text-lg font-semibold text-slate-900">{value ?? "…"}</span>
    </Link>
  );
}
