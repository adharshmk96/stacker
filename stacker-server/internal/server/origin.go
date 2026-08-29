package server

import (
	"net/url"
	"strings"
)

// originAllowed reports whether a browser origin may call the API.
func originAllowed(origin, requestHost, configuredHost string, isProduction bool) bool {
	if !isProduction {
		return true
	}
	if origin == "" {
		return false
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}

	host := strings.ToLower(parsed.Host)
	if host == strings.ToLower(requestHost) {
		return true
	}

	configuredHost = strings.ToLower(strings.TrimSpace(configuredHost))
	if configuredHost != "" && host == configuredHost {
		return true
	}
	return false
}
