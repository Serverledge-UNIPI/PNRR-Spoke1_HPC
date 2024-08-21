package scheduling

import (
	"errors"
	"log"
	"math"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/node"
	"github.com/grussorusso/serverledge/internal/solver"
	"github.com/grussorusso/serverledge/utils"
)

type DefaultLocalPolicyEnergy struct {
	queue queue
}

// Function used to set CPU demand assigned for the specific node
func setFunctionCPUDemand(req *scheduledRequest) {
	functionsAllocation := solver.GetAllocation(true)
    nodeAllocation, _ := functionsAllocation[req.Fun.Name].NodeAllocations[utils.GetIpAddress().String()]
	functionCPUDemand := math.Round((nodeAllocation.ComputationalCapacity / node.Resources.MaximumCapacity) * 100) / 100
	req.Fun.CPUDemand = functionCPUDemand
}

func (p *DefaultLocalPolicyEnergy) Init() {
	queueCapacity := config.GetInt(config.SCHEDULER_QUEUE_CAPACITY, 0)
	if queueCapacity > 0 {
		log.Printf("Configured queue with capacity %d\n", queueCapacity)
		p.queue = NewFIFOQueue(queueCapacity)
	} else {
		p.queue = nil
	}
}

func (p *DefaultLocalPolicyEnergy) OnCompletion(_ *scheduledRequest) {
	if p.queue == nil {
		return
	}

	p.queue.Lock()
	defer p.queue.Unlock()
	if p.queue.Len() == 0 {
		return
	}

	req := p.queue.Front()

	// Set function CPU deamnd
    setFunctionCPUDemand(req)

	containerID, err := node.AcquireWarmContainer(req.Fun)
	if err == nil {
		p.queue.Dequeue()
		log.Printf("[%s] Warm start from the queue (length=%d)\n", req, p.queue.Len())
		execLocally(req, containerID, true)
		return
	}

	if errors.Is(err, node.NoWarmFoundErr) {
		if node.AcquireResources(req.Fun.CPUDemand, req.Fun.MemoryMB, true) {
			log.Printf("[%s] Cold start from the queue\n", req)
			p.queue.Dequeue()

			// This avoids blocking the thread during the cold
			// start, but also allows us to check for resource
			// availability before dequeueing
			go func() {
				newContainer, err := node.NewContainerWithAcquiredResources(req.Fun)
				if err != nil {
					dropRequest(req)
				} else {
					execLocally(req, newContainer, false)
				}
			}()
			return
		}
	} else if errors.Is(err, node.OutOfResourcesErr) {
	} else {
		// other error
		p.queue.Dequeue()
		dropRequest(req)
	}
}

func (p *DefaultLocalPolicyEnergy) OnArrival(r *scheduledRequest) {
	// Set function CPU deamnd
	setFunctionCPUDemand(r)

	containerID, err := node.AcquireWarmContainer(r.Fun)
	if err == nil {
		execLocally(r, containerID, true)
		return
	}

	if errors.Is(err, node.NoWarmFoundErr) {
		if handleColdStart(r) {
			return
		}
	} else if errors.Is(err, node.OutOfResourcesErr) {
		// pass
	} else {
		// other error
		dropRequest(r)
		return
	}

	// enqueue if possible
	if p.queue != nil {
		p.queue.Lock()
		defer p.queue.Unlock()
		if p.queue.Enqueue(r) {
			log.Printf("[%s] Added to queue (length=%d)\n", r, p.queue.Len())
			return
		}
	}

	dropRequest(r)
}
