import {loadShippingRecord} from "./ShippingService";
export function consumeShippingRecord() { return loadShippingRecord(); }
export function routeImpactShipping() { return "shipping.changed.v1"; }
