<script setup lang="ts">
import {ref,onMounted,onUnmounted,onActivated,onDeactivated} from 'vue'
import PageState from './PageState.vue'
type Cert={id:string;domains:string[];mode:string;email:string;staging:boolean;account_id?:string;renew_before_days?:number}
type State={id:string;status:string;working:boolean;not_after:string;renew_at:string;last_issued:string;next_attempt:string;last_error?:string;reload_error?:string}
const props=defineProps<{certificates:Cert[];revision?:number;api:<T>(path:string,method?:string,body?:unknown)=>Promise<T>;download:(path:string,name:string)=>Promise<void>}>()
const emit=defineEmits<{add:[];edit:[Cert];remove:[Cert]}>()
const states=ref<Record<string,State>>({}),error=ref(''),busy=ref(false)
const pageLoading=ref(true),pageError=ref(''),pageLoaded=ref(false)
let active=true,fetching=false,refreshing=false,loadedAt=0
let loadedRevision:number|undefined=-1
let stopped=false,timer:ReturnType<typeof setTimeout>
function date(s?:string){return !s||s.startsWith('0001-')?'—':new Date(s).toLocaleString()}
function label(s?:State){return ({pending:'等待签发',queued:'已排队',issuing:'签发中',renewing:'续期中',ready:'可用',failed:'上次申请失败',expired:'已过期'} as Record<string,string>)[s?.status||'pending']||'等待签发'}
async function load(){const data=await props.api<{certificates:Record<string,State>}>('/certificates/');states.value=data.certificates}
async function loadPage(force=true){if(fetching||refreshing||(!force&&pageLoaded.value&&loadedRevision===props.revision&&Date.now()-loadedAt<15000))return;const revision=props.revision;fetching=true;pageLoading.value=true;pageError.value='';try{await load();pageLoaded.value=true;loadedAt=Date.now();loadedRevision=revision}catch(e){pageError.value=e instanceof Error?e.message:String(e)}finally{pageLoading.value=false;fetching=false}}
async function poll(){try{if(active&&pageLoaded.value&&!pageLoading.value&&!busy.value){refreshing=true;await load();loadedAt=Date.now()}}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{refreshing=false;if(!stopped)timer=setTimeout(poll,5000)}}
async function action(fn:()=>Promise<void>){busy.value=true;error.value='';try{await fn();await load()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function issue(c:Cert){await action(async()=>{await props.api('/certificates/'+c.id+'/issue','POST')})}
onMounted(()=>{loadPage();timer=setTimeout(poll,5000)})
onActivated(()=>{active=true;loadPage(false)})
onDeactivated(()=>{active=false})
onUnmounted(()=>{stopped=true;clearTimeout(timer)})
</script>
<template>
 <PageState :loading="pageLoading" :loaded="pageLoaded" :error="pageError" @retry="loadPage()">
 <section class="card">

  <div class="section-head"><div><h2>域名证书 <span class="heading-count">{{certificates.length}}</span></h2><p class="overview">保存后自动申请和续期，证书就绪后可用于 HTTPS 代理。</p></div><span v-if="busy" class="loading-spinner" role="status" aria-label="正在处理"></span><button v-if="certificates.length" :disabled="busy" @click="emit('add')">＋ 添加证书</button></div>
  <div v-if="!certificates.length" class="empty-state"><div class="empty-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="M12 3l8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3ZM8 12l3 3 5-6" /></svg></div><h3>为你的域名添加证书</h3><p>选择域名和验证方式，NetLite 会自动申请证书，<br>并在到期前完成续期。</p><button :disabled="busy" @click="emit('add')">＋ 添加证书</button></div>
  <article v-for="c in certificates" :key="c.id">
   <div><strong>{{c.domains.join(', ')}}</strong><p>{{c.mode==='internal'?'内部 CA':c.mode==='dns'?'ACME · DNS-01':'ACME · HTTP-01'}} {{c.mode==='internal'?'':c.staging?'· 测试 CA':'· 正式 CA'}} · {{label(states[c.id])}}</p>
    <p>到期：{{date(states[c.id]?.not_after)}} · 计划续期：{{date(states[c.id]?.renew_at)}}</p>
    <p v-if="states[c.id]?.last_error" class="form-error">{{states[c.id]?.last_error}} · 下次重试：{{date(states[c.id]?.next_attempt)}}</p>
    <p v-if="states[c.id]?.reload_error" class="form-error">{{states[c.id]?.reload_error}}</p>
   </div>
   <div class="small-actions"><button class="secondary" :disabled="busy||states[c.id]?.working" @click="issue(c)">申请／续期</button><button v-if="states[c.id]?.last_issued&&!states[c.id]?.last_issued.startsWith('0001-')" class="secondary" :disabled="busy" @click="action(()=>download('/certificates/'+c.id+'/certificate',c.id+'.pem'))">下载证书</button><button class="secondary" :disabled="busy" @click="emit('edit',c)">编辑</button><button class="danger" :disabled="busy||states[c.id]?.working" @click="emit('remove',c)">删除</button></div>
  </article>
  <button v-if="certificates.some(c=>c.mode==='internal')" class="secondary" :disabled="busy" @click="action(()=>download('/certificates/root','netlite-root-ca.pem'))">下载内部 CA 根证书</button>
  <p v-if="certificates.length" class="muted">内部 CA 和测试 CA 证书需要客户端信任。下载仅包含公开证书，不包含私钥。</p>
  <p v-if="error" class="form-error" role="alert">{{error}}</p>
 </section>
 </PageState>
</template>
