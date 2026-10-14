const {test}=require('node:test')
const assert=require('node:assert/strict')
const fs=require('node:fs')
const ts=require('typescript')
const vm=require('node:vm')
const source=fs.readFileSync(require('node:path').join(__dirname,'../src/route-certificate.ts'),'utf8')
const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText
const sandbox={exports:{}};vm.runInNewContext(compiled,sandbox)
const {routeCertificateError,certificateDomain,wildcardBases,splitSubdomain,normalizeSubdomain,firstValidCertificate}=sandbox.exports
const route={scheme:'https',domain:'app.example.com',certificate_id:'local'}

test('default certificate follows configuration order and skips unissued, expired and unknown states',()=>{
  const now=Date.parse('2026-10-15T00:00:00Z')
  const certificates=['pending','expired','unknown','ready','second'].map(id=>({id,domains:[`${id}.example.com`]}))
  const issued={status:'ready',last_issued:'2026-10-01T00:00:00Z',not_after:'2027-01-01T00:00:00Z'}
  const states={pending:{status:'pending',last_issued:'0001-01-01T00:00:00Z',not_after:'0001-01-01T00:00:00Z'},expired:{...issued,not_after:'2026-10-14T00:00:00Z'},ready:issued,second:issued}
  assert.equal(firstValidCertificate(certificates,states,now).id,'ready')
  assert.equal(firstValidCertificate(certificates.slice(0,3),states,now),undefined)
  assert.equal(firstValidCertificate([],states,now),undefined)
  assert.equal(firstValidCertificate(certificates,{},now),undefined)
  assert.equal(firstValidCertificate([certificates[3]],{ready:{...issued,not_after:'not a date'}},now),undefined)
  assert.equal(firstValidCertificate([certificates[3]],{ready:{...issued,not_after:new Date(now).toISOString()}},now),undefined)
})

test('renewal keeps an existing unexpired certificate eligible for default selection',()=>{
  const certificates=[{id:'renewing',domains:['*.example.com']}]
  const states={renewing:{status:'renewing',last_issued:'2026-10-01T00:00:00Z',not_after:'2027-01-01T00:00:00Z'}}
  assert.equal(firstValidCertificate(certificates,states,Date.parse('2026-10-15T00:00:00Z')).id,'renewing')
})

test('missing and mismatched certificates explain the domain and next step',()=>{
  const other=[{id:'local',domains:['other.example.com']}]
  assert.match(routeCertificateError(route,[]),/请先到「HTTPS 证书」添加包含「app.example.com」的证书/)
  assert.match(routeCertificateError({...route,certificate_id:''},other),/请选择包含「app.example.com」的 HTTPS 证书/)
  assert.match(routeCertificateError({...route,certificate_id:'removed'},other),/已不存在，请重新选择/)
  assert.match(routeCertificateError(route,other),/「other.example.com」，不能用于访问域名「app.example.com」/)
})

test('certificate matching agrees with exact and single-label wildcard coverage',()=>{
  for(const domain of ['app.example.com','*.example.com'])
    assert.equal(routeCertificateError(route,[{id:'local',domains:[domain]}]),'')
  assert.equal(routeCertificateError(route,[{id:'local',domains:['other.example.com','app.example.com']}]),'')
  for(const domain of ['example.com','deep.app.example.com','*.app.example.com'])
    assert.match(routeCertificateError({...route,domain},[{id:'local',domains:['*.example.com']}]),/不能用于访问域名/)
  assert.equal(routeCertificateError({...route,domain:'*.example.com'},[{id:'local',domains:['*.example.com']}]),'')
  assert.equal(routeCertificateError({...route,scheme:'http'},[]),'')
})

test('certificate selection fills concrete domains and follows subsequent selections',()=>{
  assert.equal(certificateDomain(['app.example.com'],''),'app.example.com')
  assert.equal(certificateDomain(['*.example.com','example.com','other.example.com'],''),'')
  assert.equal(certificateDomain(['other.example.com'],'app.example.com','app.example.com'),'other.example.com')
  assert.equal(certificateDomain(['*.example.com'],'app.example.com','app.example.com'),'app.example.com')
})

test('wildcard certificates support one-label subdomains without mistaking the apex for a prefix',()=>{
  assert.deepEqual(Array.from(wildcardBases(['*.iminfo.top','iminfo.top','*.other.com','*.iminfo.top'])),['iminfo.top','other.com'])
  assert.equal(splitSubdomain('nas.iminfo.top','iminfo.top'),'nas')
  for(const host of ['iminfo.top','*.iminfo.top','a.b.iminfo.top','nas.other.com',''])
    assert.equal(splitSubdomain(host,'iminfo.top'),null)
  assert.equal(normalizeSubdomain(' NAS ','iminfo.top'),'nas')
  assert.equal(normalizeSubdomain('NAS.IMINFO.TOP','iminfo.top'),'nas')
  assert.equal(normalizeSubdomain('a.b','iminfo.top'),'a.b')
  assert.equal(certificateDomain(['*.iminfo.top','iminfo.top'],'iminfo.top'),'iminfo.top')
})

test('certificate selection preserves manually entered domains and never guesses a wildcard host',()=>{
  assert.equal(certificateDomain(['other.example.com'],'custom.example.com'),'custom.example.com')
  assert.equal(certificateDomain(['*.example.com'],''),'')
  assert.equal(certificateDomain(['*.example.com'],'old.other.com','old.other.com'),'')
})
