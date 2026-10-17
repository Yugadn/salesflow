import { Plus } from "lucide-react";
import { Link, useSearchParams } from "react-router";

import { useOrders } from "../../api/hooks";
import type { OrderStatus } from "../../api/types";
import { OrderStatusBadge } from "../../components/StatusBadge";
import { Card, EmptyState, Money, PageHeader, QueryState, Table, Td } from "../../components/ui";
import { num } from "../../components/head";
import { formatDate } from "../../lib/format";

const filters: { label: string; status?: OrderStatus }[] = [
  { label: "All" },
  { label: "Draft", status: "DRAFT" },
  { label: "Confirmed", status: "CONFIRMED" },
  { label: "Fulfilling", status: "FULFILLING" },
  { label: "Delivered", status: "DELIVERED" },
  { label: "Invoiced", status: "INVOICED" },
  { label: "Rejected", status: "REJECTED" },
];

export function OrdersPage() {
  const [params, setParams] = useSearchParams();
  const status = (params.get("status") ?? undefined) as OrderStatus | undefined;
  const orders = useOrders(status);

  return (
    <>
      <PageHeader
        title="Sales orders"
        actions={
          <Link
            to="/orders/new"
            className="inline-flex items-center gap-1.5 rounded-md bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700"
          >
            <Plus className="size-4" aria-hidden /> New order
          </Link>
        }
      />
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
        <QueryState query={orders}>
          <Table
            head={["Order", "Customer", "Salesperson", "Status", "Created", num("Total")]}
            empty={
              (orders.data ?? []).length === 0 && (
                <EmptyState title="No orders" hint={status ? "No orders with this status." : "Create the first sales order."} />
              )
            }
          >
            {(orders.data ?? []).map((o) => (
              <tr key={o.id} className="hover:bg-slate-50">
                <Td>
                  <Link to={`/orders/${o.id}`} className="font-mono text-xs font-medium text-indigo-700 hover:underline">
                    {o.number}
                  </Link>
                </Td>
                <Td>{o.customer_name}</Td>
                <Td>{o.salesperson_name}</Td>
                <Td>
                  <OrderStatusBadge status={o.status} />
                </Td>
                <Td>{formatDate(o.created_at, true)}</Td>
                <Td numeric>
                  <Money value={o.total} />
                </Td>
              </tr>
            ))}
          </Table>
        </QueryState>
      </Card>
    </>
  );
}
