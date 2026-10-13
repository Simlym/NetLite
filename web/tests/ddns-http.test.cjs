const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const {webcrypto}=require('node:crypto')

const component=fs.readFileSync(path.join(__dirname,'../src/DDNSManager.vue'),'utf8')
const source=component.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText

for(const mode of ['accounts','tasks']){
  test(`${mode} page initializes over HTTP without crypto.randomUUID`,()=>{
    const mounted=[]
    const sandbox={
      exports:{},
      crypto:{getRandomValues:values=>webcrypto.getRandomValues(values)},
      defineProps:()=>({mode,api:async()=>({accounts:[],tasks:[]})}),
      withDefaults:props=>props,
      defineEmits:()=>()=>{},
      require:name=>{
        assert.equal(name,'vue')
        return {
          ref:value=>({value}),computed:fn=>({get value(){return fn()}}),
          onMounted:fn=>mounted.push(fn),onUnmounted:()=>{},nextTick:async()=>{}
        }
      }
    }
    vm.runInNewContext(compiled+'\nglobalThis.initial={account:account.value,task:task.value,nextAccount:freshAccount(),nextTask:freshTask()}',sandbox)
    assert.equal(mounted.length,1)
    const ids=Object.values(sandbox.initial).map(value=>value.id)
    for(const id of ids)assert.match(id,/^[a-f0-9]{32}$/)
    assert.equal(new Set(ids).size,ids.length)
  })
}
