import type { DeliveryStatus, OrderStatus } from "../api/types";
import { Badge, type Tone } from "./ui";

const orderTones: Record<OrderStatus, Tone> = {
  DRAFT: "gray",
  CONFIRMED: "blue",
  FULFILLING: "amber",
  DELIVERED: "indigo",
  INVOICED: "green",
  REJECTED: "red",
  CANCELLED: "gray",
};

const deliveryTones: Record<DeliveryStatus, Tone> = {
  PENDING: "amber",
  SHIPPED: "blue",
  DELIVERED: "green",
};

const label = (s: string) => s.charAt(0) + s.slice(1).toLowerCase();

export function OrderStatusBadge({ status }: { status: OrderStatus }) {
  return <Badge tone={orderTones[status]}>{label(status)}</Badge>;
}

export function DeliveryStatusBadge({ status }: { status: DeliveryStatus }) {
  return <Badge tone={deliveryTones[status]}>{label(status)}</Badge>;
}
