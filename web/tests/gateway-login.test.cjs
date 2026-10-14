const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const {webcrypto}=require('node:crypto')

function loadModule(filename){
  const source=fs.readFileSync(path.join(__dirname,'../src',filename),'utf8')
  const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
  const sandbox={exports:{},URL}
  vm.runInNewContext(compiled,sandbox)
  return sandbox.exports
}

function mountApp(responses){
  const mounted=[],requests=[]
  const component=fs.readFileSync(path.join(__dirname,'../src/App.vue'),'utf8')
  const script=component.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  const compiled=ts.transpileModule(script,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
  const modules={
    './application-path':loadModule('application-path.ts'),
    './api-response':loadModule('api-response.ts'),
    './proxy-address':loadModule('proxy-address.ts'),
    vue:{ref:value=>({value}),computed:fn=>({get value(){return fn()}}),onMounted:fn=>mounted.push(fn),nextTick:async()=>{}}
  }
  const sandbox={exports:{},URLSearchParams,AbortSignal,Error,crypto:webcrypto,window:{location:{pathname:'/app/netlite/',search:''}},
    sessionStorage:{getItem:()=>{throw new Error('gateway read local credentials')},setItem:()=>{throw new Error('gateway persisted local credentials')}},
    fetch:async(url,options)=>{
      requests.push({url,options})
      const result=responses[url]
      assert.ok(result,`unexpected request ${url}`)
      return new Response(JSON.stringify(result.body),{status:result.status||200,headers:{'Content-Type':'application/json'}})
    },
    require:name=>modules[name]||{}
  }
  vm.runInNewContext(compiled+'\nglobalThis.appState={authMode,logged,message,csrf,api}',sandbox)
  return {sandbox,requests,mount:mounted[0]}
}

test('fnOS app automatically enters using platform identity and sends CSRF on writes',async()=>{
  const app=mountApp({
    '/app/netlite/api/auth/state':{body:{mode:'fnos-gateway',initialized:true}},
    '/app/netlite/api/auth/me':{body:{username:'nas-admin',csrf_token:'platform-csrf',must_change_password:false}},
    '/app/netlite/api/config':{body:{revision:0,routes:[],certificates:[]}},
    '/app/netlite/api/status':{body:{proxy:{running:true}}},
    '/app/netlite/api/dns/accounts':{body:{accounts:[]}},
    '/app/netlite/api/dns-capabilities':{body:{certificate_providers:[]}}
  })
  await app.mount()
  assert.equal(app.sandbox.appState.logged.value,true)
  assert.equal(app.sandbox.appState.authMode.value,'fnos-gateway')
  await app.sandbox.appState.api('/config','PUT',{})
  const write=app.requests.at(-1)
  assert.equal(write.options.headers['X-CSRF-Token'],'platform-csrf')
  assert.equal(write.options.headers.Authorization,undefined)
  assert.ok(app.requests.every(r=>!r.url.endsWith('/login')&&!r.url.endsWith('/setup')))
})

test('expired platform identity stays in fnOS mode without local login fallback',async()=>{
  const app=mountApp({'/app/netlite/api/auth/state':{status:401,body:{error:'请重新登录飞牛'}}})
  await app.mount()
  assert.equal(app.sandbox.appState.logged.value,false)
  assert.equal(app.sandbox.appState.authMode.value,'fnos-gateway')
  assert.equal(app.requests.length,1)
  assert.match(app.sandbox.appState.message.value,/重新登录飞牛/)
})
