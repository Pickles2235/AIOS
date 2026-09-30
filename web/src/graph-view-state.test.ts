import { describe, expect, it } from "vitest";
import { selectGraphView } from "./graph-view-state";

const nodes = Array.from({ length: 320 }, (_, index) => ({
  id: `node-${String(index).padStart(3, "0")}`,
  label: index < 25 ? `repository-${index}` : `file-${index}`,
  path: index === 300 ? "src/needle.ts" : `src/${index}.ts`,
  type: index < 25 ? "repository" : index === 25 ? "mcp" : "file",
  community: `community-${index % 4}`,
  connections: index % 17,
}));
const edges = Array.from({ length: 700 }, (_, index) => ({ id: `edge-${index}`, source: "node-000", target: `node-${String((index % 319) + 1).padStart(3, "0")}`, type: "contains" }));

describe("graph view selector", () => {
  it("keeps the complete projection, including every repository and capability", () => {
    const skills = Array.from({ length: 300 }, (_, index) => ({ id: `skill-${index}`, label: `skill-${index}`, type: "skill", community: "capability", connections: 50 }));
    const view = selectGraphView({ nodes: [...nodes, { id: "capability", label: "Capabilities", type: "capability", community: "capability" }, ...skills], edges });
    expect(view.nodes).toHaveLength(nodes.length + skills.length + 1);
    expect(view.edges).toHaveLength(edges.length);
    expect(view.nodes.filter(node => node.type === "repository")).toHaveLength(25);
    expect(view.nodes.some(node => node.type === "mcp")).toBe(true);
    expect(view.nodes.some(node => node.type === "capability")).toBe(true);
  });

  it("ranks exact, prefix and substring matches over one-hop context", () => {
    const view = selectGraphView({ nodes, edges, query: "needle", selectedID: "node-000" });
    expect(view.matchCount).toBe(1);
    expect(view.nodes[0].id).toBe("node-300");
    expect(view.contextCount).toBeGreaterThan(0);
  });

  it("adds query neighbours without a selected node and keeps filter matches distinct from estate totals", () => {
    const view = selectGraphView({ nodes: [...nodes, { id: "needle-neighbour", label: "plain", type: "file", community: "community-0" }], edges: [...edges, { id: "needle-link", source: "node-300", target: "needle-neighbour", type: "contains" }], query: "needle" });
    expect(view.nodes.map(node => node.id)).toEqual(expect.arrayContaining(["node-300", "needle-neighbour"]));
    const estate = Array.from({ length: 14694 }, (_, index) => ({ id: `estate-${index}`, label: index < 25 ? `repo-${index}` : `file-${index}`, type: index < 25 ? "repository" : "file", community: "estate" }));
    const filtered = selectGraphView({ nodes: estate, edges: [], filter: "repository" });
    expect(filtered).toMatchObject({ totalProjectedNodes: 14694, totalProjectedEdges: 0, matchCount: 25, contextCount: 0 });
  });

  it("prioritises selected edges deterministically", () => {
    const view = selectGraphView({ nodes, edges: [...edges].reverse(), selectedID: "node-000" });
    expect(view.edges[0].id).toBe("edge-0");
  });

  it("keeps the selected node visible alongside all matching nodes", () => {
    const view = selectGraphView({ nodes, edges, query: "file", selectedID: "node-000" });
    expect(view.matchCount).toBeGreaterThan(250);
    expect(view.nodes.length).toBeGreaterThanOrEqual(view.matchCount);
    expect(view.nodes.some(node => node.id === "node-000")).toBe(true);
  });
});
