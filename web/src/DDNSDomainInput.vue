<script setup lang="ts">
import {ref,computed,watch,nextTick} from 'vue'

const props=defineProps<{modelValue:string;label:string;id:string;disabled?:boolean}>()
const emit=defineEmits<{(event:'update:modelValue',value:string):void}>()
const shortcuts=[{value:'@',name:'根域名'},{value:'*',name:'泛域名'},{value:'www',name:'子域名'}] as const
type Shortcut=typeof shortcuts[number]['value']
type DomainGroup={id:number;domain:string;records:Shortcut[]}
const groups=ref<DomainGroup[]>([]),batch=ref(false),field=ref<HTMLElement>()
const selectedDomains=computed(()=>props.modelValue.split(/[,\s，]+/).filter(Boolean))
let nextID=0,lastEmitted:string|undefined,batchSnapshot=''
function normalize(domain:string){return domain.toLowerCase().replace(/\.$/,'')}
function freshGroup():DomainGroup{return {id:nextID++,domain:'',records:['@']}}
function parse(value:string):DomainGroup[]{
  const parsed:DomainGroup[]=[]
  for(const domain of value.split(/[,\s，]+/).filter(Boolean)){
    const match=domain.match(/^(\*|www)\.(.+)$/i)
    const base=match?match[2]!:domain,record=(match?match[1]!.toLowerCase():'@') as Shortcut
    let group=parsed.find(item=>normalize(item.domain)===normalize(base))
    if(!group){group={id:nextID++,domain:base,records:[]};parsed.push(group)}
    if(!group.records.includes(record))group.records.push(record)
  }
  return parsed.length?parsed:[freshGroup()]
}
watch(()=>props.modelValue,value=>{if(value!==lastEmitted)groups.value=parse(value)},{immediate:true})
function target(group:DomainGroup,record:Shortcut){
  const domain=group.domain.trim()
  return domain?(record==='@'?domain:`${record}.${domain}`):''
}
function preview(group:DomainGroup,record:Shortcut){return target(group,record)||(record==='@'?'example.com':`${record}.example.com`)}
function publish(value:string){lastEmitted=value;emit('update:modelValue',value)}
function sync(){
  const seen=new Set<string>(),domains:string[]=[]
  for(const group of groups.value){
    for(const record of group.records){
      const domain=target(group,record),key=normalize(domain)
      if(domain&&!seen.has(key)){seen.add(key);domains.push(domain)}
    }
  }
  publish(domains.join(', '))
}
function updateDomain(group:DomainGroup,value:string){if(props.disabled)return;group.domain=value;sync()}
function finishDomain(group:DomainGroup){
  if(props.disabled)return
  // Complete records entered in the domain field retain their meaning.
  const value=group.domain.trim()
  if(/[,\s，]/.test(value)||(/^(\*|www)\./i.test(value)&&group.records.length===1&&group.records[0]==='@')){
    const parsed=parse(value),first=parsed[0]!
    group.domain=first.domain;group.records=first.records
    groups.value.splice(groups.value.indexOf(group)+1,0,...parsed.slice(1))
  }else group.domain=value
  sync()
}
function toggle(group:DomainGroup,record:Shortcut){
  if(props.disabled)return
  const index=group.records.indexOf(record)
  if(index<0)group.records.push(record);else group.records.splice(index,1)
  sync()
}
function removeGroup(group:DomainGroup){
  if(props.disabled)return
  groups.value=groups.value.filter(item=>item.id!==group.id)
  if(!groups.value.length)groups.value=[freshGroup()]
  sync()
}
async function toggleBatch(){
  if(props.disabled)return
  if(batch.value){if(props.modelValue!==batchSnapshot)groups.value=parse(props.modelValue)}
  else batchSnapshot=props.modelValue
  batch.value=!batch.value
  await nextTick()
  field.value?.querySelector<HTMLInputElement|HTMLTextAreaElement>(batch.value?'textarea':'.domain-entry input')?.focus()
}
</script>

