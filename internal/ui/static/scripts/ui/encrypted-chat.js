import { api } from "./core.js";
import { EncryptedStore, cryptoOperation, randomID, signedRequest } from "./encrypted-store.js";

const textNode=(tag,text,cls)=>{const n=document.createElement(tag);n.textContent=text;if(cls)n.className=cls;return n;};
const groupID=(conversation,s)=>`sokomoko:${conversation}:${s.id}:${s.roster_hash}`;
const identity=d=>`${d.user_id}:${d.id}`;
const sortedMembers=members=>members.map(m=>`${m.identity}=${m.public_key}`).sort().join("\n");
function verifyMembers(result,roster) {
 if(sortedMembers(result.members)!==sortedMembers(roster.map(d=>({identity:identity(d),public_key:d.public_key})))) throw new Error("The encrypted group does not match the approved device list. Nothing was sent.");
}
async function fingerprint(key) {
 const bytes=Uint8Array.from(atob(key),c=>c.charCodeAt(0));
 const hash=new Uint8Array(await crypto.subtle.digest("SHA-256",bytes));
 return Array.from(hash,b=>b.toString(16).padStart(2,"0")).join("").match(/.{1,8}/g).join(" ");
}
export class UIEncryptedChat extends HTMLElement {
 connectedCallback(){
  this.userID=Number(this.getAttribute("user-id"));this.conversation=Number(this.getAttribute("conversation-id"));
  this.store=new EncryptedStore(this.userID);this.status=this.querySelector("[data-encrypted-status]");this.active=true;
  this.events=new AbortController();const options={signal:this.events.signal};
  this.querySelector("[data-enroll]").addEventListener("click",()=>this.task(()=>this.enroll()),options);
  this.querySelector("[data-trust-roster]")?.addEventListener("click",()=>this.task(()=>this.trust()),options);
  this.querySelector("[data-encrypted-compose]")?.addEventListener("submit",e=>{e.preventDefault();this.task(()=>this.send());},options);
  const encInput=this.querySelector("#encrypted-body");
  if(encInput){
   const resize=()=>{encInput.style.height="auto";encInput.style.height=Math.min(encInput.scrollHeight,180)+"px";};
   encInput.addEventListener("input",resize,options);
   encInput.addEventListener("keydown",e=>{
    if(e.key==="Enter"&&!e.shiftKey){
     e.preventDefault();
     const form=this.querySelector("[data-encrypted-compose]");
     if(form)form.requestSubmit?form.requestSubmit():form.submit();
    }
   },options);
  }
  for(const form of this.querySelectorAll("[data-open-conversation]"))form.addEventListener("submit",e=>{e.preventDefault();this.task(()=>this.openConversation(form));},options);
  document.addEventListener("visibilitychange",()=>{clearTimeout(this.timer);if(!document.hidden)this.task(()=>this.refresh(),true);},options);
  this.task(()=>this.refresh());
 }
 disconnectedCallback(){this.active=false;clearTimeout(this.timer);this.events.abort();}
 async task(fn,background=false){
  if(!this.active)return;
  if(this.busy){if(!background)this.queued=fn;return;}this.busy=true;
  const previousDelay=this.delay||5000;
  try{this.delay=5000;await fn();}catch(error){this.status.textContent=error.message;this.delay=Math.min(previousDelay*2,60000);
   if([401,403,404].includes(error.status)){this.querySelector("[data-encrypted-history]")?.replaceChildren();this.querySelector("[data-encrypted-compose]")?.setAttribute("hidden","");this.querySelector("[data-encrypted-content]").hidden=true;}
  }finally{this.busy=false;clearTimeout(this.timer);if(this.queued){const next=this.queued;this.queued=null;await this.task(next);return;}if(this.active&&!document.hidden)this.timer=setTimeout(()=>this.task(()=>this.refresh(),true),this.delay||5000);}
 }
 async call(data,op,payload){return api("/encrypted-chat",{method:"POST",body:await signedRequest(data,op,payload)});}
 async flush(data,save){
  if(!data.outbox)return;
  const pending=data.outbox;
  let response;
  try{response=await api("/encrypted-chat",{method:"POST",body:pending.request});}
  catch(error){
   if([400,403,404,409,429].includes(error.status)){
    if(pending.previousState)data.state=pending.previousState;
    data.outbox=null;await save(data);
   }
   throw error;
  }
  if(pending.request.op==="send"){const p=JSON.parse(pending.request.payload);data.last||={};data.last[p.session_id]=response.id;}
  if(pending.text!==undefined){data.outgoing[pending.request.nonce]={text:pending.text,sessionID:pending.sessionID};}
  data.outbox=null;await save(data);
  if(pending.text!==undefined){const input=this.querySelector("#encrypted-body");if(input?.value.trim()===pending.text)input.value="";}
  return response;
 }
 async enqueue(data,save,op,payload,extra={}){
  await this.flush(data,save);
  data.outbox={request:await signedRequest(data,op,payload),...extra};
  await save(data);return this.flush(data,save);
 }
 async enroll(){
  const name=this.querySelector("#device-name").value.trim();if(!name)throw new Error("Give this browser a name.");
  await this.store.locked(async(data,save)=>{
   if(data)return;
   const deviceID=randomID();const result=await cryptoOperation(null,"identity",{identity:`${this.userID}:${deviceID}`});
   data={userID:this.userID,deviceID,state:result.state,publicKey:result.public_key,name,registered:false,trusted:{},groups:{},cursor:{},history:{},outgoing:{},outbox:null};
   await save(data);await this.register(data,save);
  });await this.refresh();
 }
 async register(data,save){
  const result=await this.call(data,"register",{name:data.name,public_key:data.publicKey});data.registered=true;await save(data);return result;
 }
 async replenish(data,save){
  // Packages and their private state are persisted before upload; network retries
  // reuse the exact signed publication rather than generating new packages.
  if(this.packagesAvailable>=10||Date.now()-(data.packagesAt||0)<60000)return;
  const packages=[];
  for(let i=0;i<5;i++){const result=await cryptoOperation(data.state,"package");data.state=result.state;packages.push({id:randomID(),package:result.message});}
  try{await this.enqueue(data,save,"packages",{packages});data.packagesAt=Date.now();await save(data);}
  catch(error){if(error.status===429){data.outbox=null;data.packagesAt=Date.now();await save(data);}else throw error;}
 }
 async devices(data,save){
  const result=await this.call(data,"devices",{});this.packagesAvailable=result.packages_available;
  const stamp=JSON.stringify(result.devices);
  if(this.deviceStamp===stamp)return result.device.approved&&!result.device.revoked;
  this.deviceStamp=stamp;
  const list=this.querySelector("[data-devices]");list.replaceChildren();
  list.append(textNode("p",`This device: ${data.name}${result.device.approved?" · approved":" · awaiting approval"}`));
  list.append(textNode("p",await fingerprint(data.publicKey),"device-fingerprint"));
  if(!result.device.approved)list.append(textNode("p","Open Messages on an approved device. Compare this fingerprint there, then approve this browser. Password reset does not approve devices."));
  for(const d of result.devices||[]){
   const row=textNode("div","","stack-sm");row.append(textNode("strong",`${d.name}${d.id===data.deviceID?" (this browser)":""} · ${d.revoked?"revoked":d.approved?"approved":"pending"}`));
   row.append(textNode("p",await fingerprint(d.public_key),"device-fingerprint"));
   if(result.device.approved&&!d.revoked&&d.id!==data.deviceID){
    const approve=!d.approved;const label=textNode("label",approve?"Enter the full fingerprint shown on the new browser to approve":"Enter this device's fingerprint to revoke it");
    const input=document.createElement("input");input.className="input";input.autocomplete="off";label.append(input);
    const button=textNode("button",approve?"Approve device":"Revoke device","btn btn-secondary");button.type="button";
    button.addEventListener("click",()=>this.task(async()=>{
     if(input.value.replaceAll(" ","").toLowerCase()!==(await fingerprint(d.public_key)).replaceAll(" ",""))throw new Error("Compare and enter the full device fingerprint first.");
     await this.store.locked(async(current,persist)=>{await this.enqueue(current,persist,approve?"approve":"revoke",{id:d.id,public_key:d.public_key});});await this.refresh();
    }),{signal:this.events.signal});row.append(label,button);
   }list.append(row);
  }
  return result.device.approved&&!result.device.revoked;
 }
 async refresh(){
  await this.store.locked(async(data,save)=>{
   this.querySelector("[data-device-enroll]").hidden=!!data;
   if(!data){this.status.textContent="Set up this browser to use encrypted messages.";return;}
   if(!data.registered)await this.register(data,save);
   if(!await this.devices(data,save)){this.querySelector("[data-device-panel]").open=true;this.status.textContent="This browser needs approval from an existing device.";return;}
   await this.flush(data,save);await this.replenish(data,save);
   this.querySelector("[data-encrypted-content]").hidden=false;
   if(this.conversation)await this.thread(data,save);else this.status.textContent="Device ready. Keys stay in this browser.";
  });
 }
 async openConversation(form){
  const target={};for(const [key,value]of new FormData(form))target[key]=key==="handle"?value:Number(value);
  await this.store.locked(async(data,save)=>{await this.flush(data,save);const result=await this.enqueue(data,save,"start",{target});location.assign(`/messages/${result.conversation_id}`);});
 }
 async rosterDisplay(roster){
  const panel=this.querySelector("[data-roster-panel]");panel.hidden=false;const list=this.querySelector("[data-roster]");list.replaceChildren();
  for(const d of roster.devices){const li=textNode("li",`@${d.username} · ${d.name}`);li.append(textNode("p",await fingerprint(d.public_key),"device-fingerprint"));list.append(li);}
  this.querySelector("[data-trust-roster]").disabled=!roster.ready;
 }
 async trust(){
  await this.store.locked(async(data,save)=>{const result=await this.call(data,"roster",{conversation_id:this.conversation});
   if(!this.roster||result.roster.hash!==this.roster.hash)throw new Error("The device list changed. Review it again before trusting it.");
   data.trusted[result.roster.hash]=true;await save(data);
  });await this.refresh();
 }
 async ensureGroup(data,save,s){
  if(data.groups[s.id])return;
  if(!data.trusted[s.roster_hash])throw new Error("Review the devices before opening this encrypted session.");
  if(s.creator===data.deviceID)throw new Error("This browser's session state is missing. Keep the original browser keys; history cannot be restored from your password.");
  const result=await cryptoOperation(data.state,"join",{group:groupID(this.conversation,s),message:s.welcome});verifyMembers(result,s.roster);
  data.state=result.state;data.groups[s.id]=true;await save(data);
 }
 async thread(data,save){
  const result=await this.call(data,"roster",{conversation_id:this.conversation});this.roster=result.roster;this.current=result;
  const compose=this.querySelector("[data-encrypted-compose]");compose.hidden=true;
  if(!data.trusted[result.roster.hash])await this.rosterDisplay(result.roster);else this.querySelector("[data-roster-panel]").hidden=true;
  const response=await this.call(data,"events",{conversation_id:this.conversation,after:data.cursor[this.conversation]||0});
  const sessions=new Map((response.sessions||[]).map(s=>[s.id,s]));
  data.history[this.conversation]||=[];
  for(const event of response.events||[]){
   const s=sessions.get(event.session_id);
   if(!data.trusted[s.roster_hash]){await this.rosterDisplay({devices:s.roster,hash:s.roster_hash,ready:true});this.roster={devices:s.roster,hash:s.roster_hash,ready:true};this.status.textContent="Review the devices for earlier encrypted messages.";return;}
   const sender=s.roster.find(d=>d.id===event.device_id);if(!sender)throw new Error("Unknown sender device.");
   let body;
   if(event.device_id===data.deviceID){body=data.outgoing[event.nonce]?.text;}
   else{
    await this.ensureGroup(data,save,s);
    const decrypted=await cryptoOperation(data.state,"receive",{group:groupID(this.conversation,s),message:event.ciphertext});verifyMembers(decrypted,s.roster);
    if(decrypted.sender!==identity(sender))throw new Error("Message sender does not match its verified device.");
    data.state=decrypted.state;body=decrypted.text;
   }
   if(event.kind==="message")data.history[this.conversation].push({id:event.id,body:body||"Message unavailable on this device.",sender:sender.username,mine:event.device_id===data.deviceID,reportable:!!body&&sender.user_id!==this.userID&&(result.conversation.kind!=="seller"||((sender.user_id===result.conversation.customer_user_id)!==(this.userID===result.conversation.customer_user_id))),time:event.created_at});
   data.cursor[this.conversation]=event.id;data.last||={};data.last[event.session_id]=event.id;await save(data);
  }
  this.renderHistory(data.history[this.conversation]);
  const last=this.querySelector("[data-encrypted-history]")?.lastElementChild;
  if(last&&!document.hidden&&last.getBoundingClientRect().bottom<=innerHeight&&data.cursor[this.conversation]>(data.read?.[this.conversation]||0)){
   await this.call(data,"read",{conversation_id:this.conversation,event_id:data.cursor[this.conversation]});data.read||={};data.read[this.conversation]=data.cursor[this.conversation];await save(data);
  }
  const c=result.conversation;const canIntro=c.state==="pending"&&!c.incoming_request&&data.history[this.conversation].length===0;
  const needsExisting=result.session&&result.session.roster_hash!==result.roster.hash&&!result.session.roster.some(d=>d.id===data.deviceID);
  const ready=!needsExisting&&data.trusted[result.roster.hash]&&result.roster.ready&&!c.closed&&!c.blocked&&(c.can_send||canIntro);
  compose.hidden=!ready;
  this.status.textContent=needsExisting?"An existing participant device must start the updated encrypted session before this new browser can send.":!result.roster.ready?"Your contact needs to set up an approved device and publish keys before you can send.":c.blocked?"Messaging is blocked for this contact.":c.closed?"This conversation is closed.":c.state==="pending"&&!canIntro?"Waiting for the message request to be accepted.":ready?"End-to-end encryption is ready on this device.":"Review and verify the conversation devices.";
  if(response.more)this.delay=250;
 }
 async session(data,save){
  const result=await this.call(data,"roster",{conversation_id:this.conversation});
  if(!data.trusted[result.roster.hash]||!result.roster.ready)throw new Error("Review and trust the current device list first.");
  if(result.session?.roster_hash===result.roster.hash){await this.ensureGroup(data,save,result.session);return result.session;}
  const claimed=await this.enqueue(data,save,"claim",{conversation_id:this.conversation,roster_hash:result.roster.hash});
  const s={id:randomID(),roster_hash:result.roster.hash,roster:result.roster.devices,creator:data.deviceID};
  const previousState=data.state;
  const created=await cryptoOperation(data.state,"create",{group:groupID(this.conversation,s),packages:claimed.packages.map(p=>p.package)});verifyMembers(created,s.roster);
  data.state=created.state;data.groups[s.id]=true;
  // Save new state and exact publication in the same encrypted local record.
  const response=await this.enqueue(data,save,"session",{conversation_id:this.conversation,roster_hash:s.roster_hash,session_id:s.id,packages:claimed.packages.map(p=>p.id),welcome:created.welcome},{previousState});
  return response.session;
 }
 async send(){
  const input=this.querySelector("#encrypted-body");const text=input.value.trim();if(!text||Array.from(text).length>4000||new TextEncoder().encode(text).length>16000)throw new Error("Use a message of 1–4,000 characters.");
  await this.store.locked(async(data,save)=>{
   const resumed=data.outbox?.text===text;await this.flush(data,save);if(resumed)return;await this.thread(data,save);const s=await this.session(data,save);
   // A self-update contributes fresh entropy before each application message.
   // Server ordering is compare-and-swap; rejected events restore local state.
   let previousState=data.state;
   if(this.current.conversation.state==="accepted"){
   const updated=await cryptoOperation(data.state,"update",{group:groupID(this.conversation,s)});verifyMembers(updated,s.roster);data.state=updated.state;
   await this.enqueue(data,save,"send",{conversation_id:this.conversation,session_id:s.id,roster_hash:s.roster_hash,kind:"update",after:data.last?.[s.id]||0,ciphertext:updated.message},{previousState});
   }
   previousState=data.state;
   const result=await cryptoOperation(data.state,"send",{group:groupID(this.conversation,s),text});verifyMembers(result,s.roster);data.state=result.state;
   await this.enqueue(data,save,"send",{conversation_id:this.conversation,session_id:s.id,roster_hash:s.roster_hash,kind:"message",after:data.last?.[s.id]||0,ciphertext:result.message},{text,sessionID:s.id,previousState});
  });input.value="";await this.refresh();
 }
 renderHistory(messages){
  const stamp=`${messages.length}:${messages.at(-1)?.id||0}`;
  if(this.historyStamp===stamp)return;this.historyStamp=stamp;
  const list=this.querySelector("[data-encrypted-history]");list.replaceChildren();
  for(const m of messages){
   const li=textNode("li","",`chat-message${m.mine?" chat-message--mine":""}`);
   const meta=textNode("div","","chat-message__meta");meta.append(textNode("strong",`@${m.sender}`),textNode("time",new Date(m.time).toLocaleString()));
   li.append(meta,textNode("p",m.body,"chat-message__body"));
   if(m.reportable){const details=document.createElement("details");details.append(textNode("summary","Report"));
    const form=document.createElement("form");form.className="stack-sm";form.append(textNode("p","Reporting will disclose the text shown above to moderators. This is a recipient-provided disclosure, not independent proof of authorship.","tiny"));
    const label=textNode("label","Reason");const select=document.createElement("select");select.className="select";
    for(const reason of ["spam","harassment","scam","personal_information","other"]){const o=textNode("option",reason.replaceAll("_"," "));o.value=reason;select.append(o);}label.append(select);form.append(label);
    const button=textNode("button","Share this message with moderators","btn btn-secondary btn-sm");button.type="submit";form.append(button);
    form.addEventListener("submit",e=>{e.preventDefault();this.task(async()=>{await this.store.locked(async(data,save)=>this.enqueue(data,save,"report",{conversation_id:this.conversation,event_id:m.id,reason:select.value,disclosed_text:m.body}));this.status.textContent="Selected message disclosed to moderators.";details.open=false;});},{signal:this.events.signal});details.append(form);li.append(details);
   }list.append(li);
  }
  const last=this.querySelector("[data-encrypted-history]")?.lastElementChild;
  if(last)last.scrollIntoView({behavior:"smooth",block:"end"});
 }
}
