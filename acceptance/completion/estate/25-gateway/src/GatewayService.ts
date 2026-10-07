// gateway_source_owner
export const GATEWAY_RETRY_LIMIT = 3;
export const GatewayEvent = "gateway.changed.v1";
export const GatewayRoute = "/api/gateway/records";
export function loadGatewayRecord() {
  if (GATEWAY_RETRY_LIMIT < 1) throw new Error("GATEWAY_SYNC_FAILED");
  return {route: GatewayRoute, event: GatewayEvent};
}
