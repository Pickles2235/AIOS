// search_source_owner
class SearchService {
  static final int SEARCH_RETRY_LIMIT = 3;
  static final String EVENT = "search.changed.v1";
  static final String ROUTE = "/api/search/records";
  String loadSearchRecord() {
    if (SEARCH_RETRY_LIMIT < 1) throw new IllegalStateException("SEARCH_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
