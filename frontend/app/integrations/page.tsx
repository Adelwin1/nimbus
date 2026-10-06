"use client";
import { useCallback, useEffect, useState } from "react";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import { ConsoleShell, consoleButton } from "@/components/console/ConsoleShell";
import { apiRequest, APIError } from "@/lib/api";

type Connection = { configured: boolean; connected: boolean; login: string; install_url: string };
type Repository = { id: number; full_name: string; installation_id: number };
type Application = { id: string; name: string };
export default function IntegrationsPage() {
 return <ProtectedRoute><Integrations /></ProtectedRoute>;
}
function Integrations() {
 const [connection,setConnection]=useState<Connection|null>(null);
 const [repositories,setRepositories]=useState<Repository[]>([]);
 const [applications,setApplications]=useState<Application[]>([]);
 const [app,setApp]=useState(""); const [repo,setRepo]=useState("");
 const [busy,setBusy]=useState(false); const [message,setMessage]=useState("");
 const [error,setError]=useState("");
 const load=useCallback(async()=>{
  setError("");
  try {
   const [status,apps]=await Promise.all([
    apiRequest<Connection>("/github/status",{authenticated:true}),
    apiRequest<{applications:Application[]}>("/apps",{authenticated:true}),
   ]);
   setConnection(status);setApplications(apps.applications);
   if(status.connected){
    const list=await apiRequest<{repositories:Repository[]}>("/github/repositories",{authenticated:true});
    setRepositories(list.repositories);
   }else{setRepositories([]);}
  } catch(e){setError(e instanceof APIError?e.message:"Could not load GitHub settings.");}
 },[]);
 useEffect(()=>{const timer=setTimeout(()=>void load(),0);return ()=>clearTimeout(timer);},[load]);
 async function connect(){
  setBusy(true);setError("");
  try{
   const result=await apiRequest<{url:string}>("/github/start",{method:"POST",authenticated:true});
   window.location.assign(result.url);
  }catch(e){setError(e instanceof APIError?e.message:"Connection could not start.");setBusy(false);}
 }
 async function disconnect(){
  setBusy(true);setMessage("");setError("");
  try{await apiRequest("/github/connection",{method:"DELETE",authenticated:true});await load();setMessage("Disconnected from Nimbus. You can also uninstall the app in GitHub settings.");}
  catch(e){setError(e instanceof APIError?e.message:"Disconnect failed.");}
  finally{setBusy(false);}
 }
 async function link(event:React.FormEvent){
  event.preventDefault();if(busy)return;
  const selected=repositories.find(r=>`${r.installation_id}:${r.id}`===repo);if(!selected||!app)return;
  setBusy(true);setError("");setMessage("");
  try{
   await apiRequest(`/github/apps/${app}/repository`,{method:"PUT",authenticated:true,body:JSON.stringify({repository_id:selected.id,installation_id:selected.installation_id})});
   setMessage(`Linked ${selected.full_name} to the selected application.`);
  }catch(e){setError(e instanceof APIError?e.message:"Repository could not be linked.");}
  finally{setBusy(false);}
 }
 const field="mt-2 block w-full rounded-md border border-white/15 bg-[#090c10] p-3 text-sm";
 return <ConsoleShell actions={<button className={consoleButton} onClick={()=>void load()} disabled={busy}>Refresh</button>}>
  <h1 className="text-2xl font-semibold">Integrations</h1>
  <section className="mt-6 max-w-2xl rounded-md border border-white/10 bg-[#0d1117] p-6">
   <h2 className="text-lg font-semibold">GitHub repositories</h2>
   <p className="mt-2 text-sm text-slate-400">Select which repositories Nimbus can read. Repository code is not executed by connecting an account.</p>
   {connection && <>
    <p className="mt-4 text-sm">{connection.connected?`Connected as ${connection.login}`:"GitHub is not connected, or authorization has expired."}</p>
    <div className="mt-4 flex flex-wrap gap-3">
     <button className={consoleButton} disabled={busy||!connection.configured} onClick={()=>void connect()}>{busy?"Working…":connection.connected?"Reauthorize":"Connect GitHub"}</button>
     <a className={consoleButton} href={connection.install_url} target="_blank" rel="noopener noreferrer">Choose repositories on GitHub</a>
     {connection.connected&&<button className={consoleButton} disabled={busy} onClick={()=>void disconnect()}>Disconnect</button>}
    </div>
    <p className="mt-3 text-xs text-slate-500">Install the app on selected repositories, then connect or refresh. Authorization currently requires reconnection when the user token expires.</p>
   </>}
   {connection?.connected&&<form onSubmit={link} className="mt-6 space-y-4">
    <label className="block text-sm">Nimbus application<select className={field} value={app} onChange={e=>setApp(e.target.value)} required disabled={busy}>
     <option value="">Select an application</option>{applications.map(a=><option key={a.id} value={a.id}>{a.name}</option>)}
    </select></label>
    <label className="block text-sm">Repository<select className={field} value={repo} onChange={e=>setRepo(e.target.value)} required disabled={busy}>
     <option value="">Select a repository</option>{repositories.map(r=><option key={`${r.installation_id}:${r.id}`} value={`${r.installation_id}:${r.id}`}>{r.full_name}</option>)}
    </select></label>
    {!repositories.length&&<p className="text-sm text-slate-400">No accessible repositories. Check the app installation, then refresh.</p>}
    <button className={consoleButton} disabled={busy||!app||!repo}>Link repository</button>
   </form>}
   {message&&<p role="status" className="mt-4 text-sm text-teal-300">{message}</p>}
   {error&&<p role="alert" className="mt-4 text-sm text-rose-300">{error}</p>}
  </section>
 </ConsoleShell>;
}
