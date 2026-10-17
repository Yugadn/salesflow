// React Query hooks, one per resource. Query keys are centralised so
// mutations invalidate exactly what they change.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../lib/api";
import type {
  CreateOrderInput,
  Customer,
  Delivery,
  DeliveryStatus,
  Invoice,
  Order,
  OrderStatus,
  Price,
  Product,
  ProductUoM,
  Region,
  Salesperson,
  Stock,
  UoM,
  Warehouse,
} from "./types";

export const keys = {
  regions: ["regions"] as const,
  customers: ["customers"] as const,
  salespersons: ["salespersons"] as const,
  uoms: ["uoms"] as const,
  products: ["products"] as const,
  product: (id: number) => ["products", id] as const,
  prices: (productId: number) => ["products", productId, "prices"] as const,
  warehouses: ["warehouses"] as const,
  stocks: (warehouseId: number) => ["warehouses", warehouseId, "stocks"] as const,
  orders: ["orders"] as const,
  order: (id: number) => ["orders", id] as const,
  deliveries: ["deliveries"] as const,
  delivery: (id: number) => ["deliveries", id] as const,
  invoices: ["invoices"] as const,
  invoice: (id: number) => ["invoices", id] as const,
};

const qs = (params: Record<string, string | number | undefined>) => {
  const entries = Object.entries(params).filter(([, v]) => v !== undefined && v !== "");
  return entries.length ? "?" + new URLSearchParams(entries.map(([k, v]) => [k, String(v)])).toString() : "";
};

// ---------------------------------------------------------------- master

export const useRegions = () => useQuery({ queryKey: keys.regions, queryFn: () => api.get<Region[]>("master", "/regions") });
export const useCustomers = () => useQuery({ queryKey: keys.customers, queryFn: () => api.get<Customer[]>("master", "/customers") });
export const useSalespersons = () =>
  useQuery({ queryKey: keys.salespersons, queryFn: () => api.get<Salesperson[]>("master", "/salespersons") });
export const useUoMs = () => useQuery({ queryKey: keys.uoms, queryFn: () => api.get<UoM[]>("master", "/uoms") });
export const useProducts = () => useQuery({ queryKey: keys.products, queryFn: () => api.get<Product[]>("master", "/products") });
export const useProduct = (id: number) =>
  useQuery({ queryKey: keys.product(id), queryFn: () => api.get<Product>("master", `/products/${id}`) });
export const usePrices = (productId: number | undefined) =>
  useQuery({
    queryKey: keys.prices(productId ?? 0),
    queryFn: () => api.get<Price[]>("master", `/products/${productId}/prices`),
    enabled: !!productId,
  });

/** Create-or-update for the simple master entities, keyed by their list. */
function useSave<T extends { id?: number }>(path: string, listKey: readonly unknown[]) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: T) => (id ? api.put<T>("master", `${path}/${id}`, body) : api.post<T>("master", path, body)),
    onSuccess: () => qc.invalidateQueries({ queryKey: listKey }),
  });
}

export const useSaveRegion = () => useSave<Partial<Region>>("/regions", keys.regions);
export const useSaveCustomer = () => useSave<Partial<Customer>>("/customers", keys.customers);
export const useSaveSalesperson = () => useSave<Partial<Salesperson>>("/salespersons", keys.salespersons);

export const useCreateUoM = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (u: UoM) => api.post<UoM>("master", "/uoms", u),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.uoms }),
  });
};

export const useSaveProduct = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: Partial<Product>) =>
      id ? api.put<Product>("master", `/products/${id}`, body) : api.post<Product>("master", "/products", body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.products }),
  });
};

export const useSetConversion = (productId: number) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (c: ProductUoM) => api.put<Product>("master", `/products/${productId}/uoms`, c),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.products }),
  });
};

export const useSetPrice = (productId: number) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: Omit<Price, "product_id">) => api.put<Price>("master", `/products/${productId}/prices`, p),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.prices(productId) }),
  });
};

// ---------------------------------------------------------------- warehouse

export const useWarehouses = () =>
  useQuery({ queryKey: keys.warehouses, queryFn: () => api.get<Warehouse[]>("warehouse", "/warehouses") });

