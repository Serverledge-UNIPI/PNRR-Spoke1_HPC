package lb

import (
	"net/url"
)

type FunctionProxyMap map[string]Proxy

// Proxy is an interface that both LBProxy and LBWeightedProxy implement
type Proxy interface {
	setPolicy(policy LBPolicy) 
	getPolicy() LBPolicy
	setTargets(targets []*url.URL)
    getTargets() []*url.URL
}

// LBProxy represents a standard proxy with a list of targets and a policy
type LBProxy struct {
	targets  []*url.URL
	lbPolicy LBPolicy
}

// LBWeightedProxy represents a proxy that includes weights for each target
type LBWeightedProxy struct {
	targets  	[]*url.URL
	weights  	[]int
	totalWeight	int
	lbPolicy 	LBPolicy
}

// LBProxy setters and getters
func (p *LBProxy) setPolicy(policy LBPolicy) {
    p.lbPolicy = policy
}

func (p *LBProxy) getPolicy() LBPolicy {
    return p.lbPolicy
}

func (p *LBProxy) setTargets(targets []*url.URL) {
    p.targets = targets
}

func (p *LBProxy) getTargets() []*url.URL {
    return p.targets
}

// LBWeightedProxy setters and getters
func (p *LBWeightedProxy) setPolicy(policy LBPolicy) {
    p.lbPolicy = policy
}

func (p *LBWeightedProxy) getPolicy() LBPolicy {
    return p.lbPolicy
}

func (p *LBWeightedProxy) setTargets(targets []*url.URL) {
    p.targets = targets
}

func (p *LBWeightedProxy) getTargets() []*url.URL {
    return p.targets
}
