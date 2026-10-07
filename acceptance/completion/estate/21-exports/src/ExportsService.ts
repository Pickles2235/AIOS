// exports_source_owner
export const EXPORTS_RETRY_LIMIT = 3;
export const ExportsEvent = "exports.changed.v1";
export const ExportsRoute = "/api/exports/records";
export function loadExportsRecord() {
  if (EXPORTS_RETRY_LIMIT < 1) throw new Error("EXPORTS_SYNC_FAILED");
  return {route: ExportsRoute, event: ExportsEvent};
}
