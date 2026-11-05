import { useQueries } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { useRef, useState, type FormEvent } from "react";
import { useNavigate } from "react-router";

import { keys, useCreateOrder, useCustomers, useProducts, useRegions, useSalespersons } from "../../api/hooks";
import type { Price, Product } from "../../api/types";
import { Button, Card, ErrorBanner, Field, Input, Money, PageHeader, Select, Spinner, Td } from "../../components/ui";
import { api } from "../../lib/api";
import { byId } from "../../lib/format";
import { newIdempotencyKey } from "../../lib/idempotency";
import { formatIDR, lineTotal } from "../../lib/money";
import { useSession } from "../../lib/session";

type Line = { key: number; productId: string; uom: string; qty: string };

let nextKey = 1;
const emptyLine = (): Line => ({ key: nextKey++, productId: "", uom: "", qty: "1" });

export function NewOrderPage() {
  const navigate = useNavigate();
  const customers = useCustomers();
  const salespersons = useSalespersons();
  const products = useProducts();
  const regions = useRegions();
  // One key for this filled-in form: a double click or a resend after a
  // timeout returns the same order instead of creating a second one.
  const [idempotencyKey] = useState(newIdempotencyKey);
  const create = useCreateOrder(idempotencyKey);
  // Ignores a second submit while the first is in flight (a double click).
  // The Idempotency-Key still protects against resends that get past this.
  const submitting = useRef(false);

  // A sales user always sells as themselves, so only their region's customers apply.
  const me = useSession()!.user;
  const self = me.role === "sales" ? salespersons.data?.find((s) => s.id === me.salesperson_id) : undefined;

  const [customerId, setCustomerId] = useState("");
  const [salespersonId, setSalespersonId] = useState("");
  const [lines, setLines] = useState<Line[]>([emptyLine()]);

  const customerOptions = (customers.data ?? []).filter((c) => !self || c.region_id === self.region_id);
  const customer = customers.data?.find((c) => c.id === Number(customerId));
  const region = customer && byId(regions.data).get(customer.region_id);
  const productsById = byId(products.data);

  // A salesperson may only sell within their region, so only offer those.
  const eligible = (salespersons.data ?? []).filter((s) => customer && s.region_id === customer.region_id);

  // Prices for every product on the order, fetched once per product.
  const productIds = [...new Set(lines.map((l) => Number(l.productId)).filter(Boolean))];
  const priceQueries = useQueries({
    queries: productIds.map((id) => ({ queryKey: keys.prices(id), queryFn: () => api.get<Price[]>("master", `/products/${id}/prices`) })),
  });
  const pricesByProduct = new Map(productIds.map((id, i) => [id, priceQueries[i]?.data]));

  const priced = lines.map((l) => {
    const product = productsById.get(Number(l.productId));
    const price = customer && pricesByProduct.get(Number(l.productId))?.find((p) => p.region_id === customer.region_id && p.uom === l.uom)?.price;
    const qty = Number(l.qty);
    const factor = product?.uoms.find((u) => u.uom === l.uom)?.factor;
    return { line: l, product, price, factor, subtotal: price && qty > 0 ? lineTotal(price, qty) : undefined };
  });
  const total = priced.reduce((sum, p) => sum + (p.subtotal ?? 0n), 0n);
  const missingPrice = priced.some((p) => p.line.uom && p.price === undefined);

  const update = (key: number, patch: Partial<Line>) => setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (submitting.current) return;
    submitting.current = true;
    create.mutate(
      {
        customer_id: Number(customerId),
        // Sales users send none: the order service assigns it from their token.
        salesperson_id: me.role === "sales" ? undefined : Number(salespersonId),
        lines: lines.map((l) => ({ product_id: Number(l.productId), uom: l.uom, qty: Number(l.qty) })),
      },
      {
        onSuccess: (o) => navigate(`/orders/${o.id}`),
        onSettled: () => {
          submitting.current = false;
        },
      },
    );
  };

  if (customers.isPending || products.isPending || salespersons.isPending) return <Spinner />;

  return (
    <>
      <PageHeader back={{ to: "/orders", label: "Orders" }} title="New sales order" description="The order is saved as a draft; confirm it to reserve stock." />
      <form onSubmit={submit} className="space-y-6">
        <Card title="Customer">
          <div className="grid gap-4 p-4 sm:grid-cols-2">
            <Field
              label="Customer"
              htmlFor="customer"
              hint={customer && `${region?.name ?? "Region #" + customer.region_id} · ${customer.payment_term_days === 0 ? "cash on delivery" : `${customer.payment_term_days}-day term`}`}
            >
              <Select
                id="customer"
                value={customerId}
                onChange={(e) => {
                  setCustomerId(e.target.value);
                  setSalespersonId("");
                }}
                required
              >
                <option value="" disabled>
                  Select a customer…
                </option>
                {customerOptions.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.code} — {c.name}
                  </option>
                ))}
              </Select>
            </Field>
            {me.role === "sales" ? (
              <Field label="Salesperson" htmlFor="salesperson" hint="Orders you create are always yours. Only customers in your region are listed.">
                <Input id="salesperson" value={self?.name ?? me.name} disabled />
              </Field>
            ) : (
              <Field
                label="Salesperson"
                htmlFor="salesperson"
                hint={customer && eligible.length === 0 ? "No salesperson covers this customer's region yet." : "Only salespeople covering the customer's region."}
              >
                <Select id="salesperson" value={salespersonId} onChange={(e) => setSalespersonId(e.target.value)} disabled={!customer} required>
                  <option value="" disabled>
                    {customer ? "Select a salesperson…" : "Choose a customer first"}
                  </option>
                  {eligible.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </Select>
              </Field>
            )}
          </div>
        </Card>

        <Card
          title="Lines"
          actions={
            <Button variant="secondary" onClick={() => setLines((ls) => [...ls, emptyLine()])}>
              <Plus className="size-4" aria-hidden /> Add line
            </Button>
          }
        >
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead className="bg-slate-50 text-xs font-semibold tracking-wide text-slate-500 uppercase">
                <tr>
                  <th className="px-4 py-2.5 text-left">Product</th>
                  <th className="px-4 py-2.5 text-left">Unit</th>
                  <th className="px-4 py-2.5 text-left">Qty</th>
                  <th className="px-4 py-2.5 text-right">Unit price</th>
                  <th className="px-4 py-2.5 text-right">Subtotal</th>
                  <th />
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {priced.map(({ line, product, price, factor, subtotal }) => (
                  <tr key={line.key}>
                    <Td className="min-w-56">
                      <Select
                        aria-label="Product"
                        value={line.productId}
                        onChange={(e) => update(line.key, { productId: e.target.value, uom: "" })}
                        required
                      >
                        <option value="" disabled>
                          Select…
                        </option>
                        {(products.data ?? []).map((p: Product) => (
                          <option key={p.id} value={p.id}>
                            {p.sku} — {p.name}
                          </option>
                        ))}
                      </Select>
                    </Td>
                    <Td>
                      <Select aria-label="Unit" value={line.uom} onChange={(e) => update(line.key, { uom: e.target.value })} disabled={!product} required>
                        <option value="" disabled>
                          —
                        </option>
                        {product?.uoms.map((u) => (
                          <option key={u.uom} value={u.uom}>
                            {u.uom}
                          </option>
                        ))}
                      </Select>
                    </Td>
                    <Td>
                      <Input aria-label="Quantity" type="number" min={1} className="w-24" value={line.qty} onChange={(e) => update(line.key, { qty: e.target.value })} required />
                      {product && factor && factor > 1 && Number(line.qty) > 0 && (
                        <p className="mt-1 text-xs text-slate-500">
                          = {(Number(line.qty) * factor).toLocaleString("id-ID")} {product.base_uom}
                        </p>
                      )}
                    </Td>
                    <Td numeric>
                      {price ? <Money value={price} /> : line.uom && customer ? <span className="text-xs text-amber-700">no price in region</span> : "—"}
                    </Td>
                    <Td numeric className="font-medium">
                      {subtotal !== undefined ? formatIDR(subtotal) : "—"}
                    </Td>
                    <Td>
                      <Button
                        variant="ghost"
                        aria-label="Remove line"
                        disabled={lines.length === 1}
                        onClick={() => setLines((ls) => ls.filter((l) => l.key !== line.key))}
                      >
                        <Trash2 className="size-4" aria-hidden />
                      </Button>
                    </Td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="border-t border-slate-200">
                  <td colSpan={4} className="px-4 py-3 text-right text-sm font-medium text-slate-600">
                    Total
                  </td>
                  <td className="tabular px-4 py-3 text-right text-base font-semibold">{formatIDR(total)}</td>
                  <td />
                </tr>
              </tfoot>
            </table>
          </div>
        </Card>

        {missingPrice && (
          <p className="text-sm text-amber-800">
            Some lines have no price for this customer's region. Set one on the product page, or the order will be refused.
          </p>
        )}
        <ErrorBanner error={create.error} />
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={() => navigate("/orders")}>
            Cancel
          </Button>
          <Button type="submit" loading={create.isPending}>
            Save draft · {formatIDR(total)}
          </Button>
        </div>
      </form>
    </>
  );
}
