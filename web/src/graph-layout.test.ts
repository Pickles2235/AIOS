import {describe, expect, it} from 'vitest';
import {communityRegions, forceLayout, nodeRadius, projectSphere, sphereLayout} from './graph-layout';
const nodes = [{id: 'a', label: 'A', type: 'repository', community: 'one'}, {id: 'b', label: 'B', type: 'file', community: 'one'}, {id: 'c', label: 'C', type: 'skill', community: 'two'}];
describe('force layout', () => {
  it('is deterministic and only retains projected edges', () => { const edges = [{id: 'ab', source: 'a', target: 'b', type: 'contains'}, {id: 'missing', source: 'a', target: 'x', type: 'contains'}]; expect([...forceLayout(nodes, edges)]).toEqual([...forceLayout(nodes, edges)]); expect(forceLayout(nodes, edges).size).toBe(3); });
  it('keeps the same map when the projection arrives in a different order', () => { const edges = [{id: 'ab', source: 'a', target: 'b', type: 'contains'}]; expect([...forceLayout(nodes, edges)]).toEqual([...forceLayout([...nodes].reverse(), edges)]); });
  it('makes more connected nodes more legible', () => { expect(nodeRadius({...nodes[0], connections: 20})).toBeGreaterThan(nodeRadius({...nodes[0], connections: 0})); });
  it('settles a connected graph inside the canvas rather than on its perimeter', () => { const many = Array.from({length: 24}, (_, i) => ({id: `n${i}`, label: `n${i}`, type: 'file', community: i < 12 ? 'one' : 'two'})); const edges = many.slice(1).map((node, i) => ({id: `e${i}`, source: many[i].id, target: node.id, type: 'contains'})); const points = [...forceLayout(many, edges).values()]; expect(points.filter(point => point.x > 12 && point.x < 88 && point.y > 12 && point.y < 88)).toHaveLength(points.length); });
  it('fixes retained coordinates while placing a new SSE node', () => { const first = forceLayout(nodes, []); const next = forceLayout([...nodes, {id: 'd', label: 'D', type: 'file', community: 'one'}], [], 250, first); expect(next.get('a')).toEqual(first.get('a')); expect(next.get('d')).toBeDefined(); });
  it('gives every full-estate ID a unique, non-overlapping lattice coordinate', () => {
    const estate = Array.from({length: 14694}, (_, index) => ({id: `estate-${index}`, label: `estate-${index}`, type: 'file', community: 'estate'}));
    const points = [...forceLayout(estate, [], Number.MAX_SAFE_INTEGER).values()];
    expect(points).toHaveLength(estate.length);
    expect(new Set(points.map(point => `${point.x}:${point.y}`)).size).toBe(estate.length);
    const byRow = new Map<number, number[]>();
    points.forEach(point => byRow.set(point.y, [...(byRow.get(point.y) || []), point.x]));
    expect(Math.min(...[...byRow.values()].flatMap(row => row.slice(1).map((x, index) => x - row[index])))).toBeGreaterThan(.5);
  });
  it('keeps full-estate communities in deterministic, distinct labelled regions', () => {
    const estate = Array.from({length: 1500}, (_, index) => ({id: `estate-${index}`, label: `estate-${index}`, type: 'file', community: index < 900 ? 'account' : 'customer'}));
    const points = forceLayout(estate, [], Number.MAX_SAFE_INTEGER);
    const regions = communityRegions(estate, points);
    expect(regions.map(region => region.community)).toEqual(['account', 'customer']);
    expect(regions[0].maxY).toBeLessThan(regions[1].minY);
    expect([...forceLayout([...estate].reverse(), [], Number.MAX_SAFE_INTEGER)]).toEqual([...points]);
  });
  it('assigns every node a unique deterministic point through the sphere volume', () => {
    const estate = Array.from({length: 14694}, (_, index) => ({id: `sphere-${index}`, label: `sphere-${index}`, type: 'file', community: index % 2 ? 'account' : 'customer'}));
    const sphere = sphereLayout(estate);
    expect(sphere.size).toBe(estate.length);
    expect(new Set([...sphere.values()].map(point => `${point.x}:${point.y}:${point.z}`)).size).toBe(estate.length);
    const radii = [...sphere.values()].map(point => Math.hypot(point.x, point.y, point.z));
    expect(radii.every(radius => radius >= 0 && radius <= .96)).toBe(true);
    expect(radii.filter(radius => radius < .45)).not.toHaveLength(0);
    expect(radii.filter(radius => radius >= .45 && radius < .75)).not.toHaveLength(0);
    expect(radii.filter(radius => radius >= .75)).not.toHaveLength(0);
    expect([...sphereLayout([...estate].reverse())]).toEqual([...sphere]);
  });
  it('projects rotation with depth-aware perspective', () => {
    const front = projectSphere({x: 0, y: 0, z: 1, community: 'one'}, {x: 0, y: 0});
    const back = projectSphere({x: 0, y: 0, z: -1, community: 'one'}, {x: 0, y: 0});
    expect(front.depth).toBeGreaterThan(back.depth);
    expect(front.scale).toBeGreaterThan(back.scale);
  });
  it('forms organic community constellations rather than axis bands', () => {
    const nodes = Array.from({length: 180}, (_, index) => ({id: `cluster-${index}`, label: `cluster-${index}`, type: 'file', community: index < 60 ? 'account' : index < 120 ? 'customer' : 'agreement'}));
    const points = sphereLayout(nodes);
    const distance = (left: string, right: string) => { const a = points.get(left)!, b = points.get(right)!; return Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z); };
    const average = (pairs: Array<[string, string]>) => pairs.reduce((sum, [left, right]) => sum + distance(left, right), 0) / pairs.length;
    const same = average(Array.from({length: 40}, (_, index): [string, string] => [`cluster-${index}`, `cluster-${index + 20}`]));
    const cross = average(Array.from({length: 40}, (_, index): [string, string] => [`cluster-${index}`, `cluster-${index + 80}`]));
    expect(same).toBeLessThan(cross);
    const means = ['account', 'customer', 'agreement'].map(community => { const values = nodes.filter(node => node.community === community).map(node => points.get(node.id)!); return values.reduce((sum, point) => sum + point.y, 0) / values.length; });
    expect(new Set(means.map(value => value.toFixed(3))).size).toBe(3);
  });
});