<template>
  <div ref="field" class="domain-field">
    <div class="domain-heading">
      <label :for="batch?id+'-batch':id+'-'+groups[0]?.id">{{label}}</label>
      <button type="button" class="domain-action" :disabled="disabled" @click="toggleBatch">{{batch?'返回选择':'批量填写'}}</button>
    </div>
    <template v-if="batch">
      <textarea :id="id+'-batch'" :value="modelValue" :disabled="disabled" :aria-describedby="id+'-help'" rows="3" placeholder="example.com, *.example.com, nas.example.net" required @input="publish(($event.target as HTMLTextAreaElement).value)"></textarea>
      <p :id="id+'-help'" class="domain-help">填写完整域名，多个域名用逗号或换行分隔。</p>
    </template>
    <template v-else>
      <div v-for="(group,index) in groups" :key="group.id" class="domain-group">
        <div class="domain-entry">
          <input :id="id+'-'+group.id" :value="group.domain" :disabled="disabled" :aria-label="label+' '+(index+1)" placeholder="填写域名，例如 example.com" autocomplete="off" autocapitalize="none" spellcheck="false" required @input="updateDomain(group,($event.target as HTMLInputElement).value)" @change="finishDomain(group)">
          <button v-if="groups.length>1" type="button" class="remove-domain" :disabled="disabled" :aria-label="'移除域名 '+(group.domain||index+1)" @click="removeGroup(group)">×</button>
        </div>
        <div class="record-options" role="group" :aria-label="(group.domain||label)+'解析记录'">
          <label v-for="shortcut in shortcuts" :key="shortcut.value" class="record-option" :class="{selected:group.records.includes(shortcut.value),disabled}">
            <input type="checkbox" :checked="group.records.includes(shortcut.value)" :disabled="disabled" :required="shortcut.value==='@'&&!group.records.length" @change="toggle(group,shortcut.value)">
            <span class="record-copy"><span class="record-label"><strong>{{shortcut.value}}</strong><span>{{shortcut.name}}</span></span><span class="record-preview" :title="target(group,shortcut.value)">{{preview(group,shortcut.value)}}</span></span>
          </label>
        </div>
        <p v-if="!group.records.length" class="domain-help selection-error" role="alert">请至少选择一种记录。</p>
      </div>

    </template>
    <p class="selected-summary" role="status"><template v-if="selectedDomains.length">已选域名：<span>{{selectedDomains.join('、')}}</span></template><template v-else>填写域名并勾选需要更新的记录</template></p>
  </div>
</template>

<style scoped>
.domain-field{margin:16px 0}.domain-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:7px}.domain-heading>label{margin:0}.domain-action{padding:2px 0;border:0;border-radius:3px;background:transparent;color:#226f66;font-size:12px;line-height:1.6}.domain-action:hover{background:#e7f3ef}.domain-action:focus-visible{outline:2px solid #98c7b6;outline-offset:3px}.domain-action:disabled{color:#a0aab2;background:transparent}.domain-group+.domain-group{margin-top:16px;padding-top:16px;border-top:1px solid #e8eef0}.domain-entry{display:flex;align-items:center;gap:8px}.domain-entry>input{margin:0;min-width:0}.remove-domain{flex-shrink:0;width:30px;padding:0;border:0;background:transparent;color:#82918d;font-size:22px}.remove-domain:hover{color:#ad4938;background:#fff0ed}.record-options{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-top:8px}.record-option{display:flex;align-items:flex-start;gap:8px;min-width:0;margin:0;padding:10px;border:1px solid #dce5e9;border-radius:8px;background:#fff;cursor:pointer;transition:background .15s,border-color .15s}.record-option:hover{border-color:#a6c9bf;background:#f7faf9}.record-option.selected{border-color:#9dc8ba;background:#eef7f3}.record-option:focus-within{outline:2px solid #98c7b6;outline-offset:1px}.record-option.disabled{cursor:default;opacity:.65}.record-option>input{width:14px;height:14px;flex-shrink:0;margin:3px 0 0;accent-color:#187669;outline:none}.record-copy{min-width:0}.record-label{display:flex;align-items:center;gap:6px;font-size:12px;line-height:20px;color:#405465}.record-label strong{font-size:13px;font-weight:600}.record-label>span{font-weight:400;color:#758594}.record-preview{display:block;margin-top:2px;font-size:12px;line-height:1.5;color:#758594;overflow-wrap:anywhere}.record-option.selected .record-label{color:#176f61}.selected-summary{font-size:12px;color:#758594;margin:8px 0 0;line-height:1.6;overflow-wrap:anywhere}.selected-summary span{color:#405465}.domain-help{font-size:12px;color:#758594;margin:7px 0 0;line-height:1.6}.selection-error{color:#b64c3c}textarea{display:block;width:100%;border:1px solid #d5dee5;border-radius:8px;padding:10px 12px;font:inherit;font-size:13px;color:#233a45;resize:vertical}textarea:focus{outline:2px solid #98c7b6;border-color:transparent}
@media(max-width:480px){.record-options{grid-template-columns:1fr}.record-option{align-items:center;padding:9px 10px}.record-option>input{margin:0}.record-copy{display:flex;align-items:baseline;justify-content:space-between;gap:8px;flex:1}.record-label{flex-shrink:0}.record-preview{margin:0;text-align:right}}
</style>