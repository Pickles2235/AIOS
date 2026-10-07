import React, { useEffect, useRef, useState } from "react";
import { Maintenance } from "./maintenance";
import type { Investigation, QueryResult, Status } from "./runtime-types";
import type { Setup } from "./mirror-setup";

type Job = { repository: string; mode: string; state: string; stale: boolean; error?: string; watch_error?: string; retention_warning?: string; deferred_reason?: string; next_attempt?: string; next_poll?: string; active_generation?: string };
type Jobs = { jobs: Job[]; durable: boolean; persistence_error?: string; mirror_interval_seconds: number };
type Resources = { state: string; deferred_reason?: string; power_source: string; signal_provider?: string; available?: boolean; load: number; idle_seconds: number; queue_depth: number; running: number; oldest_job_age_seconds: number; oldest_job_max_wait_seconds: number; max_workers: number; max_queue: number; retention_bytes: number; available_bytes: number; owned_bytes: number; max_owned_bytes: number; storage_state: string; observed_at: string };
export type ActivityEvent = { sequence: number; at: string; stage: string; repository: string; generation?: string; files: number; queryable: boolean };
export type Activity = { sequence: number; stream_id: string; oldest_sequence: number; retention_events: number; events: ActivityEvent[] };
export type ActivityView = { cursor?: number; stream?: string; events: ActivityEvent[]; warning?: string };
const ids = ["indexing", "health", "storage", "performance", "coverage", "jobs", "query"] as const;
type ID = typeof ids[number];
const title: Record<ID, string> = { indexing: "Indexing", health: "Health", storage: "Storage and size", performance: "Performance", coverage: "Coverage", jobs: "Scheduled jobs", query: "Active query" };
const storageKey = "aios.contextual-placement.v1";

// A full retained snapshot is authoritative. It can still omit events older than
// its retention window, so disclose a gap instead of manufacturing progress.
export function reconcileActivity(previous: ActivityView, snapshot: Activity): ActivityView {
  if (!Number.isSafeInteger(snapshot.sequence) || snapshot.sequence < 0 || !snapshot.stream_id || !Array.isArray(snapshot.events)) throw new Error("Activity stream unavailable");
  const sorted = [...snapshot.events].sort((a, b) => a.sequence - b.sequence);
  if (sorted.some((event, i) => !Number.isSafeInteger(event.sequence) || event.sequence <= 0 || event.sequence > snapshot.sequence || (i > 0 && event.sequence === sorted[i - 1].sequence))) throw new Error("Activity event sequence invalid");
  if (snapshot.oldest_sequence !== (sorted[0]?.sequence ?? snapshot.sequence + 1)) throw new Error("Activity retention boundary invalid");
  const restarted = previous.cursor !== undefined && ((!!previous.stream && previous.stream !== snapshot.stream_id) || snapshot.sequence < previous.cursor);
  let warning = previous.warning;
  if (previous.cursor === undefined && snapshot.oldest_sequence > 1) warning = `Earlier activity is outside the retained history (starts at event ${snapshot.oldest_sequence}). Current jobs and indexing state were rechecked.`;
  if (restarted) warning = "Activity restarted. Current jobs and indexing state were rechecked; earlier event history may be unavailable.";
  else if (previous.cursor !== undefined && snapshot.sequence > previous.cursor && snapshot.oldest_sequence > previous.cursor + 1) warning = `Activity history has a gap before event ${snapshot.oldest_sequence}. Current jobs and indexing state were rechecked.`;
  else if (previous.cursor !== undefined && snapshot.sequence > previous.cursor && !sorted.length) warning = "Activity history is unavailable. Current jobs and indexing state were rechecked.";
  else if (sorted.some((event, i) => i > 0 && event.sequence !== sorted[i - 1].sequence + 1)) warning = "Activity history has a sequence gap. Current jobs and indexing state were rechecked.";
  else if (sorted.length && sorted[sorted.length - 1].sequence < snapshot.sequence) warning = "Recent activity history is incomplete. Current jobs and indexing state were rechecked.";
  const newer = previous.cursor === undefined || restarted ? sorted : sorted.filter(event => event.sequence > previous.cursor!);
  const events = restarted ? newer : [...previous.events, ...newer];
  return { cursor: snapshot.sequence, stream: snapshot.stream_id, events: events.slice(-12), warning };
}

