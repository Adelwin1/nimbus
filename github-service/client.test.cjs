'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
const crypto=require('node:crypto');
const {appJWT,verifyWebhook,GitHubClient}=require('./client.cjs');
const {privateKey,publicKey}=crypto.generateKeyPairSync('rsa',{modulusLength:2048});
const pem=privateKey.export({type:'pkcs8',format:'pem'});
function client(handler) {return new GitHubClient({appID:42,privateKey:pem,fetchImpl:async(url,options)=>{
  const data=handler(url,options); return new Response(JSON.stringify(data));
}});}
test('JWT is signed and has bounded expiry',()=>{
  const [header,payload,signature]=appJWT(42,pem,1700000000000).split('.');
  const claims=JSON.parse(Buffer.from(payload,'base64url'));
  assert.equal(claims.iss,'42'); assert.equal(claims.exp-claims.iat,600);
  assert.ok(crypto.verify('RSA-SHA256',Buffer.from(`${header}.${payload}`),publicKey,Buffer.from(signature,'base64url')));
});
test('webhook signature checks exact bytes and rejects tampering',()=>{
  const body=Buffer.from('{"action":"created"}');
  const sig='sha256='+crypto.createHmac('sha256','secret').update(body).digest('hex');
  assert.ok(verifyWebhook(body,sig,'secret'));
  assert.equal(verifyWebhook(Buffer.from('{}'),sig,'secret'),false);
  assert.equal(verifyWebhook(body,'sha256=bad','secret'),false);
});
test('rejects another app and inaccessible installations',async()=>{
  const c=client(()=>({installations:[{id:7,app_id:99}]}));
  await assert.rejects(c.authorizeInstallation('user-token',7),/access denied/);
});
test('checks repository access as user before returning connection metadata',async()=>{
  const c=client(url=>url.includes('/repositories?')?
    {repositories:[{id:9,full_name:'owner/repo',default_branch:'main'}]}:
    {installations:[{id:7,app_id:42,account:{login:'owner'}}]});
  assert.equal((await c.authorizeRepository('user-token',7,9)).full_name,'owner/repo');
  await assert.rejects(c.authorizeRepository('user-token',7,10),/access denied/);
});
test('worker token is read-only and limited to one repository',async()=>{
  const c=client((url,opts)=>{
    assert.equal(url,'https://api.github.com/app/installations/7/access_tokens');
    assert.equal(opts.redirect,'error');
    assert.deepEqual(JSON.parse(opts.body),{repository_ids:[9],permissions:{contents:'read'}});
    return {token:'installation-token',expires_at:'2030-01-01T00:00:00Z'};
  });
  assert.equal((await c.repositoryToken(7,9)).token,'installation-token');
});
test('GitHub failures never expose response body secrets',async()=>{
  const c=new GitHubClient({appID:42,privateKey:pem,fetchImpl:async()=>new Response('secret-token',{status:403})});
  await assert.rejects(c.repositoryToken(7,9),error=>error.message==='GitHub request failed (403)');
});
