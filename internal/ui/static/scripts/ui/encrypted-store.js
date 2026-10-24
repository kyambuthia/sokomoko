// Every value containing MLS secrets or plaintext stays in this origin's
// IndexedDB, encrypted with a non-extractable local wrapping key. This protects
// the stored blob, not an unlocked browser or malicious same-origin script.
const encoder = new TextEncoder();
const decoder = new TextDecoder();
export function randomID() { return Array.from(crypto.getRandomValues(new Uint8Array(16)), b => b.toString(16).padStart(2,"0")).join(""); }
export function base64(bytes) { return btoa(String.fromCharCode(...bytes)); }
function openDB() {
  return new Promise((resolve,reject)=>{
    const r=indexedDB.open("sokomoko-encrypted-chat-v1",1);
    r.onupgradeneeded=()=>{r.result.createObjectStore("keys");r.result.createObjectStore("devices");};
    r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error);
  });
}
function get(db,store,key) { return new Promise((resolve,reject)=>{const tx=db.transaction(store);const r=tx.objectStore(store).get(key);r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error);}); }
function put(db,store,key,value) { return new Promise((resolve,reject)=>{const tx=db.transaction(store,"readwrite");tx.objectStore(store).put(value,key);tx.oncomplete=()=>resolve();tx.onerror=()=>reject(tx.error);tx.onabort=()=>reject(tx.error||new Error("Local save aborted"));}); }
let protocol;
async function loadProtocol() {
  if (!protocol) protocol=import("../../crypto/sokomoko_mls.js").then(async m=>{await m.default();return m;});
  return protocol;
}
export async function cryptoOperation(state,action,options={}) {
  const m=await loadProtocol();
  try {return JSON.parse(m.execute(JSON.stringify({state,action,...options})));}
  catch {throw new Error("Encrypted operation failed. No plaintext was sent. Reopen the conversation or review its devices.");}
}
export class EncryptedStore {
  constructor(userID) {this.userID=userID;}
  async locked(fn) {
    if (!isSecureContext || !crypto.subtle || !navigator.locks || !indexedDB) throw new Error("Encrypted messaging needs a secure browser with local storage and Web Locks.");
    return navigator.locks.request(`sokomoko-chat:${this.userID}`,async()=>{
      const db=await openDB();
      try {
        let key=await get(db,"keys",this.userID);
        if (!key) {key=await crypto.subtle.generateKey({name:"AES-GCM",length:256},false,["encrypt","decrypt"]);await put(db,"keys",this.userID,key);}
        const saved=await get(db,"devices",this.userID);
        let data=null;
        if (saved) {try {data=JSON.parse(decoder.decode(await crypto.subtle.decrypt({name:"AES-GCM",iv:saved.iv},key,saved.blob)));} catch {throw new Error("This browser's encrypted keys cannot be opened. Link a new device from an approved device.");}}
        const save=async value=>{
          const iv=crypto.getRandomValues(new Uint8Array(12));
          const blob=await crypto.subtle.encrypt({name:"AES-GCM",iv},key,encoder.encode(JSON.stringify(value)));
          await put(db,"devices",this.userID,{iv,blob});
        };
        return await fn(data,save);
      } finally {db.close();}
    });
  }
}
export async function signedRequest(data,op,payload,nonce=randomID()) {
  const raw=JSON.stringify(payload);
  const hash=base64(new Uint8Array(await crypto.subtle.digest("SHA-256",encoder.encode(raw))));
  const text=`sokomoko-e2ee-v1\n${op}\n${data.userID}\n${data.deviceID}\n${nonce}\n${hash}`;
  const result=await cryptoOperation(data.state,"sign",{text});
  return {op,device_id:data.deviceID,nonce,payload:raw,signature:result.message};
}
