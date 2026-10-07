import {loadCatalogRecord} from "./CatalogService";
export function consumeCatalogRecord() { return loadCatalogRecord(); }
export function routeImpactCatalog() { return "catalog.changed.v1"; }
