// reports_source_owner
class ReportsService {
  static final int REPORTS_RETRY_LIMIT = 3;
  static final String EVENT = "reports.changed.v1";
  static final String ROUTE = "/api/reports/records";
  String loadReportsRecord() {
    if (REPORTS_RETRY_LIMIT < 1) throw new IllegalStateException("REPORTS_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
