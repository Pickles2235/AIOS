// orders_source_owner
export const ORDERS_RETRY_LIMIT = 3;
export const OrdersEvent = "orders.changed.v1";
export const OrdersRoute = "/api/orders/records";
export function loadOrdersRecord() {
  if (ORDERS_RETRY_LIMIT < 1) throw new Error("ORDERS_SYNC_FAILED");
  return {route: OrdersRoute, event: OrdersEvent};
}
