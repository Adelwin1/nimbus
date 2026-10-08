"use client";
import { useCallback, useEffect, useState } from "react";
import { apiRequest } from "@/lib/api";
type Check = {id:string;status:string;profile:string;directory:string;error_message?:string;result?:{
 summary:string;findings:{path:string;line:number;kind:string}[];
 related_files:{path:string;from:string;relation:string}[];limitations:string[];
}};
export default function RepositoryChecks({runId,repository,commit}:{runId:string;repository:string;commit:string}) {
 const [profile,setProfile]=useState("node-test");const [directory,setDirectory]=useState(".");
 const [checks,setChecks]=useState<Check[]>([]);const [error,setError]=useState("");const [busy,setBusy]=useState(false);
 const load=useCallback(async()=>{
  try{const response=await apiRequest<{checks:Check[]}>(`/github/runs/${runId}/checks`,{authenticated:true});setChecks(response.checks);}
  catch{setError("Repository check results could not be loaded.");}
 },[runId]);
 useEffect(()=>{
  let cancelled=false;
  async function refresh(){try{
   const response=await apiRequest<{checks:Check[]}>(`/github/runs/${runId}/checks`,{authenticated:true});
   if(!cancelled)setChecks(response.checks);
  }catch{if(!cancelled)setError("Repository check results could not be loaded.");}}
  void refresh();const timer=setInterval(()=>void refresh(),5000);
  return()=>{cancelled=true;clearInterval(timer);};
 },[runId]);
 async function queue(){setBusy(true);setError("");try{
  await apiRequest(`/github/runs/${runId}/checks`,{authenticated:true,method:"POST",body:JSON.stringify({profile,directory})});await load();
 }catch{setError("Check could not be queued. Confirm repository access and that repository checks are enabled on the server.");}finally{setBusy(false);}}
 function link(file:string,line?:number){return `https://github.com/${repository.split("/").map(encodeURIComponent).join("/")}/blob/${encodeURIComponent(commit)}/${file.split("/").map(encodeURIComponent).join("/")}${line?`#L${line}`:""}`;}
 return <section className="mt-6 rounded-md border border-white/10 p-4">
  <h3 className="text-sm font-semibold">Check this commit’s code</h3>
  <p className="mt-2 text-xs text-slate-400">Tests run in a separate worker. These profiles need dependencies already available or vendored; they do not install packages or access a test database.</p>
  <div className="mt-4 grid gap-3 sm:grid-cols-2">
   <label className="text-xs">Test profile<select value={profile} onChange={event=>setProfile(event.target.value)} className="mt-1 block w-full rounded border border-white/10 bg-[#090c10] p-2"><option value="node-test">Node built-in tests</option><option value="go-test">Go tests (offline)</option></select></label>
   <label className="text-xs">Repository directory<input value={directory} onChange={event=>setDirectory(event.target.value)} placeholder=". or backend" className="mt-1 block w-full rounded border border-white/10 bg-[#090c10] p-2" /></label>
  </div>
  <button type="button" disabled={busy} onClick={()=>void queue()} className="mt-4 rounded border border-teal-500/40 px-3 py-2 text-xs text-teal-300 disabled:opacity-50">{busy?"Queueing…":"Run repository checks"}</button>
  {error&&<p role="alert" className="mt-3 text-xs text-rose-300">{error}</p>}
  {checks.length===0&&<p className="mt-4 text-xs text-slate-400">No repository checks yet.</p>}
  {checks.map(check=><div key={check.id} className="mt-4 border-t border-white/10 pt-4">
   <p className="text-sm">{check.profile} · {check.directory} · <strong>{check.status}</strong></p>
   {check.status==="queued"&&<p className="mt-2 text-xs text-slate-400">Waiting for execution. Hosted checks can take a few minutes to start.</p>}
   {check.error_message&&<p className="mt-2 text-xs text-rose-300">{check.error_message}</p>}
   {check.result&&<>
    <p className="mt-2 text-sm text-slate-300">{check.result.summary}</p>
    <h4 className="mt-4 text-xs font-semibold">Locations reported by tests</h4>
    {check.result.findings.length===0&&<p className="mt-2 text-xs text-slate-400">No unambiguous source location was reported.</p>}
    <ul className="mt-2 space-y-2 text-xs">{check.result.findings.map(item=><li key={`${item.path}:${item.line}`}><a href={link(item.path,item.line)} target="_blank" rel="noreferrer" className="text-teal-300">{item.path}:{item.line}</a> · {item.kind}</li>)}</ul>
    {check.result.related_files.length>0&&<><h4 className="mt-4 text-xs font-semibold">Related source files</h4><ul className="mt-2 space-y-2 text-xs">{check.result.related_files.map(item=><li key={item.path}><a href={link(item.path)} target="_blank" rel="noreferrer" className="text-teal-300">{item.path}</a><p className="text-slate-400">Imported from {item.from} · {item.relation}</p></li>)}</ul></>}
    <details className="mt-4 text-xs text-slate-400"><summary>Limits of this check</summary><ul className="mt-2 space-y-2">{check.result.limitations.map(item=><li key={item}>{item}</li>)}</ul></details>
   </>}
  </div>)}
 </section>;
}
