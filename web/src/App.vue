<script setup lang="ts">
import {ref, computed, onMounted, nextTick, watch} from 'vue'
import {splitTarget, joinTarget} from './proxy-address'
import {readAPIResponse} from './api-response'
import {routeCertificateError,certificateDomain,firstValidCertificate,type CertificateState} from './route-certificate'
import {applicationBase,apiPath} from './application-path'
import DDNSManager from './DDNSManager.vue'
import CertificateManager from './CertificateManager.vue'
import PanelSettings from './PanelSettings.vue'
import PageState from './PageState.vue'
import RouteDomainInput from './RouteDomainInput.vue'
type Route={id:string;name?:string;domain:string;port:number;scheme:string;upstream:string;certificate_id:string;enabled:boolean}
type Cert={id:string;domains:string[];mode:string;email:string;staging:boolean;account_id?:string;renew_before_days?:number}
type Config={revision:number;routes:Route[];certificates:Cert[]}
const token=ref(''),logged=ref(false),busy=ref(false),message=ref(''),tab=ref('routes'),status=ref<{proxy?:{running:boolean;modules?:string[]}}>({})
const authMode=ref(applicationBase(window.location.pathname)==='/'?'local':'fnos-gateway'),initialized=ref(true),username=ref('admin'),password=ref(''),setupCode=ref(''),remember=ref(false),showPassword=ref(false),mustChange=ref(false),newPassword=ref(''),authReady=ref(false),csrf=ref(''),preserveDraft=ref(false)
const config=ref<Config>({revision:0,routes:[],certificates:[]})
const route=ref<Route>(newRoute()),cert=ref<Cert>(newCert()),domains=ref(''),editing=ref(false)
const autoFilledDomain=ref('')
const selectedRouteCertificate=computed(()=>config.value.certificates.find(c=>c.id===route.value.certificate_id))
const target=ref(splitTarget(route.value.upstream))
const editor=ref<HTMLDialogElement>(), viewer=ref<HTMLDialogElement>(), deletion=ref<HTMLDialogElement>()
const dnsAccounts=ref<{id:string;name:string;provider:string}[]>([]),certificateProviders=ref<string[]>([])
const copyMessage=ref('')
const modalError=ref(''), viewError=ref(''), viewBusy=ref(false), viewTab=ref<'draft'|'applied'>('draft')
const caddyView=ref<{draft:unknown;applied:unknown}>({draft:null,applied:null})
const pendingDelete=ref<{kind:'routes'|'certificates';key:string;name:string}>()
const certificateUsers=computed(()=>pendingDelete.value?.kind==='certificates'?config.value.routes.filter(r=>r.scheme==='https'&&r.certificate_id===pendingDelete.value?.key):[])
const savedChanges=ref(false)
const messageError=ref(false)
watch(()=>[message.value,messageError.value,savedChanges.value,busy.value,tab.value] as const,([text,error,pending,working,page],_,onCleanup)=>{
  if(page!=='routes'||!text||error||pending||working)return
  const timer=setTimeout(()=>{message.value=''},4000)
  onCleanup(()=>clearTimeout(timer))
})
const pageLoading=ref(false),pageError=ref(''),pageLoaded=ref<Record<string,number>>({}),dataRevision=ref(0)
let pageRequest=0
const navigation=[
 {title:'服务管理',items:[
  {key:'routes',title:'反向代理',description:'通过域名访问你的内网服务，集中管理访问入口。',icon:'M4 7h16M4 17h16M16 3l4 4-4 4M8 13l-4 4 4 4'},
  {key:'ddns',title:'动态域名',description:'公网 IP 变化时，自动更新域名解析。',icon:'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0ZM3 12h18M12 3c-5 5-5 13 0 18M12 3c5 5 5 13 0 18'},
  {key:'certificates',title:'HTTPS 证书',description:'管理域名证书与自动续期，为服务启用安全访问。',icon:'M12 3l8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3ZM8 12l3 3 5-6'}]},
 {title:'配置与维护',items:[
  {key:'dnsaccounts',title:'DNS 账号',description:'配置服务商账号，供动态域名和证书验证共用。',icon:'M20 8c0 3-4 5-8 5S4 11 4 8s4-5 8-5 8 2 8 5ZM4 8v8c0 3 4 5 8 5s8-2 8-5V8M4 12c0 3 4 5 8 5s8-2 8-5'},
  {key:'panel',title:'面板设置',description:'管理登录密码与面板 HTTPS 访问。',icon:'M12 3v3M12 18v3M3 12h3M18 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2M8 12a4 4 0 1 0 8 0 4 4 0 0 0-8 0'},
  {key:'engine',title:'代理引擎',description:'查看代理运行状态、校验配置与排查问题。',icon:'M4 4h16v6H4V4ZM4 14h16v6H4v-6ZM8 7h.01M8 17h.01M12 7h5M12 17h5'}]}
]
const currentPage=computed(()=>navigation.flatMap(group=>group.items).find(item=>item.key===tab.value)!)
async function navigate(key:string){if(busy.value)return;if(key==='panel'&&authMode.value==='token'){message.value='当前为兼容令牌模式；面板设置在本地账号模式下提供。';return}tab.value=key;editing.value=false;message.value='';await loadPage(false,false)}
async function loadPage(keepDraft=false,force=true){
  const request=++pageRequest,key=tab.value
  pageError.value=''
  if(!force&&pageLoaded.value[key]&&Date.now()-pageLoaded.value[key]<15000){pageLoading.value=false;return}
  const needsConfig=['routes','certificates','engine'].includes(key)||(key==='panel'&&authMode.value==='local')
  const needsStatus=key==='routes'||key==='engine'
  const needsIdentity=key==='panel'&&authMode.value==='fnos-gateway'
  pageLoading.value=needsConfig||needsStatus||needsIdentity
  try{
    const [nextConfig,nextStatus,identity]=await Promise.all([
      needsConfig&&!keepDraft?api<Config>('/config'):undefined,
      needsStatus?api<typeof status.value>('/status'):undefined,
      needsIdentity?api<{username:string}>('/auth/me'):undefined
    ])
    if(request!==pageRequest)return
    if(nextConfig)config.value=nextConfig
    if(nextStatus)status.value=nextStatus
    if(identity)username.value=identity.username
    pageLoaded.value[key]=Date.now()
  }catch(e){if(request===pageRequest)pageError.value=e instanceof Error?e.message:String(e)}
  finally{if(request===pageRequest)pageLoading.value=false}
}
function reloadIdentity(){window.location.reload()}

