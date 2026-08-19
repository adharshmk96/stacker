package node

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// The browser talks to a node's shell over one websocket:
//
//	client → server  binary   keystrokes, passed to the pty untouched
//	client → server  text     a JSON control frame, currently only `resize`
//	server → client  binary   whatever the shell printed
//	server → client  text     a JSON `exit` frame when the shell ends
//
// Splitting input by frame type rather than by an envelope keeps the hot path —
// every keystroke — free of encoding, and leaves control frames readable.

// terminalWriteTimeout bounds a single write to the browser. A client that has
// stopped reading must not pin the shell and its pty open indefinitely.
const terminalWriteTimeout = 10 * time.Second

var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Same stance as the API's CORS: stacker is reached from its own origin in
	// production and from the Nuxt dev server in development, and the session
	// token — not the origin — is what authorises the connection.
	CheckOrigin: func(*http.Request) bool { return true },
	// The token travels as a subprotocol because a browser cannot set headers
	// on a websocket; echoing it back is what completes the handshake.
	Subprotocols: []string{terminalSubprotocol},
}

// terminalSubprotocol is the fixed first entry of the `Sec-WebSocket-Protocol`
// list, with the session token as the second. Auth's middleware reads it.
const terminalSubprotocol = "stacker.terminal"

// terminalControl is a client control frame.
type terminalControl struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// terminal opens an interactive shell on the node and pipes it to the browser.
//
// Everything that can fail before the shell exists — an unknown node, a missing
// key, an unverified one — is answered as an ordinary HTTP error, so the client
// sees a status and a message rather than a socket that opens and shuts.
func (h *Handler) terminal(c *gin.Context) {
	cols, rows := terminalSize(c)

	session, item, err := h.service.StartTerminal(c.Param("id"), cols, rows)
	if err != nil {
		respondError(c, err)
		return
	}

	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade already wrote its own response.
		session.Close()
		return
	}

	pipeTerminal(conn, session)
	h.service.log.Info("terminal session closed", "node", item.ID, "name", item.Name)
}

// pipeTerminal runs the session until either side ends, then closes both.
//
// The two directions are separate goroutines because they block on unrelated
// things — a socket read and a pty read — and neither can be polled. Whichever
// finishes first closes the session, which unblocks the other.
func pipeTerminal(conn *websocket.Conn, session *TerminalSession) {
	var once sync.Once
	var writeMu sync.Mutex

	stop := func() {
		once.Do(func() {
			session.Close()
			_ = conn.Close()
		})
	}
	defer stop()

	write := func(kind int, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(terminalWriteTimeout))
		return conn.WriteMessage(kind, payload)
	}

	done := make(chan struct{})

	// Shell → browser.
	go func() {
		defer close(done)
		defer stop()

		buf := make([]byte, 4096)
		for {
			n, err := session.Read(buf)
			if n > 0 {
				if werr := write(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				// The pty reports the shell's exit as a read error, so this is
				// the normal end of a session as well as the failure path.
				notice, _ := json.Marshal(map[string]string{
					"type":    "exit",
					"message": exitMessage(err),
				})
				_ = write(websocket.TextMessage, notice)
				return
			}
		}
	}()

	// Browser → shell.
	for {
		kind, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}

		switch kind {
		case websocket.BinaryMessage:
			if _, err := session.Write(payload); err != nil {
				stop()
			}
		case websocket.TextMessage:
			var control terminalControl
			if json.Unmarshal(payload, &control) == nil && control.Type == "resize" {
				_ = session.Resize(control.Cols, control.Rows)
			}
		}
	}

	stop()
	<-done
}

// exitMessage turns the pty's end-of-session read error into a line worth
// printing in the terminal.
func exitMessage(err error) string {
	if errors.Is(err, io.EOF) {
		return "Session ended."
	}
	// A closed pty is what Close does on the way out; it is not news.
	if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) {
		return "Session closed."
	}
	return "Session ended: " + err.Error()
}

// terminalSize reads the browser's initial window size. It is a query parameter
// because the shell is started before the socket exists, and starting at the
// wrong size makes the first screen of output wrap.
func terminalSize(c *gin.Context) (cols, rows uint16) {
	cols, rows = 80, 24
	if v, err := strconv.Atoi(c.Query("cols")); err == nil && v > 0 && v <= 1000 {
		cols = uint16(v)
	}
	if v, err := strconv.Atoi(c.Query("rows")); err == nil && v > 0 && v <= 1000 {
		rows = uint16(v)
	}
	return cols, rows
}
