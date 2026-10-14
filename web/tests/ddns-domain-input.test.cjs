const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const vue=require('vue')

const source=fs.readFileSync(path.join(__dirname,'../src/DDNSDomainInput.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText
function domainInput(value=''){
  const props=vue.reactive({modelValue:value,disabled:false}),changes=[]
  const context={exports:{},defineProps:()=>props,
    defineEmits:()=>((event,value)=>{assert.equal(event,'update:modelValue');changes.push(value);props.modelValue=value}),
    require:name=>{assert.equal(name,'vue');return vue}}
  vm.runInNewContext(compiled+'\nglobalThis.input={groups,batch,selectedDomains,target,updateDomain,finishDomain,toggle,removeGroup,toggleBatch,publish}',context)
  return {props,changes,...context.input}
}
function domains(input){return input.props.modelValue.split(/[,\s，]+/).filter(Boolean).sort()}

test('editing existing tasks groups common records without changing or losing custom hosts',()=>{
  const input=domainInput('*.example.com, example.com, www.example.com, nas.example.com, home.other.co.uk')
  assert.deepEqual(input.changes,[])
  assert.equal(input.groups.value.length,3)
  const group=input.groups.value[0]
  assert.equal(group.domain,'example.com')
  input.toggle(group,'www')
  assert.deepEqual(domains(input),['*.example.com','example.com','home.other.co.uk','nas.example.com'])
  input.toggle(group,'www')
  assert.deepEqual(domains(input),['*.example.com','example.com','home.other.co.uk','nas.example.com','www.example.com'])
})

test('a domain is entered once and selected records follow domain edits and can be deselected',async()=>{
  const input=domainInput(),group=input.groups.value[0]
  input.updateDomain(group,'example.com')
  input.toggle(group,'*')
  input.toggle(group,'www')
  input.toggle(group,'@')
  assert.deepEqual(domains(input),['*.example.com','www.example.com'])
  assert.deepEqual(Array.from(input.selectedDomains.value),['*.example.com','www.example.com'])
  input.updateDomain(group,'other.net')
  await vue.nextTick()
  assert.equal(input.groups.value[0].id,group.id)
  assert.deepEqual(domains(input),['*.other.net','www.other.net'])
  input.toggle(group,'*')
  input.toggle(group,'www')
  await vue.nextTick()
  assert.equal(input.props.modelValue,'')
  assert.equal(input.groups.value[0].domain,'other.net')
  assert.equal(input.groups.value[0].records.length,0)
})

test('existing groups can be removed without changing other domains',async()=>{
  const input=domainInput('example.com, other.net')
  const extra=input.groups.value[1]
  input.toggle(extra,'*')
  input.toggle(extra,'@')
  await vue.nextTick()
  assert.equal(input.groups.value.length,2)
  assert.deepEqual(domains(input),['*.other.net','example.com'])
  input.removeGroup(input.groups.value[0])
  assert.deepEqual(domains(input),['*.other.net'])
  input.removeGroup(extra)
  assert.equal(input.props.modelValue,'')
  assert.equal(input.groups.value.length,1)
  assert.equal(input.groups.value[0].domain,'')
})

test('batch input preserves custom hosts and parses Chinese commas and complete wildcard records',()=>{
  const input=domainInput('example.com')
  input.toggleBatch()
  input.publish('*.other.net，www.other.net\nnas.example.com')
  input.toggleBatch()
  assert.equal(input.batch.value,false)
  assert.equal(input.groups.value.length,2)
  input.toggle(input.groups.value[0],'@')
  assert.deepEqual(domains(input),['*.other.net','nas.example.com','other.net','www.other.net'])
  const group=input.groups.value[1]
  input.updateDomain(group,'*.third.org')
  input.finishDomain(group)
  assert.equal(group.domain,'third.org')
  assert.deepEqual(domains(input),['*.other.net','*.third.org','other.net','www.other.net'])
})

test('record selection deduplicates equivalent DNS names and separate IP families remain independent',()=>{
  const v4=domainInput('EXAMPLE.COM., *.example.com, www.example.com, example.com')
  const v6=domainInput('v6.example.net')
  v4.updateDomain(v4.groups.value[0],'EXAMPLE.COM.')
  assert.equal(domains(v4).length,3)
  assert.equal(v6.props.modelValue,'v6.example.net')
  v4.props.disabled=true
  const before=v4.props.modelValue
  v4.toggle(v4.groups.value[0],'@')
  v4.updateDomain(v4.groups.value[0],'changed.org')
  assert.equal(v4.props.modelValue,before)
})
