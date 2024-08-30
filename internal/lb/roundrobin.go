package lb

import (
	"log"
	"net/url"
	"sync"
)

// RoundRobinPolicy is a load balancer that uses the Round Robin policy
type RoundRobinPolicy struct {
	mu      sync.Mutex
	index   int
	lbProxy *LBProxy
}

func newRoundRobinPolicy(lbProxy *LBProxy) *RoundRobinPolicy {
	log.Printf("RoundRobinPolicy created")
	return &RoundRobinPolicy{
		index:   0,
		lbProxy: lbProxy,
	}
}

func (r *RoundRobinPolicy) selectTarget() *url.URL {
	r.mu.Lock()
	defer r.mu.Unlock()
	nodes := r.lbProxy.targets
	if len(nodes) == 0 {
		return nil
	}
	targetIndex := r.index % len(nodes)
	r.index = targetIndex
	r.index = r.index + 1
	return nodes[targetIndex]
}