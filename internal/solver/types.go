package solver

import (
	"sync"
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