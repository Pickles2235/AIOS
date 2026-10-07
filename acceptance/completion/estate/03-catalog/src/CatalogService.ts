// catalog_source_owner
export const CATALOG_RETRY_LIMIT = 3;
export const CatalogEvent = "catalog.changed.v1";
export const CatalogRoute = "/api/catalog/records";
export function loadCatalogRecord() {
  if (CATALOG_RETRY_LIMIT < 1) throw new Error("CATALOG_SYNC_FAILED");
  return {route: CatalogRoute, event: CatalogEvent};
}
