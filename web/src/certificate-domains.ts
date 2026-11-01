export type CertificateDomainEntry = {domain:string;wildcard:boolean;includeBase:boolean}

export function certificateEntries(domains:string[]):CertificateDomainEntry[] {
  const entries:CertificateDomainEntry[]=[]
  for(const value of domains){
    const wildcard=value.startsWith('*.')
    const domain=(wildcard?value.slice(2):value).toLowerCase()
    let entry=entries.find(e=>e.domain===domain)
    if(!entry){entry={domain,wildcard:false,includeBase:false};entries.push(entry)}
    if(wildcard)entry.wildcard=true
    else entry.includeBase=true
  }
  return entries.length?entries:[{domain:'',wildcard:false,includeBase:true}]
}

export function normalizeCertificateEntry(entry:CertificateDomainEntry):void {
  entry.domain=entry.domain.trim().toLowerCase()
  if(entry.domain.startsWith('*.')){
    entry.domain=entry.domain.slice(2)
    entry.wildcard=true
  }
}

export function certificateDomains(entries:CertificateDomainEntry[]):string[] {
  const domains:string[]=[]
  for(const original of entries){
    const entry={...original}
    normalizeCertificateEntry(entry)
    if(!entry.domain||entry.domain.length>253||!entry.domain.split('.').every(label=>/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label)))
      throw new Error('请输入有效域名，不含协议、端口或路径；国际化域名请使用 Punycode。')
    if(entry.includeBase||!entry.wildcard)domains.push(entry.domain)
    if(entry.wildcard)domains.push(`*.${entry.domain}`)
  }
  return [...new Set(domains)]
}
