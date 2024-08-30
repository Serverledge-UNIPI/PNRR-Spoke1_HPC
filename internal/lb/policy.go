package lb

import (
	"net/url"
)

// LBPolicy is the interface for implementing load balancing policies
type LBPolicy interface {
	selectTarget() *url.URL
}