const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const vue=require('vue')
const {webcrypto}=require('node:crypto')

function deferred(){let resolve,reject;const promise=new Promise((ok,fail)=>{resolve=ok;reject=fail});return {promise,resolve,reject}}
function script(name,props,expose,extras={}){
  const hooks={mounted:[],activated:[],deactivated:[]}
  let now=100000
  const context=vm.createContext({exports:{},Error,URL,URLSearchParams,AbortSignal,crypto:webcrypto,
    Date:class extends Date{static now(){return now}},
    defineProps:()=>props,withDefaults:p=>p,defineEmits:()=>()=>{},
    window:{location:{pathname:'/',search:''}},...extras,
    require:id=>{
      if(id==='vue')return {...vue,onMounted:fn=>hooks.mounted.push(fn),onUnmounted:()=>{},onActivated:fn=>hooks.activated.push(fn),onDeactivated:fn=>hooks.deactivated.push(fn)}
      if(id.endsWith('.vue'))return {}
      const source=fs.readFileSync(path.join(__dirname,'../src',id+'.ts'),'utf8')
      const module={exports:{}}
      vm.runInNewContext(ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText,{exports:module.exports,URL})
      return module.exports
    }
  })
  const source=fs.readFileSync(path.join(__dirname,'../src',name),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  vm.runInContext(ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText+`\nglobalThis.state={${expose}}`,context)
  return {state:context.state,hooks,advance:ms=>{now+=ms}}
}
function app(fetch){return script('App.vue',{},'navigate,loadPage,pageLoading,pageError,pageLoaded,config,status,tab,api',{fetch})}
function json(body){return new Response(JSON.stringify(body),{headers:{'Content-Type':'application/json'}})}

test('proxy navigation refreshes data and reuses fresh cache on repeated visits',async()=>{
  const calls=[]
  const pending=deferred()
  let configRequests=0
  const x=app(async url=>{calls.push(url);if(url==='/api/config')return ++configRequests===1?pending.promise:json({revision:3,routes:[],certificates:[]});return json({proxy:{running:true}})})
  const loading=x.state.navigate('routes')
  assert.equal(x.state.pageLoading.value,true)
  assert.deepEqual(calls,['/api/config','/api/status'])
  pending.resolve(json({revision:3,routes:[],certificates:[]}))
  await loading
  assert.equal(x.state.config.value.revision,3)
  assert.equal(x.state.pageLoading.value,false)
  await x.state.navigate('routes')
  assert.equal(calls.length,2)
  x.advance(15001)
  const refresh=x.state.navigate('routes')
  assert.equal(x.state.pageLoaded.value.routes,100000)
  assert.equal(x.state.config.value.revision,3)
  await refresh
  assert.equal(calls.length,4)
})

test('late responses from a previous menu cannot overwrite the active page',async()=>{
  const pending=[]
  const x=app(()=>{const p=deferred();pending.push(p);return p.promise})
  const first=x.state.navigate('routes')
  const second=x.state.navigate('engine')
  pending[2].resolve(json({revision:2,routes:[],certificates:[]}))
  pending[3].resolve(json({proxy:{running:true}}))
  await second
  pending[0].resolve(json({revision:1,routes:[],certificates:[]}))
  pending[1].resolve(json({proxy:{running:false}}))
  await first
  assert.equal(x.state.tab.value,'engine')
  assert.equal(x.state.config.value.revision,2)
  assert.equal(x.state.status.value.proxy.running,true)
})

test('refresh failures retain existing data and successful writes invalidate the cache',async()=>{
  let fail=false
  const x=app(async(url,options)=>{
    if(fail)throw new Error('network unavailable')
    return json(url==='/api/config'?{revision:4,routes:[],certificates:[]}:{proxy:{running:true}})
  })
  await x.state.navigate('routes')
  fail=true
  await x.state.loadPage()
  assert.equal(x.state.config.value.revision,4)
  assert.ok(x.state.pageLoaded.value.routes)
  assert.equal(x.state.pageError.value,'network unavailable')
  fail=false
  await x.state.api('/config','PUT',{})
  assert.equal(x.state.pageLoaded.value.routes,1)
})

for(const name of ['DDNSManager.vue','CertificateManager.vue','PanelSettings.vue']){
  test(`${name} keeps cached data during slow refresh and retries failures`,async()=>{
    let pending=deferred(),calls=0
    const props={mode:'tasks',certificates:[],revision:0,api:async()=>{calls++;return pending.promise}}
    const x=script(name,props,'loadPage,pageLoading,pageLoaded,pageError')
    const first=x.state.loadPage()
    assert.equal(x.state.pageLoading.value,true)
    assert.equal(x.state.pageLoaded.value,false)
    pending.resolve({accounts:[],tasks:[],certificates:{},enabled:false,domain:'',certificate_id:'',redirect:true})
    await first
    assert.equal(x.state.pageLoaded.value,true)
    const initialCalls=calls
    await x.state.loadPage(false)
    assert.equal(calls,initialCalls)
    x.advance(15001)
    pending=deferred()
    const refresh=x.state.loadPage(false)
    assert.equal(x.state.pageLoading.value,true)
    assert.equal(x.state.pageLoaded.value,true)
    pending.reject(new Error('offline'))
    await refresh
    assert.equal(x.state.pageLoaded.value,true)
    assert.equal(x.state.pageError.value,'offline')
    pending=deferred()
    pending.resolve({accounts:[],tasks:[],certificates:{},enabled:false,domain:'',certificate_id:'',redirect:true})
    await x.state.loadPage()
    assert.equal(x.state.pageError.value,'')
    props.revision++
    await x.state.loadPage(false)
    assert.ok(calls>initialCalls+2)
  })
}

test('fast requests never show loading feedback and slow requests delay it',async()=>{
  const props=vue.reactive({loading:true,loaded:false})
  const timers=new Map()
  let timerID=0
  const x=script('PageState.vue',props,'showFeedback',{
    setTimeout:(fn,delay)=>{assert.equal(delay,180);timers.set(++timerID,fn);return timerID},
    clearTimeout:id=>timers.delete(id)
  })
  assert.equal(x.state.showFeedback.value,false)
  props.loading=false
  props.loaded=true
  await vue.nextTick()
  assert.equal(timers.size,0)
  assert.equal(x.state.showFeedback.value,false)
  props.loading=true
  await vue.nextTick()
  assert.equal(x.state.showFeedback.value,false)
  timers.values().next().value()
  assert.equal(x.state.showFeedback.value,true)
  props.loading=false
  await vue.nextTick()
  assert.equal(x.state.showFeedback.value,false)
})
