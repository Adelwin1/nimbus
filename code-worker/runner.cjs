const {spawn} = require("node:child_process");
const {randomUUID} = require("node:crypto");
const fs = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
function validDirectory(value) {return value === "." || (typeof value === "string" && value.length <= 180 && /^(?:[A-Za-z0-9_-]+\/)*[A-Za-z0-9_-]+$/.test(value));}
function safeFile(value) {
 return typeof value === "string" && value.length <= 512 && !value.startsWith("/") && !value.includes("\\") && !value.includes("\0") && value.split("/").every(part => part && part !== "." && part !== "..");
}
function dockerArguments({profile,directory,source,name}, images = {}) {
 if (!validDirectory(directory) || !["node-test","go-test"].includes(profile)) throw new Error("Unsupported execution profile or directory.");
 const image = profile === "node-test" ? images.node || "node:24-bookworm-slim" : images.go || "golang:1.26.4-bookworm";
 const args = ["run","--pull=never","--name",name,"--network=none","--read-only","--user","65534:65534","--cap-drop=ALL","--security-opt","no-new-privileges","--pids-limit","128","--memory","1g","--memory-swap","1g","--cpus","1","--tmpfs","/tmp:rw,size=512m,mode=1777","--mount",`type=bind,source=${source},target=/repo,readonly`,"--workdir",directory === "." ? "/repo" : `/repo/${directory}`];
 if (profile === "go-test") args.push("--env","GOPROXY=off","--env","GOSUMDB=off","--env","GOTOOLCHAIN=local","--env","GOCACHE=/tmp/go-cache","--env","GOMODCACHE=/tmp/go-mod");
 args.push(image, ...(profile === "node-test" ? ["node","--test","--test-reporter=tap"] : ["go","test","-json","./...","-count=1"]));
 return args;
}
function executeDocker(args, timeout=90000) {
 return new Promise(resolve => {
  const child=spawn("docker",args,{stdio:["ignore","pipe","pipe"]});
  let output="",bytes=0,timedOut=false,overflow=false;
  const timer=setTimeout(()=>{timedOut=true;child.kill("SIGKILL");},timeout);
  function collect(chunk){bytes+=chunk.length;if(bytes>262144){overflow=true;child.kill("SIGKILL");return;}output+=chunk.toString();}
  child.stdout.on("data",collect);child.stderr.on("data",collect);
  child.on("error",()=>{clearTimeout(timer);resolve({code:null,output:"Docker could not start. Confirm the engine and execution image are available.",timedOut,overflow});});
  child.on("close",code=>{clearTimeout(timer);resolve({code,output,timedOut,overflow});});
 });
}
function evidence(output, files, directory) {
 const known=new Set(files.map(file=>file.path));const findings=[];
 for(const match of output.matchAll(/(?:\/repo\/)?([A-Za-z0-9_./-]+\.(?:go|js|cjs|mjs|ts|tsx)):(\d+)(?::\d+)?/g)) {
  const raw=match[1].replace(/^\/repo\//,"");
  const candidates=[raw,directory === "." ? raw : `${directory}/${raw}`].filter(value=>known.has(value));
  let file=candidates[0];
  if(!file){const suffix=files.filter(item=>item.path.endsWith("/"+raw)||item.path===raw);if(suffix.length===1)file=suffix[0].path;}
  const line=Number(match[2]);
  if(file && line>0 && line<1000000 && !findings.some(item=>item.path===file&&item.line===line)) findings.push({path:file,line,kind:"test output location"});
  if(findings.length>=20)break;
 }
 return findings;
}
function relatedFiles(findings,files) {
 const known=new Map(files.map(file=>[file.path,file]));const result=[];const visited=new Set(findings.map(item=>item.path));
 const pending=findings.map(item=>({path:item.path,depth:0}));
 const module=files.find(file=>file.path.endsWith("go.mod"));
 const moduleName=module && Buffer.from(module.content,"base64").toString().match(/^module\s+(\S+)/m)?.[1];
 while(pending.length && result.length<12){
  const item=pending.shift();if(item.depth>=2)continue;
  const file=known.get(item.path);if(!file)continue;
  const text=Buffer.from(file.content,"base64").toString("utf8").slice(0,65536);
  const imports=[...text.matchAll(/(?:\bfrom\s*|\brequire\s*\(\s*)["']([^"']+)["']/g)].map(match=>match[1]);
  if(item.path.endsWith(".go") && moduleName)for(const match of text.matchAll(/"([^"\n]+)"/g))if(match[1].startsWith(moduleName+"/"))imports.push(match[1]);
  for(const imported of imports){
   let choices=[];
   if(imported.startsWith(".")){
    const base=path.posix.normalize(path.posix.join(path.posix.dirname(item.path),imported));
    choices=[base,...[".ts",".tsx",".js",".cjs",".mjs","/index.ts","/index.js"].map(extension=>base+extension)].filter(value=>known.has(value));
   }else if(moduleName && imported.startsWith(moduleName+"/")){
    const base=path.posix.join(path.posix.dirname(module.path),imported.slice(moduleName.length+1));
    choices=files.filter(candidate=>path.posix.dirname(candidate.path)===base&&candidate.path.endsWith(".go")&&!candidate.path.endsWith("_test.go")).slice(0,3).map(candidate=>candidate.path);
   }
   for(const target of choices){if(visited.has(target)||result.length>=12)continue;visited.add(target);result.push({path:target,from:item.path,relation:"static import; execution not proven"});pending.push({path:target,depth:item.depth+1});}
  }
 }
 return result;
}
function summarize(execution,files,directory) {
 const infrastructure=execution.code===null||execution.code===125||execution.code===126||execution.code===127||execution.timedOut||execution.overflow||/module lookup disabled|GOPROXY=off|Cannot find module|ERR_MODULE_NOT_FOUND|TEST_DATABASE_URL is required|no test files found/i.test(execution.output);
 const empty=/^# tests 0(?:\r?\n|$)/m.test(execution.output)||(/\[no test files\]/.test(execution.output)&&! /"Action":"(?:pass|fail)","Package":.*"Test":/.test(execution.output));
 const findings=evidence(execution.output,files,directory);
 return {status:infrastructure||empty?"error":execution.code===0?"passed":"failed",exit_code:execution.code,timed_out:execution.timedOut,output_truncated:execution.overflow,findings,related_files:relatedFiles(findings,files),summary:infrastructure?"Execution could not complete. Check dependencies, test environment, image availability or resource limits.":empty?"No runnable tests were detected. Configure tests for this profile.":execution.code===0?"Repository tests passed for this profile.":"Repository tests failed. Listed locations are test evidence, not a confirmed root cause of the browser failure.",limitations:["Network and credentials are unavailable inside the container.","Dependencies must be vendored or already available in the execution image.","Import relationships are static; they do not prove a runtime call chain.","A passing test suite does not prove the preview serves this commit or every feature works."]};
}
async function runSnapshot(snapshot,profile,directory,images={}) {
 if(!validDirectory(directory)||!Array.isArray(snapshot.files)||snapshot.files.length>5000)throw new Error("Invalid snapshot.");
 const temporary=await fs.mkdtemp(path.join(os.tmpdir(),"nimbus-code-"));const source=path.join(temporary,"source");const name="nimbus-check-"+randomUUID();
 try {
  await fs.mkdir(source,{mode:0o755});let total=0;const seen=new Set();
  for(const file of snapshot.files){
   if(!safeFile(file.path)||seen.has(file.path)||typeof file.content!=="string"||!/^[A-Za-z0-9+/]*={0,2}$/.test(file.content))throw new Error("Invalid repository file.");
   seen.add(file.path);const data=Buffer.from(file.content,"base64");total+=data.length;
   if(data.length>1048576||total>10485760)throw new Error("Snapshot exceeds limits.");
   if(path.posix.basename(file.path).startsWith(".env"))continue;
   const target=path.join(source,...file.path.split("/"));await fs.mkdir(path.dirname(target),{recursive:true,mode:0o755});await fs.writeFile(target,data,{mode:0o444});
  }
  if(!(await fs.stat(path.join(source,directory))).isDirectory())throw new Error("Check directory not found.");
  const execution=await executeDocker(dockerArguments({profile,directory,source,name},images));
  return summarize(execution,snapshot.files,directory);
 }finally{
  await executeDocker(["rm","-f",name],10000);
  await fs.rm(temporary,{recursive:true,force:true});
 }
}
module.exports={validDirectory,safeFile,dockerArguments,evidence,relatedFiles,summarize,runSnapshot};
