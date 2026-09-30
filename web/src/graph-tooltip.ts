import type {KnowledgeNode} from './knowledge-state';

export type GraphTooltip = {title: string; type: string; group: string; detail: string; connections: number; path?: string};

export const graphTooltip = (node: KnowledgeNode): GraphTooltip => ({
  title: node.label,
  type: node.type,
  group: node.community || 'knowledge',
  detail: node.summary || node.evidence || 'Local evidence indexed during this build.',
  connections: node.connections || 0,
  path: node.path,
});
