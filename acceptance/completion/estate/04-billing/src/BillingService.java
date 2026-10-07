// billing_source_owner
class BillingService {
  static final int BILLING_RETRY_LIMIT = 3;
  static final String EVENT = "billing.changed.v1";
  static final String ROUTE = "/api/billing/records";
  String loadBillingRecord() {
    if (BILLING_RETRY_LIMIT < 1) throw new IllegalStateException("BILLING_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
