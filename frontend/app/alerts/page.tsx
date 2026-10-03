"use client";

import Link from "next/link";
import {useCallback,useEffect,useState} from "react";
import {ProtectedRoute} from "@/components/auth/ProtectedRoute";
import {ConsoleShell,Status,consoleButton} from "@/components/console/ConsoleShell";
import {apiRequest} from "@/lib/api";

type Rule={id:string;name:string;rule:string};
type Alert={
 id:string;name:string;title:string;
 severity:string;status:string;created_at:string;
};

export default function AlertsPage(){
 return <ProtectedRoute><Alerts/></ProtectedRoute>;
}

function Alerts(){
 const [rules,setRules]=useState<Rule[]>([]);
 const [items,setItems]=useState<Alert[]>([]);
 const [error,setError]=useState("");
 const [loading,setLoading]=useState(true);

 const load=useCallback(async()=>{
  try{
   const [r,a]=await Promise.all([
    apiRequest<{applications:Rule[]}>("/alerts/rules",{authenticated:true}),
    apiRequest<{alerts:Alert[]}>("/alerts/",{authenticated:true}),
   ]);
   setRules(r.applications);setItems(a.alerts);setError("");
  }catch(e){
   setError(e instanceof Error?e.message:"Alerts unavailable.");
  }finally{setLoading(false)}
 },[]);

 useEffect(()=>{
  let running=false;
  const refresh=async()=>{
   if(running)return;
   running=true;
   try{await load()}finally{running=false}
  };
  const first=setTimeout(()=>void refresh(),0);
  const timer=setInterval(()=>{
   if(document.visibilityState==="visible")void refresh();
  },15000);
  return()=>{clearTimeout(first);clearInterval(timer)};
 },[load]);

 return <ConsoleShell actions={
  <button className={consoleButton} onClick={()=>void load()}>Refresh</button>
 }>
  <h1 className="text-2xl font-semibold">Alerts</h1>
  <p className="mt-2 text-sm text-slate-400">
   Delivery channel: in-app inbox.
   Matching incidents opened after a rule is enabled appear here.
  </p>
  {error?<p role="alert" className="mt-4 text-rose-300">{error}</p>:null}
  {loading?<p className="mt-5 text-slate-500">Loading alerts…</p>:null}

  <section className="mt-6 rounded-md border border-white/10 bg-[#0d1117] p-5">
   <h2 className="font-semibold">Notification rules</h2>
   <div className="mt-4 space-y-3">
    {rules.map(rule=>
     <RuleEditor key={`${rule.id}:${rule.rule}`} item={rule} onSaved={load}/>
    )}
    {!loading&&!rules.length?<p className="text-sm text-slate-500">
     Add an application to configure alerts.
    </p>:null}
   </div>
  </section>

  <section className="mt-6 rounded-md border border-white/10 bg-[#0d1117]">
   <h2 className="border-b border-white/10 p-5 font-semibold">
    Inbox · latest 50 matching incidents
   </h2>
   {items.map(item=>
    <Link key={item.id} href={`/incidents/${item.id}`}
     className="block border-b border-white/10 p-5 last:border-0 hover:bg-white/[0.03]">
     <div className="flex flex-wrap justify-between gap-3">
      <p className="text-sm font-medium">{item.title}</p>
      <Status value={item.status}/>
     </div>
     <p className="mt-2 text-xs text-slate-500">
      {item.name} · {item.severity} · {new Date(item.created_at).toLocaleString()}
     </p>
    </Link>
   )}
   {!loading&&!items.length?<p className="p-5 text-sm text-slate-500">
    No matching incidents yet.
   </p>:null}
  </section>
 </ConsoleShell>;
}

function RuleEditor({item,onSaved}:{
 item:Rule;onSaved:()=>Promise<void>;
}){
 const [value,setValue]=useState(item.rule);
 const [saving,setSaving]=useState(false);
 const [error,setError]=useState("");

 async function save(e:React.FormEvent){
  e.preventDefault();
  if(saving)return;
  setSaving(true);setError("");
  try{
   await apiRequest(`/alerts/rules/${item.id}`,{
    method:"PUT",authenticated:true,
    body:JSON.stringify({rule:value}),
   });
   await onSaved();
  }catch(err){
   setError(err instanceof Error?err.message:"Save failed.");
  }finally{setSaving(false)}
 }

 return <form onSubmit={save}
  className="rounded-md border border-white/10 p-3">
  <div className="flex flex-wrap items-center gap-3">
   <label className="flex min-w-0 flex-1 flex-wrap items-center gap-3 text-sm">
    {item.name}
    <select value={value} onChange={e=>setValue(e.target.value)}
     disabled={saving}
     className="rounded-md border border-white/15 bg-[#090c10] p-2 text-sm">
     <option value="off">Off</option>
     <option value="critical">Critical only</option>
     <option value="all">All incidents</option>
    </select>
   </label>
   <button className={consoleButton} disabled={saving||value===item.rule}>
    {saving?"Saving…":"Save rule"}
   </button>
  </div>
  {error?<p role="alert" className="mt-2 text-sm text-rose-300">{error}</p>:null}
 </form>;
}
