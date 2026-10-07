// audit_source_owner
class AuditService {
  static final int AUDIT_RETRY_LIMIT = 3;
  static final String EVENT = "audit.changed.v1";
  static final String ROUTE = "/api/audit/records";
  String loadAuditRecord() {
    if (AUDIT_RETRY_LIMIT < 1) throw new IllegalStateException("AUDIT_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
