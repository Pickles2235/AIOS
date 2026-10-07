class BillingConsumer {
  String consumeBillingRecord() { return new BillingService().loadBillingRecord(); }
  String routeImpactBilling() { return "billing.changed.v1"; }
}
