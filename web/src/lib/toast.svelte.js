// Toast notifications: toast.success('Saved'), toast.error(err.message).

export const toasts = $state([])
let nextId = 1

function push(kind, message, timeout = kind === 'error' ? 6000 : 3500) {
  const id = nextId++
  toasts.push({ id, kind, message })
  setTimeout(() => dismiss(id), timeout)
}

export function dismiss(id) {
  const i = toasts.findIndex((t) => t.id === id)
  if (i >= 0) toasts.splice(i, 1)
}

export const toast = {
  success: (m) => push('success', m),
  error: (m) => push('error', m),
  info: (m) => push('info', m),
}
