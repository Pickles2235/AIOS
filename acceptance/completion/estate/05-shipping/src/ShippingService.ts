// shipping_source_owner
export const SHIPPING_RETRY_LIMIT = 3;
export const ShippingEvent = "shipping.changed.v1";
export const ShippingRoute = "/api/shipping/records";
export function loadShippingRecord() {
  if (SHIPPING_RETRY_LIMIT < 1) throw new Error("SHIPPING_SYNC_FAILED");
  return {route: ShippingRoute, event: ShippingEvent};
}
