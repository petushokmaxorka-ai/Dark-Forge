export function getNonce(): string {
  return Math.random().toString(36).slice(2)
}
