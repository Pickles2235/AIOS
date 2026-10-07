// imports_source_owner
class ImportsService {
  static final int IMPORTS_RETRY_LIMIT = 3;
  static final String EVENT = "imports.changed.v1";
  static final String ROUTE = "/api/imports/records";
  String loadImportsRecord() {
    if (IMPORTS_RETRY_LIMIT < 1) throw new IllegalStateException("IMPORTS_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
