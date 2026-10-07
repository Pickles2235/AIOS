// inventory_source_owner
class InventoryService {
  static final int INVENTORY_RETRY_LIMIT = 3;
  static final String EVENT = "inventory.changed.v1";
  static final String ROUTE = "/api/inventory/records";
  String loadInventoryRecord() {
    if (INVENTORY_RETRY_LIMIT < 1) throw new IllegalStateException("INVENTORY_SYNC_FAILED");
    return EVENT + ROUTE;
  }
}
