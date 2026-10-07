// accounts_source_owner
class AccountsService {
  static final int ACCOUNTS_RETRY_LIMIT = 3;
  static final String EVENT = "accounts.changed.v1";
  static final String ROUTE = "/api/accounts/records";
  String loadAccountsRecord() {
    if (ACCOUNTS_RETRY_LIMIT < 1) throw new IllegalStateException("ACCOUNTS_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
