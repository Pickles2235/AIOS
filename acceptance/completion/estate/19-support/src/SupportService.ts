// support_source_owner
export const SUPPORT_RETRY_LIMIT = 3;
export const SupportEvent = "support.changed.v1";
export const SupportRoute = "/api/support/records";
export function loadSupportRecord() {
  if (SUPPORT_RETRY_LIMIT < 1) throw new Error("SUPPORT_SYNC_FAILED");
  return {route: SupportRoute, event: SupportEvent};
}
