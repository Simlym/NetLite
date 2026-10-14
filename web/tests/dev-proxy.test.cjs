const {test}=require('node:test')
const assert=require('node:assert/strict')
const http=require('node:http')
const path=require('node:path')

test('Vite API proxy preserves same-origin writes and rejects foreign origins',async t=>{
  const {createServer}=await import('vite')
  const received=[]
  const backend=http.createServer((req,res)=>{
    received.push({method:req.method,url:req.url,...req.headers})
    res.setHeader('Content-Type','application/json')
    if(new URL(req.headers.origin).host!==req.headers.host){
      res.writeHead(403).end(JSON.stringify({error:'跨站请求被拒绝'}))
      return
    }
    if(req.headers.cookie!=='netlite-session=test-session'||req.headers['x-csrf-token']!=='test-csrf'){
      res.writeHead(401).end(JSON.stringify({error:'请求验证失败'}))
      return
    }
    res.end(JSON.stringify({ok:true}))
  })
  await new Promise(resolve=>backend.listen(0,'127.0.0.1',resolve))
  t.after(()=>new Promise(resolve=>backend.close(resolve)))
  const root=path.join(__dirname,'..')
  const vite=await createServer({
    root,configFile:path.join(root,'vite.config.ts'),logLevel:'silent',
    server:{host:'127.0.0.1',port:0,hmr:false,open:false,
      proxy:{'/api':{target:`http://127.0.0.1:${backend.address().port}`}}}
  })
  t.after(()=>vite.close())
  await vite.listen()
  const origin=`http://127.0.0.1:${vite.httpServer.address().port}`
  const route='/api/dns/accounts/test-account'
  for(const method of ['POST','PUT','DELETE']){
    const headers={Origin:origin,Cookie:'netlite-session=test-session','X-CSRF-Token':'test-csrf'}
    const response=await fetch(origin+route,{method,headers})
    assert.equal(response.status,200,`${method} rejected a same-origin write`)
    assert.deepEqual(await response.json(),{ok:true})
    const forwarded=received.at(-1)
    assert.equal(forwarded.host,new URL(origin).host)
    assert.equal(forwarded.origin,origin)
    assert.equal(forwarded.cookie,headers.Cookie)
    assert.equal(forwarded['x-csrf-token'],headers['X-CSRF-Token'])
    assert.equal(forwarded.method,method)
    assert.equal(forwarded.url,route)

    const foreign=await fetch(origin+route,{method,headers:{...headers,Origin:'https://attacker.example'}})
    assert.equal(foreign.status,403,`${method} accepted a foreign origin`)
    await foreign.text()
  }
  const missingCSRF=await fetch(origin+route,{method:'DELETE',headers:{Origin:origin,Cookie:'netlite-session=test-session'}})
  assert.equal(missingCSRF.status,401)
  await missingCSRF.text()
})
