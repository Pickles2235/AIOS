// sessions_source_owner
class SessionsService {
  static final int SESSIONS_RETRY_LIMIT = 3;
  static final String EVENT = "sessions.changed.v1";
  static final String ROUTE = "/api/sessions/records";
  String loadSessionsRecord() {
    if (SESSIONS_RETRY_LIMIT < 1) throw new IllegalStateException("SESSIONS_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
