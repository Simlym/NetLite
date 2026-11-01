export type NetworkInfo={mode:string;min_port:number;mappings:{public_port:number;listen_port:number}[];mappings_known:boolean}
export type AccessRoute={id:string;name?:string;domain:string;port:number;scheme:string;upstream:string;certificate_id:string;enabled:boolean}
export type ProxyState={running:boolean;listening_ports?:number[];applied_revision?:number;applied_routes?:AccessRoute[]|null;modules?:string[]}

export function defaultPort(info:NetworkInfo|undefined,scheme:string):number{
  if(info?.mode==='host'||info?.mode==='native')return scheme==='https'?443:80
  return scheme==='https'?18443:18080
}
export function publicPort(route:Pick<AccessRoute,'port'>,info:NetworkInfo|undefined):number|undefined{
  if(!info||info.mode==='unknown')return undefined
  if(info.mode!=='bridge')return route.port
  if(!info.mappings_known)return undefined
  return info.mappings.find(p=>p.listen_port===route.port)?.public_port
}
export function accessURL(route:Pick<AccessRoute,'domain'|'port'|'scheme'>,info:NetworkInfo|undefined):string{
  const port=publicPort(route,info)
  if(port===undefined||!route.domain)return ''
  return `${route.scheme}://${route.domain}${port===(route.scheme==='https'?443:80)?'':':'+port}`
}
export function accessWarning(route:Pick<AccessRoute,'port'>,info:NetworkInfo|undefined):string{
  if(info?.mode==='bridge'){
    if(!info.mappings_known)return '端口发布情况未确认，请核对 Docker 映射；容器监听成功不代表外部可访问。'
    if(publicPort(route,info)===undefined)return `监听端口 ${route.port} 尚未声明发布到宿主机；请补充 Docker 端口映射及部署声明，否则外部无法通过此端口访问。`
  }
  if(!info||info.mode==='unknown')return '部署网络方式未确认，外部入口地址未知；请配置部署网络信息并核对端口映射。'
  return ''
}
export function routeState(route:AccessRoute,proxy:ProxyState|undefined):{label:string;tone:string;detail:string}{
  const applied=proxy?.applied_routes?.find(r=>r.id===route.id)
  const same=applied&&(['name','domain','port','scheme','upstream','certificate_id','enabled'] as const).every(key=>(route[key]??'')===(applied[key]??''))
  if(!same)return {label:'待应用',tone:'pending',detail:'已保存的规则与最近应用的规则不同'}
  if(!route.enabled)return {label:'已停用',tone:'disabled',detail:'此规则不接收请求'}
  if(!proxy?.running||!proxy.listening_ports?.includes(route.port))return {label:'未监听',tone:'failed',detail:'代理未运行或端口未监听，请刷新状态并检查应用结果'}
  return {label:'已监听',tone:'enabled',detail:'规则已应用，公网可达性未验证；目标连接可在编辑时测试'}
}
