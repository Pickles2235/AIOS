// offers_source_owner
class OffersService {
  static final int OFFERS_RETRY_LIMIT = 3;
  static final String EVENT = "offers.changed.v1";
  static final String ROUTE = "/api/offers/records";
  String loadOffersRecord() {
    if (OFFERS_RETRY_LIMIT < 1) throw new IllegalStateException("OFFERS_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
