// analytics_source_owner
export const ANALYTICS_RETRY_LIMIT = 3;
export const AnalyticsEvent = "analytics.changed.v1";
export const AnalyticsRoute = "/api/analytics/records";
export function loadAnalyticsRecord() {
  if (ANALYTICS_RETRY_LIMIT < 1) throw new Error("ANALYTICS_SYNC_FAILED");
  return {route: AnalyticsRoute, event: AnalyticsEvent};
}
