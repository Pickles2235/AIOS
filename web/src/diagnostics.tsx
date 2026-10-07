import React, {useState} from "react";

type Details = {
  schema_version: number;
  product_version: string;
  go_version: string;
  knowledge_ir_format: string;
  projection_state?: string;
  coverage: {configured_repositories: number; active_repositories: number};
  stages: Record<string, number>;
  timings_ms: Record<string, number>;
  upgrade_activity?: Record<string, number>;
  otel: {available: boolean; local_only: boolean; correlation: boolean; retention_bytes: number; operations: Record<string, number>; dropped: number};
};

export function Diagnostics({request, csrfToken}: {request:<T,>(path:string)=>Promise<T>; csrfToken:()=>string}) {
  const [open, setOpen] = useState(false);
  const [details, setDetails] = useState<Details>();
  const [expanded, setExpanded] = useState(false);
  const [acknowledge, setAcknowledge] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  async function show() {
    if (open) {setOpen(false); return;}
    setOpen(true); setNotice("");
    try {setDetails(await request<Details>("/api/v1/diagnostics/details"));}
    catch {setNotice("Local diagnostics are unavailable. Check the owned data directory and retry.");}
  }
  async function download() {
    setBusy(true); setNotice("");
    try {
      const response = await fetch("/api/v1/diagnostics/export", {method:"POST", credentials:"same-origin", headers:{"Content-Type":"application/json", "X-CSRF-Token":csrfToken()}, body:JSON.stringify({expanded, acknowledge_warning: expanded && acknowledge})});
      if (!response.ok) throw new Error("export failed");
      const blob = await response.blob();
      if (blob.size > (1<<20)) throw new Error("archive exceeds limit");
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a"); link.href=url; link.download="aios-diagnostics.zip"; link.click();
      window.setTimeout(()=>URL.revokeObjectURL(url), 1000);
      setNotice("Diagnostic archive downloaded to this browser.");
    } catch {setNotice("Diagnostic export failed. Retry after checking local storage health.");}
    finally {setBusy(false);}
  }
  return <section aria-label="Local diagnostics"><button type="button" onClick={()=>void show()} aria-expanded={open}>Diagnostics</button>
    {open && <div><h2>Local diagnostics</h2><p>Telemetry stays on this Mac. Diagnostic archives exclude source text, query text, credentials, paths and remotes.</p>
      {details && <><p>Product {details.product_version} · {details.knowledge_ir_format} · {details.go_version}</p>
        <p>Coverage: {details.coverage.active_repositories} active of {details.coverage.configured_repositories} configured repositories. Projection: {details.projection_state || "unavailable"}.</p>
        <p>Local telemetry: {details.otel.available ? "available" : "unavailable"} · bounded to {details.otel.retention_bytes} bytes · {details.otel.dropped} dropped writes.</p>
        <details><summary>Operational stages and timings</summary><pre>{JSON.stringify({stages:details.stages, timings_ms:details.timings_ms, operations:details.otel.operations, upgrade_activity:details.upgrade_activity || {}} ,null,2)}</pre></details></>}
      <label><input type="checkbox" checked={expanded} onChange={event=>{setExpanded(event.target.checked);setAcknowledge(false)}}/> Include expanded developer diagnostics</label>
      {expanded && <div role="alert"><p>Expanded diagnostics include individual redacted operation spans and timing. Review the archive before sharing it.</p>
        <label><input type="checkbox" checked={acknowledge} onChange={event=>setAcknowledge(event.target.checked)}/> I understand the expanded diagnostic warning</label></div>}
      <button type="button" disabled={busy || !details?.otel.available || (expanded && !acknowledge)} onClick={()=>void download()}>{busy ? "Exporting…" : "Download diagnostic archive"}</button>
      {notice && <p role="status">{notice}</p>}
    </div>}
  </section>;
}
