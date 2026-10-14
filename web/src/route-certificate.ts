type Certificate = {id:string;domains:string[]}
type Route = {scheme:string;domain:string;certificate_id:string}
export type CertificateState = {status:string;not_after:string;last_issued:string}

export function firstValidCertificate<T extends Certificate>(certificates:T[],states:Record<string,CertificateState>,now=Date.now()):T|undefined {
  return certificates.find(c=>{
    const state=states[c.id]
    return c.domains.length>0&&state&&state.status!=='expired'&&Date.parse(state.last_issued)>0&&Date.parse(state.last_issued)<=now&&Date.parse(state.not_after)>now
  })
}

function covers(pattern:string,host:string):boolean {
  if(pattern===host)return true
  if(!pattern.startsWith('*.')||host.startsWith('*.'))return false
  const suffix=pattern.slice(1)
  return host.endsWith(suffix)&&!host.slice(0,-suffix.length).includes('.')
}

export function certificateDomain(domains:string[],current:string,previousAutoFilled=''):string {
  if(current&&current!==previousAutoFilled)return current
  if(current&&domains.some(d=>covers(d,current)))return current
  return domains.some(d=>d.startsWith('*.'))?'':domains[0]||''
}

export function wildcardBases(domains:string[]):string[] {
  return [...new Set(domains.filter(d=>d.startsWith('*.')).map(d=>d.slice(2)))]
}

export function splitSubdomain(host:string,base:string):string|null {
  if(!base||!host||host.startsWith('*.')||!covers(`*.${base}`,host))return null
  return host.slice(0,-base.length-1)
}

export function normalizeSubdomain(value:string,base:string):string {
  const input=value.trim().toLowerCase()
  return base&&input.endsWith('.'+base)?input.slice(0,-base.length-1):input
}

export function routeCertificateError(route:Route,certificates:Certificate[]):string {
  if(route.scheme!=='https')return ''
  const domain=route.domain?`「${route.domain}」`:'访问域名'
  if(!certificates.length)return `使用 HTTPS 访问需要域名证书。请先到「HTTPS 证书」添加包含${domain}的证书，再返回这里选择。`
  if(!route.certificate_id)return `请选择包含${domain}的 HTTPS 证书。如果列表中没有合适的证书，请先到「HTTPS 证书」添加。`
  const certificate=certificates.find(c=>c.id===route.certificate_id)
  if(!certificate)return '之前选择的 HTTPS 证书已不存在，请重新选择；如需添加新证书，请前往「HTTPS 证书」。'
  if(route.domain&&!certificate.domains.some(d=>covers(d,route.domain)))return `所选证书适用于「${certificate.domains.join('、')}」，不能用于访问域名${domain}。请选择包含该访问域名的证书，或先到「HTTPS 证书」添加。`
  return ''
}
