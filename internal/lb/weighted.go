package lb

import (
	"log"
	"sync"
	"time"
	"math/rand"
	"net/url"
)

type Weighted struct {
	mu		sync.Mutex
	lbProxy *LBWeightedProxy
}

func newWeightedPolicy(lbWeightedProxy *LBWeightedProxy) LBPolicy {
	log.Println("WeightedPolicy created")

	return &Weighted{
		lbProxy: lbWeightedProxy,
	}
}

func (r *Weighted) selectTarget() *url.URL {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodes := r.lbProxy.getTargets()
	if len(nodes) == 0 {
		log.Println("No targets available")
		return nil
	}

	rand.Seed(time.Now().UnixNano())
	randomWeight := rand.Intn(r.lbProxy.totalWeight) + 1

	cumulativeWeight := 0
	for i, node := range nodes {
		cumulativeWeight += r.lbProxy.weights[i]
		if randomWeight <= cumulativeWeight {
			return node
		}
	}

	log.Println("No node selected. This might indicate a configuration issue")
	return nil
}

