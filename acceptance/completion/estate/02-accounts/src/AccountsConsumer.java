class AccountsConsumer {
  String consumeAccountsRecord() { return new AccountsService().loadAccountsRecord(); }
  String routeImpactAccounts() { return "accounts.changed.v1"; }
}
