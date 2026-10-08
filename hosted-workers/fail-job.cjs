const {createRequire}=require('node:module');const path=require('node:path');const {jobOptions}=require('./job.cjs');
const {Pool}=createRequire(path.resolve(__dirname,'../browser-worker/package.json'))('pg');
(async()=>{
 const {id}=jobOptions();if(!id||!process.env.DATABASE_URL||!['browser','repository'].includes(process.env.NIMBUS_JOB_KIND))throw new Error('Missing job configuration.');
 const pool=new Pool({connectionString:process.env.DATABASE_URL,connectionTimeoutMillis:5000,query_timeout:10000});
 try{const table=process.env.NIMBUS_JOB_KIND==='browser'?'browser_journey_runs':'repository_checks';
 await pool.query(`UPDATE ${table} SET status='error',error_message='Hosted runner could not finish. Check the Nimbus hosted check workflow.',finished_at=now(),lease_token=NULL,lease_expires_at=NULL WHERE id=$1 AND status IN ('queued','running')`,[id]);
 if(table==='repository_checks')await pool.query("UPDATE repository_checks SET download_token=NULL WHERE id=$1 AND status='error'",[id]);
 }finally{await pool.end();}
})().catch(()=>{console.error('Unable to record hosted job failure.');process.exitCode=1;});
