<script setup lang="ts">
import {ref,computed,onMounted,onUnmounted,onActivated,onDeactivated} from 'vue'
import PageState from './PageState.vue'
type Cert={id:string;domains:string[];mode:string;staging:boolean}
type Panel={enabled:boolean;domain:string;certificate_id:string;redirect:boolean;port:number;pending:boolean;url:string;rollback_seconds:number}
const props=defineProps<{api:<T>(path:string,method?:string,body?:unknown)=>Promise<T>;certificates:Cert[];revision?:number}>()

const panel=ref<Panel>(),enabled=ref(true),domain=ref(''),certificate=ref(''),redirect=ref(true),busy=ref(false),error=ref(''),message=ref(''),states=ref<Record<string,{status:string;not_after:string}>>({})
const newPassword=ref(''),oldPassword=ref(''),showPassword=ref(false)
const feedbackTarget=ref<'https'|'password'>('https')
const httpsChanged=computed(()=>!!panel.value&&(enabled.value!==panel.value.enabled||(enabled.value&&(domain.value!==panel.value.domain||certificate.value!==panel.value.certificate_id||redirect.value!==panel.value.redirect))))
const pageLoading=ref(true),pageError=ref(''),pageLoaded=ref(false)
let active=true,fetching=false,refreshing=false,loadedAt=0
let loadedRevision:number|undefined=-1
let poll:ReturnType<typeof setInterval>|undefined
async function load(update=false){const settings=await props.api<Panel>('/panel');panel.value=settings;const res=await props.api<{certificates:Record<string,{status:string;not_after:string}>}>('/certificates/');states.value=res.certificates;if(update){enabled.value=settings.enabled;domain.value=settings.domain;certificate.value=settings.certificate_id;redirect.value=settings.enabled?settings.redirect:true}}
async function loadPage(force=true){if(fetching||refreshing||(!force&&pageLoaded.value&&loadedRevision===props.revision&&Date.now()-loadedAt<15000))return;const revision=props.revision;fetching=true;pageLoading.value=true;pageError.value='';try{await load(!pageLoaded.value);pageLoaded.value=true;loadedAt=Date.now();loadedRevision=revision}catch(e){pageError.value=e instanceof Error?e.message:String(e)}finally{pageLoading.value=false;fetching=false}}
async function refreshPending(){if(!active||!panel.value?.pending||busy.value||pageLoading.value||refreshing)return;refreshing=true;try{await load()}catch(e){feedbackTarget.value='https';message.value='';error.value=e instanceof Error?e.message:String(e)}finally{refreshing=false}}
function selectCertificate(){const c=props.certificates.find(c=>c.id===certificate.value);if(c&&!c.domains.some(d=>d===domain.value)){domain.value=c.domains.find(d=>!d.startsWith('*.'))||''}}
async function action(target:'https'|'password',fn:()=>Promise<void>){feedbackTarget.value=target;busy.value=true;error.value='';message.value='';try{await fn()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function enable(){await action('https',async()=>{const result=await props.api<{pending:boolean;url:string}>('/panel','PUT',{enabled:enabled.value,domain:domain.value,certificate_id:certificate.value,redirect:redirect.value});if(result.pending){message.value='HTTPS 已开启测试。请打开新地址，登录后在本页确认；两分钟内未确认会自动恢复原设置。';await load()}else{window.location.assign(result.url)}})}
async function confirm(){await action('https',async()=>{await props.api('/panel/confirm','POST');await load();message.value='面板 HTTPS 已启用，证书续期后会自动更新。'})}
async function cancel(){await action('https',async()=>{await props.api('/panel/cancel','POST');await load(true);message.value='已恢复原访问设置。'})}
async function password(){await action('password',async()=>{await props.api('/auth/password','POST',{old_password:oldPassword.value,new_password:newPassword.value});oldPassword.value='';newPassword.value='';const me=await props.api<{csrf_token:string}>('/auth/me');sessionStorage.setItem('netlite-csrf',me.csrf_token);message.value='密码已修改，其他登录会话已注销。'})}
onMounted(()=>{loadPage();poll=setInterval(refreshPending,5000)})
onActivated(()=>{active=true;loadPage(false)})
onDeactivated(()=>{active=false})
onUnmounted(()=>{if(poll)clearInterval(poll)})
</script>
<template>
 <PageState :loading="pageLoading" :loaded="pageLoaded" :error="pageError" @retry="loadPage()">
 <div class="panel-settings">
 <section class="card panel-settings-card" aria-labelledby="panel-https-title">
  <div class="panel-card-head">
   <span class="panel-card-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="m12 3 8 4v5c0 5-8 9-8 9s-8-4-8-9V7l8-4Z"/><path d="m8.5 12 2.5 2.5 4.5-5"/></svg></span>
   <div class="panel-card-title"><h2 id="panel-https-title">面板 HTTPS</h2><p>使用已有证书加密访问，续期后自动更新。</p></div>
   <span v-if="panel" :class="['panel-status',{secure:panel.enabled&&!panel.pending,pending:panel.pending}]"><span class="dot"></span>{{panel.pending?'等待确认':panel.enabled?'HTTPS 已启用':'当前使用 HTTP'}}</span>
  </div>
  <div v-if="panel?.pending" class="panel-pending" role="status">
   <strong>确认新地址可以访问</strong><p>请在新地址登录后确认启用。两分钟内未确认，将自动恢复原设置。</p>
   <a class="panel-address" :href="panel.url+'/?panel=confirm'" target="_blank" rel="noopener">{{panel.url}} ↗</a>
   <div class="panel-pending-actions"><button :disabled="busy" @click="confirm"><span v-if="busy&&feedbackTarget==='https'" class="loading-spinner button-spinner" aria-hidden="true"></span>确认启用</button><button class="secondary" :disabled="busy" @click="cancel">取消切换</button></div>
  </div>
  <form v-else class="panel-form" @submit.prevent="enable">
   <label class="panel-toggle"><span class="panel-toggle-copy"><strong>启用 HTTPS 访问</strong><span>通过加密连接保护登录信息和管理操作。</span></span><input v-model="enabled" class="panel-switch" type="checkbox" role="switch" :disabled="busy" aria-label="启用 HTTPS 访问"></label>
   <fieldset v-if="enabled" class="panel-https-fields" :disabled="busy">
    <label>使用证书<select v-model="certificate" required @change="selectCertificate"><option value="" disabled>请选择已签发的证书</option><option v-for="c in certificates" :key="c.id" :value="c.id" :disabled="!states[c.id]?.not_after||states[c.id]?.not_after.startsWith('0001-')||Date.parse(states[c.id]?.not_after)<=Date.now()">{{c.domains.join(', ')}} · {{c.mode==='internal'?'内部 CA':c.staging?'测试 CA':'正式 CA'}} · {{states[c.id]?.not_after&&!states[c.id]?.not_after.startsWith('0001-')?new Date(states[c.id].not_after).toLocaleDateString():'尚未签发'}}</option></select></label>
    <p v-if="!certificates.length" class="panel-field-hint">还没有证书，请先在「HTTPS 证书」中添加并完成签发。</p>
    <p v-if="certificates.find(c=>c.id===certificate)?.mode==='internal'||certificates.find(c=>c.id===certificate)?.staging" class="panel-trust-note">此证书需要浏览器额外信任；普通访问推荐正式 CA 证书。</p>
    <div class="panel-endpoint-fields"><label>访问域名<input v-model="domain" required placeholder="netlite.example.com" autocomplete="off" aria-describedby="panel-domain-hint"></label><label>管理端口<input :value="panel?.port" readonly></label></div>
    <p id="panel-domain-hint" class="panel-field-hint">域名需包含在证书内，并解析到本机。通过 IP 访问会出现证书不匹配提示。</p>
    <label class="panel-check"><input v-model="redirect" type="checkbox">HTTP 自动跳转 HTTPS</label>
    <div v-if="domain&&panel" class="panel-address-preview"><span>新访问地址</span><code>https://{{domain}}:{{panel.port}}</code></div>
   </fieldset>
   <div v-else class="panel-access-note"><strong>{{panel?.enabled?'保存后将切换为 HTTP 访问':'当前通过 HTTP 访问管理面板'}}</strong><p>{{panel?.enabled?'停用后，管理面板不再使用证书加密连接。':'开启 HTTPS 并选择已签发的证书，即可加密访问管理面板。'}}</p></div>
   <div class="panel-form-footer"><span class="panel-field-hint">{{httpsChanged?'设置修改后需要保存':'设置已保存'}}</span><button :class="{secondary:!enabled}" :disabled="busy||!httpsChanged"><span v-if="busy&&feedbackTarget==='https'" class="loading-spinner button-spinner" aria-hidden="true"></span>{{busy&&feedbackTarget==='https'?'处理中…':enabled?(panel?.enabled?'保存 HTTPS 设置':'检查并启用'):panel?.enabled?'停用 HTTPS':'保存设置'}}</button></div>
  </form>
  <p v-if="error&&feedbackTarget==='https'" class="message error panel-feedback" role="alert">{{error}}</p><p v-if="message&&feedbackTarget==='https'" class="message panel-feedback" role="status">{{message}}</p>
  <details class="panel-recovery"><summary>无法访问面板？</summary><p>在服务器执行 <code>sudo netlite reset-https</code>，再重启 netlite 服务。</p></details>
 </section>
 <section class="card panel-settings-card" aria-labelledby="panel-password-title">
  <div class="panel-card-head"><span class="panel-card-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 5v2"/></svg></span><div class="panel-card-title"><h2 id="panel-password-title">修改管理员密码</h2><p>更新登录密码，保护管理面板安全。</p></div></div>
  <form class="panel-form panel-password-form" @submit.prevent="password">
   <label>当前密码<input v-model="oldPassword" :type="showPassword?'text':'password'" autocomplete="current-password" placeholder="请输入当前密码" :disabled="busy" required></label>
   <label>新密码<input v-model="newPassword" :type="showPassword?'text':'password'" autocomplete="new-password" placeholder="请输入至少 12 位新密码" aria-describedby="panel-password-hint" :disabled="busy" minlength="12" required></label>
   <p id="panel-password-hint" class="panel-field-hint">至少 12 位，建议组合使用字母、数字和符号。</p>
   <label class="panel-check"><input v-model="showPassword" type="checkbox">显示密码</label>
   <div class="panel-session-note"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10v1"/></svg><span>修改成功后，其他登录会话将自动退出。</span></div>
   <div class="panel-form-footer"><button :disabled="busy"><span v-if="busy&&feedbackTarget==='password'" class="loading-spinner button-spinner" aria-hidden="true"></span>{{busy&&feedbackTarget==='password'?'处理中…':'修改密码'}}</button></div>
  </form>
  <p v-if="error&&feedbackTarget==='password'" class="message error panel-feedback" role="alert">{{error}}</p><p v-if="message&&feedbackTarget==='password'" class="message panel-feedback" role="status">{{message}}</p>
 </section>
 </div>
 </PageState>
</template>
<style scoped>
.panel-settings{display:grid;grid-template-columns:minmax(0,1.15fr) minmax(0,1fr);gap:22px;align-items:start;max-width:1200px}
.panel-settings-card{min-width:0;padding:26px}
.panel-card-head{display:flex;align-items:center;gap:12px;margin-bottom:24px;flex-wrap:wrap}
.panel-card-icon{display:grid;place-items:center;width:42px;height:42px;flex-shrink:0;border-radius:12px;background:#edf6f2;color:var(--accent)}
.panel-card-icon svg,.panel-session-note svg{width:22px;height:22px;fill:none;stroke:currentColor;stroke-width:1.7;stroke-linecap:round;stroke-linejoin:round}
.panel-card-title{flex:1;min-width:160px}.panel-card-title p{font-size:12px;color:var(--muted);margin:4px 0 0;line-height:1.7}
.panel-status{display:inline-flex;align-items:center;gap:6px;flex-shrink:0;white-space:nowrap;padding:4px 9px;border-radius:20px;background:#f1f4f6;color:#71838f;font-size:11px}
.panel-status.secure{background:#e9f5ee;color:var(--accent)}.panel-status.secure .dot{background:#258b72}.panel-status.pending{background:#fff5df;color:#967029}.panel-status.pending .dot{background:#c29439}
.panel-form{gap:18px;max-width:none;padding:0}.panel-form label{display:block;margin:0}.panel-form input:not([type=checkbox]),.panel-form select{height:44px;margin-top:7px;min-width:0}.panel-form input[readonly]{background:#f5f7f9;color:var(--muted)}
.panel-form .panel-toggle{display:flex;align-items:center;justify-content:space-between;gap:18px;padding:16px;background:#f6f9f8;border:1px solid #e8eeeb;border-radius:10px}
.panel-toggle-copy{display:grid;gap:4px}.panel-toggle-copy strong{font-size:13px;font-weight:600}.panel-toggle-copy>span{font-size:12px;color:var(--muted);font-weight:400;line-height:1.7}
.panel-form .panel-switch{appearance:none;position:relative;width:40px;height:24px;flex-shrink:0;border:0;border-radius:20px;margin:0;padding:0;background:#c8d3d8;cursor:pointer;transition:background .15s}
.panel-switch:before{content:'';position:absolute;top:3px;left:3px;width:18px;height:18px;border-radius:50%;background:#fff;box-shadow:0 1px 3px #203e4c26;transition:transform .15s}
.panel-form .panel-switch:checked{background:var(--accent)}.panel-switch:checked:before{transform:translateX(16px)}.panel-form .panel-switch:disabled{cursor:default;opacity:.5}
.panel-https-fields{display:grid;gap:16px;padding:0;margin:0;border:0;min-width:0}.panel-endpoint-fields{display:grid;grid-template-columns:minmax(0,1fr) 100px;gap:12px}
.panel-field-hint{font-size:12px;color:var(--muted);line-height:1.7;margin:0}.panel-https-fields>label+p,.panel-endpoint-fields+p,.panel-password-form>label+p{margin-top:-10px}
.panel-form .panel-check{font-weight:400;gap:8px!important}.panel-check input{margin:0;accent-color:var(--accent);width:15px!important;height:15px;flex-shrink:0}
.panel-access-note{padding:4px 0}.panel-access-note strong{font-size:13px;font-weight:500}.panel-access-note p{font-size:12px;color:var(--muted);margin:6px 0 0;line-height:1.8}
.panel-address-preview{display:grid;gap:5px;padding:12px 14px;border-radius:8px;background:#f3f7f8}.panel-address-preview>span{font-size:11px;color:var(--muted)}.panel-address-preview code{font-size:13px;color:#365c59;overflow-wrap:anywhere}
.panel-form-footer{display:flex;align-items:center;justify-content:flex-end;gap:12px;border-top:1px solid #edf1f3;padding-top:18px;margin-top:2px}.panel-form-footer>.panel-field-hint{margin-right:auto}.panel-form-footer button{font-size:13px;flex-shrink:0}
.panel-recovery{margin-top:20px;border-top:1px solid #edf1f3;padding-top:16px;color:var(--muted);font-size:12px}.panel-recovery summary{cursor:pointer;width:fit-content}.panel-recovery summary:focus-visible{outline:2px solid #98c7b6;outline-offset:4px;border-radius:3px}.panel-recovery p{margin:10px 0 0;line-height:1.9;overflow-wrap:anywhere}.panel-recovery code{color:#425c68;background:#f1f4f6;border-radius:4px;padding:2px 5px}
.panel-session-note{display:flex;align-items:center;gap:8px;background:#f5f7f9;border-radius:8px;padding:12px;color:var(--muted);font-size:12px}.panel-session-note svg{width:16px;height:16px;flex-shrink:0}
.panel-feedback{margin:18px 0 0;overflow-wrap:anywhere}.panel-pending{background:#fffaf0;border:1px solid #efdfbb;border-radius:10px;padding:18px;font-size:13px}.panel-pending>strong{color:#816323}.panel-pending p{color:#8b7853;font-size:12px;margin:8px 0 12px;line-height:1.8}.panel-address{display:block;color:var(--accent);overflow-wrap:anywhere;line-height:1.8}.panel-pending-actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:18px}.panel-pending-actions button{font-size:13px}.panel-trust-note{font-size:12px;line-height:1.7;color:#8b6c37;background:#fff8ec;border:1px solid #eee2c9;padding:10px 12px;border-radius:8px;margin:0}
@media(max-width:1180px){.panel-settings{grid-template-columns:minmax(0,1fr);max-width:720px}}
@media(max-width:600px){.panel-settings{gap:16px}.panel-settings-card{padding:20px}.panel-card-head{display:grid;grid-template-columns:38px minmax(0,1fr);gap:10px;margin-bottom:20px}.panel-card-icon{width:38px;height:38px}.panel-card-title{min-width:0}.panel-card-title h2{font-size:16px}.panel-status{grid-column:2;justify-self:start}.panel-form .panel-toggle{padding:14px}.panel-endpoint-fields{grid-template-columns:minmax(0,1fr) 80px;gap:10px}.panel-form-footer{gap:8px}.panel-form-footer>.panel-field-hint{font-size:11px}}
@media(prefers-reduced-motion:reduce){.panel-switch,.panel-switch:before{transition:none}}
</style>
