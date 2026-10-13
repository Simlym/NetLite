const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const ts=require('typescript')
const vm=require('node:vm')
const source=fs.readFileSync(require('node:path').join(__dirname,'../src/application-path.ts'),'utf8')
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
const sandbox={exports:{}};vm.runInNewContext(compiled,sandbox)
const {applicationBase,apiPath}=sandbox.exports

test('gateway requests and downloads remain under the registered application',()=>{
  for(const pathname of ['/app/netlite','/app/netlite/','/app/netlite/callback']){
    assert.equal(apiPath(pathname,'/auth/state'),'/app/netlite/api/auth/state')
    assert.equal(apiPath(pathname,'/certificates/local/root'),'/app/netlite/api/certificates/local/root')
  }
})
test('standalone deployments and similarly named paths retain root API routes',()=>{
  for(const pathname of ['/','/app/other/','/app/netlite-other/']){
    assert.equal(applicationBase(pathname),'/')
    assert.equal(apiPath(pathname,'/config'),'/api/config')
  }
})
