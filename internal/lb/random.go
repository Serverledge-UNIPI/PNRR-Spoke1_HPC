package lb

import (
	"log"
	"math/rand"
	"net/url"
)

// RandomPolicy is a load balancer that uses a random policy
type RandomPolicy struct {
	lbProxy *LBProxy
}

func newRandomPolicy(lbProxy *LBProxy) *RandomPolicy {
	log.Println("RandomPolicy created")
	return &RandomPolicy{
		lbProxy: lbProxy,
	}
}

func (r *RandomPolicy) selectTarget() *url.URL {
	nodes := r.lbProxy.getTargets()
	if len(nodes) == 0 {
		return nil
	}
	return nodes[rand.Intn(len(nodes))]
}