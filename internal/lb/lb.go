package lb

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// ProxyServer defines the interface for proxy behavior
type ProxyServer interface {
    StartReverseProxy(e *echo.Echo, region string)
	newBalancer(targets []*middleware.ProxyTarget) middleware.ProxyBalancer
}

var currentTargets []*middleware.ProxyTarget