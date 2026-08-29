package node

import (
	"net/http"
	"net/url"
	"strings"
)

var (
	terminalAllowedHost string
	terminalProduction  bool
)

func setTerminalOrigin(allowedHost string, production bool) {
	terminalAllowedHost = allowedHost
	terminalProduction = production
}

func terminalOriginAllowed(r *http.Request) bool {
	if !terminalProduction {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if host == strings.ToLower(r.Host) {
		return true
	}
	allowed := strings.ToLower(strings.TrimSpace(terminalAllowedHost))
	return allowed != "" && host == allowed
}
