// media_source_owner
class MediaService {
  static final int MEDIA_RETRY_LIMIT = 3;
  static final String EVENT = "media.changed.v1";
  static final String ROUTE = "/api/media/records";
  String loadMediaRecord() {
    if (MEDIA_RETRY_LIMIT < 1) throw new IllegalStateException("MEDIA_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
