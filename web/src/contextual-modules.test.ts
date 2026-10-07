import { describe, expect, it } from "vitest";
import { clampPosition, reconcileActivity, type Activity } from "./contextual-modules";

const event = (sequence: number) => ({ sequence, at: "2026-10-07T00:00:00Z", stage: "discovered", repository: "fixture", files: sequence, queryable: false });
const snapshot = (stream_id: string, sequence: number, first: number): Activity => ({ stream_id, sequence, oldest_sequence: first, retention_events: 512, events: Array.from({ length: sequence - first + 1 }, (_, i) => event(first + i)) });

describe("contextual activity continuity", () => {
  it("deduplicates repeated snapshots and keeps real stage ordering", () => {
    const first = reconcileActivity({ events: [] }, snapshot("one", 2, 1));
    const same = reconcileActivity(first, snapshot("one", 2, 1));
    expect(same.events.map(e => e.sequence)).toEqual([1, 2]);
    expect(reconcileActivity(same, snapshot("one", 3, 1)).events.map(e => e.sequence)).toEqual([1, 2, 3]);
  });
  it("discloses retention gaps and a new epoch even when sequence increases", () => {
    expect(reconcileActivity({ events: [] }, snapshot("one", 515, 4)).warning).toMatch(/outside the retained history/);
    const first = reconcileActivity({ events: [] }, snapshot("one", 2, 1));
    const gap = reconcileActivity(first, snapshot("one", 515, 4));
    expect(gap.warning).toMatch(/gap/);
    expect(gap.events.some(e => e.sequence === 3)).toBe(false);
    const restart = reconcileActivity(gap, snapshot("two", 520, 519));
    expect(restart.warning).toMatch(/restarted/);
    expect(restart.events.map(e => e.sequence)).toEqual([519, 520]);
  });
  it("rejects malformed snapshots instead of displaying fake progress", () => {
    expect(() => reconcileActivity({ events: [] }, { ...snapshot("one", 2, 1), oldest_sequence: 9 })).toThrow();
    expect(() => reconcileActivity({ events: [] }, { ...snapshot("one", 2, 1), events: [event(1), event(1)] })).toThrow();
    expect(reconcileActivity({ events: [] }, { ...snapshot("one", 3, 1), events: [event(1), event(2)] }).warning).toMatch(/incomplete/);
  });
});

it("clamps persisted and keyboard placement inside the viewport", () => {
  expect(clampPosition({ x: 10000, y: -100 }, { width: 300, height: 140 }, { width: 800, height: 600 })).toEqual({ x: 488, y: 12 });
  expect(clampPosition({ x: 400, y: 400 }, { width: 300, height: 140 }, { width: 320, height: 200 })).toEqual({ x: 12, y: 48 });
});
