// A random UUID v4. crypto.randomUUID exists only in secure contexts (HTTPS or localhost), and the
// app is often served over plain HTTP on a LAN host, where calling it throws. getRandomValues works
// in every context.
export function uuid() {
  if (globalThis.crypto?.randomUUID && globalThis.isSecureContext) return crypto.randomUUID()
  const b = crypto.getRandomValues(new Uint8Array(16))
  b[6] = (b[6] & 0x0f) | 0x40
  b[8] = (b[8] & 0x3f) | 0x80
  const h = [...b].map((x) => x.toString(16).padStart(2, '0')).join('')
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`
}
