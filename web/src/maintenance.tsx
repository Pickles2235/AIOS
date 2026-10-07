import React, {useEffect, useRef, useState} from "react";

type Job = {repository:string;mode:string;state:string;last_success:string;next_attempt:string;next_poll:string;active_revision:string;active_generation:string;attempted_revision:string;changed_files:number;attempts:number;stale:boolean;error?:string;watch_error?:string;deferred_reason?:string;retention_warning?:string};
type Jobs = {jobs:Job[];mirror_interval_seconds:number;durable:boolean;persistence_error?:string};
type Resources = {state:"normal"|"constrained"|"idle_opportunity";deferred_reason?:string;power_source:string;load:number;idle_seconds:number;queue_depth:number;running:number;oldest_job_age_seconds:number;oldest_job_max_wait_seconds:number;max_workers:number;max_queue:number;retention_bytes:number;available_bytes:number;owned_bytes:number;max_owned_bytes:number;storage_state:string;observed_at:string};
const date = (value:string) => value && !value.startsWith("0001-") ? new Date(value).toLocaleString() : "None yet";
export function Maintenance({request,onChanged}:{request:<T>(path:string,body?:unknown)=>Promise<T>;onChanged:()=>void}) {
  const [status,setStatus]=useState<Jobs>(),[resources,setResources]=useState<Resources>(),[error,setError]=useState(""),[interval,setIntervalValue]=useState("900"),[busy,setBusy]=useState("");
  const generations=useRef<string>(),changed=useRef(onChanged);changed.current=onChanged;
  useEffect(()=>{let live=true;let inFlight=false;
    async function poll(){if(inFlight)return;inFlight=true;try{const [next,policy]=await Promise.all([request<Jobs>("/api/v1/jobs"),request<Resources>("/api/v1/resources")]);if(!Array.isArray(next.jobs))throw new Error("Repository health response is unavailable");if(!live)return;setStatus(next);setResources(policy);setError("");
      const signature=next.jobs.map(j=>j.active_generation).join(":");if(generations.current!==undefined&&signature!==generations.current)changed.current();generations.current=signature;
    }catch(e){if(live)setError(e instanceof Error?e.message:"Repository health unavailable")}finally{inFlight=false}}
    void poll();const timer=window.setInterval(()=>void poll(),2000);return()=>{live=false;window.clearInterval(timer)};
  },[request]);
  useEffect(()=>{if(status)setIntervalValue(String(status.mirror_interval_seconds))},[status?.mirror_interval_seconds]);
  async function check(repository:string){setBusy(repository);setError("");try{await request("/api/v1/repositories/check-now",{repository});setStatus(await request<Jobs>("/api/v1/jobs"))}catch(e){setError(e instanceof Error?e.message:"Unable to check repository")}finally{setBusy("")}}
  async function cancel(repository:string){setBusy(repository);setError("");try{await request("/api/v1/jobs/cancel",{repository});setStatus(await request<Jobs>("/api/v1/jobs"))}catch(e){setError(e instanceof Error?e.message:"Unable to cancel repository update")}finally{setBusy("")}}
  async function configure(e:React.FormEvent){e.preventDefault();setBusy("polling");setError("");try{setStatus(await request<Jobs>("/api/v1/jobs/configure",{mirror_interval_seconds:Number(interval)}))}catch(e){setError(e instanceof Error?e.message:"Unable to save polling interval")}finally{setBusy("")}}
  const warning=status?.persistence_error||error;
  if(!status?.jobs.length&&!warning)return null;
  const unhealthy=status?.jobs.filter(j=>j.stale||j.error||j.watch_error||j.retention_warning)||[],updating=status?.jobs.filter(j=>j.state==="pending"||j.state==="running").length||0;
  return <section aria-label="Repository maintenance">
    {warning&&<p role="alert">{warning}</p>}
    {unhealthy.map(j=><p role="status" key={j.repository}>{j.repository}: {j.error||j.watch_error||j.retention_warning||(j.active_generation?"Knowledge may be stale.":"Waiting for the first successful build.")} {j.active_generation&&"The last successful knowledge remains available."}</p>)}
    <details><summary>Repository health · {updating?`${updating} updating`:unhealthy.length?`${unhealthy.length} need attention`:"Up to date"}</summary>
      {resources&&<section aria-label="Resource policy"><h3>Background resources</h3><p role="status">{resources.state.replaceAll("_"," ")}{resources.deferred_reason?` · deferred: ${resources.deferred_reason}`:""} · {resources.power_source} power · storage {resources.storage_state}</p><p>{resources.running} running · {resources.queue_depth} queued · oldest waiting {Math.round(resources.oldest_job_age_seconds)}s of {Math.round(resources.oldest_job_max_wait_seconds)}s maximum defer</p><p>Workers: {resources.max_workers} maximum; queue: {resources.max_queue} maximum; maintenance journal: {Math.round(resources.retention_bytes/1024)} KiB maximum; owned data: {Math.round(resources.owned_bytes/(1024**3))} of {Math.round(resources.max_owned_bytes/(1024**3))} GiB admission budget; available volume space: {Math.round(resources.available_bytes/(1024**3))} GiB.</p></section>}
      {status?.jobs.some(j=>j.mode==="mirror")&&<form onSubmit={configure}><label>Mirror check interval (seconds)<input type="number" min={10} max={86400} required value={interval} onChange={e=>setIntervalValue(e.target.value)}/></label><button disabled={busy!==""}>Save check interval</button></form>}
      {status?.jobs.map(j=><article aria-label={`Health for ${j.repository}`} key={j.repository}><h3>{j.repository}</h3><p>{j.mode==="direct"?"Working tree · watches local edits":"Mirror · checks remote revisions"} · {j.state.replaceAll("_"," ")}</p><dl>
        <dt>Last success</dt><dd>{date(j.last_success)}</dd><dt>Active revision</dt><dd><code>{j.active_revision||"None yet"}</code></dd><dt>Attempted revision</dt><dd><code>{j.attempted_revision||"None yet"}</code></dd><dt>Next check</dt><dd>{date(j.state==="retry_wait"||j.state==="pending"?j.next_attempt:j.next_poll)}</dd><dt>Files changed in last update</dt><dd>{j.changed_files}</dd>
      </dl>{j.deferred_reason&&<p>Update deferred: {j.deferred_reason}. An old request will run within the maximum defer window.</p>}{j.state==="exhausted"&&<p>Automatic retries reached the limit. Correct source access, then Check now, or wait for the next scheduled check.</p>}<button disabled={busy!==""} onClick={()=>void check(j.repository)}>Check now for {j.repository}</button>{(j.state==="pending"||j.state==="retry_wait"||j.state==="running")&&<button disabled={busy!==""} onClick={()=>void cancel(j.repository)}>Cancel update for {j.repository}</button>}</article>)}
    </details>
  </section>;
}
