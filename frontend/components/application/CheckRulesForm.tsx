"use client";

import {useEffect,useState} from "react";
import {apiRequest} from "@/lib/api";
import {consoleButton} from "@/components/console/ConsoleShell";

type Rules={
 expected_status:number|null;
 required_text:string;
 json_pointer:string|null;
 json_expected:unknown;
};

const field=
 "mt-2 block w-full rounded-md border border-white/15 bg-[#090c10] p-3 text-sm text-slate-200 outline-none focus:border-teal-400 disabled:opacity-50";

export function CheckRulesForm({applicationId}:{applicationId:string}){
 const [status,setStatus]=useState("");
 const [text,setText]=useState("");
 const [jsonEnabled,setJsonEnabled]=useState(false);
 const [pointer,setPointer]=useState("/database/connected");
 const [expected,setExpected]=useState("true");
 const [loaded,setLoaded]=useState(false);
 const [saving,setSaving]=useState(false);
 const [error,setError]=useState("");
 const [success,setSuccess]=useState("");

 useEffect(()=>{
  let active=true;
  apiRequest<Rules>(`/apps/${applicationId}/check-rules`,{
   authenticated:true,
  }).then(r=>{
   if(active){
    setStatus(r.expected_status===null?"":String(r.expected_status));
    setText(r.required_text);
    setJsonEnabled(r.json_pointer!==null);
    setPointer(r.json_pointer??"/database/connected");
    setExpected(
     r.json_pointer===null?"true":JSON.stringify(r.json_expected,null,2)
    );
    setLoaded(true);
   }
  }).catch(()=>{
   if(active)setError(
    "Check settings could not be loaded. Refresh this page to retry."
   );
  });
  return()=>{active=false};
 },[applicationId]);

 async function save(event:React.FormEvent){
  event.preventDefault();
  if(!loaded||saving)return;
  setError("");setSuccess("");

  let value:unknown=null;
  if(jsonEnabled){
   try{value=JSON.parse(expected)}
   catch{
    setError('Expected value must be JSON: true, 42, "connected", null, or an object.');
    return;
   }
  }

  const code=status.trim()===""?null:Number(status);
  if(code!==null&&(!Number.isInteger(code)||code<100||code>599)){
   setError("Expected status must be between 100 and 599.");
   return;
  }

  setSaving(true);
  try{
   await apiRequest(`/apps/${applicationId}/check-rules`,{
    method:"PUT",authenticated:true,
    body:JSON.stringify({
     expected_status:code,
     required_text:text,
     json_pointer:jsonEnabled?pointer:null,
     json_expected:value,
    }),
   });
   setSuccess(
    "Check rules saved. They apply to upcoming monitoring and release verification checks."
   );
  }catch(err){
   setError(err instanceof Error?err.message:"Check rules could not be saved.");
  }finally{setSaving(false)}
 }

 return <section className="mt-8 rounded-md border border-white/10 bg-[#0d1117] p-5">
  <h2 className="text-lg font-semibold">Response checks</h2>
  <p className="mt-2 text-sm text-slate-400">
   All enabled assertions must pass. Responses are limited to 64 KB.
  </p>

  <form onSubmit={save} className="mt-5 space-y-5">
   <fieldset disabled={!loaded||saving} className="space-y-5">
    <label className="block text-sm">
     Expected HTTP status
     <input type="number" min={100} max={599} step={1}
      value={status} onChange={e=>setStatus(e.target.value)}
      placeholder="Blank accepts any 2xx status" className={field}/>
    </label>

    <label className="block text-sm">
     Required response text
     <textarea rows={2} maxLength={2048}
      value={text} onChange={e=>setText(e.target.value)}
      placeholder="Optional, case-sensitive text" className={field}/>
    </label>

    <label className="flex items-center gap-2 text-sm">
     <input type="checkbox" checked={jsonEnabled}
      onChange={e=>setJsonEnabled(e.target.checked)}/>
     Check a JSON value
    </label>

    {jsonEnabled?<div className="space-y-4">
     <label className="block text-sm">
      JSON Pointer
      <input value={pointer} onChange={e=>setPointer(e.target.value)}
       placeholder="/database/connected" className={field}/>
      <span className="mt-2 block text-xs text-slate-500">
       Use /items/0/status for arrays. Blank selects the entire JSON response.
      </span>
     </label>
     <label className="block text-sm">
      Expected JSON value
      <textarea rows={3} required value={expected}
       onChange={e=>setExpected(e.target.value)}
       className={`${field} font-mono`}/>
     </label>
    </div>:null}

    <button className={consoleButton}>
     {saving?"Saving…":loaded?"Save check rules":"Loading…"}
    </button>
   </fieldset>

   {error?<p role="alert" className="text-sm text-rose-300">{error}</p>:null}
   {success?<p role="status" className="text-sm text-teal-300">{success}</p>:null}
  </form>
 </section>;
}
