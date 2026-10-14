const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const {webcrypto}=require('node:crypto')
const vue=require('vue')

const component=fs.readFileSync(path.join(__dirname,'../src/DDNSManager.vue'),'utf8')
const source=component.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText

function manager(api){
  const sandbox={
    exports:{},crypto:webcrypto,Error,
    defineProps:()=>({mode:'tasks',api}),withDefaults:props=>props,
    require:name=>{
      assert.equal(name,'vue')
      return {...vue,onMounted:()=>{},onUnmounted:()=>{},onActivated:()=>{},onDeactivated:()=>{}}
    }
  }
  vm.runInNewContext(compiled+'\nglobalThis.manager={showDetails,showHistory,load,loadHistory,perform,changeHistoryPage,detailTab,historyPage,historyPages,visibleHistory,historyLoaded,error}',sandbox)
  return sandbox.manager
}

for(const count of [0,1,10,11,100]){
  test(`${count} history records can be browsed in bounded pages without omissions`,async()=>{
    const records=Array.from({length:count},(_,i)=>({message:`record-${count-i}`}))
    const app=manager(async()=>({history:records}))
    await app.showDetails({id:'task'})
    await app.showHistory()
    const seen=[]
    for(let page=1;page<=app.historyPages.value;page++){
      app.changeHistoryPage(page)
      assert.ok(app.visibleHistory.value.length<=10)
      seen.push(...app.visibleHistory.value.map(h=>h.message))
    }
    assert.deepEqual(seen,records.map(h=>h.message))
    app.changeHistoryPage(999)
    assert.equal(app.historyPage.value,app.historyPages.value)
    app.changeHistoryPage(0)
    assert.equal(app.historyPage.value,1)
  })
}

test('history loads on demand and remains stable while task status refreshes',async()=>{
  const calls=[]
  let records=Array.from({length:100},(_,i)=>({message:`record-${i}`}))
  const app=manager(async url=>{
    calls.push(url)
    return url.endsWith('/history')?{history:records}:{accounts:[],tasks:[]}
  })
  await app.showDetails({id:'first'})
  assert.equal(app.detailTab.value,'status')
  assert.equal(calls.length,0)
  await app.showHistory()
  app.changeHistoryPage(3)
  const before=app.visibleHistory.value.map(h=>h.message)
  records=[{message:'new'},...records].slice(0,100)
  await app.load()
  await app.showHistory()
  assert.equal(app.historyPage.value,3)
  assert.deepEqual(app.visibleHistory.value.map(h=>h.message),before)
  assert.equal(calls.filter(url=>url.endsWith('/history')).length,1)
  await app.perform(app.loadHistory)
  assert.equal(app.historyPage.value,1)
  assert.equal(app.visibleHistory.value[0].message,'new')
  await app.showDetails({id:'second'})
  assert.equal(app.detailTab.value,'status')
  assert.equal(app.historyLoaded.value,false)
  assert.equal(app.visibleHistory.value.length,0)
  await app.showHistory()
  assert.equal(calls.at(-1),'/dns/tasks/second/history')
})

test('failed history requests can be retried without showing a false empty state',async()=>{
  let attempts=0
  const app=manager(async()=>{
    if(++attempts===1)throw new Error('history unavailable')
    return {history:[]}
  })
  await app.showDetails({id:'task'})
  await app.showHistory()
  assert.equal(app.historyLoaded.value,false)
  assert.equal(app.error.value,'history unavailable')
  await app.showHistory()
  assert.equal(app.historyLoaded.value,true)
  assert.equal(app.error.value,'')
})
