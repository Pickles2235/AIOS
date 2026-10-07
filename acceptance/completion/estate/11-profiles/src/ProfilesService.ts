// profiles_source_owner
export const PROFILES_RETRY_LIMIT = 3;
export const ProfilesEvent = "profiles.changed.v1";
export const ProfilesRoute = "/api/profiles/records";
export function loadProfilesRecord() {
  if (PROFILES_RETRY_LIMIT < 1) throw new Error("PROFILES_SYNC_FAILED");
  return {route: ProfilesRoute, event: ProfilesEvent};
}