export function clampPosition(position: { x: number; y: number }, size: { width: number; height: number }, viewport: { width: number; height: number }) {
  const margin = 12;
  return { x: Math.max(margin, Math.min(position.x, Math.max(margin, viewport.width - size.width - margin))), y: Math.max(margin, Math.min(position.y, Math.max(margin, viewport.height - size.height - margin))) };
}
function savedPositions(): Partial<Record<ID, { x: number; y: number }>> {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey) || "{}");
    if (!value || typeof value !== "object") return {};
    return Object.fromEntries(ids.filter(id => Number.isFinite(value[id]?.x) && Number.isFinite(value[id]?.y)).map(id => [id, { x: value[id].x, y: value[id].y }]));
  } catch { return {}; }
}
function savePosition(id: ID, position?: { x: number; y: number }) {
  try {
    const saved = savedPositions();
    if (position) saved[id] = position; else delete saved[id];
    localStorage.setItem(storageKey, JSON.stringify(saved));
  } catch { /* private browser storage may be unavailable */ }
}
function Floating({ id, children, warning }: { id: ID; children: React.ReactNode; warning?: boolean }) {
  const ref = useRef<HTMLElement>(null);
  const [position, setPosition] = useState<{ x: number; y: number } | undefined>(() => savedPositions()[id]);
  const drag = useRef<{ x: number; y: number; origin: { x: number; y: number } }>();
  const constrain = (candidate: { x: number; y: number }) => clampPosition(candidate, { width: ref.current?.offsetWidth || 300, height: ref.current?.offsetHeight || 140 }, { width: window.innerWidth, height: window.innerHeight });
  const move = (candidate: { x: number; y: number }) => setPosition(constrain(candidate));
  useEffect(() => {
    const resize = () => setPosition(current => current ? constrain(current) : undefined);
    resize();
    window.addEventListener("resize", resize);
    const observer = new ResizeObserver(resize);
    if (ref.current) observer.observe(ref.current);
    return () => { window.removeEventListener("resize", resize); observer.disconnect(); };
  }, []);
  useEffect(() => { savePosition(id, position); }, [id, position?.x, position?.y]);
  function reset() { setPosition(undefined); savePosition(id); }
  return <section ref={ref} className="context-module" aria-label={`${title[id]} module`} data-context-module={id} data-warning={warning ? "true" : undefined} data-detached={position ? "true" : undefined} style={position ? { left: position.x, top: position.y } : undefined}>
    <header className="context-module-header" onPointerDown={event => { if ((event.target as HTMLElement).closest("button")) return; event.currentTarget.setPointerCapture(event.pointerId); const rect = ref.current?.getBoundingClientRect(); drag.current = { x: event.clientX, y: event.clientY, origin: position || { x: rect?.left || 12, y: rect?.top || 12 } }; }} onPointerMove={event => { if (drag.current) move({ x: drag.current.origin.x + event.clientX - drag.current.x, y: drag.current.origin.y + event.clientY - drag.current.y }); }} onPointerUp={() => { drag.current = undefined; }} onPointerCancel={() => { drag.current = undefined; }}>
      <strong>{title[id]}</strong><button type="button" aria-label={`Move ${title[id]} module`} aria-describedby={`move-hint-${id}`} onClick={event => event.currentTarget.focus()} onKeyDown={event => { const step = event.shiftKey ? 40 : 12; const delta = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] }[event.key] as number[] | undefined; const rect = ref.current?.getBoundingClientRect(); const origin = position || { x: rect?.left || 12, y: rect?.top || 12 }; if (delta) { event.preventDefault(); move({ x: origin.x + delta[0], y: origin.y + delta[1] }); } else if (event.key === "Home") { event.preventDefault(); reset(); } }}>Move</button><button type="button" onClick={reset}>Reset position</button><span id={`move-hint-${id}`} className="sr-only">Use arrow keys to move; Shift plus arrow moves faster; Home returns to the dock.</span>
    </header>{children}
  </section>;
}