async function validateConfig(){await action(async()=>{await api('/proxy/validate','POST');message.value='配置校验通过，可以应用到代理。'})}
async function applyConfig(){await action(async()=>{const r=await api<{applied_revision:number}>('/proxy/apply','POST');savedChanges.value=false;message.value='已应用版本 '+r.applied_revision+'；代理配置已更新。';await refresh()})}
function closeEditor(){if(!busy.value)editor.value?.close()}
async function openEditor(){modalError.value=''; await nextTick(); editor.value?.showModal();editor.value?.querySelector<HTMLElement>('select,input')?.focus()}
async function add(){
  if(tab.value==='certificates')await action(loadDNSAccounts)
  editing.value=false;route.value=newRoute();autoFilledDomain.value='';target.value=splitTarget(route.value.upstream);cert.value=newCert();domains.value=''
  if(tab.value==='routes'&&config.value.certificates.length)await action(async()=>{
    const data=await api<{certificates:Record<string,CertificateState>}>('/certificates/')
    route.value.certificate_id=firstValidCertificate(config.value.certificates,data.certificates)?.id||''
    selectRouteCertificate()
  })
  await openEditor()
}
function askDelete(kind:'routes'|'certificates',key:string,name:string){pendingDelete.value={kind,key,name};modalError.value='';deletion.value?.showModal()}
async function showCaddy(){viewer.value?.showModal();await loadCaddy()}
async function loadCaddy(){copyMessage.value='';viewBusy.value=true;viewError.value='';try{const view=await api<{draft:unknown;applied:unknown;draft_error?:string}>('/proxy/config');caddyView.value=view;if(view.draft_error)viewError.value=view.draft_error}catch(e){caddyView.value={draft:null,applied:null};viewError.value=e instanceof Error?e.message:String(e)}finally{viewBusy.value=false}}
async function copyConfig(){try{await navigator.clipboard.writeText(JSON.stringify(caddyView.value[viewTab.value],null,2));copyMessage.value='配置已复制'}catch{viewError.value='浏览器不支持复制，请选择配置文本手动复制。'}}
async function logout(){await action(async()=>{if(authMode.value==='local')await api('/auth/logout','POST');logged.value=false;token.value='';csrf.value='';password.value='';message.value='';sessionStorage.removeItem('netlite-token');sessionStorage.removeItem('netlite-csrf')})}
function id(){return Array.from(crypto.getRandomValues(new Uint8Array(8)),b=>b.toString(16).padStart(2,'0')).join('')}
function newRoute():Route{return {id:id(),name:'',domain:'',port:18080,scheme:'https',upstream:'http://127.0.0.1:8080',certificate_id:'',enabled:true}}
function newCert():Cert{return {id:id(),domains:[],renew_before_days:0,mode:'internal',email:'',account_id:'',staging:true}}
async function api<T>(url:string,method='GET',body?:unknown):Promise<T>{const headers:Record<string,string>={'Content-Type':'application/json'};if(authMode.value==='token')headers.Authorization='Bearer '+token.value;else headers['X-CSRF-Token']=csrf.value;const r=await fetch(apiPath(window.location.pathname,url),{method,headers,body:body===undefined?undefined:JSON.stringify(body),signal:method==='GET'?AbortSignal.timeout(20000):undefined}).catch(e=>{if(e?.name==='TimeoutError')throw new Error('请求超时，请检查网络后重试。');throw e});if((r.status===401||(authMode.value==='fnos-gateway'&&r.status===403))&&logged.value){logged.value=false;preserveDraft.value=true;message.value=authMode.value==='fnos-gateway'?'飞牛登录状态或权限已变化，请使用管理员账号重新登录飞牛。':'登录已过期，请重新登录；未保存的编辑内容仍保留。'}if(authMode.value==='fnos-gateway'&&r.redirected&&!r.headers.get('Content-Type')?.includes('application/json')){logged.value=false;preserveDraft.value=true;throw new Error('飞牛会话已失效，请重新登录飞牛后打开 NetLite。')}const result=await readAPIResponse<T>(r,url);if(!['GET','HEAD','OPTIONS'].includes(method)){dataRevision.value++;for(const key of Object.keys(pageLoaded.value))pageLoaded.value[key]=1}return result}
async function download(path:string,name:string){const response=await fetch(apiPath(window.location.pathname,path),{headers:authMode.value==='token'?{Authorization:'Bearer '+token.value}:{}});if(!response.ok)throw new Error('证书文件尚未就绪或登录状态已失效');const url=URL.createObjectURL(await response.blob());const a=document.createElement('a');a.href=url;a.download=name;a.click();URL.revokeObjectURL(url)}
async function action(fn:()=>Promise<void>){busy.value=true;message.value='';messageError.value=false;try{await fn()}catch(e){messageError.value=true;message.value=e instanceof Error?e.message:String(e)}finally{busy.value=false}}
async function refresh(){status.value=await api('/status')}
async function enter(){if(authMode.value!=='token'){const me=await api<{username:string;must_change_password:boolean;csrf_token:string}>('/auth/me');username.value=me.username;mustChange.value=me.must_change_password;csrf.value=me.csrf_token;if(mustChange.value)return;password.value='';setupCode.value=''}else sessionStorage.setItem('netlite-token',token.value);const keepDraft=preserveDraft.value;preserveDraft.value=false;logged.value=true;await loadPage(keepDraft)}
async function login(){await action(async()=>{if(authMode.value==='local'){await api(initialized.value?'/auth/login':'/auth/setup','POST',{username:username.value,password:password.value,code:setupCode.value,remember:remember.value});initialized.value=true}await enter()})}
async function changeInitialPassword(){await action(async()=>{await api('/auth/password','POST',{old_password:password.value,new_password:newPassword.value});password.value='';newPassword.value='';await enter()})}
async function save(){config.value=await api('/config','PUT',config.value);savedChanges.value=true;message.value=tab.value==='routes'?'规则已保存，应用配置后生效。':'配置已保存。证书自动申请和续期；代理规则需点击「应用配置」生效。'}
function editRoute(x:Route){route.value={...x};autoFilledDomain.value='';target.value=splitTarget(x.upstream);editing.value=true;openEditor()}
function selectRouteCertificate(){
  const selected=selectedRouteCertificate.value
  if(!selected)return
  const domain=certificateDomain(selected.domains,route.value.domain,autoFilledDomain.value)
  if(domain!==route.value.domain){route.value.domain=domain;autoFilledDomain.value=domain}
  modalError.value=''
}
async function loadDNSAccounts(){const a=await api<{accounts:{id:string;name:string;provider:string}[]}>('/dns/accounts');dnsAccounts.value=a.accounts;const c=await api<{certificate_providers:string[]}>('/dns-capabilities');certificateProviders.value=c.certificate_providers}
async function editCert(x:Cert){await action(loadDNSAccounts);cert.value={...x,domains:[...x.domains]};domains.value=x.domains.join(', ');editing.value=true;openEditor()}
function upsert<T extends {id:string}>(list:T[],x:T){const i=list.findIndex(a=>a.id===x.id);if(i<0)list.push(x);else list[i]=x}
async function submitRoute(){modalError.value=routeCertificateError(route.value,config.value.certificates);if(modalError.value)return;try{route.value.upstream=joinTarget(target.value)}catch(e){modalError.value=e instanceof Error?e.message:String(e);return}await action(async()=>{const old=JSON.parse(JSON.stringify(config.value));upsert(config.value.routes,{...route.value});try{await save();route.value=newRoute();editing.value=false;editor.value?.close()}catch(e){config.value=old;modalError.value=e instanceof Error?e.message:String(e);throw e}})}
async function submitCert(){modalError.value='';await action(async()=>{const old=JSON.parse(JSON.stringify(config.value));upsert(config.value.certificates,{...cert.value,account_id:cert.value.mode==='dns'?cert.value.account_id:'',domains:domains.value.split(/[,\s]+/).filter(Boolean)});try{await save();cert.value=newCert();domains.value='';editing.value=false;editor.value?.close()}catch(e){config.value=old;modalError.value=e instanceof Error?e.message:String(e);throw e}})}
async function remove(kind:'routes'|'certificates',key:string){await action(async()=>{const old=JSON.parse(JSON.stringify(config.value));config.value[kind]=config.value[kind].filter(x=>x.id!==key) as Route[]&Cert[];try{await save();deletion.value?.close()}catch(e){config.value=old;modalError.value=e instanceof Error?e.message:String(e);throw e}})}
onMounted(async()=>{if(new URLSearchParams(window.location.search).get('panel')==='confirm')tab.value='panel';try{const state=await api<{mode:string;initialized:boolean}>('/auth/state');authMode.value=state.mode;initialized.value=state.initialized;if(state.mode==='token'){token.value=sessionStorage.getItem('netlite-token')||'';if(token.value)await enter()}else if(state.mode==='fnos-gateway'){await enter()}else if(state.initialized){try{await enter()}catch{}}}catch(e){message.value=e instanceof Error?e.message:String(e)}finally{authReady.value=true}})
</script>
<template>
 <main :class="logged?'app-shell':'login-shell'">
 <template v-if="!logged"><div class="brand login-brand"><span class="logo">N</span><div><h1>NetLite</h1><p>让网络管理轻一点</p></div></div><section class="login card"><span class="eyebrow">管理面板</span><h2>{{mustChange?'设置你的新密码':initialized?'欢迎使用 NetLite':'初始化管理员'}}</h2><p class="muted">{{mustChange?'请替换临时密码后继续使用。':'登录后，管理服务访问、域名解析与 HTTPS 证书。'}}</p>
 <template v-if="authReady&&authMode==='fnos-gateway'"><p class="muted">请使用飞牛管理员账号登录飞牛，再从应用入口打开 NetLite。</p><button :disabled="busy" @click="reloadIdentity">重新检查登录状态</button></template>
 <form v-else-if="authReady" @submit.prevent="mustChange?changeInitialPassword():login()">
  <template v-if="authMode==='token'"><label>管理令牌<input v-model="token" type="password" autocomplete="current-password" required></label><p class="muted">fnOS 兼容模式：令牌位于应用私有目录 admin-token。</p></template>
  <template v-else-if="mustChange"><label>临时密码<input v-model="password" :type="showPassword?'text':'password'" autocomplete="current-password" required></label><label>新密码<input v-model="newPassword" :type="showPassword?'text':'password'" autocomplete="new-password" minlength="12" required></label></template>
  <template v-else><label v-if="!initialized">初始化码<input v-model="setupCode" type="password" autocomplete="off" required></label><label>用户名<input v-model="username" autocomplete="username" required></label><label>密码<input v-model="password" :type="showPassword?'text':'password'" :autocomplete="initialized?'current-password':'new-password'" :minlength="initialized?undefined:12" required></label><label class="panel-check"><input v-model="remember" type="checkbox">在此设备保持登录</label></template>
  <label v-if="authMode==='local'" class="panel-check"><input v-model="showPassword" type="checkbox">显示密码</label>
  <button :disabled="busy">{{busy?'处理中…':mustChange?'设置密码并进入':initialized?'进入管理面板':'创建管理员并进入'}}</button>
 </form><p v-else class="muted">正在检查登录状态…</p><p v-if="authMode==='local'" class="muted">{{initialized?'忘记密码？在服务器执行 sudo netlite reset-password，通过标准输入提供新密码。':'在服务器执行 sudo netlite info，获取初始化码；源码运行请附加 --data 数据目录。'}}</p><p v-if="message" class="form-error" role="alert">{{message}}</p></section></template>
 <template v-else>
 <aside class="sidebar">
  <div class="brand"><span class="logo">N</span><div><h1>NetLite</h1><p>让网络管理轻一点</p></div></div>
  <nav class="side-nav" aria-label="主导航"><div v-for="group in navigation" :key="group.title" class="nav-group"><p class="nav-label">{{group.title}}</p><button v-for="item in group.items" :key="item.key" :class="['nav-item',{active:tab===item.key}]" :aria-current="tab===item.key?'page':undefined" :disabled="busy" @click="navigate(item.key)"><svg viewBox="0 0 24 24" aria-hidden="true"><path :d="item.icon" /></svg><span>{{item.title}}</span></button></div></nav>
  <div class="sidebar-bottom"><div class="engine-summary"><span :class="['dot',status.proxy?.running?'on':'']"></span><span>代理{{status.proxy?.running?'运行中':'未运行'}}</span></div><div class="sidebar-meta"><span>{{authMode==='fnos-gateway'?username+' · 飞牛管理员':'v0.1 · 本地管理'}}</span><button v-if="authMode!=='fnos-gateway'" class="text-button" @click="logout">退出登录</button><button v-else class="text-button" @click="reloadIdentity">刷新身份</button></div></div>
 </aside>
 <div class="workspace">
 <header class="page-header"><div><p class="breadcrumb">{{tab==='engine'||tab==='dnsaccounts'||tab==='panel'?'配置与维护':'服务管理'}} <span>/</span> {{currentPage.title}}</p><h2>{{tab==='ddns'?'动态域名（DDNS）':currentPage.title}}</h2><p class="page-description">{{currentPage.description}}</p></div><span v-if="busy" class="loading-spinner" role="status" aria-label="正在处理"></span><span v-else class="local-badge"><span class="dot on"></span>本地管理面板</span></header>
 <div v-if="message&&(tab!=='routes'||messageError||!savedChanges)" :class="['message',{error:messageError,'route-toast':tab==='routes'}]" :role="messageError?'alert':'status'"><span>{{message}}</span><button class="icon-button" aria-label="关闭提示" @click="message=''">×</button></div>

 <PageState :loading="pageLoading" :loaded="Boolean(pageLoaded[tab])" :error="pageError" @retry="loadPage()">
 <KeepAlive><PanelSettings :revision="dataRevision" v-if="tab==='panel'&&authMode==='local'" :api="api" :certificates="config.certificates" /></KeepAlive>
 <section v-if="tab==='panel'&&authMode==='fnos-gateway'" class="card"><h2>飞牛账号与访问</h2><p>当前账号：{{username}}（飞牛管理员）</p><p class="muted">登录状态、密码和 HTTPS 访问由飞牛管理。修改密码或退出账号，请返回飞牛进行操作。</p><button class="secondary" @click="reloadIdentity">刷新登录状态</button></section>
 <div v-if="tab==='routes'" class="content-list">
 <section class="card"><div class="section-head routes-heading"><div><h2>代理规则 <span class="heading-count">{{config.routes.length}}</span></h2><p class="overview">将域名和端口转发到内网服务，支持 WebSocket。</p></div><div v-if="config.routes.length||savedChanges" class="route-toolbar"><span v-if="savedChanges" class="pending-changes" role="status">待应用</span><button class="secondary" :disabled="busy" @click="applyConfig">应用配置</button><button v-if="config.routes.length" :disabled="busy" @click="add">＋ 添加代理</button></div></div>
 <div v-if="!config.routes.length" class="empty-state"><div class="empty-icon" aria-hidden="true"><svg viewBox="0 0 24 24"><path d="M4 7h16M4 17h16M16 3l4 4-4 4M8 13l-4 4 4 4" /></svg></div><h3>添加你的第一个服务</h3><p>把家庭相册、下载工具或其他内网服务，<br>连接到一个方便访问的域名。</p><button :disabled="busy" @click="add">＋ 添加代理</button><div class="setup-steps"><span><b>1</b>填写访问域名</span><span><b>2</b>设置目标服务</span><span><b>3</b>保存并应用</span></div></div>
 <div v-if="config.routes.length" class="proxy-table-wrap desktop-routes" tabindex="0" aria-label="代理规则表格，窄屏可左右滚动">
  <table class="proxy-table"><colgroup><col style="width:17%"><col style="width:28%"><col style="width:27%"><col style="width:10%"><col style="width:18%"></colgroup><thead><tr><th scope="col">名称 / 备注</th><th scope="col">访问地址</th><th scope="col">目标服务</th><th scope="col">状态</th><th scope="col" class="actions-heading">操作</th></tr></thead><tbody><tr v-for="r in config.routes" :key="r.id">
   <td><span :class="['route-name',{unnamed:!r.name}]">{{r.name || '未命名服务'}}</span></td>
   <td><strong class="domain-name">{{r.domain}}</strong><div class="endpoint-meta"><span :class="['protocol-badge',r.scheme]">{{r.scheme.toUpperCase()}}</span><span class="endpoint-port">端口 {{r.port}}</span></div></td>
   <td><span class="target-address">{{splitTarget(r.upstream).address}}</span><div class="endpoint-meta"><span :class="['protocol-badge',splitTarget(r.upstream).scheme]">{{splitTarget(r.upstream).scheme.toUpperCase()}}</span><span class="endpoint-port">端口 {{splitTarget(r.upstream).port}}</span></div></td>
   <td><span :class="['route-status',r.enabled?'enabled':'disabled']">{{r.enabled?'启用':'停用'}}</span></td><td><div class="small-actions"><button class="secondary" :disabled="busy" :aria-label="'编辑 '+r.domain+':'+r.port" @click="editRoute(r)">编辑</button><button class="danger" :disabled="busy" :aria-label="'删除 '+r.domain+':'+r.port" @click="askDelete('routes',r.id,(r.name?r.name+' · ':'')+r.domain+':'+r.port)">删除</button></div></td>
  </tr></tbody></table>
 </div>
 <div v-if="config.routes.length" class="mobile-routes"><article v-for="r in config.routes" :key="r.id" class="mobile-route"><div class="mobile-route-head"><strong>{{r.name||'未命名服务'}}</strong><span :class="['route-status',r.enabled?'enabled':'disabled']">{{r.enabled?'启用':'停用'}}</span></div><div class="mobile-endpoint"><span class="muted">访问地址</span><strong class="domain-name">{{r.domain}}</strong><div class="endpoint-meta"><span :class="['protocol-badge',r.scheme]">{{r.scheme.toUpperCase()}}</span><span class="endpoint-port">端口 {{r.port}}</span></div></div><div class="mobile-endpoint"><span class="muted">目标服务</span><span class="target-address">{{splitTarget(r.upstream).address}}</span><div class="endpoint-meta"><span :class="['protocol-badge',splitTarget(r.upstream).scheme]">{{splitTarget(r.upstream).scheme.toUpperCase()}}</span><span class="endpoint-port">端口 {{splitTarget(r.upstream).port}}</span></div></div><div class="small-actions"><button class="secondary" :disabled="busy" :aria-label="'编辑 '+r.domain+':'+r.port" @click="editRoute(r)">编辑</button><button class="danger" :disabled="busy" :aria-label="'删除 '+r.domain+':'+r.port" @click="askDelete('routes',r.id,(r.name?r.name+' · ':'')+r.domain+':'+r.port)">删除</button></div></article></div>
 </section>
 </div>
 <KeepAlive><CertificateManager :revision="dataRevision" v-if="tab==='certificates'" :certificates="config.certificates" :api="api" :download="download" @add="add" @edit="editCert" @remove="c=>askDelete('certificates',c.id,c.domains.join(', '))" /></KeepAlive>
 <KeepAlive><DDNSManager :revision="dataRevision" v-if="tab==='ddns'||tab==='dnsaccounts'" :key="tab" :api="api" :mode="tab==='dnsaccounts'?'accounts':'tasks'" /></KeepAlive>
 <template v-if="tab==='engine'"><section class="card engine-card"><div class="section-head"><div><h2>运行状态</h2><p class="overview">反向代理由独立 Caddy 进程提供。</p></div><button class="secondary" :disabled="busy" @click="loadPage()">刷新状态</button></div><div class="engine-metrics"><div><span>代理服务</span><strong :class="['engine-status',{running:status.proxy?.running}]"><span :class="['dot',status.proxy?.running?'on':'']"></span>{{status.proxy?.running?'运行中':'未运行'}}</strong></div><div><span>代理引擎</span><strong>Caddy</strong></div><div><span>已保存配置版本</span><strong>{{config.revision}}</strong></div></div><p v-if="!status.proxy?.running" class="empty">代理尚未运行，请检查独立 Caddy 进程后刷新状态。</p></section><section class="card engine-card"><div class="section-head"><div><h2>配置与诊断</h2><p class="overview">校验已保存的规则，或查看配置详情以排查问题。</p></div></div><div class="diagnostic-row"><div><strong>校验配置</strong><p>检查当前规则能否生成有效的代理配置。</p></div><button class="secondary" :disabled="busy" @click="validateConfig">校验配置</button></div><div class="diagnostic-row"><div><strong>查看 Caddy 配置</strong><p>查看配置草稿和最近已应用的配置，支持复制。</p></div><button class="secondary" :disabled="busy" @click="showCaddy">查看配置</button></div><div class="diagnostic-row"><div><strong>应用配置</strong><p>将已保存的代理规则与证书配置应用到代理。</p></div><button :disabled="busy" @click="applyConfig">应用配置</button></div></section></template>
 </PageState>
 <footer>NetLite · 让网络管理轻一点</footer>
 </div>
 <dialog ref="editor" class="editor-dialog" :class="{'route-editor':tab==='routes'}" aria-labelledby="editor-title" @cancel="busy && $event.preventDefault()">
  <section v-if="tab==='routes'">
   <div class="dialog-head"><h2 id="editor-title">{{editing?'编辑':'添加'}}代理</h2><button type="button" class="icon-button" aria-label="关闭" :disabled="busy" @click="closeEditor">×</button></div>
   <form class="route-form" @submit.prevent="submitRoute">
    <div class="route-form-body">
    <label>名称 / 备注（选填）<input v-model="route.name" placeholder="例如：家庭相册、下载工具"></label>
    <fieldset class="endpoint-fields">
     <legend>访问端</legend>
     <div v-if="route.scheme==='https'" class="endpoint-certificate-row">
      <label for="route-certificate">HTTPS 证书</label><select id="route-certificate" v-model="route.certificate_id" @change="selectRouteCertificate"><option value="">请选择证书</option><option v-for="c in config.certificates" :key="c.id" :value="c.id">{{c.domains.join(', ')}}</option></select>
      <p v-if="!config.certificates.length" class="muted">还没有可选证书，请先到「HTTPS 证书」添加。</p>
     </div>
     <div class="endpoint-row">
      <label class="endpoint-protocol">访问协议<select v-model="route.scheme"><option value="http">HTTP</option><option value="https">HTTPS</option></select></label>
      <RouteDomainInput :key="route.id" v-model="route.domain" :domains="route.scheme==='https'?selectedRouteCertificate?.domains:undefined" @update:model-value="autoFilledDomain=''" />
      <label class="endpoint-port-field">访问端口<input v-model.number="route.port" type="number" min="1024" max="65535" required></label>
     </div>
    </fieldset>
    <fieldset class="endpoint-fields">
     <legend>目标端</legend>
     <div class="endpoint-row">
      <label class="endpoint-protocol">目标协议<select v-model="target.scheme"><option value="http">HTTP</option><option value="https">HTTPS</option></select></label>
      <label class="endpoint-address">目标地址<input v-model="target.address" placeholder="192.168.1.10 或 service.local" required></label>
      <label class="endpoint-port-field">目标端口<input v-model.number="target.port" type="number" min="1" max="65535" required></label>
     </div>
     <p class="muted">目标地址填写实际提供服务的 IP 或域名。127.0.0.1 表示运行 NetLite 的这台设备。</p>
    </fieldset>
    <label class="check"><input v-model="route.enabled" type="checkbox">启用规则</label>
    <p v-if="modalError" class="form-error" role="alert">{{modalError}}</p>
    </div>
    <div class="dialog-actions"><button type="button" class="secondary" :disabled="busy" @click="closeEditor">取消</button><button :disabled="busy"><span v-if="busy" class="loading-spinner button-spinner" aria-hidden="true"></span>{{busy?'保存中…':'保存规则'}}</button></div>
   </form>
  </section>
