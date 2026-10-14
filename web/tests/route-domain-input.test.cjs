const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const vue=require('vue')

function domainInput(domain,domains){
  const props=vue.reactive({modelValue:domain,domains}),changes=[]
  const helpers={exports:{}}
  const compile=source=>ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText
  vm.runInNewContext(compile(fs.readFileSync(path.join(__dirname,'../src/route-certificate.ts'),'utf8')),helpers)
  const context={exports:{},defineProps:()=>props,withDefaults:p=>p,
    defineEmits:()=>((event,value)=>{assert.equal(event,'update:modelValue');changes.push(value);props.modelValue=value}),
    require:name=>name==='vue'?vue:helpers.exports}
  const source=fs.readFileSync(path.join(__dirname,'../src/RouteDomainInput.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  vm.runInNewContext(compile(source)+'\nglobalThis.input={mode,base,prefix,updatePrefix,changeBase,toggleMode}',context)
  return {props,changes,...context.input}
}

test('editing an apex preserves it until the user chooses to fill a subdomain',()=>{
  const input=domainInput('iminfo.top',['*.iminfo.top','iminfo.top'])
  assert.equal(input.mode.value,'full')
  assert.deepEqual(input.changes,[])
  input.toggleMode()
  assert.equal(input.mode.value,'subdomain')
  assert.equal(input.props.modelValue,'')
  input.updatePrefix('nas')
  assert.equal(input.props.modelValue,'nas.iminfo.top')
  input.toggleMode()
  assert.equal(input.mode.value,'full')
  assert.equal(input.props.modelValue,'nas.iminfo.top')
  input.toggleMode()
  assert.equal(input.prefix.value,'nas')
})

test('subdomain editing and pasting a complete hostname never append the suffix twice',()=>{
  const input=domainInput('photos.iminfo.top',['*.iminfo.top','iminfo.top'])
  assert.equal(input.mode.value,'subdomain')
  assert.equal(input.prefix.value,'photos')
  input.updatePrefix(' NAS.IMINFO.TOP ')
  assert.equal(input.prefix.value,'nas')
  assert.equal(input.props.modelValue,'nas.iminfo.top')
  input.updatePrefix('')
  assert.equal(input.props.modelValue,'')
  input.updatePrefix('a.b')
  assert.equal(input.prefix.value,'a.b')
  assert.equal(input.props.modelValue,'a.b.iminfo.top')
})

test('multiple wildcard suffixes can be switched while preserving the prefix',async()=>{
  const input=domainInput('nas.other.com',['*.iminfo.top','*.other.com'])
  assert.equal(input.base.value,'other.com')
  input.changeBase('iminfo.top')
  assert.equal(input.props.modelValue,'nas.iminfo.top')
  input.props.domains=['exact.example.com']
  await vue.nextTick()
  assert.equal(input.mode.value,'full')
  assert.equal(input.props.modelValue,'nas.iminfo.top')
})
