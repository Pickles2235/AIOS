// reviews_source_owner
export const REVIEWS_RETRY_LIMIT = 3;
export const ReviewsEvent = "reviews.changed.v1";
export const ReviewsRoute = "/api/reviews/records";
export function loadReviewsRecord() {
  if (REVIEWS_RETRY_LIMIT < 1) throw new Error("REVIEWS_SYNC_FAILED");
  return {route: ReviewsRoute, event: ReviewsEvent};
}
