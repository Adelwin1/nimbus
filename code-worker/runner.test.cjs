const test=require("node:test");const assert=require("node:assert/strict");
const {validDirectory,safeFile,dockerArguments,evidence,relatedFiles,summarize}=require("./runner.cjs");
const file=(path,text="")=>({path,content:Buffer.from(text).toString("base64")});
test("rejects traversal paths and executable profile injection",()=>{
 for(const path of ["..","/tmp","a/../b","a;touch","a b",".git"]){assert.equal(validDirectory(path),false);}
 assert.equal(validDirectory("backend/internal"),true);assert.equal(validDirectory("."),true);
 for(const path of ["../a.js","/a.js","a\\b.js","a\0.js"]){assert.equal(safeFile(path),false);}
 assert.throws(()=>dockerArguments({profile:"sh",directory:".",source:"/tmp/safe",name:"check"}));
});
test("container arguments isolate the workload without credentials or host sockets",()=>{
 const args=dockerArguments({profile:"go-test",directory:"backend",source:"/tmp/source",name:"check"});
 for(const flag of ["--network=none","--read-only","--cap-drop=ALL","no-new-privileges","65534:65534","GOPROXY=off"]){assert.ok(args.includes(flag));}
 assert.ok(!args.join(" ").includes("docker.sock"));assert.ok(!args.join(" ").includes("DATABASE_URL"));assert.ok(args.includes("/repo/backend"));
});
test("maps test evidence to known files and rejects ambiguous basenames",()=>{
 const files=[file("test/login.test.cjs"),file("a/shared.go"),file("b/shared.go")];
 assert.deepEqual(evidence("/repo/test/login.test.cjs:12:2\nshared.go:4",files,"."),[{path:"test/login.test.cjs",line:12,kind:"test output location"}]);
});
test("follows bounded relative imports without claiming a runtime trace",()=>{
 const files=[file("test/login.test.cjs",'const login=require("../src/login.cjs")'),file("src/login.cjs",'const validate=require("./validate.cjs")'),file("src/validate.cjs")];
 const related=relatedFiles([{path:"test/login.test.cjs",line:12}],files);
 assert.deepEqual(related.map(item=>item.path),["src/login.cjs","src/validate.cjs"]);
 assert.ok(related.every(item=>item.relation.includes("execution not proven")));
});
test("separates dependency and empty-test failures from test assertions",()=>{
 const execution={code:1,output:"module lookup disabled by GOPROXY=off",timedOut:false,overflow:false};
 assert.equal(summarize(execution,[],".").status,"error");
 assert.equal(summarize({...execution,output:"AssertionError: wrong value"},[],".").status,"failed");
 assert.equal(summarize({...execution,code:0,output:"# tests 0\n"},[],".").status,"error");
 assert.equal(summarize({...execution,code:0,output:"# tests 2\n"},[],".").status,"passed");
});
