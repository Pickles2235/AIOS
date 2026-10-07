import {loadOrdersRecord} from "./OrdersService";
export function consumeOrdersRecord() { return loadOrdersRecord(); }
export function routeImpactOrders() { return "orders.changed.v1"; }
