// Mirrors the JSON the services return. Money is a decimal string.

export type Money = string;

// ---- master
export interface Region {
  id: number;
  code: string;
  name: string;
}

export interface Customer {
  id: number;
  code: string;
  name: string;
  region_id: number;
  payment_term_days: number;
}

export interface Salesperson {
  id: number;
  code: string;
  name: string;
  region_id: number;
}

export interface UoM {
  code: string;
  name: string;
}

export interface ProductUoM {
  uom: string;
  factor: number;
}

export interface Product {
  id: number;
  sku: string;
  name: string;
  base_uom: string;
  uoms: ProductUoM[];
}

export interface Price {
  product_id: number;
  region_id: number;
  uom: string;
  price: Money;
}

// ---- order
export type OrderStatus = "DRAFT" | "CONFIRMED" | "FULFILLING" | "DELIVERED" | "INVOICED" | "REJECTED" | "CANCELLED";

export interface OrderLine {
  line_no: number;
  product_id: number;
  sku: string;
  product_name: string;
  uom: string;
  conversion_factor: number;
  qty: number;
  base_qty: number;
  unit_price: Money;
  subtotal: Money;
}

export interface Order {
  id: number;
  number: string;
  status: OrderStatus;
  customer_id: number;
  customer_code: string;
  customer_name: string;
  region_id: number;
  payment_term_days: number;
  salesperson_id: number;
  salesperson_name: string;
  total: Money;
  reject_reason?: string;
  invoice_number?: string;
  lines: OrderLine[];
  created_at: string;
  updated_at: string;
  confirmed_at?: string;
}

export interface CreateOrderInput {
  customer_id: number;
  salesperson_id: number;
  lines: { product_id: number; uom: string; qty: number }[];
}

// ---- warehouse
export interface Warehouse {
  id: number;
  code: string;
  name: string;
  region_id: number;
}

export interface Stock {
  warehouse_id: number;
  product_id: number;
  on_hand: number;
  reserved: number;
}

export type DeliveryStatus = "PENDING" | "SHIPPED" | "DELIVERED";

export interface DeliveryLine {
  line_no: number;
  product_id: number;
  sku: string;
  product_name: string;
  uom: string;
  conversion_factor: number;
  ordered_qty: number;
  delivered_qty: number | null;
  unit_price: Money;
}

export interface Delivery {
  id: number;
  number: string;
  order_id: number;
  order_number: string;
  warehouse_id: number;
  customer_id: number;
  customer_name: string;
  payment_term_days: number;
  status: DeliveryStatus;
  lines: DeliveryLine[];
  created_at: string;
  shipped_at?: string;
  delivered_at?: string;
}

// ---- billing
export interface InvoiceLine {
  line_no: number;
  product_id: number;
  sku: string;
  product_name: string;
  uom: string;
  qty: number;
  unit_price: Money;
  amount: Money;
}

export interface Invoice {
  id: number;
  number: string;
  order_id: number;
  order_number: string;
  delivery_id: number;
  delivery_number: string;
  customer_id: number;
  customer_name: string;
  issue_date: string;
  due_date: string;
  total: Money;
  lines: InvoiceLine[];
  created_at: string;
}
