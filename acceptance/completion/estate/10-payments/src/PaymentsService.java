// payments_source_owner
class PaymentsService {
  static final int PAYMENTS_RETRY_LIMIT = 3;
  static final String EVENT = "payments.changed.v1";
  static final String ROUTE = "/api/payments/records";
  String loadPaymentsRecord() {
    if (PAYMENTS_RETRY_LIMIT < 1) throw new IllegalStateException("PAYMENTS_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
