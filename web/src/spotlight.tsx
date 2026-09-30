import React, {useEffect, useRef, useState} from "react";
import {QueryFeedback, type Entity, type QueryResult} from "./runtime-types";

export function Spotlight({open, close, search, select}: {open: boolean; close: () => void; search: (text: string) => Promise<QueryResult>; select: (entity: Entity) => Promise<void>}) {
  const dialog = useRef<HTMLDialogElement>(null), input = useRef<HTMLInputElement>(null), requestID = useRef(0);
  const [text, setText] = useState(""), [result, setResult] = useState<QueryResult>(), [error, setError] = useState(""), [busy, setBusy] = useState(false), [index, setIndex] = useState(0);
  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    dialog.current?.showModal(); input.current?.focus();
    return () => {requestID.current++; setBusy(false); dialog.current?.close(); opener?.focus();};
  }, [open]);
  async function submit(e: React.FormEvent) {
    e.preventDefault(); const id = ++requestID.current; setBusy(true); setError(""); setResult(undefined);
    try {const value = await search(text); if (id === requestID.current) {setResult(value); setIndex(0);}}
    catch (e) {if (id === requestID.current) setError(e instanceof Error ? e.message : "Search unavailable");}
    finally {if (id === requestID.current) setBusy(false);}
  }
  async function choose(entity: Entity) {close(); await select(entity);}
  return <dialog ref={dialog} aria-labelledby="spotlight-title" onCancel={e => {e.preventDefault(); close();}}>
    <header><h2 id="spotlight-title">Search knowledge</h2><button onClick={close}>Close Spotlight</button></header>
    <form onSubmit={submit}><label>Spotlight query<input ref={input} value={text} role="combobox" aria-expanded={Boolean(result?.entities.length)} aria-controls="spotlight-results" aria-autocomplete="list" aria-activedescendant={result?.entities[index] ? `spotlight-result-${index}` : undefined}
      onChange={e => {setText(e.target.value); setResult(undefined); requestID.current++; setBusy(false);}}
      onKeyDown={e => {
        const length = result?.entities.length || 0;
        if (length && (e.key === "ArrowDown" || e.key === "ArrowUp")) {e.preventDefault(); setIndex(i => (i + (e.key === "ArrowDown" ? 1 : length - 1)) % length);}
        if (length && e.key === "Enter") {e.preventDefault(); void choose(result!.entities[index]);}
      }}/></label><button disabled={busy}>{busy ? "Searching…" : "Search captured evidence"}</button></form>
    {error && <p role="alert">{error}</p>}<QueryFeedback result={result}/>
    <ul id="spotlight-results" role="listbox" aria-label="Spotlight results">{result?.entities.map((entity, i) => <li key={entity.handle} id={`spotlight-result-${i}`} role="option" aria-selected={index === i} onClick={() => void choose(entity)}>
      <button onFocus={() => setIndex(i)}>{entity.label} · {entity.repository}/{entity.path}</button><small>generation {entity.generation} · {entity.confidence.toFixed(2)} confidence</small>
    </li>)}</ul><p>↑ ↓ select · Enter opens evidence · Escape closes</p>
  </dialog>;
}
