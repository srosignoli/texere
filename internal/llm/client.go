package llm

import (
	"net"
	"net/http"
	"time"
)

// httpClient bounds connection setup and time-to-first-byte, but deliberately
// leaves the response body untimed so long streamed completions are not
// truncated. Cancellation of an in-flight stream is via the request context.
var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		ResponseHeaderTimeout: 60 * time.Second,
	},
}
