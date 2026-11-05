import { useState, type FormEvent } from "react";
import { useParams } from "react-router";

import { usePrices, useProduct, useRegions, useSetConversion, useSetPrice, useUoMs } from "../../api/hooks";
import type { Product } from "../../api/types";
import { Button, Card, EmptyState, ErrorBanner, Field, Input, Money, PageHeader, QueryState, Select, Table, Td } from "../../components/ui";
import { num } from "../../components/head";
import { isValidPrice } from "../../lib/money";
import { can } from "../../lib/permissions";
import { useSession } from "../../lib/session";

export function ProductDetailPage() {
  const id = Number(useParams().id);
  const product = useProduct(id);

  return (
    <QueryState query={product}>
      {product.data && (
        <>
          <PageHeader
            back={{ to: "/products", label: "Products" }}
            title={product.data.name}
            description={
              <>
                <span className="font-mono">{product.data.sku}</span> · stock counted in <strong>{product.data.base_uom}</strong>
              </>
            }
          />
          <div className="grid items-start gap-6 lg:grid-cols-[2fr_3fr]">
            <Conversions product={product.data} />
            <PriceMatrix product={product.data} />
          </div>
        </>
      )}
    </QueryState>
  );
}

function Conversions({ product }: { product: Product }) {
  const canEdit = can.editMasterData(useSession()?.user);
  const uoms = useUoMs();
  const save = useSetConversion(product.id);
  const [uom, setUom] = useState("");
  const [factor, setFactor] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate({ uom, factor: Number(factor) }, { onSuccess: () => (setUom(""), setFactor("")) });
  };
  const candidates = (uoms.data ?? []).filter((u) => u.code !== product.base_uom);

  return (
    <Card title="Units">
      <Table head={["Unit", "Contains"]}>
        {product.uoms.map((u) => (
          <tr key={u.uom}>
            <Td className="font-medium">{u.uom}</Td>
            <Td>{u.uom === product.base_uom ? <span className="text-slate-500">base unit</span> : `${u.factor} ${product.base_uom}`}</Td>
          </tr>
        ))}
      </Table>
      {canEdit && (
      <form onSubmit={submit} className="space-y-3 border-t border-slate-200 p-4">
        <p className="text-sm font-medium text-slate-700">Add or change a unit</p>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Unit" htmlFor="conv-uom">
            <Select id="conv-uom" value={uom} onChange={(e) => setUom(e.target.value)} required>
              <option value="" disabled>
                Select…
              </option>
              {candidates.map((u) => (
                <option key={u.code} value={u.code}>
                  {u.code} — {u.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={`= how many ${product.base_uom}`} htmlFor="conv-factor">
            <Input id="conv-factor" type="number" min={2} value={factor} onChange={(e) => setFactor(e.target.value)} required />
          </Field>
        </div>
        <ErrorBanner error={save.error} />
        <Button type="submit" loading={save.isPending}>
          Save unit
        </Button>
      </form>
      )}
    </Card>
  );
}

function PriceMatrix({ product }: { product: Product }) {
  const canEdit = can.editMasterData(useSession()?.user);
  const regions = useRegions();
  const prices = usePrices(product.id);
  const save = useSetPrice(product.id);
  const [form, setForm] = useState({ region_id: "", uom: "", price: "" });

  const priceOf = (regionId: number, uom: string) => prices.data?.find((p) => p.region_id === regionId && p.uom === uom)?.price;

  const pick = (regionId: number, uom: string) => {
    save.reset();
    setForm({ region_id: String(regionId), uom, price: priceOf(regionId, uom) ?? "" });
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate({ region_id: Number(form.region_id), uom: form.uom, price: form.price.trim() });
  };
  const priceInvalid = form.price !== "" && !isValidPrice(form.price);

  return (
    <Card title="Prices by region">
      <QueryState query={{ isPending: regions.isPending || prices.isPending, error: regions.error ?? prices.error }}>
        {(regions.data ?? []).length === 0 ? (
          <EmptyState title="No regions yet" hint="Create a region first; prices are set per region." />
        ) : (
          <Table head={["Region", ...product.uoms.map((u) => num(`per ${u.uom}`))]}>
            {(regions.data ?? []).map((r) => (
              <tr key={r.id}>
                <Td className="font-medium">{r.name}</Td>
                {product.uoms.map((u) => {
                  const price = priceOf(r.id, u.uom);
                  const selected = form.region_id === String(r.id) && form.uom === u.uom;
                  return (
                    <Td key={u.uom} numeric>
                      <button
                        type="button"
                        disabled={!canEdit}
                        onClick={() => pick(r.id, u.uom)}
                        className={
                          "rounded px-1.5 py-0.5 enabled:hover:bg-indigo-50 " + (selected ? "bg-indigo-100 ring-1 ring-indigo-300" : "")
                        }
                        aria-label={`Set price for ${r.name} per ${u.uom}`}
                      >
                        {price ? <Money value={price} /> : <span className="text-slate-400">not set</span>}
                      </button>
                    </Td>
                  );
                })}
              </tr>
            ))}
          </Table>
        )}
      </QueryState>
      {canEdit && (
      <form onSubmit={submit} className="space-y-3 border-t border-slate-200 p-4">
        <p className="text-sm font-medium text-slate-700">Set a price</p>
        <p className="text-xs text-slate-500">Click a cell above, or choose below. Orders already placed keep their original price.</p>
        <div className="grid grid-cols-3 gap-3">
          <Field label="Region" htmlFor="price-region">
            <Select id="price-region" value={form.region_id} onChange={(e) => setForm({ ...form, region_id: e.target.value })} required>
              <option value="" disabled>
                Select…
              </option>
              {(regions.data ?? []).map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Per unit" htmlFor="price-uom">
            <Select id="price-uom" value={form.uom} onChange={(e) => setForm({ ...form, uom: e.target.value })} required>
              <option value="" disabled>
                Select…
              </option>
              {product.uoms.map((u) => (
                <option key={u.uom} value={u.uom}>
                  {u.uom}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Price (Rp)" htmlFor="price-value" hint={priceInvalid ? "Use digits, at most 2 decimals" : undefined}>
            <Input
              id="price-value"
              inputMode="decimal"
              placeholder="96000"
              value={form.price}
              onChange={(e) => setForm({ ...form, price: e.target.value })}
              aria-invalid={priceInvalid}
              required
            />
          </Field>
        </div>
        <ErrorBanner error={save.error} />
        <Button type="submit" loading={save.isPending} disabled={priceInvalid}>
          Save price
        </Button>
      </form>
      )}
    </Card>
  );
}
