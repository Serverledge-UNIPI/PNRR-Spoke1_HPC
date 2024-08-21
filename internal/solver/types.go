package solver

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
	FunctionsCapacity       map[int][]interface{}   `json:"functions_capacity"`
}

// NodeAllocationInfo contains allocation details for a specific node
type NodeAllocationInfo struct {
    Instances          		int     `json:"instances"`           	// Number of instances allocated
    ComputationalCapacity	float64 `json:"computational_capacity"` // Computation capacity to assign to the function on this node
}

// FunctionNodeAllocation maps a node address to its allocation information for a specific function
type FunctionNodeAllocation struct {
    NodeAllocations	map[string]NodeAllocationInfo `json:"node_allocations"` // Key: Node IP, Value: Node allocation info
}

// SystemFunctionsAllocation maps a function name to its allocation across different nodes
type SystemFunctionsAllocation map[string]FunctionNodeAllocation
