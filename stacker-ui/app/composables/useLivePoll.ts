/**
 * Runs a callback on an interval for as long as the component is mounted.
 *
 * Two things every live view here needs and none of them should reimplement:
 * the timer is cleared on unmount, and it stops while the tab is hidden — a
 * dashboard left open in a background tab otherwise polls docker all day for
 * nobody. Coming back to the tab runs the callback immediately, so what the user
 * sees on return is current rather than however stale the last tick left it.
 *
 * Ticks never overlap: if one call is still in flight when the next is due, the
 * tick is skipped rather than queued behind it.
 */
export function useLivePoll(callback: () => unknown, intervalMs = 5000) {
  let timer: ReturnType<typeof setInterval> | null = null
  let running = false

  async function tick() {
    if (running) return
    running = true
    try {
      await callback()
    } finally {
      running = false
    }
  }

  function start() {
    if (timer) return
    timer = setInterval(tick, intervalMs)
  }

  function stop() {
    if (!timer) return
    clearInterval(timer)
    timer = null
  }

  function onVisibility() {
    if (document.hidden) {
      stop()
      return
    }
    void tick()
    start()
  }

  onMounted(() => {
    void tick()
    start()
    document.addEventListener('visibilitychange', onVisibility)
  })

  onUnmounted(() => {
    stop()
    document.removeEventListener('visibilitychange', onVisibility)
  })

  return { tick, start, stop }
}
