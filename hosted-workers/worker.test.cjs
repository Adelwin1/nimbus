const {test}=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs/promises');const os=require('node:os');const path=require('node:path');const {spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'..');const id='8355ee4a-0724-4760-977f-fb60247952b0';
test('both hosted workers claim only the dispatched ID and exit without polling',async()=>{
 const dir=await fs.mkdtemp(path.join(os.tmpdir(),'nimbus-hosted-test-'));
 try{
 const mock=path.join(dir,'pg-mock.cjs');await fs.writeFile(mock,`const Module=require('node:module');const load=Module._load;Module._load=function(name,...args){if(name==='pg')return {Pool:class {async query(sql,values){if(/WITH (?:next_run|candidate)/.test(sql)){if(!sql.includes('$2::uuid')||values[1]!==${JSON.stringify(id)})throw new Error('Incorrect claim');console.log('targeted-claim');}return {rows:[]};}async end(){console.log('pool-closed');}}};return load.call(this,name,...args);};`);
 for(const file of ['browser-worker/worker.cjs','code-worker/worker.cjs']){
 const result=spawnSync(process.execPath,[file],{cwd:root,env:{PATH:process.env.PATH,HOME:dir,NODE_OPTIONS:`--require=${mock}`,DATABASE_URL:'postgres://fake',NIMBUS_JOB_ID:id,NIMBUS_API_ORIGIN:'https://nimbus-api-vjw7.onrender.com',JOURNEY_ALLOWED_ORIGINS:'https://nimbus-alpha-livid.vercel.app'},encoding:'utf8',timeout:4000});
 assert.equal(result.status,0,result.stderr);assert.match(result.stdout,/targeted-claim/);assert.match(result.stdout,/pool-closed/);
 }
 }finally{await fs.rm(dir,{recursive:true,force:true});}
});
