<script setup lang="ts">
import {ref,onMounted,onUnmounted} from 'vue'
type Cert={id:string;domains:string[];mode:string;email:string;staging:boolean;account_id?:string;renew_before_days?:number}
type State={id:string;status:string;working:boolean;not_after:string;renew_at:string;last_issued:string;next_attempt:string;last_error?:string;reload_error?:string}
const props=defineProps<{certificates:Cert[];api:<T>(path:string,method?:string,body?:unknown)=>Promise<T>;download:(path:string,name:string)=>Promise<void>}>()
const emit=defineEmits<{add:[];edit:[Cert];remove:[Cert]}>()
const states=ref<Record<string,State>>({}),error=ref(''),busy=ref(false)
let stopped=false,timer:ReturnType<typeof setTimeout>
function date(s?:string){return !s||s.startsWith('0001-')?'—':new Date(s).toLocaleString()}
function label(s?:State){return ({pending:'等待签发',queued:'已排队',issuing:'签发中',renewing:'续期中',ready:'可用',failed:'上次申请失败',expired:'已过期'} as Record<string,string>)[s?.status||'pending']||'等待签发'}
async function load(){const data=await props.api<{certificates:Record<string,State>}>('/certificates/');states.value=data.certificates}
async function poll(){try{await load()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{if(!stopped)timer=setTimeout(poll,5000)}}
async function action(fn:()=>Promise<void>){busy.value=true;error.value='';try{await fn();await load()}catch(e){error.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function issue(c:Cert){await action(async()=>{await props.api('/certificates/'+c.id+'/issue','POST')})}
onMounted(poll)
onUnmounted(()=>{stopped=true;clearTimeout(timer)})
</script>
<template>
 <section class="card">
  <div class="section-head"><h2>证书管理</h2><button :disabled="busy" @click="emit('add')">＋ 添加证书</button></div>
  <p class="muted">保存后自动申请和续期。证书就绪后可应用 HTTPS 代理；续期会自动更新正在运行的代理。</p>
  <p v-if="!certificates.length" class="empty">添加域名与验证方式，开始管理证书。</p>
  <article v-for="c in certificates" :key="c.id">
   <div><strong>{{c.domains.join(', ')}}</strong><p>{{c.mode==='internal'?'内部 CA':c.mode==='dns'?'ACME · DNS-01':'ACME · HTTP-01'}} {{c.mode==='internal'?'':c.staging?'· 测试 CA':'· 正式 CA'}} · {{label(states[c.id])}}</p>
    <p>到期：{{date(states[c.id]?.not_after)}} · 计划续期：{{date(states[c.id]?.renew_at)}}</p>
    <p v-if="states[c.id]?.last_error" class="form-error">{{states[c.id]?.last_error}} · 下次重试：{{date(states[c.id]?.next_attempt)}}</p>
    <p v-if="states[c.id]?.reload_error" class="form-error">{{states[c.id]?.reload_error}}</p>
   </div>
   <div class="small-actions"><button class="secondary" :disabled="busy||states[c.id]?.working" @click="issue(c)">申请／续期</button><button v-if="states[c.id]?.last_issued&&!states[c.id]?.last_issued.startsWith('0001-')" class="secondary" :disabled="busy" @click="action(()=>download('/certificates/'+c.id+'/certificate',c.id+'.pem'))">下载证书</button><button class="secondary" :disabled="busy" @click="emit('edit',c)">编辑</button><button class="danger" :disabled="busy||states[c.id]?.working" @click="emit('remove',c)">删除</button></div>
  </article>
  <button v-if="certificates.some(c=>c.mode==='internal')" class="secondary" :disabled="busy" @click="action(()=>download('/certificates/root','netlite-root-ca.pem'))">下载内部 CA 根证书</button>
  <p class="muted">内部 CA 和测试 CA 证书需要客户端信任。下载仅包含公开证书，不包含私钥。</p>
  <p v-if="error" class="form-error" role="alert">{{error}}</p>
 </section>
</template>
