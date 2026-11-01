<script setup lang="ts">
import {computed,ref} from 'vue'
import {certificateEntries,certificateDomains,normalizeCertificateEntry,type CertificateDomainEntry} from './certificate-domains'
type Cert={id:string;domains:string[];mode:string;email:string;staging:boolean;account_id?:string;renew_before_days?:number}
const props=defineProps<{certificate:Cert;accounts:{id:string;name:string;provider:string}[];providers:string[];busy:boolean;error:string}>()
const emit=defineEmits<{save:[Cert];cancel:[]}>()
const draft=ref({...props.certificate})
const entries=ref(certificateEntries(props.certificate.domains))
const localError=ref('')
const advanced=ref(false)
const customRenewal=ref(!!draft.value.renew_before_days)
const supportedAccounts=computed(()=>props.accounts.filter(a=>props.providers.includes(a.provider)))
const hasWildcard=computed(()=>entries.value.some(e=>e.wildcard))
function normalize(entry:CertificateDomainEntry){
  normalizeCertificateEntry(entry)
  if(entry.wildcard)draft.value.mode='dns'
  localError.value=''
}
function toggleWildcard(entry:CertificateDomainEntry){
  if(entry.wildcard)draft.value.mode='dns'
  else entry.includeBase=true
}
function submit(){
  try{
    entries.value.forEach(normalize)
    const domains=certificateDomains(entries.value)
    if(draft.value.mode==='dns'&&!draft.value.account_id)throw new Error('请选择用于证书验证的 DNS 账号。')
    emit('save',{...draft.value,domains,account_id:draft.value.mode==='dns'?draft.value.account_id:'',renew_before_days:customRenewal.value?draft.value.renew_before_days:0})
  }catch(e){localError.value=e instanceof Error?e.message:String(e)}
}
</script>
<template>
 <form class="certificate-form" @submit.prevent="submit">
  <div class="certificate-form-body">
  <fieldset :disabled="busy" class="certificate-controls">
   <div class="cert-domains">
    <div v-for="(entry,index) in entries" :key="index" class="cert-domain-row">
     <div class="cert-domain-heading"><label :for="`cert-domain-${index}`">{{entries.length>1?`域名 ${index+1}`:'域名'}}</label><button v-if="entries.length>1" type="button" class="text-button" :aria-label="`移除域名 ${index+1}`" @click="entries.splice(index,1)">移除</button></div>
     <input :id="`cert-domain-${index}`" v-model="entry.domain" placeholder="example.com" required autocomplete="off" autocapitalize="none" spellcheck="false" :aria-describedby="`cert-domain-help-${index}`" @input="normalize(entry)">
     <label class="check"><input v-model="entry.wildcard" type="checkbox" @change="toggleWildcard(entry)">同时包含泛域名<span v-if="entry.domain" class="muted">（*.{{entry.domain}}）</span></label>
     <label v-if="entry.wildcard&&!entry.includeBase" class="check"><input v-model="entry.includeBase" type="checkbox">同时包含 {{entry.domain}}</label>
     <p :id="`cert-domain-help-${index}`" class="cert-help">请输入域名，不含协议、端口或路径。</p>
     <p v-if="entry.wildcard&&!entry.includeBase" class="cert-help">当前策略仅包含泛域名，不包含该域名本身。</p>
    </div>
    <button type="button" class="text-button cert-add-domain" @click="entries.push({domain:'',wildcard:false,includeBase:true})">＋ 添加其他域名</button>
   </div>
   <fieldset class="cert-methods"><legend>验证方式</legend>
    <label :class="['cert-method',{selected:draft.mode==='dns'}]"><input v-model="draft.mode" type="radio" value="dns"><span><strong>DNS 验证</strong><small>支持泛域名，无需开放验证端口</small></span></label>
    <label :class="['cert-method',{selected:draft.mode==='acme'}]"><input v-model="draft.mode" type="radio" value="acme" :disabled="hasWildcard"><span><strong>HTTP 验证</strong><small>无需 DNS 账号，需要公网 80 端口可达</small></span></label>
    <label :class="['cert-method',{selected:draft.mode==='internal'}]"><input v-model="draft.mode" type="radio" value="internal" :disabled="hasWildcard"><span><strong>内部 CA</strong><small>适用于本地环境，需手动信任根证书</small></span></label>
   </fieldset>
   <template v-if="draft.mode==='dns'">
    <label>DNS 账号<select v-model="draft.account_id" required aria-describedby="cert-dns-help"><option value="">请选择 DNS 账号</option><option v-for="a in supportedAccounts" :key="a.id" :value="a.id">{{a.name}} · {{a.provider}}</option></select></label>
    <p id="cert-dns-help" class="cert-help">{{supportedAccounts.length?'需要 TXT 记录的查询、创建和删除权限；账号与 DDNS 共用，无需启用 DDNS。':'暂无支持证书验证的账号，请先在「DNS 账号」中添加。'}}</p>
   </template>
   <p v-if="draft.mode==='acme'" class="cert-notice">请将公网 80 端口映射到独立验证端口 18082，并将域名解析到本机公网 IP。</p>
   <label v-if="draft.mode!=='internal'">联系邮箱<input v-model="draft.email" type="email" required autocomplete="email" placeholder="用于接收证书服务通知"></label>
   <details :open="advanced" class="cert-advanced" @toggle="advanced=($event.target as HTMLDetailsElement).open"><summary>高级设置 <span>{{customRenewal?'自定义续期':'自动续期'}}{{draft.mode!=='internal'?(draft.staging?' · 测试环境':' · 正式环境'):''}}</span></summary>
    <label>续期方式<select v-model="customRenewal"><option :value="false">自动（推荐）</option><option :value="true">自定义提前天数</option></select></label>
    <label v-if="customRenewal">到期前多少天续期<input v-model.number="draft.renew_before_days" type="number" min="1" max="60" required></label>
    <label v-if="draft.mode!=='internal'">证书环境<select v-model="draft.staging"><option :value="false">正式 · 浏览器信任</option><option :value="true">测试 · 仅验证配置</option></select></label>
   </details>
   <p v-if="draft.mode!=='internal'&&draft.staging" class="cert-notice" role="status">当前使用 Let's Encrypt 测试环境，签发的证书不受浏览器信任。</p>
   <p v-if="localError||error" class="form-error" role="alert">{{localError||error}}</p>
  </fieldset>
  </div>
  <div class="dialog-actions"><button type="button" class="secondary" :disabled="busy" @click="emit('cancel')">取消</button><button :disabled="busy"><span v-if="busy" class="loading-spinner button-spinner" aria-hidden="true"></span>{{busy?'保存中…':'保存并申请'}}</button></div>
 </form>
</template>
