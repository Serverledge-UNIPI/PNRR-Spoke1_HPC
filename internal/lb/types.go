package lb

import (
    "net/http"
	"bytes"
	"github.com/labstack/echo/v4/middleware"
)

var currentTargets []*middleware.ProxyTarget

// Custom structure for recording the response
type responseRecorder struct {
    http.ResponseWriter
    buffer bytes.Buffer
    status int
}