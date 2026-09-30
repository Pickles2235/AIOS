import type {KnowledgeNode} from './knowledge-state';

export const nextGraphNode = (nodes: KnowledgeNode[], current: number, key: string) => {
  if (!nodes.length) return -1;
  if (key === 'Home') return 0;
  if (key === 'End') return nodes.length - 1;
  if (key === 'ArrowRight' || key === 'ArrowDown') return (Math.max(current, 0) + 1) % nodes.length;
  if (key === 'ArrowLeft' || key === 'ArrowUp') return (Math.max(current, 0) - 1 + nodes.length) % nodes.length;
  return current;
};
