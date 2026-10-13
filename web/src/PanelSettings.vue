<script setup lang="ts">
import {ref,onMounted,onUnmounted} from 'vue'
type Cert={id:string;domains:string[];mode:string;staging:boolean}
type Panel={enabled:boolean;domain:string;certificate_id:string;redirect:boolean;port:number;pending:boolean;url:string;rollback_seconds:number}
const props=defineProps<{api:<T>(path:string,method?:string,body?:unknown)=>Promise<T>;certificates:Cert[];selected?:string}>()
const emit=defineEmits<{certificates:[]}>()
const panel=ref<Panel>(),enabled=ref(true),domain=ref(''),certificate=ref(''),redirect=ref(true),busy=ref(false),error=ref(''),message=ref(''),states=ref<Record<string,{status:string;not_after:string}>>({})
const newPassword=ref(''),oldPassword=ref(''),showPassword=ref(false)
let poll:ReturnType<typeof setInterval>|undefined
async function load(update=false){panel.value=await props.api('/panel');const res=await props.api<{certificates:Record<string,{status:string;not_after:string}>}>('/certificates/');states.value=res.certificates;if(update){enabled.value=panel.value.enabled||Boolean(props.selected);domain.value=panel.value.domain;certificate.value=props.selected||panel.value.certificate_id;redirect.value=panel.value.enabled?panel.value.redirect:true;if(props.selected)selectCertificate()}}
function selectCertificate(){const c=props.certificates.find(c=>c.id===certificate.value);if(c&&!c.domains.some(d=>d===domain.value)){domain.value=c.domains.find(d=>!d.startsWith('*.'))||''}}
async function action(fn:()=>Promise<void>){busy.value=true;error.value='';message.value='';try{await fn()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function enable(){await action(async()=>{const result=await props.api<{pending:boolean;url:string}>('/panel','PUT',{enabled:enabled.value,domain:domain.value,certificate_id:certificate.value,redirect:redirect.value});if(result.pending){message.value='HTTPS 已开启测试。请打开新地址，登录后在本页确认；两分钟内未确认会自动恢复原设置。';await load()}else{window.location.assign(result.url)}})}
async function confirm(){await action(async()=>{await props.api('/panel/confirm','POST');await load();message.value='面板 HTTPS 已启用，证书续期后会自动更新。'})}
async function cancel(){await action(async()=>{await props.api('/panel/cancel','POST');await load(true);message.value='已恢复原访问设置。'})}
async function password(){await action(async()=>{await props.api('/auth/password','POST',{old_password:oldPassword.value,new_password:newPassword.value});oldPassword.value='';newPassword.value='';const me=await props.api<{csrf_token:string}>('/auth/me');sessionStorage.setItem('netlite-csrf',me.csrf_token);message.value='密码已修改，其他登录会话已注销。'})}
onMounted(()=>{action(()=>load(true));poll=setInterval(()=>{if(panel.value?.pending)action(()=>load())},5000)})
onUnmounted(()=>{if(poll)clearInterval(poll)})
</script>
<template>
 <div class="content-list">
 <section class="card">
  <div class="section-head"><div><h2>面板 HTTPS</h2><p class="overview">使用已有证书保护管理页面，证书续期后自动更新。</p></div><span v-if="panel" class="local-badge">{{panel.pending?'等待访问确认':panel.enabled?'已启用 HTTPS':'当前使用 HTTP'}}</span></div>
  <div v-if="panel?.pending" class="message" role="status"><div><strong>请确认新 HTTPS 地址可以访问</strong><p>在新地址登录后点击确认。未在两分钟内确认将恢复原设置。</p><a :href="panel.url" target="_blank" rel="noopener">{{panel.url}} ↗</a><div class="small-actions"><button :disabled="busy" @click="confirm">我已通过新地址访问，确认启用</button><button class="secondary" :disabled="busy" @click="cancel">取消切换</button></div></div></div>
  <form v-else class="panel-form" @submit.prevent="enable">
   <label class="panel-check"><input v-model="enabled" type="checkbox">启用 HTTPS 访问</label>
   <template v-if="enabled">
    <label>使用证书<select v-model="certificate" required @change="selectCertificate"><option value="" disabled>请选择已签发的证书</option><option v-for="c in certificates" :key="c.id" :value="c.id" :disabled="!states[c.id]?.not_after||states[c.id]?.not_after.startsWith('0001-')||Date.parse(states[c.id]?.not_after)<=Date.now()">{{c.domains.join(', ')}} · {{c.mode==='internal'?'内部 CA':c.staging?'测试 CA':'正式 CA'}} · {{states[c.id]?.not_after&&!states[c.id]?.not_after.startsWith('0001-')?new Date(states[c.id].not_after).toLocaleDateString():'尚未签发'}}</option></select></label>
    <p v-if="!certificates.length" class="muted">还没有证书。<button type="button" class="text-button" @click="emit('certificates')">去申请证书</button></p>
    <p v-if="certificates.find(c=>c.id===certificate)?.mode==='internal'||certificates.find(c=>c.id===certificate)?.staging" class="message">此证书需要浏览器额外信任；普通访问推荐正式 CA 证书。</p>
    <label>访问域名<input v-model="domain" required placeholder="netlite.example.com" autocomplete="off"></label>
    <p class="muted">域名应覆盖在证书内，并从你的网络解析到本机。通过 IP 访问域名证书会出现不匹配提示。</p>
    <label>管理端口<input :value="panel?.port" readonly></label>
    <label class="panel-check"><input v-model="redirect" type="checkbox">HTTP 自动跳转 HTTPS</label>
    <p v-if="domain&&panel" class="muted">新访问地址：https://{{domain}}:{{panel.port}}</p>
   </template>
   <button :disabled="busy">{{busy?'正在检查…':enabled?'检查并启用':'停用 HTTPS'}}</button>
  </form>
  <p class="muted">无法访问时，在服务器执行 sudo netlite reset-https，再重启 netlite 服务。</p>
 </section>
 <section class="card"><div class="section-head"><div><h2>修改管理员密码</h2><p class="overview">修改后，其他登录会话会失效。</p></div></div><form class="panel-form" @submit.prevent="password"><label>当前密码<input v-model="oldPassword" :type="showPassword?'text':'password'" autocomplete="current-password" required></label><label>新密码<input v-model="newPassword" :type="showPassword?'text':'password'" autocomplete="new-password" minlength="12" required></label><label class="panel-check"><input v-model="showPassword" type="checkbox">显示密码</label><button :disabled="busy">修改密码</button></form></section>
 <p v-if="error" class="message error" role="alert">{{error}}</p><p v-if="message" class="message" role="status">{{message}}</p>
 </div>
</template>
