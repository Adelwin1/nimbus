const test=require("node:test");const assert=require("node:assert/strict");const {runSnapshot}=require("./runner.cjs");
const file=(path,text)=>({path,content:Buffer.from(text).toString("base64")});
test("real restricted container passes correct tests and locates a failed assertion",{timeout:220000},async()=>{
 const files=[file("src/add.cjs","module.exports=(a,b)=>a+b;"),file("test/add.test.cjs",`const test=require("node:test");
const assert=require("node:assert/strict");
const add=require("../src/add.cjs");
test("adds numbers",()=>assert.equal(add(2,3),5));
`)];
 const passed=await runSnapshot({files},"node-test",".");assert.equal(passed.status,"passed",JSON.stringify(passed));
 files[1]=file("test/add.test.cjs",Buffer.from(files[1].content,"base64").toString().replace("add(2,3),5","add(2,3),99"));
 const failed=await runSnapshot({files},"node-test",".");assert.equal(failed.status,"failed",JSON.stringify(failed));
 assert.ok(failed.findings.some(item=>item.path==="test/add.test.cjs"));
 assert.ok(failed.related_files.some(item=>item.path==="src/add.cjs"));
});