export function ContextualModules({ request, status, setupRunning, result, queryBusy, queryError, spotlight, onChanged, onActivity }: { request: <T>(path: string, body?: unknown) => Promise<T>; status: Status; setupRunning: boolean; result?: QueryResult; queryBusy: boolean; queryError: string; spotlight?: { open: boolean; busy: boolean; result?: Investigation; stale: boolean; error: string }; onChanged: () => void; onActivity: (state: ActivityView) => void }) {
  const [jobs, setJobs] = useState<Jobs>(), [resources, setResources] = useState<Resources>(), [onboarding, setOnboarding] = useState<Setup>(), [activity, setActivity] = useState<ActivityView>({ events: [] }), [error, setError] = useState(""), [manual, setManual] = useState<ID[]>([]);
  const [placementReset, setPlacementReset] = useState(0);
  const activityRef = useRef<ActivityView>({ events: [] });
  const generationSignature = useRef<string>();
  const changed = useRef(onChanged); changed.current = onChanged;
  const activityChanged = useRef(onActivity); activityChanged.current = onActivity;
  useEffect(() => {
    let live = true, inFlight = false;
    async function poll() {
      if (inFlight) return;
      inFlight = true;
      try {
        const [nextJobs, nextResources, nextSetup, nextActivity] = await Promise.all([request<Jobs>("/api/v1/jobs"), request<Resources>("/api/v1/resources"), request<Setup>("/api/v1/onboarding"), request<Activity>("/api/v1/activity")]);
        if (!live) return;
        if (!Array.isArray(nextJobs.jobs)) throw new Error("Repository health response unavailable");
        activityRef.current = reconcileActivity(activityRef.current, nextActivity);
        setJobs(nextJobs); setResources(nextResources); setOnboarding(nextSetup); setActivity(activityRef.current); setError("");
        activityChanged.current(activityRef.current);
        const signature = nextJobs.jobs.map(job => `${job.repository}:${job.active_generation || ""}`).sort().join("|");
        if (generationSignature.current !== undefined && generationSignature.current !== signature) changed.current();
        generationSignature.current = signature;
      } catch (cause) { if (live) { const detail = cause instanceof Error ? cause.message : "Operational state unavailable"; setError(detail); activityChanged.current({ ...activityRef.current, warning: [activityRef.current.warning, `Operational snapshot unavailable: ${detail}. Last observed activity may be stale.`].filter(Boolean).join(" ") }); } }
      finally { inFlight = false; }
    }
    void poll(); const timer = window.setInterval(() => void poll(), 1000);
    return () => { live = false; window.clearInterval(timer); };
  }, [request]);
  const actualSetup = onboarding;
  const running = setupRunning || actualSetup?.state === "syncing" || actualSetup?.state === "ingesting";
  const unhealthy = jobs?.jobs.filter(job => job.stale || job.error || job.watch_error || job.retention_warning || job.state === "exhausted") || [];
  const journalUnavailable = !!jobs?.jobs.length && jobs?.durable === false;
  const storageWarning = resources && resources.storage_state !== "available";
  const warnings: Partial<Record<ID, boolean>> = { indexing: !!actualSetup?.error || !!activity.warning || !!error, health: !!jobs?.persistence_error || journalUnavailable || unhealthy.length > 0, storage: !!storageWarning, coverage: status.projection_state === "unavailable", jobs: journalUnavailable || !!jobs?.jobs.some(job => job.state === "exhausted"), query: !!queryError || !!spotlight?.stale || !!spotlight?.error };
  const recent = activity.events.filter(event => Date.now() - Date.parse(event.at) < 30000 && Date.now() >= Date.parse(event.at));
  const relevant: Partial<Record<ID, boolean>> = { indexing: !!running || recent.length > 0, health: !!warnings.health, storage: !!warnings.storage, performance: resources?.state === "constrained" || !!resources?.deferred_reason, coverage: status.projection_state === "unavailable" || result?.coverage?.complete === false || spotlight?.result?.coverage?.complete === false, jobs: !!jobs?.jobs.some(job => ["pending", "running", "retry_wait"].includes(job.state)), query: queryBusy || !!spotlight?.busy || (!!result && (result.status === "unknown" || result.truncated)) || (!!spotlight?.result && (spotlight.result.status === "unknown" || spotlight.result.truncated)) };
  const visible = ids.filter(id => manual.includes(id) || relevant[id] || warnings[id]);
  const toggle = (id: ID) => setManual(current => current.includes(id) ? current.filter(value => value !== id) : [...current, id]);
  const gib = (value: number) => Number.isFinite(value) ? (value / (1024 ** 3)).toFixed(2) : "unknown";
  const acknowledgeActivity = () => { activityRef.current = { ...activityRef.current, warning: undefined }; setActivity(activityRef.current); activityChanged.current(activityRef.current); };
  return <>
    <nav className="context-controls" aria-label="Operational modules"><span>Operations</span>{ids.map(id => <button key={id} type="button" aria-pressed={visible.includes(id)} onClick={() => toggle(id)}>{title[id]}{warnings[id] ? " !" : ""}</button>)}<button type="button" onClick={() => { try { localStorage.removeItem(storageKey); } catch { /* private browser storage may be unavailable */ } setPlacementReset(value => value + 1); }}>Reset all module positions</button></nav>
    <aside className="context-dock" aria-label="Visible operational modules">{visible.map(id => <Floating key={`${id}-${placementReset}`} id={id} warning={warnings[id]}>
      {id === "indexing" && <>{error && <p role="alert">Operational snapshot unavailable: {error}. Last observed data may be stale.</p>}{actualSetup?.error && <p role="alert">Setup: {actualSetup.error}</p>}{activity.warning && <><p role="alert">{activity.warning}</p><button type="button" onClick={acknowledgeActivity}>Acknowledge activity history notice</button></>}<p role="status">{running ? `${actualSetup?.state || "Indexing"} · ${actualSetup?.completed_repositories ?? 0} repositories completed` : "No active indexing"}</p><p>Events are observations; staged evidence is not queryable before promotion.</p><ol>{activity.events.slice(-5).map(event => <li key={event.sequence}>#{event.sequence} {event.repository} · {event.stage}{event.files ? ` · ${event.files} observed files` : ""} · {event.queryable ? "promotion reported" : "not queryable"}</li>)}</ol></>}
      {id === "health" && <><p role={warnings.health ? "alert" : "status"}>{jobs?.persistence_error || (journalUnavailable ? "Maintenance journal unavailable" : unhealthy.length ? `${unhealthy.length} repositories need attention` : "No reported repository warnings")}</p>{unhealthy.map(job => <p key={job.repository}>{job.repository}: {job.error || job.watch_error || job.retention_warning || (job.stale ? "Knowledge may be stale" : job.state)}</p>)}<Maintenance request={request} onChanged={onChanged} /></>}
      {id === "storage" && <><p role={storageWarning ? "alert" : "status"}>Storage {resources?.storage_state || "unavailable"}</p><p>Owned data {resources ? gib(resources.owned_bytes) : "unknown"} GiB of {resources ? gib(resources.max_owned_bytes) : "unknown"} GiB admission budget.</p><p>Available volume space {resources ? gib(resources.available_bytes) : "unknown"} GiB · maintenance journal cap {resources ? Math.round(resources.retention_bytes / 1024) : "unknown"} KiB.</p></>}
      {id === "performance" && <><p>{resources ? `${resources.state.replaceAll("_", " ")} · ${resources.power_source} power` : "Resource measurements unavailable"}</p>{resources?.deferred_reason && <p role="status">Deferred: {resources.deferred_reason}</p>}<p>Observed load {resources?.available ? resources.load.toFixed(2) : "unavailable"} · {resources?.running ?? "?"} running · {resources?.queue_depth ?? "?"} queued.</p><p>Local resource sample {resources?.observed_at || "unavailable"}. Query latency is not inferred from system load.</p></>}
      {id === "coverage" && <><p role={warnings.coverage ? "alert" : "status"}>Projection {status.projection_state} · {status.repositories.filter(repo => repo.active).length} active of {status.repositories.length} configured repositories.</p><p>These counts do not prove complete query coverage.</p>{result?.coverage && <p>Last direct query: {result.coverage.complete ? "complete within its applicable scope" : "incomplete or uncertain"} · {result.coverage.generations?.length || 0} generations.{result.coverage.uncertainty?.length ? ` ${result.coverage.uncertainty.join(" · ")}` : ""}</p>}{spotlight?.result?.coverage && <p>Last Spotlight query: {spotlight.result.coverage.complete ? "complete within its applicable scope" : "incomplete or uncertain"} · {spotlight.result.coverage.generations?.length || 0} generations.</p>}</>}
      {id === "jobs" && <><p>{jobs ? `${jobs.jobs.length} configured repository jobs · ${jobs.durable ? "durable journal" : "journal unavailable"}` : "Job state unavailable"}</p><ul>{jobs?.jobs.map(job => <li key={job.repository}>{job.repository} · {job.state}{job.deferred_reason ? ` · deferred: ${job.deferred_reason}` : ""}{job.next_attempt || job.next_poll ? ` · next ${new Date(job.next_attempt || job.next_poll!).toLocaleString()}` : ""}</li>)}</ul></>}
      {id === "query" && <>{queryError && <p role="alert">Direct query: {queryError}</p>}{spotlight?.stale && <p role="alert">Spotlight result is stale; search again for current evidence.</p>}{spotlight?.error && <p role="alert">Spotlight: {spotlight.error}</p>}<p role="status">{spotlight?.busy ? "Spotlight query in progress; results are pending." : queryBusy ? "Query in progress; results are pending." : spotlight?.result ? `Spotlight result: ${spotlight.result.status}${spotlight.result.truncated ? " · bounded results" : ""}` : result ? `Last direct query: ${result.status}${result.truncated ? " · bounded results" : ""}` : "No active query"}</p>{(spotlight?.result?.coverage || result?.coverage) && <p>Coverage {(spotlight?.result?.coverage || result?.coverage)?.complete ? "complete for this query" : "incomplete"} · generations {(spotlight?.result?.coverage || result?.coverage)?.generations?.join(", ") || "none"}</p>}<p>Open the query view for cited source evidence.</p></>}
    </Floating>)}</aside>
  </>;
}
