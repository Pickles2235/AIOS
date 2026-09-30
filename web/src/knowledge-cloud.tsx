import React from "react";
import type {Entity, Projection} from "./runtime-types";

// Layout only: every rendered node and edge comes from this bounded server page.
export function KnowledgeCloud({projection, colour, selected, inspect}: {projection: Projection; colour: string; selected?: string; inspect: (entity: Entity) => void}) {
  const nodes = projection.nodes.map((entity, i) => {
    const angle = 2 * Math.PI * i / Math.max(1, projection.nodes.length), radius = projection.nodes.length === 1 ? 0 : 155 + (i % 3) * 30;
    return {entity, x: 400 + Math.cos(angle) * radius, y: 260 + Math.sin(angle) * radius};
  });
  const positions = new Map(nodes.map(node => [node.entity.handle, node]));
  const edges = projection.edges.filter(edge => positions.has(edge.subject) && positions.has(edge.object));
  return <section aria-label="Knowledge cloud"><h2>Knowledge cloud</h2>
    <p>{projection.repository} · generation {projection.generation} · {projection.nodes.length} nodes · {projection.edges.length} evidence-backed edges · {edges.length} edges connect visible nodes{projection.truncated ? " · bounded projection page" : ""}</p>
    {projection.applied_limits && <p>Page bounds: {Object.entries(projection.applied_limits).map(([key, value]) => `${key} ${value}`).join(" · ")}</p>}
    <svg viewBox="0 0 800 520" role="group" aria-label="Active-generation cloud nodes">
      {edges.map(edge => {const a = positions.get(edge.subject)!, b = positions.get(edge.object)!; return <line key={edge.handle} x1={a.x} y1={a.y} x2={b.x} y2={b.y} className="cloud-edge"><title>{edge.predicate} · {edge.confidence.toFixed(2)} · {edge.derivation}</title></line>;})}
      {nodes.map(({entity, x, y}) => <g key={entity.handle} role="button" tabIndex={0} aria-label={`Cloud node: ${entity.label}, ${entity.path}`} aria-pressed={selected === entity.handle} onClick={() => inspect(entity)} onKeyDown={e => {if (e.key === "Enter" || e.key === " ") {e.preventDefault(); inspect(entity);}}}>
        <circle cx={x} cy={y} r={selected === entity.handle ? 13 : 9} fill={colour}/><text x={x} y={y + 26} textAnchor="middle">{entity.label.slice(0, 32)}</text><title>{entity.label} · {entity.kind} · {entity.repository}/{entity.path} · generation {entity.generation}</title>
      </g>)}
    </svg>
  </section>;
}
