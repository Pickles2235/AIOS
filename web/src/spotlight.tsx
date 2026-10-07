import React, {useEffect, useRef, useState} from "react";
import type {Entity, Investigation} from "./runtime-types";

type Props = {
  open: boolean;
  close: () => void;
  search: (text: string, signal: AbortSignal) => Promise<Investigation>;
  history: () => Promise<string[]>;
  clearHistory: () => Promise<void>;
  validate: (result: Investigation) => Promise<boolean>;
  select: (entity: Entity) => Promise<void>;
};

export function Spotlight({open, close, search, history, clearHistory, validate, select}: Props) {
  const dialog = useRef<HTMLDialogElement>(null), input = useRef<HTMLInputElement>(null), controller = useRef<AbortController>();
  const requestID = useRef(0), historyIndex = useRef(-1);
  const [text, setText] = useState(""), [result, setResult] = useState<Investigation>(), [entries, setEntries] = useState<string[]>([]);
  const [error, setError] = useState(""), [busy, setBusy] = useState(false), [index, setIndex] = useState(0), [copied, setCopied] = useState(false), [stale, setStale] = useState(false);
  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    setResult(undefined); setCopied(false); setStale(false); setError(""); historyIndex.current = -1;
    dialog.current?.showModal(); input.current?.focus();
    history().then(setEntries).catch(() => setError("Local history unavailable"));
    return () => {requestID.current++; controller.current?.abort(); setBusy(false); dialog.current?.close(); opener?.focus();};
  }, [open]);
  useEffect(() => {
    if (!open || !result) return;
    let active = true;
    const check = () => {void validate(result).then(current => {if (active && !current) setStale(true);}).catch(() => {if (active) setStale(true);});};
    const timer = window.setInterval(check, 2000);
    check();
    return () => {active = false; window.clearInterval(timer);};
  }, [open, result]);
  function cancel() {requestID.current++; controller.current?.abort(); setBusy(false); setError("Search cancelled"); input.current?.focus();}
  async function submit(e: React.FormEvent) {
    e.preventDefault(); controller.current?.abort(); const id = ++requestID.current; const abort = new AbortController(); controller.current = abort;
    setBusy(true); setError(""); setResult(undefined); setCopied(false);
    try {
      const value = await search(text, abort.signal);
      if (id === requestID.current) {setResult(value); setStale(false); setIndex(0); historyIndex.current = -1; setEntries(await history());}
    } catch (e) {if (id === requestID.current && !abort.signal.aborted) setError(e instanceof Error ? e.message : "Search unavailable");}
    finally {if (id === requestID.current) setBusy(false);}
  }
  async function choose(entity: Entity) {if (!result || stale || !(await validate(result).catch(() => false))) {setStale(true); return;} close(); await select(entity);}
  async function copy() {
    if (!result || stale || !(await validate(result).catch(() => false))) {setStale(true); return;}
    try {await navigator.clipboard.writeText(JSON.stringify(result, null, 2)); setCopied(true);}
    catch {setError("Clipboard unavailable; copy the evidence shown below manually.");}
  }
  async function clear() {
    try {await clearHistory(); setEntries([]); historyIndex.current = -1; input.current?.focus();}
    catch {setError("Cannot clear local history");}
  }
  return <dialog ref={dialog} aria-labelledby="spotlight-title" onCancel={e => {e.preventDefault(); close();}}>
    <header><h2 id="spotlight-title">Search knowledge</h2><button type="button" onClick={close}>Close Spotlight</button></header>
    <form onSubmit={submit}><label>Spotlight query<input ref={input} value={text} role="combobox" aria-expanded={Boolean(result?.findings.length)} aria-controls="spotlight-results" aria-autocomplete="list" aria-activedescendant={result?.findings[index] ? `spotlight-result-${index}` : undefined}
      onChange={e => {setText(e.target.value); setResult(undefined); setStale(false); requestID.current++; controller.current?.abort(); setBusy(false); setCopied(false); historyIndex.current = -1;}}
      onKeyDown={e => {
        const length = result?.findings.length || 0;
        if (length && (e.key === "ArrowDown" || e.key === "ArrowUp")) {e.preventDefault(); setIndex(i => (i + (e.key === "ArrowDown" ? 1 : length - 1)) % length);}
        else if (!length && entries.length && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
          e.preventDefault(); historyIndex.current = e.key === "ArrowUp" ? Math.min(historyIndex.current + 1, entries.length - 1) : Math.max(historyIndex.current - 1, -1);
          setText(historyIndex.current < 0 ? "" : entries[historyIndex.current]);
        }
        if (length && e.key === "Enter") {e.preventDefault(); void choose(result!.findings[index].entity);}
        if (e.key === "Escape") {e.preventDefault(); close();}
      }}/></label><button disabled={busy}>{busy ? "Searching…" : "Search captured evidence"}</button>
      {busy && <button type="button" onClick={cancel}>Cancel search</button>}</form>
    <p>Commands: /question, /symbol, /path, /log, /event, /route, /config. Add @repository to filter. Search uses captured evidence; causal depth needs source inspection.</p>
    <section aria-label="Search history"><h3>Recent searches</h3><button type="button" onClick={() => void clear()} disabled={!entries.length}>Clear history</button>
      <ul>{entries.map(entry => <li key={entry}><button type="button" onClick={() => {setText(entry); setResult(undefined); input.current?.focus();}}>{entry}</button></li>)}</ul></section>
    {error && <p role="alert">{error}</p>}
    {stale && <p role="alert">Stale result: an active source generation changed. Search again for current evidence.</p>}
    {result && <section aria-label="Investigation result">
      <p role="status">{result.status === "found" ? `Found ${result.findings.length} cited result${result.findings.length === 1 ? "" : "s"}.` : result.status === "not_found" ? "Not found within complete applicable coverage." : "Unknown: available evidence cannot establish an answer."}{result.truncated ? " Results or excerpts are truncated; more may exist." : ""}</p>
      <p>Intent {result.intent} · generations {result.generations.join(", ") || "none"} · coverage {result.coverage?.complete ? "complete" : "incomplete or unavailable"}</p>
      {result.search_term && result.search_term !== result.query && <p>Searched captured evidence for: {result.search_term}</p>}
      {result.freshness.map(source => <p key={source.id}>{source.id}: active generation {source.generation} · captured revision {source.revision}</p>)}
      {result.coverage?.uncertainty?.length ? <p>Coverage uncertainty: {result.coverage.uncertainty.join(" · ")}</p> : null}
      {result.unknowns.length ? <p>Unknowns: {result.unknowns.join(" · ")}</p> : null}
      <p>Bounds: {Object.entries(result.budget).map(([name, value]) => `${name} ${value}`).join(" · ")}</p>
      {result.relationships.length ? <details><summary>Cited relationships</summary><ul>{result.relationships.map((claim, i) => <li key={i}>{claim.predicate} · {claim.subject} → {claim.object} · evidence {claim.evidence}</li>)}</ul></details> : null}
      <button type="button" disabled={stale} onClick={() => void copy()}>Copy investigation payload</button>{copied && <span role="status">Copied the displayed version 1 evidence payload.</span>}
    </section>}
    <ul id="spotlight-results" role="listbox" aria-label="Spotlight results">{result?.findings.map(({entity, evidence}, i) => {
      const excerpt = result.canonical_evidence.find(item => item.evidence === evidence);
      return <li key={entity.handle} id={`spotlight-result-${i}`} role="option" aria-selected={index === i}>
        <button type="button" disabled={stale} onClick={() => void choose(entity)} onFocus={() => setIndex(i)}>{entity.label} · {entity.repository}/{entity.path}:{entity.span.start_line}</button>
        <small>generation {entity.generation} · {entity.confidence.toFixed(2)} confidence · canonical evidence {evidence}</small>
        {excerpt && <pre>{excerpt.lines.join("\n")}</pre>}
      </li>;
    })}</ul><p>↑ ↓ select or browse history · Enter opens evidence · Escape closes</p>
  </dialog>;
}
