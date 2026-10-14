const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const ts=require('typescript')
const vm=require('node:vm')
const source=fs.readFileSync(require('node:path').join(__dirname,'../src/api-response.ts'),'utf8')
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
const sandbox={exports:{}};vm.runInNewContext(compiled,sandbox)
const {readAPIResponse}=sandbox.exports

test('old backend plain text 404 explains version mismatch',async()=>{
  await assert.rejects(readAPIResponse(new Response('404 page not found\n',{status:404}),'/proxy/config'),/更新并重启 NetLite 后端/)
})
test('HTML gateway errors do not expose response contents',async()=>{
  await assert.rejects(readAPIResponse(new Response('<html>private content</html>',{status:502}),'/config'),/非 JSON 响应（HTTP 502）/)
})
test('JSON errors retain the backend message',async()=>{
  await assert.rejects(readAPIResponse(new Response('{"error":"invalid management token"}',{status:401}),'/config'),/invalid management token/)
})

test('legacy HTTPS certificate errors give a Chinese explanation and next step',async()=>{
  await assert.rejects(readAPIResponse(new Response(JSON.stringify({error:'HTTPS route needs a covering certificate'}),{status:400}),'/config'),/HTTPS 代理需要选择包含访问域名的证书.*「HTTPS 证书」/)
})
test('draft and absent applied checkpoint load successfully',async()=>{
  const result=await readAPIResponse(new Response('{"draft":{"apps":{}},"applied":null}'),'/proxy/config')
  assert.equal(result.applied,null)
  assert.ok(result.draft.apps)
})
