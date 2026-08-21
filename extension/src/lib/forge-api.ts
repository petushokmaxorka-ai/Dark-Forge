import * as vscode from 'vscode'

export interface StreamEvent { type: 'token'|'done'|'error'; content?: string; error?: string }

/** Resolve the Forge backend base URL from settings (darkforge.baseUrl),
 *  falling back to the default local address. */
export function forgeBaseUrl(): string {
  const configured = vscode.workspace.getConfiguration('darkforge').get<string>('baseUrl')
  const url = (configured || 'http://127.0.0.1:9091').replace(/\/+$/, '')
  return url
}

export async function* streamChat(payload: { message: string; contextFiles?: string[] }): AsyncGenerator<StreamEvent> {
  const baseUrl = forgeBaseUrl()
  const res = await fetch(`${baseUrl}/api/chat/stream`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message: payload.message, context_files: payload.contextFiles || [] }),
  })
  if (!res.ok || !res.body) { yield { type: 'error', error: `HTTP ${res.status} — is Dark Forge backend running at ${baseUrl}?` }; return }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    for (const line of lines) {
      const t = line.trim()
      if (!t) continue
      try { const p = JSON.parse(t); if (p.error) yield {type:'error',error:p.error}; else if (p.done) yield {type:'done'}; else if (p.content) yield {type:'token',content:p.content} }
      catch { yield { type: 'token', content: t } }
    }
  }
  yield { type: 'done' }
}
export interface ParallelAgentResult { agent: string; display: string; glyph: string; color: string; content: string; error?: string }
export async function sendParallel(payload: { message: string; agents?: string[]; contextFiles?: string[] }): Promise<{results:ParallelAgentResult[]}|{error:string}> {
  const baseUrl = forgeBaseUrl()
  const res = await fetch(`${baseUrl}/api/parallel`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message: payload.message, agents: payload.agents, context_files: payload.contextFiles || [] }),
  })
  if (!res.ok) return { error: `HTTP ${res.status} — is Dark Forge backend running at ${baseUrl}?` }
  return await res.json()
}
