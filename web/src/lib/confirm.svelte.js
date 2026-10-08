// Promise-based confirmation dialog, rendered by <ConfirmDialog /> in App:
//   if (await confirm({ title: 'Rotate key?', message: '…', danger: true })) …

export const pending = $state({ current: null })

export function confirm({ title, message, confirmLabel = 'Confirm', danger = false }) {
  return new Promise((resolve) => {
    pending.current = { title, message, confirmLabel, danger, resolve }
  })
}

export function settle(ok) {
  const p = pending.current
  pending.current = null
  p?.resolve(ok)
}
