import React, {useEffect, useRef, useState} from "react";
import {createRoot} from "react-dom/client";
import "./style.css";
import {MirrorSetup, IndexingCard, type Setup} from "./mirror-setup";
import {NamespaceSettings} from "./namespace-settings";
import {Maintenance} from "./maintenance";
import {InstanceBrand, type Instance} from "./instance-brand";
import {KnowledgeCloud} from "./knowledge-cloud";
import {Spotlight} from "./spotlight";
import {QueryFeedback, type Entity, type Excerpt, type Projection, type QueryResult, type Status} from "./runtime-types";

let csrf = "";
class ApiError extends Error {constructor(message: string, public status: number, public details?: {suggestions?:string[]}) {super(message);}}
const request = async <T,>(path: string, body?: unknown): Promise<T> => {
  const r = await fetch(path, {method: body === undefined ? "GET" : "POST", credentials: "same-origin", headers: body === undefined ? undefined : {"Content-Type": "application/json", "X-CSRF-Token": csrf}, body: body === undefined ? undefined : JSON.stringify(body)});
  if (!r.ok) {const details=await r.json().catch(()=>({error:"Request failed"}));throw new ApiError(details.error,r.status,details);}
  return r.json();
};
const message = (e: unknown) => e instanceof Error ? e.message : "Knowledge unavailable";
function App() {
  const [showSetup, setShowSetup] = useState(false), [spotlight, setSpotlight] = useState(false), [instance, setInstance] = useState<Instance>(), [status, setStatus] = useState<Status>();
  const [repo, setRepo] = useState(""), [projection, setProjection] = useState<Projection>(), [selected, setSelected] = useState<Entity>(), [evidence, setEvidence] = useState<Excerpt>();
  const [query, setQuery] = useState(""), [result, setResult] = useState<QueryResult>(), [notice, setNotice] = useState(""), [mapNotice, setMapNotice] = useState(""), [busy, setBusy] = useState(false);
  const [daemonState, setDaemonState] = useState<"connected" | "stopping" | "disconnected">("connected");
  const [indexing,setIndexing]=useState(false);
  const inspection = useRef(0), loading = useRef(0), searching = useRef(0);
  function clearSelection() {inspection.current++; setSelected(undefined); setEvidence(undefined); searching.current++; setResult(undefined); setBusy(false);}
  async function load(wanted?: string) {
    const id = ++loading.current; clearSelection(); setProjection(undefined); setNotice(""); setMapNotice("");
    try {
      const s = await request<Status>("/api/v1/status"), brand = await request<Instance>("/api/v1/instance");
      if (id !== loading.current) return;
      setStatus(s); setInstance(brand);
      const setup=await request<Setup>("/api/v1/onboarding");if(id!==loading.current)return;
      if(setup.state==="syncing"||setup.state==="ingesting")setIndexing(true);
      const active = s.repositories.filter(r => r.active), next = active.find(r => r.id === (wanted || repo))?.id || active[0]?.id || "";
      setRepo(next);
      if (next) {
        try {const p = await request<Projection>(`/api/v1/projection?repo=${encodeURIComponent(next)}`); if (id === loading.current) setProjection(p);}
        catch (e) {if (id === loading.current) setMapNotice(`Map projection unavailable. ${message(e)}`);}
      }
    } catch (e) {if (id === loading.current) setNotice(message(e));}
  }
  useEffect(() => {
    const token = new URL(location.href).hash.match(/token=([^&]+)/)?.[1]; history.replaceState(null, "", location.pathname);
    request<{csrf_token: string}>("/api/v1/session", token ? {token} : undefined).then(session => {csrf = session.csrf_token; return load();}).catch(() => setNotice("This secure launch link or session has expired. Open a fresh local launch link."));
  }, []);
  useEffect(() => {
    if (!status || daemonState === "disconnected") return;
    const timer = window.setInterval(() => {
      void request<{csrf_token: string}>("/api/v1/session").then(session => {
        csrf = session.csrf_token;
        if (daemonState === "stopping") return request<{state: string}>("/api/v1/daemon/status").then(state => {
          if (state.state === "failed") {setDaemonState("connected"); setNotice("Stop failed. The daemon is still connected. Run aios daemon status and stop from your login terminal.");}
        });
      }).catch(e => {if (e instanceof ApiError) {setNotice("Your session has expired. Open a fresh launch link with aios daemon open.");} else {setDaemonState("disconnected");}});
    }, 2000);
    return () => window.clearInterval(timer);
  }, [status, daemonState]);
  async function stopDaemon() {
    try {await request("/api/v1/daemon/stop", {}); setDaemonState("stopping");}
    catch (e) {setNotice(`Unable to stop daemon: ${message(e)}`);}
  }
  useEffect(() => {
    const key = (e: KeyboardEvent) => {if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k" && status) {e.preventDefault(); setSpotlight(value => !value);}};
    window.addEventListener("keydown", key); return () => window.removeEventListener("keydown", key);
  }, [status]);
  async function inspect(entity: Entity) {
    const id = ++inspection.current; setSelected(undefined); setEvidence(undefined); setNotice("");
    try {
      const fresh = await request<Entity>("/api/v1/entity", {handle: entity.handle});
      const excerpt = await request<Excerpt>("/api/v1/evidence", {evidence: fresh.evidence, before: 2, after: 2, max_lines: 40});
      if (id === inspection.current) {setSelected(fresh); setEvidence(excerpt);}
    } catch (e) {if (id === inspection.current) setNotice(e instanceof ApiError && e.status === 409 ? "Stale selection: the active generation changed or this handle is unknown. Refresh and select current evidence." : `Evidence unavailable: ${message(e)}`);}
  }
  const queryRequest = (text: string) => request<QueryResult>("/api/v1/query", {text, repository: repo, limit: 20});
  async function search(e: React.FormEvent) {
    e.preventDefault(); clearSelection(); const id = ++searching.current; setNotice(""); setBusy(true);
    try {const r = await queryRequest(query); if (id !== searching.current) return; setResult(r); if (r.entities[0]) await inspect(r.entities[0]);}
    catch (e) {if (id === searching.current) setNotice(`Search unavailable: ${message(e)}`);}
    finally {if (id === searching.current) setBusy(false);}
  }
  async function nextPage() {
    if (!projection?.next_cursor) return;
    const id = ++loading.current; clearSelection(); setMapNotice("");
    try {const p = await request<Projection>(`/api/v1/projection?repo=${encodeURIComponent(repo)}&cursor=${encodeURIComponent(projection.next_cursor)}`); if (id === loading.current) setProjection(p);}
    catch (e) {if (id === loading.current) {setProjection(undefined); setMapNotice(`Projection page stale or unavailable. Refresh to reload the active generation. ${message(e)}`);}}
  }
  if (!status) return <main><h1>Repository knowledge</h1><p role={notice ? "alert" : "status"}>{notice || "Opening local knowledge…"}</p></main>;
  const brand = instance && <InstanceBrand instance={instance} onSave={async value => setInstance(await request<Instance>("/api/v1/instance", value))} onGenerate={async()=>{const result=await request<{instance:Instance}>("/api/v1/instance/logo/generate",{seed:crypto.randomUUID()});setInstance(result.instance);return result.instance.logo||""}}/>;
  return <main>{brand}<header><p>Read-only repository knowledge</p><h1>{status.projection_state === "no_active_generation" ? "No active generation" : "Knowledge map"}</h1>
    <button onClick={() => setSpotlight(true)}>Open Spotlight</button><small>⌘ / Ctrl K</small><button onClick={() => void load()}>Refresh</button><button onClick={() => setShowSetup(!showSetup)}>Repository setup</button>
    <button disabled={daemonState !== "connected"} onClick={() => void stopDaemon()}>Stop daemon</button>
  </header>
    {daemonState !== "connected" && <p role="alert">{daemonState === "stopping" ? "Stopping daemon… Waiting for disconnection." : "Daemon disconnected. Saved knowledge and configuration remain on this Mac. Run aios daemon start, then aios daemon open to reconnect."}</p>}
    {status.projection_state === "no_active_generation" && <p>No repository checkout has been changed.</p>}
    {(showSetup || status.projection_state === "no_active_generation") && !indexing && <><NamespaceSettings request={request}/><MirrorSetup request={request} onBuild={()=>{setShowSetup(false);setIndexing(true)}}/></>}
    {indexing && <IndexingCard request={request} onReady={async()=>{await load();setIndexing(false)}} onRetry={()=>{setIndexing(false);setShowSetup(true)}}/>}
    {status.projection_state === "unavailable" && <section><h2>Knowledge is safe</h2><p>Canonical source evidence remains intact</p><p>A derived projection is unavailable. Search will report its own available coverage.</p></section>}
    {notice && <p role="alert">{notice}</p>}{mapNotice && <p role="status">{mapNotice}</p>}
    {daemonState === "connected" && !indexing && <Maintenance request={request} onChanged={()=>void load()}/>}
    {status.repositories.some(r => r.active) && <label>Active repository<select value={repo} onChange={e => void load(e.target.value)}>{status.repositories.filter(r => r.active).map(r => <option key={r.id} value={r.id}>{r.id}</option>)}</select></label>}
    <form onSubmit={search}><label>Query<input value={query} onChange={e => setQuery(e.target.value)}/></label><button disabled={busy}>{busy ? "Searching…" : "Search"}</button></form>
    <QueryFeedback result={result}/>{result?.entities.length ? <ul aria-label="Query results">{result.entities.map(entity => <li key={entity.handle}><button onClick={() => void inspect(entity)}>{entity.label} · {entity.path}</button></li>)}</ul> : null}
    <div className="knowledge-workspace"><div>{projection && <>
      <KnowledgeCloud projection={projection} colour={instance?.seed_colour || "#5865f2"} selected={selected?.handle} inspect={entity => void inspect(entity)}/>
      {projection.next_cursor && <button onClick={() => void nextPage()}>Next projection page</button>}
      <section aria-label="Knowledge graph"><h2>Entities</h2><ul aria-label="Select visible graph node">{projection.nodes.map(entity => <li key={entity.handle}><button onClick={() => void inspect(entity)} aria-pressed={selected?.handle === entity.handle}>{entity.label}</button> <small>{entity.kind} · {entity.confidence.toFixed(2)} confidence · {entity.evidence_count} claims</small></li>)}</ul>
      <h2>Relationships</h2><ul>{projection.edges.map(edge => <li key={edge.handle}>{edge.predicate} · {edge.confidence.toFixed(2)} · {edge.derivation}</li>)}</ul></section>
    </>}</div><aside aria-label="Evidence inspector"><h2>{selected?.label || "Select an entity"}</h2>
      {selected && <p>{selected.repository}/{selected.path} · generation {selected.generation} · lines {selected.span.start_line}–{selected.span.end_line}</p>}
      {evidence && <><p>{evidence.working_tree?"Working-tree snapshot · Git HEAD:":"Captured commit:"} {evidence.git_commit} · SHA256: {evidence.sha256}</p><p>Excerpt generation {evidence.generation} · lines {evidence.start_line}–{evidence.end_line}{evidence.truncated ? " · bounded excerpt" : ""}</p><pre>{evidence.lines.join("\n")}</pre></>}
    </aside></div>
    <Spotlight open={spotlight} close={() => setSpotlight(false)} search={queryRequest} select={inspect}/>
  </main>;
}
createRoot(document.getElementById("root")!).render(<App/>);
