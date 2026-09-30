import {describe, expect, it} from 'vitest';
import {nextGraphNode} from './graph-navigation';

const nodes = [{id: 'a'}, {id: 'b'}, {id: 'c'}] as any;
describe('graph keyboard navigation', () => {
  it('cycles with arrows without adding a tab stop per node', () => {
    expect(nextGraphNode(nodes, 0, 'ArrowRight')).toBe(1);
    expect(nextGraphNode(nodes, 0, 'ArrowLeft')).toBe(2);
    expect(nextGraphNode(nodes, 1, 'Home')).toBe(0);
    expect(nextGraphNode(nodes, 1, 'End')).toBe(2);
  });
});
