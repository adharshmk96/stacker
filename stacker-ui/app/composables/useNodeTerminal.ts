import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import type { Node } from '~/types/node'

/**
 * One browser terminal attached to a node's shell.
 *
 * The server runs the shell under a pty — directly on the local node, over ssh
 * on a remote one — and pipes it through a websocket:
 *
 *   binary out → what the shell printed
 *   binary in  → keystrokes
 *   text  in   → a JSON control frame (`resize`)
 *   text  out  → a JSON `exit` frame when the shell ends
 *
 * Nothing is buffered across connections: closing the socket ends the shell, so
 * reconnecting is a new session, and the composable says so on screen rather
 * than pretending the old one continued.
 */

export type TerminalStatus = 'connecting' | 'open' | 'closed' | 'error'

/** Sent as the first subprotocol; the session token is the second. */
const subprotocol = 'stacker.terminal'

export function useNodeTerminal() {
  const api = useApi()

  const status = ref<TerminalStatus>('connecting')
  const error = ref<string | null>(null)

  let term: Terminal | null = null
  let fit: FitAddon | null = null
  let socket: WebSocket | null = null
  let observer: ResizeObserver | null = null

  /** Builds the terminal in `el` and opens the socket. */
  function open(el: HTMLElement, node: Node) {
    dispose()

    term = new Terminal({
      convertEol: true,
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
      // Matches the dashboard's dark surfaces; xterm needs literal colours.
      theme: {
        background: '#0a0a0a',
        foreground: '#e5e5e5',
        cursor: '#22d3ee',
        selectionBackground: '#164e63'
      }
    })

    fit = new FitAddon()
    term.loadAddon(fit)
    term.open(el)
    fit.fit()

    // The window size follows the panel, not the other way round: the shell is
    // told the new size so full-screen programs redraw correctly.
    observer = new ResizeObserver(() => {
      try {
        fit?.fit()
      } catch {
        // fit throws while the element is hidden mid-transition; the next
        // observation covers it.
      }
      sendResize()
    })
    observer.observe(el)

    // Registered once, not per connection: reconnecting reuses this terminal,
    // and a second handler would send every keystroke twice.
    term.onData(data => send(data))

    connect(node)
    term.focus()
  }

  function connect(node: Node) {
    if (!term) return

    status.value = 'connecting'
    error.value = null

    const token = authToken()
    // The shell starts before the socket exists, so it needs the size up front
    // — otherwise the first screen of output wraps at the default 80x24.
    const url = api.wsUrl(`/nodes/${node.id}/terminal`, {
      cols: term.cols,
      rows: term.rows
    })

    // A browser can set no headers on a websocket, so the token rides in the
    // subprotocol list, which the server reads and echoes back.
    socket = new WebSocket(url, token ? [subprotocol, token] : [subprotocol])
    socket.binaryType = 'arraybuffer'

    socket.onopen = () => {
      status.value = 'open'
      sendResize()
      term?.focus()
    }

    socket.onmessage = (event) => {
      if (typeof event.data === 'string') {
        // The only text frame is the shell's exit notice.
        const message = safeParse(event.data)
        if (message?.type === 'exit') term?.writeln(`\r\n\x1b[2m${message.message}\x1b[0m`)
        return
      }
      term?.write(new Uint8Array(event.data as ArrayBuffer))
    }

    socket.onerror = () => {
      // The browser never says why a websocket failed, and the useful reasons —
      // an unverified key, a host that is down — were already refused by the
      // handshake with a status code.
      status.value = 'error'
      error.value = 'The connection to this node failed. Check that it is online and its key works.'
    }

    socket.onclose = () => {
      socket = null
      if (status.value !== 'error') status.value = 'closed'
    }
  }

  /** Keystrokes go out as binary, so the server never has to decode them. */
  function send(data: string) {
    if (socket?.readyState === WebSocket.OPEN) socket.send(new TextEncoder().encode(data))
  }

  function sendResize() {
    if (!term || socket?.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
  }

  /** Opens a fresh session in the existing terminal, keeping the scrollback. */
  function reconnect(node: Node) {
    if (!term) return
    socket?.close()
    socket = null
    term.writeln('\r\n\x1b[2mReconnecting…\x1b[0m')
    connect(node)
  }

  function dispose() {
    observer?.disconnect()
    observer = null
    socket?.close()
    socket = null
    term?.dispose()
    term = null
    fit = null
  }

  function safeParse(value: string): { type?: string, message?: string } | null {
    try {
      return JSON.parse(value)
    } catch {
      return null
    }
  }

  return { status, error, open, reconnect, dispose }
}
