export async function readAPIResponse<T>(response: Response, path: string): Promise<T> {
  const text = await response.text()
  let result: any
  try {
    result = JSON.parse(text)
  } catch {
    if (response.status === 404 && path === '/proxy/config') {
      throw new Error('Caddy 配置查看接口不存在（HTTP 404）。请更新并重启 NetLite 后端，确保前后端版本一致。')
    }
    throw new Error(`接口返回了非 JSON 响应（HTTP ${response.status}）。请检查 NetLite 后端及反向代理配置。`)
  }
  if (!response.ok) {
    if (response.status === 404 && path === '/proxy/config') {
      throw new Error('Caddy 配置查看接口不存在（HTTP 404）。请更新并重启 NetLite 后端，确保前后端版本一致。')
    }
    throw new Error(typeof result?.error === 'string' ? result.error : `请求失败（HTTP ${response.status}）`)
  }
  return result as T
}
