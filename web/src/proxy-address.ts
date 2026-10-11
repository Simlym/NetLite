export type TargetAddress = {scheme:string;address:string;port:number}

export function splitTarget(value:string):TargetAddress {
  const url=new URL(value)
  return {scheme:url.protocol.slice(0,-1),address:url.hostname.replace(/^\[|\]$/g,''),port:Number(url.port || (url.protocol==='https:'?443:80))}
}

export function joinTarget(target:TargetAddress):string {
  const address=target.address.trim().replace(/^\[|\]$/g,'')
  if (!address || /[\s/@?#]/.test(address)) throw new Error('目标地址只填写 IP 或域名，不包含协议、端口或路径。')
  if (!['http','https'].includes(target.scheme) || !Number.isInteger(target.port) || target.port<1 || target.port>65535) throw new Error('目标端口需在 1–65535 之间。')
  const host=address.includes(':')?`[${address}]`:address
  const result=`${target.scheme}://${host}:${target.port}`
  try {new URL(result)} catch {throw new Error('请填写有效的目标 IP 或域名。')}
  return result
}
