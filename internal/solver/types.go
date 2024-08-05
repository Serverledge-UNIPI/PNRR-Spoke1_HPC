package solver

import (
	"sync"
	"log"
)

type NodeInformation struct {
	TotalMemoryMB			[]int
	ComputationalCapacity	[]int
	MaximumCapacity			[]int
	IPC						[]int
	PowerConsumption		[]int
}

type FunctionInformation struct {
	MemoryMB		[]int
	Workload   		[]int
	Deadline   		[]int
	Invocations 	[]int
}

type SolverResults struct {
	SolverStatusName        string              	`json:"solver_status_name"`
	SolverWalltime          float64             	`json:"solver_walltime"`
	ObjectiveValue          float64             	`json:"objective_value"`
	ActiveNodesIndexes      []int32             	`json:"active_nodes_indexes"`
	NodesInstances          map[int][]interface{} 	`json:"nodes_instances"`
	FunctionsCapacity       []float64				`json:"functions_capacity"`
}

type FunctionAllocation struct {
	Capacity  float64
	Instances map[string]int
}

type FunctionsAllocation map[string]FunctionAllocation

var (
    Allocation FunctionsAllocation
    mu         sync.RWMutex
)

func ModifyInstance(funcName, nodeIp string, newValue int) {
    mu.Lock()        
    defer mu.Unlock()

    allocation, exists := Allocation[funcName]
    if !exists {
        log.Println("Allocation for function '%s' not found", funcName)
        return
    }

	if newValue == 0 {
		// Remove nodeIp from Instances
		delete(allocation.Instances, nodeIp)
	} else {
		// Ppdate the instance with new value
		allocation.Instances[nodeIp] = newValue
	}

	Allocation[funcName] = allocation

	err := saveAllocationToEtcd(Allocation)
	if err != nil {
        log.Println("Error")
	}
}