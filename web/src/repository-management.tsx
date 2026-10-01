import React, {useEffect, useState} from "react";

type Rules={include?:string[];exclude?:string[]};
type Setup={mode?:string;state:string;error?:string;pending_removal?:string;local_repositories?:{id:string;path:string}[];repositories?:{id:string;url:string;ref:string}[];rules?:Record<string,Rules>};
const patterns=(value:string)=>value.split(",").map(s=>s.trim()).filter(Boolean);
export function RepositoryManagement({request,onChanged}:{request:<T>(path:string,body?:unknown)=>Promise<T>;onChanged:()=>void}) {
  const [open,setOpen]=useState(false),[setup,setSetup]=useState<Setup>(),[error,setError]=useState(""),[notice,setNotice]=useState(""),[busy,setBusy]=useState(false),[removing,setRemoving]=useState("");
  const [id,setID]=useState(""),[source,setSource]=useState(""),[ref,setRef]=useState("refs/heads/main"),[include,setInclude]=useState(""),[exclude,setExclude]=useState("");
  useEffect(()=>{if(!open)return;let live=true,inFlight=false;async function poll(){if(inFlight)return;inFlight=true;try{const next=await request<Setup>("/api/v1/onboarding");if(live)setSetup(next)}catch(e){if(live)setError(e instanceof Error?e.message:"Repository controls unavailable")}finally{inFlight=false}}void poll();const timer=window.setInterval(()=>void poll(),2000);return()=>{live=false;window.clearInterval(timer)}},[open,request]);
  async function act(path:string,body:unknown,text:string){setBusy(true);setError("");setNotice("");try{await request(path,body);setNotice(text);setSetup(await request<Setup>("/api/v1/onboarding"));onChanged();return true}catch(e){setError(e instanceof Error?e.message:"Repository action failed");onChanged();return false}finally{setBusy(false)}}
  async function add(e:React.FormEvent){e.preventDefault();const body={rules:{include:patterns(include),exclude:patterns(exclude)},...(setup?.mode==="local"?{local_repository:{id,path:source}}:{mirror_repository:{id,url:source,ref}})};if(await act("/api/v1/repositories/add",body,"Repository approved; its first validated generation will appear after indexing.")){setID("");setSource("");setInclude("");setExclude("")}}
  const entries=setup?.mode==="local"?setup.local_repositories:setup?.repositories;
  const running=setup?.state==="syncing"||setup?.state==="ingesting";
  return <section aria-label="Repository management"><button aria-expanded={open} onClick={()=>setOpen(!open)}>Manage repositories</button>{open&&<div>
    <h2>Approved repositories</h2><p>{setup?.mode==="local"?"Direct working trees":"Managed Git mirrors"}. Changes index in the background. Last successful knowledge remains available until a validated replacement activates.</p>
    {error&&<p role="alert">{error}</p>}{notice&&<p role="status">{notice}</p>}{setup?.error&&<p role="status">{setup.error}</p>}
    {setup?.pending_removal&&<p role="alert">Cleanup for {setup.pending_removal} is pending. Evidence reads and maintenance are paused. <button disabled={busy} onClick={()=>void act("/api/v1/repositories/remove",{repository:setup.pending_removal},"Owned data cleanup completed.")}>Finish removal of {setup.pending_removal}</button></p>}
    {running&&<p role="status">A build is running. Removing a repository cancels the build before purging owned data.</p>}
    <button disabled={busy||running||!entries?.length} onClick={()=>void act("/api/v1/repositories/rebuild",{},"Rebuilding the whole knowledge base. Identity and approved configuration are preserved.")}>Rebuild knowledge base</button>
    {entries?.map(entry=><article aria-label={`Manage ${entry.id}`} key={entry.id}><h3>{entry.id}</h3>
      <ScopeEditor id={entry.id} rules={setup?.rules?.[entry.id]} disabled={busy||running} save={rules=>act("/api/v1/repositories/rules",{repository:entry.id,rules},`Updated scope for ${entry.id}; replacement knowledge is queued.`)}/>
      <button disabled={busy||running} onClick={()=>void act("/api/v1/repositories/retry",{repository:entry.id},`Retry queued for ${entry.id}.`)}>Retry {entry.id}</button>
      <button disabled={busy||running} onClick={()=>void act("/api/v1/repositories/rebuild",{repository:entry.id},`Fresh compilation started for ${entry.id}.`)}>Force rebuild {entry.id}</button>
      <button disabled={busy} onClick={()=>setRemoving(entry.id)}>Remove {entry.id}</button>
    </article>)}
    {removing&&<section aria-label="Confirm repository removal"><h3>Remove {removing}?</h3><p>This purges all owned knowledge, history, snapshots, mirrors and cache references for this repository. Its source workspace stays on disk. Active builds are cancelled first.</p><button disabled={busy} onClick={()=>void act("/api/v1/repositories/remove",{repository:removing},`Removed ${removing} and its owned knowledge.`).then(ok=>{if(ok)setRemoving("")})}>Confirm remove {removing}</button><button disabled={busy} onClick={()=>setRemoving("")}>Keep repository</button></section>}
    <form onSubmit={add}><h3>Add a repository</h3><label>New repository ID<input required pattern="[a-z0-9][a-z0-9._-]{0,62}" value={id} onChange={e=>setID(e.target.value)}/></label><label>{setup?.mode==="local"?"New workspace path":"New Git URL"}<input required value={source} onChange={e=>setSource(e.target.value)}/></label>{setup?.mode!=="local"&&<label>New full Git ref<input required value={ref} onChange={e=>setRef(e.target.value)}/></label>}<label>New include patterns<input value={include} onChange={e=>setInclude(e.target.value)}/></label><label>New exclude patterns<input value={exclude} onChange={e=>setExclude(e.target.value)}/></label><button disabled={busy||running||(entries?.length||0)>=100}>Approve and index repository</button></form>
  </div>}</section>;
}
function ScopeEditor({id,rules,disabled,save}:{id:string;rules?:Rules;disabled:boolean;save:(rules:Rules)=>Promise<boolean>}) {
  const [include,setInclude]=useState(""),[exclude,setExclude]=useState("");
  useEffect(()=>{setInclude((rules?.include||[]).join(", "));setExclude((rules?.exclude||[]).join(", "))},[rules?.include?.join(","),rules?.exclude?.join(",")]);
  return <form onSubmit={e=>{e.preventDefault();void save({include:patterns(include),exclude:patterns(exclude)})}}><label>Include patterns for {id}<input value={include} onChange={e=>setInclude(e.target.value)}/></label><label>Exclude patterns for {id}<input value={exclude} onChange={e=>setExclude(e.target.value)}/></label><button disabled={disabled}>Save scope for {id}</button></form>;
}
