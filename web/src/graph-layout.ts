import {forceCenter, forceCollide, forceLink, forceManyBody, forceSimulation, forceX, forceY} from 'd3-force';
import type {KnowledgeEdge, KnowledgeNode} from './knowledge-state';
export type Point = {x: number; y: number; community: string};
type SimNode = {id: string; community: string; radius: number; x?: number; y?: number; vx?: number; vy?: number; fx?: number; fy?: number};
const hash = (value: string) => [...value].reduce((result, char) => (result * 31 + char.charCodeAt(0)) >>> 0, 2166136261);
const random = (seed: number) => ((seed * 1664525 + 1013904223) >>> 0) / 4294967296;
// Preserve the hierarchy in geometry: repositories are anchors, while dense
// file-level evidence remains deliberately quiet.
export const nodeRadius = (node: KnowledgeNode) => {
  const centrality = Math.min(1, (node.connections || 0) / 16);
  const base = node.type === 'repository' ? 2.35 : node.type === 'skill' || node.type === 'mcp' ? 1.5 : node.type === 'wiki' ? 1.05 : .66;
  const influence = node.type === 'repository' ? .72 : .9;
  return Number((base + centrality * influence).toFixed(2));
};
export const forceLayout = (nodes: KnowledgeNode[], edges: KnowledgeEdge[], limit = 250, previous = new Map<string, Point>()): Map<string, Point> => {
  const shown = [...nodes].sort((a, b) => a.id.localeCompare(b.id)).slice(0, limit), communities = [...new Set(shown.map(node => node.community || node.type || 'knowledge'))].sort();
  // A force simulation is useful for a compact overview, but is neither timely
  // nor stable enough for the complete local estate. The deterministic lattice
  // gives every projected item its own world-space slot for Canvas zoom/pan.
  if (shown.length > 1_000) {
    const groups = new Map<string, KnowledgeNode[]>();
    shown.forEach(node => { const key = node.community || node.type || 'knowledge'; groups.set(key, [...(groups.get(key) || []), node]); });
    const ordered = [...groups.entries()].sort(([left], [right]) => left.localeCompare(right));
    const columns = 150;
    const rows = ordered.reduce((total, [, group]) => total + Math.ceil(group.length / columns) + 1, -1);
    let rowOffset = 0;
    const points = new Map<string, Point>();
    ordered.forEach(([community, group]) => {
      group.forEach((node, index) => points.set(node.id, {
        x: 3 + (index % columns) * 94 / (columns - 1),
        y: 3 + (rowOffset + Math.floor(index / columns)) * 94 / Math.max(rows - 1, 1),
        community,
      }));
      rowOffset += Math.ceil(group.length / columns) + 1;
    });
    return points;
  }
  const anchors = new Map(communities.map((community, i) => { const a = i * Math.PI * 2 / Math.max(communities.length, 1), r = Math.min(16, communities.length * 1.5); return [community, {x: 50 + Math.cos(a) * r, y: 50 + Math.sin(a) * r}]; }));
  const simNodes: SimNode[] = shown.map(node => { const seed = hash(node.id), community = node.community || node.type || 'knowledge', anchor = anchors.get(community)!, retained = previous.get(node.id); return retained ? {id: node.id, community, radius: nodeRadius(node), x: retained.x, y: retained.y, fx: retained.x, fy: retained.y} : {id: node.id, community, radius: nodeRadius(node), x: anchor.x + (random(seed) - .5) * 10, y: anchor.y + (random(seed ^ 0x9e3779b9) - .5) * 10}; });
  const ids = new Set(simNodes.map(node => node.id)); const links = edges.filter(edge => ids.has(edge.source) && ids.has(edge.target)).slice(0, 700).map(edge => ({source: edge.source, target: edge.target}));
  const simulation = forceSimulation(simNodes).force('link', forceLink<SimNode, {source: string; target: string}>(links).id(node => node.id).distance(8).strength(.32)).force('charge', forceManyBody().strength(-16).distanceMax(42)).force('center', forceCenter(50, 50)).force('collide', forceCollide<SimNode>().radius(node => node.radius + .55).strength(.85)).force('x', forceX<SimNode>(node => anchors.get(node.community)!.x).strength(.09)).force('y', forceY<SimNode>(node => anchors.get(node.community)!.y).strength(.09)).stop();
  for (let i = 0; i < 280; i++) simulation.tick();
  const xs = simNodes.map(node => node.x || 50), ys = simNodes.map(node => node.y || 50), minX = Math.min(...xs), maxX = Math.max(...xs), minY = Math.min(...ys), maxY = Math.max(...ys), scale = 72 / Math.max(maxX - minX, maxY - minY, 1);
  return new Map(simNodes.map(node => [node.id, previous.get(node.id) || {x: 50 + ((node.x || 50) - (minX + maxX) / 2) * scale, y: 50 + ((node.y || 50) - (minY + maxY) / 2) * scale, community: node.community}]));
};
export type CommunityRegion = { community: string; minX: number; maxX: number; minY: number; maxY: number };
export const communityRegions = (nodes: KnowledgeNode[], points: Map<string, Point>): CommunityRegion[] => {
  const bounds = new Map<string, CommunityRegion>();
  nodes.forEach(node => { const point = points.get(node.id); if (!point) return; const current = bounds.get(point.community); if (current) { current.minX = Math.min(current.minX, point.x); current.maxX = Math.max(current.maxX, point.x); current.minY = Math.min(current.minY, point.y); current.maxY = Math.max(current.maxY, point.y); } else bounds.set(point.community, {community: point.community, minX: point.x, maxX: point.x, minY: point.y, maxY: point.y}); });
  return [...bounds.values()].sort((left, right) => left.community.localeCompare(right.community));
};
export type SpherePoint = { x: number; y: number; z: number; community: string };
export type SphereProjection = { x: number; y: number; depth: number; scale: number; community: string };
const direction = (seed: number) => {
  const z = random(seed) * 2 - 1, theta = random(seed ^ 0x9e3779b9) * Math.PI * 2, radial = Math.sqrt(1 - z * z);
  return {x: Math.cos(theta) * radial, y: Math.sin(theta) * radial, z};
};
// Each real community gets a stable volumetric centroid. Nodes keep an
// independent deterministic local offset, producing readable constellations
// without fabricated topology or latitude bands.
export const sphereLayout = (nodes: KnowledgeNode[]) => {
  const shown = [...nodes].sort((left, right) => left.id.localeCompare(right.id));
  const centroids = new Map<string, {x: number; y: number; z: number}>();
  shown.forEach(node => { const community = node.community || node.type || 'knowledge'; if (centroids.has(community)) return; const seed = hash(community); const axis = direction(seed); const radius = .16 + .48 * Math.cbrt(random(seed ^ 0x85ebca6b)); centroids.set(community, {x: axis.x * radius, y: axis.y * radius, z: axis.z * radius}); });
  return new Map(shown.map(node => {
    const community = node.community || node.type || 'knowledge', seed = hash(node.id), center = centroids.get(community)!;
    const axis = direction(seed), radius = .035 + .27 * Math.cbrt(random(seed ^ 0xc2b2ae35));
    let x = center.x + axis.x * radius, y = center.y + axis.y * radius, z = center.z + axis.z * radius;
    const length = Math.hypot(x, y, z); if (length > .96) { x *= .96 / length; y *= .96 / length; z *= .96 / length; }
    return [node.id, {x, y, z, community}];
  }));
};
export const projectSphere = (point: SpherePoint, rotation: {x: number; y: number}) => {
  const cosY = Math.cos(rotation.y), sinY = Math.sin(rotation.y), cosX = Math.cos(rotation.x), sinX = Math.sin(rotation.x);
  const x = point.x * cosY - point.z * sinY;
  const zY = point.x * sinY + point.z * cosY;
  const y = point.y * cosX - zY * sinX;
  const z = point.y * sinX + zY * cosX;
  const scale = 1 / (2.45 - z);
  return {x: x * scale, y: y * scale, depth: z, scale, community: point.community};
};
export const communityColor = (community: string) => ['#87a8ff', '#63e1b4', '#f2bb75', '#d898f4', '#70d4ec', '#f58ba8', '#a4d47a'][hash(community) % 7];
