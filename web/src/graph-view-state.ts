import type { KnowledgeEdge, KnowledgeNode } from "./knowledge-state";

// Rendering is Canvas-backed, so the UI is no longer a bounded SVG sample.
export const graphNodeLimit = Number.MAX_SAFE_INTEGER;
export const graphEdgeLimit = Number.MAX_SAFE_INTEGER;

export type GraphView = {
  nodes: KnowledgeNode[];
  edges: KnowledgeEdge[];
  totalProjectedNodes: number;
  totalProjectedEdges: number;
  matchCount: number;
  displayedMatchCount: number;
  contextCount: number;
};

const compareNodes = (left: KnowledgeNode, right: KnowledgeNode) =>
  (right.connections || 0) - (left.connections || 0) || left.id.localeCompare(right.id);

const searchable = (node: KnowledgeNode) =>
  `${node.label} ${node.path || ""} ${node.type} ${node.community || ""}`.toLowerCase();

const matchRank = (node: KnowledgeNode, query: string) => {
  const value = query.toLowerCase();
  const label = node.label.toLowerCase();
  const path = (node.path || "").toLowerCase();
  const type = node.type.toLowerCase();
  const community = (node.community || "").toLowerCase();
  if ([label, path, type, community].some(field => field === value)) return 0;
  if ([label, path, type, community].some(field => field.startsWith(value))) return 1;
  return searchable(node).includes(value) ? 2 : 3;
};

const isPinned = (node: KnowledgeNode) =>
  ["repository", "capability", "skill", "mcp"].includes(node.type.toLowerCase());

const pinnedRank = (node: KnowledgeNode) => {
  const rank: Record<string, number> = { repository: 0, capability: 1, mcp: 2, skill: 3 };
  return rank[node.type.toLowerCase()] ?? 4;
};

const comparePinned = (left: KnowledgeNode, right: KnowledgeNode) =>
  pinnedRank(left) - pinnedRank(right) || compareNodes(left, right);

const balanced = (nodes: KnowledgeNode[], limit: number) => {
  const groups = new Map<string, KnowledgeNode[]>();
  nodes.forEach(node => {
    const group = node.community || node.type || "knowledge";
    groups.set(group, [...(groups.get(group) || []), node]);
  });
  const queues = [...groups.entries()].sort(([left], [right]) => left.localeCompare(right)).map(([, group]) => group.sort(compareNodes));
  const result: KnowledgeNode[] = [];
  for (let index = 0; result.length < limit; index++) {
    let added = false;
    for (const queue of queues) {
      const node = queue[index];
      if (node) { result.push(node); added = true; if (result.length === limit) break; }
    }
    if (!added) break;
  }
  return result;
};

const unique = (nodes: KnowledgeNode[]) => {
  const ids = new Set<string>();
  return nodes.filter(node => !ids.has(node.id) && (ids.add(node.id), true));
};

export const selectGraphView = ({
  nodes,
  edges,
  filter = "all",
  query = "",
  selectedID,
}: {
  nodes: KnowledgeNode[];
  edges: KnowledgeEdge[];
  filter?: string;
  query?: string;
  selectedID?: string;
}): GraphView => {
  const scoped = nodes.filter(node => filter === "all" || node.type === filter);
  const value = query.trim().toLowerCase();
  const matches = value
    ? scoped.filter(node => matchRank(node, value) < 3).sort((left, right) => matchRank(left, value) - matchRank(right, value) || compareNodes(left, right))
    : filter !== "all" ? [...scoped].sort(compareNodes) : [];
  const byID = new Map(nodes.map(node => [node.id, node]));
  const selected = selectedID ? byID.get(selectedID) : undefined;
  const selectedNeighbourIDs = selected
    ? unique(edges.filter(edge => edge.source === selected.id || edge.target === selected.id)
      .map(edge => byID.get(edge.source === selected.id ? edge.target : edge.source))
      .filter((node): node is KnowledgeNode => Boolean(node))
      .sort(compareNodes)).slice(0, 12).map(node => node.id)
    : [];
  const matchedIDs = new Set(matches.map(node => node.id));
  const selectedNeighbours = selectedNeighbourIDs.map(id => byID.get(id)!).filter(node => !matchedIDs.has(node.id));
  const context = unique(edges.filter(edge => matchedIDs.has(edge.source) || matchedIDs.has(edge.target)).flatMap(edge => [edge.source, edge.target]).map(id => byID.get(id)).filter((node): node is KnowledgeNode => Boolean(node) && !matchedIDs.has(node.id)).sort(compareNodes));
  const primary = matches.length ? matches : scoped.filter(isPinned).sort(comparePinned);
  const leading = unique(primary).slice(0, graphNodeLimit);
  if (selected && !leading.some(node => node.id === selected.id)) {
    if (leading.length === graphNodeLimit) leading[graphNodeLimit - 1] = selected;
    else leading.push(selected);
  }
  const occupied = new Set([...(selected ? [selected.id] : []), ...primary.map(item => item.id), ...selectedNeighbours.map(item => item.id), ...context.map(item => item.id)]);
  const chosen = unique([
    ...leading,
    ...selectedNeighbours,
    ...context,
    ...(matches.length ? [] : balanced(scoped.filter(node => !occupied.has(node.id)), graphNodeLimit)),
  ]).slice(0, graphNodeLimit);
  const selectedIDs = new Set(chosen.map(node => node.id));
  const selectedEdges = edges.filter(edge => selectedIDs.has(edge.source) && selectedIDs.has(edge.target)).sort((left, right) => {
    const leftSelected = selectedID && (left.source === selectedID || left.target === selectedID) ? 0 : 1;
    const rightSelected = selectedID && (right.source === selectedID || right.target === selectedID) ? 0 : 1;
    return leftSelected - rightSelected || left.source.localeCompare(right.source) || left.target.localeCompare(right.target) || left.id.localeCompare(right.id);
  }).slice(0, graphEdgeLimit);
  return { nodes: chosen, edges: selectedEdges, totalProjectedNodes: nodes.length, totalProjectedEdges: edges.length, matchCount: matches.length, displayedMatchCount: matches.filter(node => selectedIDs.has(node.id)).length, contextCount: unique([...selectedNeighbours, ...context]).filter(node => selectedIDs.has(node.id)).length };
};
