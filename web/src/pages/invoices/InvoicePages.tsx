import { Link, useParams } from "react-router";

import { useInvoice, useInvoices } from "../../api/hooks";
import { Card, EmptyState, Money, PageHeader, QueryState, Table, Td } from "../../components/ui";
import { num } from "../../components/head";
import { formatDate } from "../../lib/format";

export function InvoicesPage() {
  const invoices = useInvoices();
  return (
    <>
      <PageHeader title="Invoices" description="Issued automatically when a delivery is completed, for the quantities actually delivered." />
      <Card>
        <QueryState query={invoices}>
          <Table
            head={["Invoice", "Customer", "Order", "Issued", "Due", num("Total")]}
            empty={(invoices.data ?? []).length === 0 && <EmptyState title="No invoices yet" hint="Complete a delivery to issue one." />}
          >
            {(invoices.data ?? []).map((inv) => (
              <tr key={inv.id} className="hover:bg-slate-50">
                <Td>
                  <Link to={`/invoices/${inv.id}`} className="font-mono text-xs font-medium text-indigo-700 hover:underline">
                    {inv.number}
                  </Link>
                </Td>
                <Td>{inv.customer_name}</Td>
                <Td>
                  <Link to={`/orders/${inv.order_id}`} className="font-mono text-xs text-slate-600 hover:underline">
                    {inv.order_number}
                  </Link>
                </Td>
                <Td>{formatDate(inv.issue_date)}</Td>
                <Td>{formatDate(inv.due_date)}</Td>
                <Td numeric className="font-medium">
                  <Money value={inv.total} />
                </Td>
              </tr>
            ))}
          </Table>
        </QueryState>
      </Card>
    </>
  );
}

export function InvoiceDetailPage() {
  const id = Number(useParams().id);
  const invoice = useInvoice(id);
  const inv = invoice.data;

  return (
    <QueryState query={invoice}>
      {inv && (
        <>
          <PageHeader back={{ to: "/invoices", label: "Invoices" }} title={<span className="font-mono">{inv.number}</span>} />
          <Card>
            <div className="grid gap-6 border-b border-slate-200 p-6 sm:grid-cols-2">
              <div>
                <p className="text-xs font-semibold tracking-wide text-slate-500 uppercase">Bill to</p>
                <p className="mt-1 text-base font-semibold text-slate-900">{inv.customer_name}</p>
              </div>
              <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                <dt className="text-slate-500">Issued</dt>
                <dd className="text-right">{formatDate(inv.issue_date)}</dd>
                <dt className="text-slate-500">Due</dt>
                <dd className="text-right font-medium">{formatDate(inv.due_date)}</dd>
                <dt className="text-slate-500">Order</dt>
                <dd className="text-right">
                  <Link to={`/orders/${inv.order_id}`} className="font-mono text-indigo-700 hover:underline">
                    {inv.order_number}
                  </Link>
                </dd>
                <dt className="text-slate-500">Delivery</dt>
                <dd className="text-right">
                  <Link to={`/deliveries/${inv.delivery_id}`} className="font-mono text-indigo-700 hover:underline">
                    {inv.delivery_number}
                  </Link>
                </dd>
              </dl>
            </div>
            <Table head={["Product", "Qty delivered", num("Unit price"), num("Amount")]}>
              {inv.lines.map((l) => (
                <tr key={l.line_no}>
                  <Td>
                    <div className="font-medium text-slate-900">{l.product_name}</div>
                    <div className="font-mono text-xs text-slate-500">{l.sku}</div>
                  </Td>
                  <Td>
                    {l.qty} {l.uom}
                  </Td>
                  <Td numeric>
                    <Money value={l.unit_price} />
                  </Td>
                  <Td numeric className="font-medium">
                    <Money value={l.amount} />
                  </Td>
                </tr>
              ))}
              <tr className="bg-slate-50">
                <Td className="font-semibold">Total</Td>
                <Td>{""}</Td>
                <Td>{""}</Td>
                <Td numeric className="text-base font-semibold">
                  <Money value={inv.total} />
                </Td>
              </tr>
            </Table>
          </Card>
          {inv.lines.length === 0 && <EmptyState title="No lines" />}
        </>
      )}
    </QueryState>
  );
}
