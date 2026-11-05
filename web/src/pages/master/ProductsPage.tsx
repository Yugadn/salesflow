import { ChevronRight } from "lucide-react";
import { Link } from "react-router";

import { useProducts, useSaveProduct, useUoMs } from "../../api/hooks";
import type { Product } from "../../api/types";
import { CrudPage } from "../../components/CrudPage";
import { can } from "../../lib/permissions";
import { useSession } from "../../lib/session";

export function ProductsPage() {
  const uoms = useUoMs();
  const canEdit = can.editMasterData(useSession()?.user);
  return (
    <CrudPage<Product>
      title="Products"
      description="Stock is counted in each product's base unit. Open a product to set its other units and regional prices."
      noun="product"
      canCreate={canEdit}
      editable={canEdit}
      query={useProducts()}
      save={useSaveProduct()}
      rowKey={(p) => p.id}
      columns={[
        { header: "SKU", cell: (p) => <span className="font-mono text-xs">{p.sku}</span> },
        {
          header: "Name",
          cell: (p) => (
            <Link to={`/products/${p.id}`} className="font-medium text-indigo-700 hover:underline">
              {p.name}
            </Link>
          ),
        },
        { header: "Base unit", cell: (p) => p.base_uom },
        {
          header: "Sold in",
          cell: (p) =>
            p.uoms.map((u) => (u.factor === 1 ? u.uom : `${u.uom} (${u.factor} ${p.base_uom})`)).join(" · "),
        },
      ]}
      fields={[
        { name: "sku", label: "SKU" },
        { name: "name", label: "Name" },
        {
          name: "base_uom",
          label: "Base unit",
          type: "select",
          options: (uoms.data ?? []).map((u) => ({ value: u.code, label: `${u.code} — ${u.name}` })),
          hint: "The smallest unit you count stock in. It cannot change later.",
          createOnly: true,
        },
      ]}
      rowActions={(p) => (
        <Link
          to={`/products/${p.id}`}
          className="inline-flex items-center gap-0.5 rounded-md px-2 py-1.5 text-sm font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900"
        >
          Units & prices <ChevronRight className="size-4" aria-hidden />
        </Link>
      )}
    />
  );
}
