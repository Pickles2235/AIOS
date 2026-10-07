// returns_source_owner
export const RETURNS_RETRY_LIMIT = 3;
export const ReturnsEvent = "returns.changed.v1";
export const ReturnsRoute = "/api/returns/records";
export function loadReturnsRecord() {
  if (RETURNS_RETRY_LIMIT < 1) throw new Error("RETURNS_SYNC_FAILED");
  return {route: ReturnsRoute, event: ReturnsEvent};
}
