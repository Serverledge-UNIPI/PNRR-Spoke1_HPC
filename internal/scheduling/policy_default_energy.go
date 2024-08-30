package scheduling

import (
	"log"

	"github.com/grussorusso/serverledge/internal/node"
)

type DefaultLocalPolicyEnergy struct{}

func (p *DefaultLocalPolicyEnergy) Init() {
}

func (p *DefaultLocalPolicyEnergy) OnCompletion(_ *scheduledRequest) {

}

func (p *DefaultLocalPolicyEnergy) OnArrival(r *scheduledRequest) {
	containerID, err := node.AcquireWarmContainer(r.Fun)
	if err == nil {
		log.Printf("Using a warm container for: %v\n", r)
		execLocally(r, containerID, true)
	} else {
		log.Printf("Request cannot be handled: %v\n", err)
		dropRequest(r)
	}
}
