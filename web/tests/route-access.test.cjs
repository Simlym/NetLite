const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const ts=require('typescript')
const vm=require('node:vm')
const source=fs.readFileSync(require('node:path').join(__dirname,'../src/route-access.ts'),'utf8')
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText
const sandbox={exports:{}};vm.runInNewContext(compiled,sandbox)
const {defaultPort,publicPort,accessURL,accessWarning,routeState}=sandbox.exports
const bridge={mode:'bridge',min_port:1,mappings_known:true,mappings:[{public_port:443,listen_port:18443}]}
const route={id:'app',domain:'xq.teanap.top',port:18443,scheme:'https',upstream:'http://host.docker.internal:8080',certificate_id:'cert',enabled:true}

test('bridge shows external 443 while preserving the internal listener',()=>{
  assert.equal(publicPort(route,bridge),443)
  assert.equal(accessURL(route,bridge),'https://xq.teanap.top')
  assert.equal(defaultPort(bridge,'https'),18443)
  assert.equal(accessWarning(route,bridge),'')
  assert.match(accessWarning({...route,port:8443},bridge),/尚未声明发布/)
  assert.equal(accessURL({...route,port:8443},bridge),'')
})
test('missing deployment metadata never invents a public address',()=>{
  for(const info of [undefined,{...bridge,mappings_known:false},{mode:'unknown'}]){
    assert.equal(publicPort(route,info),undefined)
    assert.equal(accessURL(route,info),'')
    assert.notEqual(accessWarning(route,info),'')
  }
})
test('direct deployments default to 443/80 and fnOS retains high ports',()=>{
  for(const mode of ['host','native']){
    assert.equal(defaultPort({mode},'https'),443)
    assert.equal(defaultPort({mode},'http'),80)
    assert.equal(accessURL({...route,port:8443},{mode}),'https://xq.teanap.top:8443')
  }
  assert.equal(defaultPort({mode:'fnos'},'https'),18443)
})
test('saved, applied, running and disabled are separate states',()=>{
  const proxy={running:true,applied_routes:[{...route,name:''}],listening_ports:[18443]}
  assert.equal(routeState(route,proxy).label,'已监听')
  assert.equal(routeState({...route,upstream:'http://localhost:9000'},proxy).label,'待应用')
  assert.equal(routeState({...route,enabled:false},proxy).label,'待应用')
  assert.equal(routeState(route,{...proxy,running:false}).label,'未监听')
  assert.equal(routeState(route,{...proxy,listening_ports:[]}).label,'未监听')
  assert.equal(routeState(route,undefined).label,'待应用')
  assert.equal(routeState({...route,enabled:false},{...proxy,applied_routes:[{...route,enabled:false}]}).label,'已停用')
})
