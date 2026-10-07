import { describe, expect, it } from "vitest";
import {
  initialCamera,
  nodeIdentity,
  position,
  project,
  nodeColour,
  type CloudNode,
} from "./cloud-types";
const node: CloudNode = {
  handle: "one",
  kind: "file",
  label: "file.ts",
  repository: "repo",
  path: "src/file.ts",
  generation: "g1",
  aggregate: true,
  member_count: 12,
  file_count: 1,
};
describe("canonical cloud geometry", () => {
  it("keeps logical positions stable across atomic generation changes", () => {
    const next = { ...node, handle: "two", generation: "g2", member_count: 13 };
    expect(nodeIdentity(next)).toBe(nodeIdentity(node));
    expect(position(next)).toEqual(position(node));
  });
  it("retains depth and applies real perspective, rotation, pan and zoom", () => {
    const a = project([0.5, 0.3, 0.6], initialCamera, 1),
      b = project([0.5, 0.3, -0.6], initialCamera, 1);
    expect(a.x).not.toBe(b.x);
    expect(a.z).not.toBe(b.z);
    expect(
      project([0.5, 0.3, 0.6], { ...initialCamera, yaw: 1 }, 1),
    ).not.toEqual(a);
    expect(
      project([0.5, 0.3, 0.6], { ...initialCamera, panX: 1 }, 1).x,
    ).toBeCloseTo(a.x + 1);
    expect(
      project(
        [0.5, 0.3, 0.6],
        { ...initialCamera, zoom: initialCamera.zoom * 2 },
        1,
      ).x,
    ).toBeCloseTo(a.x * 2);
  });
  it("uses explicit semantic colours and bounded volumetric positions", () => {
    expect(nodeColour(node)).toBe("#65d8c0");
    for (let i = 0; i < 300; i++) {
      const point = position({ ...node, path: `src/${i}.ts` });
      expect(Math.hypot(...point)).toBeLessThanOrEqual(0.95);
      expect(point[2]).not.toBe(0);
    }
  });
});
