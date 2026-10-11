const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const ts=require('typescript')
const vm=require('node:vm')
const source=fs.readFileSync(require('node:path').join(__dirname,'../src/proxy-address.ts'),'utf8')
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
const sandbox={exports:{},URL};vm.runInNewContext(compiled,sandbox)
const {splitTarget,joinTarget}=sandbox.exports

test('existing HTTP, HTTPS and IPv6 targets retain destination through editing',()=>{
  for(const value of ['http://127.0.0.1:8080','https://service.local:9443','http://[::1]:8080','https://[2001:db8::1]:443']){
    const parsed=splitTarget(value)
    const actual=new URL(joinTarget(parsed)),expected=new URL(value)
    assert.equal(actual.href,expected.href)
  }
})
test('implicit ports display the effective HTTP or HTTPS port',()=>{
  assert.equal(splitTarget('http://service.local').port,80)
  assert.equal(splitTarget('https://service.local').port,443)
})
test('source and target protocols can differ and bare IPv6 is encoded correctly',()=>{
  assert.equal(joinTarget({scheme:'http',address:'::1',port:8080}),'http://[::1]:8080')
  assert.equal(joinTarget({scheme:'https',address:'service.local',port:9443}),'https://service.local:9443')
})
test('reject URLs, credentials, paths and invalid ports in address-only fields',()=>{
  for(const address of ['http://localhost','localhost:8080','user@localhost','localhost/path','localhost?x','localhost#x',''])
    assert.throws(()=>joinTarget({scheme:'http',address,port:8080}))
  for(const port of [0,65536,1.5,NaN]) assert.throws(()=>joinTarget({scheme:'http',address:'localhost',port}))
})
