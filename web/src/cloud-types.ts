import type { Claim, Entity, Status } from "./runtime-types";
export type CloudNode = {
  handle: string;
  kind: string;
  label: string;
  repository: string;
  path: string;
  generation: string;
  aggregate: boolean;
  member_count: number;
  file_count: number;
  child_scope?: string;
  entity?: Entity;
};
export type CloudPage = {
  snapshot: string;
  level: "estate" | "repository" | "package" | "file";
  scope: string;
  label: string;
  nodes: CloudNode[];
  edges: Claim[];
  total_nodes: number;
  total_entities: number;
  total_files: number;
  edge_count: number;
  total_claims: number;
  edges_truncated: boolean;
  focus?: string;
  next_cursor?: string;
  truncated: boolean;
  generations: Status["repositories"];
  coverage: { complete: boolean; uncertainty?: string[] };
  count_scope: string;
};
export type SourceActions = {
  generation: string;
  revision: string;
  actions: {
    kind: string;
    label: string;
    url?: string;
    available: boolean;
    reason?: string;
  }[];
};
export type Camera = {
  yaw: number;
  pitch: number;
  panX: number;
  panY: number;
  zoom: number;
};
export const initialCamera: Camera = {
  yaw: 0.35,
  pitch: -0.18,
  panX: 0,
  panY: 0,
  zoom: 1.8,
};
export const semanticColours: Record<string, string> = {
  repository: "#80adff",
  directory: "#c4a1f4",
  file: "#65d8c0",
  symbol: "#ffcb7a",
  selected: "#ffffff",
  result: "#ff80ac",
  relationship: "#6180a6",
};
export function nodeColour(node: CloudNode) {
  return semanticColours[
    node.aggregate
      ? node.kind === "repository"
        ? "repository"
        : node.kind === "file"
          ? "file"
          : "directory"
      : "symbol"
  ];
}
export const nodeIdentity = (node: CloudNode) =>
  [
    node.repository,
    node.path,
    node.aggregate ? node.kind : node.entity?.identity || node.label,
  ].join("\0");
function hash(text: string) {
  let n = 2166136261;
  for (const char of text) {
    n ^= char.charCodeAt(0);
    n = Math.imul(n, 16777619);
  }
  n ^= n >>> 16;
  n = Math.imul(n, 0x85ebca6b);
  n ^= n >>> 13;
  n = Math.imul(n, 0xc2b2ae35);
  return (n ^ (n >>> 16)) >>> 0;
}
export function position(node: CloudNode): [number, number, number] {
  const key = nodeIdentity(node),
    a = (hash(key) / 4294967296) * Math.PI * 2,
    z = (hash(key + "z") / 4294967296) * 2 - 1,
    r = 0.2 + 0.75 * Math.cbrt(hash(key + "r") / 4294967296),
    xy = Math.sqrt(1 - z * z);
  return [Math.cos(a) * xy * r, Math.sin(a) * xy * r, z * r];
}
export function project(
  point: [number, number, number],
  camera: Camera,
  aspect: number,
) {
  const [a, b, c] = point,
    x = a * Math.cos(camera.yaw) - c * Math.sin(camera.yaw),
    z = a * Math.sin(camera.yaw) + c * Math.cos(camera.yaw),
    y = b * Math.cos(camera.pitch) - z * Math.sin(camera.pitch),
    depth = b * Math.sin(camera.pitch) + z * Math.cos(camera.pitch),
    scale = camera.zoom / (2.8 - depth);
  return {
    x: (x * scale) / aspect + camera.panX,
    y: y * scale + camera.panY,
    z: -depth / 3,
    scale,
  };
}
