package scheduling

import (
	"errors"
	"log"

	"github.com/grussorusso/serverledge/internal/node"
	"github.com/grussorusso/serverledge/internal/solver"
)

type DefaultLocalPolicyEnergy struct{}

func (p *DefaultLocalPolicyEnergy) Init() {
}

func (p *DefaultLocalPolicyEnergy) OnCompletion(_ *scheduledRequest) {

}

func (p *DefaultLocalPolicyEnergy) OnArrival(r *scheduledRequest) {
	containerID, err := node.AcquireWarmContainer(r.Fun)
	if err == nil {
		log.Printf("Solver status: %s. Using a warm container for: %v\n", solver.SolverStatus, r)
		execLocally(r, containerID, true)
		return
	} 
	
	solver.StatusMu.Lock()
	defer solver.StatusMu.Unlock()

	if solver.SolverStatus == "INFEASIBLE" && errors.Is(err, node.NoWarmFoundErr) {
		if handleColdStart(r) {
			log.Printf("Solver status: %s. Using a cold container for: %v\n", solver.SolverStatus, r)
			return
		}
	} 
	
	// Other error
	log.Printf("Request cannot be handled: %v\n", err)
	dropRequest(r)
	return
}
