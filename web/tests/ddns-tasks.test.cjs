const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const path=require('node:path')
const ts=require('typescript')
const vm=require('node:vm')
const vue=require('vue')
const {webcrypto}=require('node:crypto')
const {parse}=require('@vue/compiler-sfc')
const {parse:parseTemplate}=require('@vue/compiler-dom')

const component=fs.readFileSync(path.join(__dirname,'../src/DDNSManager.vue'),'utf8')
const {descriptor}=parse(component)
const compiled=ts.transpileModule(descriptor.scriptSetup.content,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
const template=parseTemplate(descriptor.template.content)
function elements(node){return [node,...(node.children||[]).flatMap(elements)].filter(x=>x.tag)}
const nodes=elements(template)
function directive(node,name,expression){return node.props.find(p=>p.type===7&&p.name===name&&p.exp?.content===expression)}

function manager(api){
  const sandbox=vm.createContext({exports:{},crypto:webcrypto,Error,
    defineProps:()=>({mode:'tasks',api}),withDefaults:p=>p,
    setTimeout:()=>1,clearTimeout:()=>{},
    require:id=>{
      assert.equal(id,'vue')
      return {...vue,onMounted:()=>{},onUnmounted:()=>{},onActivated:()=>{},onDeactivated:()=>{}}
    }
  })
  vm.runInContext(compiled+'\nglobalThis.app={taskName,load,save,remove,openTask,task,v4,confirmation,detailDialog,pending,error,notice,tasks,busy}',sandbox)
  return {app:sandbox.app,click:(expression,t)=>{sandbox.t=t;vm.runInContext(expression,sandbox)}}
}

const task={id:'task',name:'',account_id:'account',ipv4_domains:['home.example.com'],ipv6_domains:['v6.example.com'],enabled:false}

test('task list exposes deletion, requires confirmation and refreshes after success',async()=>{
  const row=nodes.find(n=>n.tag==='tr'&&directive(n,'for','t in tasks'))
  const button=elements(row).find(n=>n.tag==='button'&&n.children.some(c=>c.content==='删除'))
  assert.ok(button,'deletion must be available directly in the task list')
  const click=button.props.find(p=>p.name==='on'&&p.arg?.content==='click').exp.content
  const calls=[]
  let deleted=false,confirmed=false
  const {app,click:clickRow}=manager(async(url,method)=>{
    calls.push([url,method])
    if(method==='DELETE'){deleted=true;return {deleted:true}}
    return {accounts:[],tasks:deleted?[]:[task]}
  })
  app.confirmation.value={showModal:()=>{confirmed=true},close:()=>{confirmed=false}}
  await app.load()
  clickRow(click,task)
  assert.equal(confirmed,true)
  assert.equal(deleted,false)
  assert.equal(app.pending.value.name,'home.example.com')
  await app.remove()
  assert.ok(calls.some(([url,method])=>url==='/dns/tasks/task'&&method==='DELETE'))
  assert.equal(confirmed,false)
  assert.equal(app.tasks.value.length,0)
  assert.equal(app.notice.value,'任务已删除')
})

test('failed task deletion retains the task and confirmation for retry',async()=>{
  let attempts=0,open=true
  const {app}=manager(async(url,method)=>{
    if(method==='DELETE'&&++attempts===1)throw new Error('无法删除任务')
    return {accounts:[],tasks:attempts>1?[]:[task]}
  })
  app.confirmation.value={close:()=>{open=false}}
  app.pending.value={kind:'tasks',id:'task',name:'home.example.com'}
  await app.load()
  await app.remove()
  assert.equal(open,true)
  assert.equal(app.tasks.value.length,1)
  assert.equal(app.error.value,'无法删除任务')
  assert.equal(app.busy.value,false)
  await app.remove()
  assert.equal(open,false)
  assert.equal(app.error.value,'')
  assert.equal(app.tasks.value.length,0)
})

test('task names are optional and unnamed tasks display their current domains',async()=>{
  const input=nodes.find(n=>n.tag==='input'&&directive(n,'model','task.name'))
  assert.ok(input)
  assert.ok(!input.props.some(p=>p.name==='required'))
  let saved
  const {app}=manager(async(url,method,body)=>{
    if(method==='PUT')saved=body
    return {accounts:[],tasks:[]}
  })
  assert.equal(app.taskName(task),'home.example.com')
  assert.equal(app.taskName({...task,name:'  家庭网络  '}),'家庭网络')
  assert.equal(app.taskName({...task,name:'  ',ipv4_domains:[]}),'v6.example.com')
  await app.openTask(task)
  await app.save()
  assert.equal(saved.name,'')
  assert.equal(saved.ipv4_domains[0],'home.example.com')
})
