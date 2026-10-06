'use strict';
const crypto = require('node:crypto');
const API = 'https://api.github.com';
function positiveID(value) {
  if (!Number.isSafeInteger(value) || value < 1) throw new Error('Invalid GitHub ID');
  return value;
}
function appJWT(appID, privateKey, now = Date.now()) {
  if (!/^[1-9][0-9]*$/.test(String(appID))) throw new Error('Invalid app ID');
  const key = crypto.createPrivateKey(privateKey);
  if (key.asymmetricKeyType !== 'rsa') throw new Error('RSA private key required');
  const seconds = Math.floor(now / 1000);
  const encode = value => Buffer.from(JSON.stringify(value)).toString('base64url');
  const body = `${encode({alg:'RS256',typ:'JWT'})}.${encode({iat:seconds-60,exp:seconds+540,iss:String(appID)})}`;
  return `${body}.${crypto.sign('RSA-SHA256',Buffer.from(body),key).toString('base64url')}`;
}
function verifyWebhook(body, signature, secret) {
  if (!Buffer.isBuffer(body) || !secret || !/^sha256=[a-f0-9]{64}$/.test(signature || '')) return false;
  const expected = crypto.createHmac('sha256',secret).update(body).digest();
  return crypto.timingSafeEqual(expected,Buffer.from(signature.slice(7),'hex'));
}
class GitHubClient {
  #fetch; #appID; #key;
  constructor({appID,privateKey,fetchImpl=fetch}) {
    appJWT(appID,privateKey);
    this.#appID=appID; this.#key=privateKey; this.#fetch=fetchImpl;
  }
  async #request(path, token, {method='GET',body}={}) {
    if (!path.startsWith('/') || path.startsWith('//')) throw new Error('Invalid API path');
    const response = await this.#fetch(API+path,{
      method, redirect:'error', signal:AbortSignal.timeout(15000),
      headers:{Accept:'application/vnd.github+json',Authorization:`Bearer ${token}`,
        'X-GitHub-Api-Version':'2022-11-28','User-Agent':'Nimbus','Content-Type':'application/json'},
      ...(body ? {body:JSON.stringify(body)} : {})
    });
    // Never relay GitHub error bodies: they may contain repository details or credentials.
    if (!response.ok) throw new Error(`GitHub request failed (${response.status})`);
    let size=0; const chunks=[];
    for await (const chunk of response.body) {
      size+=chunk.length;
      if (size>2*1024*1024) throw new Error('GitHub response too large');
      chunks.push(Buffer.from(chunk));
    }
    return JSON.parse(Buffer.concat(chunks).toString('utf8'));
  }
  // A user OAuth token is required to prove access; installation IDs alone are not authorization.
  async authorizeInstallation(userToken,installationID) {
    positiveID(installationID);
    if (typeof userToken !== 'string' || !userToken) throw new Error('User authorization required');
    for (let page=1;page<=100;page++) {
      const data=await this.#request(`/user/installations?per_page=100&page=${page}`,userToken);
      if (!Array.isArray(data.installations)) throw new Error('Invalid installation response');
      const item=data.installations.find(i=>i.id===installationID && String(i.app_id)===String(this.#appID));
      if (item) {
        if (item.suspended_at) throw new Error('Installation suspended');
        return {id:item.id,account:item.account?.login};
      }
      if (data.installations.length<100) break;
    }
    throw new Error('Installation access denied');
  }
  async authorizeRepository(userToken,installationID,repositoryID) {
    positiveID(repositoryID);
    await this.authorizeInstallation(userToken,installationID);
    for (let page=1;page<=100;page++) {
      const data=await this.#request(`/user/installations/${installationID}/repositories?per_page=100&page=${page}`,userToken);
      if (!Array.isArray(data.repositories)) throw new Error('Invalid repository response');
      const repo=data.repositories.find(r=>r.id===repositoryID);
      if (repo) return {id:repo.id,full_name:repo.full_name,default_branch:repo.default_branch};
      if (data.repositories.length<100) break;
    }
    throw new Error('Repository access denied');
  }
  // Internal worker only: callers must obtain IDs from a tenant-authorized stored connection.
  async repositoryToken(installationID,repositoryID) {
    positiveID(installationID); positiveID(repositoryID);
    const result=await this.#request(`/app/installations/${installationID}/access_tokens`,
      appJWT(this.#appID,this.#key),{method:'POST',body:{repository_ids:[repositoryID],permissions:{contents:'read'}}});
    if (typeof result.token!=='string' || !result.token || !Number.isFinite(Date.parse(result.expires_at))) {
      throw new Error('Invalid token response');
    }
    return {token:result.token,expires_at:result.expires_at};
  }
}
module.exports={appJWT,verifyWebhook,GitHubClient};
