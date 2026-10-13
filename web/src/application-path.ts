// Releases use relative assets so the same build serves standalone and fnOS.
export function applicationBase(pathname:string):string {
  return pathname==='/app/netlite'||pathname.startsWith('/app/netlite/')?'/app/netlite/':'/'
}

export function apiPath(pathname:string,path:string):string {
  return applicationBase(pathname)+'api'+path
}
