// security_source_owner
export const SECURITY_RETRY_LIMIT = 3;
export const SecurityEvent = "security.changed.v1";
export const SecurityRoute = "/api/security/records";
export function loadSecurityRecord() {
  if (SECURITY_RETRY_LIMIT < 1) throw new Error("SECURITY_SYNC_FAILED");
  return {route: SecurityRoute, event: SecurityEvent};
}