export const useSaveWarehouse = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: Partial<Warehouse>) =>
      id ? api.put<Warehouse>("warehouse", `/warehouses/${id}`, body) : api.post<Warehouse>("warehouse", "/warehouses", body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.warehouses }),
  });
};

export const useStocks = (warehouseId: number) =>
  useQuery({ queryKey: keys.stocks(warehouseId), queryFn: () => api.get<Stock[]>("warehouse", `/warehouses/${warehouseId}/stocks`) });

export const useReceiveStock = (warehouseId: number) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (r: { product_id: number; qty: number }) => api.post<Stock>("warehouse", `/warehouses/${warehouseId}/receipts`, r),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.stocks(warehouseId) }),
  });
};

export const useDeliveries = (filter: { status?: DeliveryStatus; order_id?: number } = {}, poll = false) =>
  useQuery({
    queryKey: [...keys.deliveries, filter],
    queryFn: () => api.get<Delivery[]>("warehouse", "/deliveries" + qs({ limit: 200, ...filter })),
    refetchInterval: poll ? 1500 : false,
  });

export const useDelivery = (id: number) =>
  useQuery({ queryKey: keys.delivery(id), queryFn: () => api.get<Delivery>("warehouse", `/deliveries/${id}`) });

export const useShip = (id: number) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<Delivery>("warehouse", `/deliveries/${id}/ship`),
    onSuccess: (d) => {
      qc.setQueryData(keys.delivery(id), d);
      qc.invalidateQueries({ queryKey: keys.deliveries });
    },
  });
};

export const useDeliver = (id: number) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (lines: { line_no: number; delivered_qty: number }[]) =>
      api.post<Delivery>("warehouse", `/deliveries/${id}/deliver`, { lines }),
    onSuccess: (d) => {
      qc.setQueryData(keys.delivery(id), d);
      qc.invalidateQueries({ queryKey: keys.deliveries });
      qc.invalidateQueries({ queryKey: ["warehouses"] });
      qc.invalidateQueries({ queryKey: keys.orders });
    },
  });
};

// ---------------------------------------------------------------- order

/** Statuses another service is about to change within moments, via an event. */
export const awaitingEvent = (s: OrderStatus) => s === "CONFIRMED" || s === "DELIVERED";

/**
 * How often to re-read an order. Every non-final status can change without
 * this page doing anything: an event is due any moment (fast poll), or the
 * warehouse may ship and deliver from another screen (slow poll).
 */
export function orderPollInterval(s: OrderStatus): number | false {
  if (awaitingEvent(s)) return 1000;
  if (s === "FULFILLING") return 3000;
  return false;
}

export const useOrders = (status?: OrderStatus) =>
  useQuery({ queryKey: [...keys.orders, status ?? "all"], queryFn: () => api.get<Order[]>("order", "/orders" + qs({ status, limit: 200 })) });

export const useOrder = (id: number) =>
  useQuery({
    queryKey: keys.order(id),
    queryFn: () => api.get<Order>("order", `/orders/${id}`),
    refetchInterval: (q) => (q.state.data ? orderPollInterval(q.state.data.status) : false),
  });

export const useCreateOrder = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateOrderInput) => api.post<Order>("order", "/orders", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.orders }),
  });
};

export const useOrderAction = (id: number, action: "confirm" | "cancel") => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<Order>("order", `/orders/${id}/${action}`),
    onSuccess: (o) => {
      qc.setQueryData(keys.order(id), o);
      qc.invalidateQueries({ queryKey: keys.orders });
    },
  });
};

// ---------------------------------------------------------------- billing

export const useInvoices = (filter: { customer_id?: number; order_id?: number } = {}) =>
  useQuery({
    queryKey: [...keys.invoices, filter],
    queryFn: () => api.get<Invoice[]>("billing", "/invoices" + qs({ limit: 200, ...filter })),
  });

export const useInvoice = (id: number) =>
  useQuery({ queryKey: keys.invoice(id), queryFn: () => api.get<Invoice>("billing", `/invoices/${id}`) });
