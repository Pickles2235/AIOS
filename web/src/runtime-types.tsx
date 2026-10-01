export type Span = {start_line: number; end_line: number};
export type Entity = {handle: string; kind: string; label: string; identity: string; repository: string; path: string; generation: string; evidence: string; confidence: number; evidence_count: number; span: Span};
export type Claim = {handle: string; subject: string; object: string; predicate: string; evidence: string; confidence: number; derivation: string};
export type Projection = {repository: string; generation: string; nodes: Entity[]; edges: Claim[]; truncated: boolean; next_cursor?: string; applied_limits?: Record<string, number>};
export type Status = {projection_state: string; repositories: {id: string; generation: string; revision: string; active: boolean}[]};
export type Excerpt = {path: string; repository: string; generation: string; git_commit: string; working_tree?:boolean; sha256: string; start_line: number; end_line: number; lines: string[]; truncated: boolean};
export type QueryResult = {status: string; entities: Entity[]; trace: {kind: string; detail: string}[]; truncated: boolean; applied_limits?: Record<string, number>; coverage?: {complete: boolean; generations: string[]; repositories: string[]; exclusions?: string[]; uncertainty?: string[]}};
export function queryNotice(result: QueryResult) {
  const kinds = new Set(result.trace.map(t => t.kind));
  if (kinds.has("empty_query")) return "Enter a query to search captured source evidence.";
  if (kinds.has("unsupported_query")) return "Unsupported query: use at most 256 characters and supported input bounds.";
  if (kinds.has("projection_unavailable")) return "Search projection unavailable or stale. Refresh after rebuilding projections; canonical evidence remains intact.";
  if (kinds.has("no_active_generation")) return "Unknown: this scope has no active captured generation. Complete repository setup first.";
  if (kinds.has("budget_exhausted")) return "Query budget exhausted. Refine the query; these results cannot establish absence.";
  if (result.status === "not_found") return "Not found: no evidence matched within complete applicable active coverage.";
  if (result.status === "unknown") return "Unknown: coverage is incomplete; this search cannot establish absence.";
  if (result.truncated) return "Found bounded results. More matches may exist; refine the query.";
  return `Found ${result.entities.length} evidence-backed ${result.entities.length === 1 ? "result" : "results"}.`;
}
export function QueryFeedback({result}: {result?: QueryResult}) {
  if (!result) return null;
  return <section aria-label="Query feedback"><p role="status">{queryNotice(result)}</p>
    {result.coverage && <p>Coverage: {result.coverage.complete ? "complete for this search" : "incomplete"} · generations {result.coverage.generations?.join(", ") || "none"}</p>}
    {result.coverage?.uncertainty?.length ? <p>{result.coverage.uncertainty.join(" · ")}</p> : null}
    {result.coverage?.exclusions?.length ? <details><summary>Coverage exclusions</summary><ul>{result.coverage.exclusions.map((e, i) => <li key={i}>{e}</li>)}</ul></details> : null}
    {result.applied_limits && <p>Applied bounds: {Object.entries(result.applied_limits).map(([key, value]) => `${key} ${value}`).join(" · ")}</p>}
  </section>;
}
