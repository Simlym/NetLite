<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {wildcardBases, splitSubdomain, normalizeSubdomain} from './route-certificate'

const props=withDefaults(defineProps<{modelValue:string;domains?:string[]}>(),{domains:()=>[]})
const emit=defineEmits<{(e:'update:modelValue',value:string):void}>()
const bases=computed(()=>wildcardBases(props.domains))
const mode=ref<'full'|'subdomain'>('full'),base=ref('')
const exactDomains=computed(()=>props.domains.filter(d=>!d.startsWith('*.')))
const prefix=computed(()=>base.value&&props.modelValue.endsWith('.'+base.value)?props.modelValue.slice(0,-base.value.length-1):props.modelValue)

watch(()=>props.domains,()=>{
  const matched=bases.value.find(b=>splitSubdomain(props.modelValue,b)!==null)
  base.value=matched||bases.value[0]||''
  mode.value=base.value&&(!props.modelValue||matched)?'subdomain':'full'
},{immediate:true})

function updatePrefix(value:string){
  const label=normalizeSubdomain(value,base.value)
  emit('update:modelValue',label?`${label}.${base.value}`:'')
}
function changeBase(value:string){
  const label=prefix.value
  base.value=value
  updatePrefix(label)
}
function toggleMode(){
  if(mode.value==='subdomain'){mode.value='full';return}
  mode.value='subdomain'
  if(splitSubdomain(props.modelValue,base.value)===null)emit('update:modelValue','')
}
</script>

<template>
 <div class="endpoint-address">
  <div class="domain-label"><label for="route-domain">访问域名</label><button v-if="bases.length" type="button" class="text-button domain-mode" :title="mode==='subdomain'?'手动输入完整地址，例如 nas.'+base:'只填写 nas 等前缀，后缀由证书提供'" @click="toggleMode">{{mode==='subdomain'?'自定义完整域名':'仅填写子域名前缀'}}</button></div>
  <div v-if="mode==='subdomain'" class="subdomain-input">
   <input id="route-domain" :value="prefix" placeholder="例如 nas" aria-label="子域名前缀" aria-describedby="route-domain-help" autocomplete="off" autocapitalize="none" spellcheck="false" required pattern="[a-z0-9](?:[a-z0-9\-]{0,61}[a-z0-9])?" title="请输入一段子域名前缀，可使用字母、数字和连字符，例如 nas。" @input="updatePrefix(($event.target as HTMLInputElement).value)">
   <span v-if="bases.length===1" class="domain-suffix" :title="'.'+base">.{{base}}</span>
   <select v-else :value="base" aria-label="域名后缀" @change="changeBase(($event.target as HTMLSelectElement).value)"><option v-for="suffix in bases" :key="suffix" :value="suffix">.{{suffix}}</option></select>
  </div>
  <input v-else id="route-domain" :value="modelValue" :list="exactDomains.length?'route-certificate-domains':undefined" placeholder="app.example.com" autocomplete="off" autocapitalize="none" spellcheck="false" required @input="emit('update:modelValue',($event.target as HTMLInputElement).value)">
  <datalist v-if="exactDomains.length" id="route-certificate-domains"><option v-for="domain in exactDomains" :key="domain" :value="domain" /></datalist>
  <p v-if="mode==='subdomain'" id="route-domain-help" class="muted domain-preview">{{modelValue?'完整域名：'+modelValue:'只需填写前缀，例如 nas → nas.'+base}}</p>
 </div>
</template>
