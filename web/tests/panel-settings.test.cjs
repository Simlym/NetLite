const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const vue=require('vue')

function settings(api){
  const context={exports:{},Error,Date,defineProps:()=>({api,certificates:[]}),
    sessionStorage:{setItem:()=>{}},
    require:name=>name==='vue'?{...vue,onMounted:()=>{},onUnmounted:()=>{},onActivated:()=>{},onDeactivated:()=>{}}:{}}
  const source=fs.readFileSync(path.join(__dirname,'../src/PanelSettings.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  vm.runInNewContext(ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText+'\nglobalThis.state={panel,enabled,domain,certificate,redirect,httpsChanged,feedbackTarget,error,message,oldPassword,newPassword,password,enable,refreshPending,pageLoading}',context)
  return context.state
}

test('HTTPS saves are available only when the persisted settings change',()=>{
  const s=settings(async()=>({}))
  s.panel.value={enabled:false,domain:'',certificate_id:'',redirect:false}
  s.enabled.value=false
  assert.equal(s.httpsChanged.value,false)
  s.enabled.value=true
  assert.equal(s.httpsChanged.value,true)
  s.panel.value={enabled:true,domain:'panel.example.com',certificate_id:'cert',redirect:true}
  s.domain.value='panel.example.com';s.certificate.value='cert';s.redirect.value=true
  assert.equal(s.httpsChanged.value,false)
  for(const [field,value] of [['domain','new.example.com'],['certificate','other'],['redirect',false],['enabled',false]]){
    const previous=s[field].value
    s[field].value=value
    assert.equal(s.httpsChanged.value,true,field)
    s[field].value=previous
    assert.equal(s.httpsChanged.value,false,field)
  }
})

test('password failure keeps the draft and shows feedback in the password card',async()=>{
  const s=settings(async()=>{throw new Error('当前密码错误')})
  s.oldPassword.value='incorrect';s.newPassword.value='new long password'
  await s.password()
  assert.equal(s.feedbackTarget.value,'password')
  assert.equal(s.error.value,'当前密码错误')
  assert.equal(s.newPassword.value,'new long password')
})

test('successful password updates clear both fields and isolate subsequent HTTPS errors',async()=>{
  const calls=[]
  const s=settings(async(path,method,body)=>{
    calls.push({path,method,body})
    if(path==='/panel')throw new Error('请选择已签发的证书')
    return path==='/auth/me'?{csrf_token:'updated'}:{}
  })
  s.oldPassword.value='old long password';s.newPassword.value='new long password'
  await s.password()
  assert.equal(calls[0].body.old_password,'old long password')
  assert.equal(calls[0].body.new_password,'new long password')
  assert.equal(s.oldPassword.value,'');assert.equal(s.newPassword.value,'')
  assert.equal(s.feedbackTarget.value,'password')
  assert.match(s.message.value,/密码已修改/)
  await s.enable()
  assert.equal(s.feedbackTarget.value,'https')
  assert.equal(s.message.value,'')
  assert.equal(s.error.value,'请选择已签发的证书')
})

test('pending HTTPS refresh errors stay with the HTTPS card after a password operation',async()=>{
  const s=settings(async()=>{throw new Error('offline')})
  s.panel.value={pending:true}
  s.feedbackTarget.value='password'
  s.message.value='密码已修改'
  s.pageLoading.value=false
  await s.refreshPending()
  assert.equal(s.feedbackTarget.value,'https')
  assert.equal(s.error.value,'offline')
  assert.equal(s.message.value,'')
})
