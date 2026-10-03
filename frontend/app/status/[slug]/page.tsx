"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect,useState } from "react";
import { apiRequest } from "@/lib/api";
import { Status } from "@/components/console/ConsoleShell";

type PublicStatus={
 name:string;status:string;
 last_checked_at:string|null;generated_at:string;
};

export default function PublicStatusPage(){
 const {slug}=useParams<{slug:string}>();
 const [data,setData]=useState<PublicStatus|null>(null);
 const [error,setError]=useState("");

 useEffect(()=>{
  let active=true;
  let busy=false;
  async function load(){
   if(busy)return;
   busy=true;
   try{
    const result=await apiRequest<PublicStatus>(
     `/status/${encodeURIComponent(slug)}`
    );
    if(active){setData(result);setError("")}
   }catch(e){
    if(active){
     setData(null);
     setError(e instanceof Error?e.message:"Status unavailable.");
    }
   }finally{busy=false}
  }
  const initial=setTimeout(()=>void load(),0);
  const timer=setInterval(()=>{
   if(document.visibilityState==="visible")void load();
  },30000);
  return()=>{active=false;clearTimeout(initial);clearInterval(timer)};
 },[slug]);

 return <main id="main-content"
  className="nimbus-console min-h-screen bg-[#090c10] px-5 py-16 text-slate-200">
  <div className="mx-auto max-w-2xl">
   <Link href="/" className="text-sm font-semibold text-teal-300">
    Nimbus / public status
   </Link>
   {error?<p role="alert" className="mt-8 text-slate-400">{error}</p>:
    !data?<p className="mt-8 text-slate-500">Loading status…</p>:
    <section className="mt-8 rounded-md border border-white/10 bg-[#0d1117] p-6">
     <h1 className="text-2xl font-semibold">{data.name}</h1>
     <div className="mt-5"><Status value={data.status}/></div>
     <p className="mt-5 text-sm text-slate-400">
      Last checked: {data.last_checked_at
       ?new Date(data.last_checked_at).toLocaleString()
       :"No checks recorded"}
     </p>
     <p className="mt-3 text-xs text-slate-500">
      Refreshes every 30 seconds while visible.
      Status reflects the latest recorded check; use its timestamp to judge freshness.
     </p>
    </section>}
  </div>
 </main>;
}
