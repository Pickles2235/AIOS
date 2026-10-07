// pricing_source_owner
export const PRICING_RETRY_LIMIT = 3;
export const PricingEvent = "pricing.changed.v1";
export const PricingRoute = "/api/pricing/records";
export function loadPricingRecord() {
  if (PRICING_RETRY_LIMIT < 1) throw new Error("PRICING_SYNC_FAILED");
  return {route: PricingRoute, event: PricingEvent};
}
