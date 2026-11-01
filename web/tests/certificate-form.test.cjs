const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const vue=require('vue')
const compile=source=>ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText
const helpers={exports:{}}
vm.runInNewContext(compile(fs.readFileSync(path.join(__dirname,'../src/certificate-domains.ts'),'utf8')),helpers)
const {certificateEntries,certificateDomains}=helpers.exports
const plain=value=>JSON.parse(JSON.stringify(value))
function form(certificate={}){
  const saved=[]
  const context={exports:{},defineProps:()=>({certificate:{id:'cert',domains:[],mode:'dns',email:'admin@example.com',staging:false,account_id:'dns',...certificate}}),defineEmits:()=>((event,value)=>saved.push(value)),require:name=>name==='vue'?vue:helpers.exports}
  const source=fs.readFileSync(path.join(__dirname,'../src/CertificateForm.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  vm.runInNewContext(compile(source)+'\nglobalThis.form={draft,entries,customRenewal,localError,normalize,toggleWildcard,submit}',context)
  return {...context.form,saved}
}

test('entering a domain once includes the apex and optional wildcard, without duplicates',()=>{
  const input=form()
  const entry=input.entries.value[0]
  entry.domain=' Example.COM '
  input.submit()
  assert.deepEqual(plain(input.saved[0].domains),['example.com'])
  entry.wildcard=true
  input.toggleWildcard(entry)
  input.submit()
  assert.deepEqual(plain(input.saved[1].domains),['example.com','*.example.com'])
  assert.deepEqual(plain(certificateDomains([entry,entry])),['example.com','*.example.com'])
})

test('pasting a wildcard switches HTTP validation to DNS and keeps the apex',()=>{
  const input=form({mode:'acme'})
  const entry=input.entries.value[0]
  entry.domain='*.EXAMPLE.COM'
  input.normalize(entry)
  assert.equal(entry.domain,'example.com')
  assert.equal(entry.wildcard,true)
  assert.equal(input.draft.value.mode,'dns')
  input.submit()
  assert.deepEqual(plain(input.saved[0].domains),['example.com','*.example.com'])
})

test('editing mixed and wildcard-only legacy policies preserves their coverage',()=>{
  for(const domains of [['*.example.com'],['example.com','*.example.com','other.com'],['*.a.example.com','b.example.com','*.b.example.com']]){
    assert.deepEqual(plain(certificateDomains(certificateEntries(domains))).sort(),[...domains].sort())
  }
  const input=form({domains:['*.example.com']})
  input.submit()
  assert.deepEqual(plain(input.saved[0].domains),['*.example.com'])
  input.entries.value[0].wildcard=false
  input.toggleWildcard(input.entries.value[0])
  input.submit()
  assert.deepEqual(plain(input.saved[1].domains),['example.com'])
})

test('invalid domains and missing DNS accounts prevent saving',()=>{
  const input=form()
  for(const domain of ['','https://example.com','example.com/path','example.com:443','a..com','-a.com','a_foo.com']){
    input.entries.value[0].domain=domain
    input.submit()
    assert.match(input.localError.value,/有效域名/)
  }
  assert.equal(input.saved.length,0)
  input.entries.value[0].domain='example.com'
  input.draft.value.account_id=''
  input.submit()
  assert.match(input.localError.value,/DNS 账号/)
  assert.equal(input.saved.length,0)
})

test('automatic renewal resets the submitted offset while custom renewal retains it',()=>{
  const input=form({domains:['example.com'],renew_before_days:20,mode:'acme',staging:true})
  input.submit()
  assert.equal(input.saved[0].renew_before_days,20)
  assert.equal(input.saved[0].account_id,'')
  assert.equal(input.saved[0].staging,true)
  input.customRenewal.value=false
  input.submit()
  assert.equal(input.saved[1].renew_before_days,0)
})
