// notifications_source_owner
export const NOTIFICATIONS_RETRY_LIMIT = 3;
export const NotificationsEvent = "notifications.changed.v1";
export const NotificationsRoute = "/api/notifications/records";
export function loadNotificationsRecord() {
  if (NOTIFICATIONS_RETRY_LIMIT < 1) throw new Error("NOTIFICATIONS_SYNC_FAILED");
  return {route: NotificationsRoute, event: NotificationsEvent};
}
