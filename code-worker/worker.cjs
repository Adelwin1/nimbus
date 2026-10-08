const {createRequire}=require("node:module");
const path=require("node:path");
const {randomUUID}=require("node:crypto");
const {runSnapshot}=require("./runner.cjs");
const {Pool}=createRequire(path.resolve(__dirname,"../browser-worker/package.json"))("pg");
const origin=new URL(process.env.NIMBUS_API_ORIGIN||"http://localhost:8080");
if(!process.env.DATABASE_URL||!["http:","https:"].includes(origin.protocol)||(!["localhost","127.0.0.1"].includes(origin.hostname)&&origin.protocol!=="https:")||origin.username||origin.password||origin.pathname!=="/"||origin.search||origin.hash){console.error("Set DATABASE_URL and a valid NIMBUS_API_ORIGIN.");process.exit(1);}
const {jobOptions}=require("../hosted-workers/job.cjs");const hosted=jobOptions();
const pool=new Pool({connectionString:process.env.DATABASE_URL,max:2,connectionTimeoutMillis:5000,query_timeout:10000});
let stopping=false;process.on("SIGINT",()=>stopping=true);process.on("SIGTERM",()=>stopping=true);
async function cycle(){
 await pool.query("UPDATE repository_checks SET status='error',error_message='Worker stopped or exceeded its lease.',finished_at=now(),download_token=NULL,lease_token=NULL,lease_expires_at=NULL WHERE status='running' AND lease_expires_at<now()");
 await pool.query("UPDATE repository_checks SET status='error',error_message='No worker claimed this check within 30 minutes.',finished_at=now(),download_token=NULL WHERE status='queued' AND queued_at<now()-interval '30 minutes'");
 const lease=randomUUID();
 const {rows}=await pool.query(`WITH candidate AS (SELECT id FROM repository_checks WHERE status='queued' AND ($2::uuid IS NULL OR id=$2::uuid) ORDER BY queued_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE repository_checks c SET status='running',lease_token=$1,lease_expires_at=now()+interval '4 minutes' FROM candidate WHERE c.id=candidate.id RETURNING c.id,c.profile,c.directory,c.download_token`,[lease,hosted.id]);
 const check=rows[0];if(!check)return false;
 let result=null,message=null,status="error";
 try{
  const response=await fetch(new URL(`/api/v1/github/code-checks/${check.id}/source`,origin),{headers:{Authorization:`Bearer ${check.download_token}`},signal:AbortSignal.timeout(65000),redirect:"error"});
  if(!response.ok)throw new Error("Repository snapshot unavailable. Check access, server configuration and snapshot limits.");
  const chunks=[];let bytes=0;for await(const chunk of response.body){bytes+=chunk.length;if(bytes>16*1024*1024)throw new Error("Snapshot exceeds limits.");chunks.push(chunk);}
  const snapshot=JSON.parse(Buffer.concat(chunks).toString());
  result=await runSnapshot(snapshot,check.profile,check.directory,{node:process.env.NIMBUS_NODE_TEST_IMAGE,go:process.env.NIMBUS_GO_TEST_IMAGE});
  // Store bounded structured evidence, never raw test output or credentials.
  status=result.status;
 }catch{message="Repository check could not execute. Confirm GitHub access, API configuration, repository limits and Docker image availability.";}
 await pool.query("UPDATE repository_checks SET status=$3,result=$4,error_message=$5,finished_at=now(),download_token=NULL,lease_token=NULL,lease_expires_at=NULL WHERE id=$1 AND lease_token=$2 AND status='running'",[check.id,lease,status,result?JSON.stringify(result):null,message]);
 console.log(`Repository check ${check.id}: ${status}`);return true;
}
(async()=>{console.log("Nimbus repository worker started.");try{while(!stopping){try{const worked=await cycle();if(hosted.once)break;if(worked)continue;}catch{console.error("Worker cycle failed. Check database configuration.");if(hosted.once)throw new Error("Hosted queue unavailable.");}await new Promise(resolve=>setTimeout(resolve,2000));}}finally{await pool.end();}})().catch(()=>{console.error("Repository worker stopped.");process.exitCode=1;});