<section v-if="tab==='certificates'"><div class="dialog-head"><h2 id="editor-title">{{editing?'编辑':'添加'}}证书策略</h2><button type="button" class="icon-button" aria-label="关闭" :disabled="busy" @click="closeEditor">×</button></div><form @submit.prevent="submitCert"><label>域名（逗号分隔）<input v-model="domains" placeholder="*.example.com, example.com" required></label><label>签发方式<select v-model="cert.mode"><option value="internal">内部 CA（本地测试）</option><option value="acme">ACME（HTTP-01）</option><option value="dns">DNS-01（引用 DNS 账号）</option></select></label><label v-if="cert.mode!=='internal'">联系邮箱<input v-model="cert.email" type="email" required></label><template v-if="cert.mode==='dns'"><label>DNS 账号<select v-model="cert.account_id" required><option value="">请选择 DNS 账号</option><option v-for="a in dnsAccounts.filter(a=>certificateProviders.includes(a.provider))" :key="a.id" :value="a.id">{{a.name}} · {{a.provider}}</option></select></label><p class="muted">账号与 DDNS 共用，证书验证需要 TXT 查询、创建和删除权限，无需启用 DDNS。</p><p v-if="!dnsAccounts.some(a=>certificateProviders.includes(a.provider))" class="muted">暂无支持证书验证的 DNS 账号，请先在左侧「DNS 账号」中添加。</p></template><label>提前续期天数（0 表示自动）<input v-model.number="cert.renew_before_days" type="number" min="0" max="60"></label><label v-if="cert.mode!=='internal'" class="check"><input v-model="cert.staging" type="checkbox">使用 Let's Encrypt 测试 CA</label><p class="muted">保存后自动申请并管理续期。HTTP-01 需要公网 80 映射到独立验证端口 18082；DNS-01 无需开放验证端口。</p><p v-if="modalError" class="form-error" role="alert">{{modalError}}</p><div class="dialog-actions"><button type="button" class="secondary" :disabled="busy" @click="closeEditor">取消</button><button :disabled="busy"><span v-if="busy" class="loading-spinner button-spinner" aria-hidden="true"></span>{{busy?'保存中…':'保存策略'}}</button></div></form></section></dialog>
 <dialog ref="deletion" class="delete-dialog" aria-labelledby="delete-title" @cancel="busy && $event.preventDefault()">
  <div class="dialog-head"><h2 id="delete-title">{{certificateUsers.length?'无法删除证书':'确认删除'}}</h2></div>
  <template v-if="certificateUsers.length">
   <p>证书「{{pendingDelete?.name}}」仍被以下反向代理规则引用：</p>
   <ul class="certificate-users"><li v-for="r in certificateUsers" :key="r.id"><strong>{{r.name||r.id}}</strong><span>{{r.domain}}:{{r.port}} · {{r.enabled?'已启用':'已停用'}}</span></li></ul>
   <p class="muted">请先到「反向代理」为这些规则更换证书，或删除不再使用的规则，再删除此证书。已停用的 HTTPS 规则也需要有效证书。</p>
  </template>
  <template v-else><p>确定删除「{{pendingDelete?.name}}」？</p><p class="muted">删除后需应用到代理，反代与证书的变更才会生效。</p></template>
  <p v-if="modalError" class="form-error" role="alert">{{modalError}}</p>
  <div class="dialog-actions"><button class="secondary" :disabled="busy" @click="deletion?.close()">{{certificateUsers.length?'关闭':'取消'}}</button><button v-if="!certificateUsers.length" class="danger" :disabled="busy" @click="pendingDelete && remove(pendingDelete.kind,pendingDelete.key)"><span v-if="busy" class="loading-spinner button-spinner" aria-hidden="true"></span>{{busy?'删除中…':'确认删除'}}</button></div>
 </dialog>
 <dialog ref="viewer" class="config-dialog" aria-labelledby="config-title"><div class="dialog-head"><h2 id="config-title">Caddy 配置</h2><button class="icon-button" aria-label="关闭配置" @click="viewer?.close()">×</button></div><p class="muted">草稿由当前保存的规则生成；已应用配置为最近成功应用后保存的配置。凭据已隐藏。</p><div class="config-toolbar"><button :class="['secondary',{selected:viewTab==='draft'}]" @click="viewTab='draft'">配置草稿</button><button :class="['secondary',{selected:viewTab==='applied'}]" @click="viewTab='applied'">已应用配置</button><button class="secondary" :disabled="viewBusy" @click="loadCaddy">刷新</button><button class="secondary" :disabled="viewBusy||!caddyView[viewTab]" @click="copyConfig">复制配置</button></div><p v-if="copyMessage" class="muted" role="status">{{copyMessage}}</p><p v-if="viewError" class="form-error" role="alert">{{viewError}}</p><p v-if="viewBusy" class="empty">正在加载配置…</p><pre v-else-if="caddyView[viewTab]" tabindex="0">{{JSON.stringify(caddyView[viewTab],null,2)}}</pre><p v-else-if="!viewError" class="empty">{{viewTab==='applied'?'暂无已应用的代理规则。':'暂无配置草稿。'}}</p><div class="dialog-actions"><button @click="viewer?.close()">关闭</button></div></dialog>
 </template></main>
</template>

