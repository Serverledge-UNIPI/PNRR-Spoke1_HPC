package solver

/*
#cgo CFLAGS: -I/usr/include/python3.10
#cgo LDFLAGS: -lpython3.10
#include <Python.h>
#include <stdlib.h>

extern void initializePython();
extern void finalizePython();
extern int* allocateMemory(int size);
extern void freeMemory(int* arr);
extern const char* startSolver(int numberOfNodes, int numberOfFunctions, int* nodeMemory, int* nodeCapacity, int* maximumCapacity, int* nodeIpc, int* nodePowerConsumption, int* functionMemory, int* functionWorkload, int* functionDeadline, int* functionInvocations);
*/
import "C"
import (
	"encoding/json"
	"log"
	"time"
	"fmt"
	"math"
	"sync"
	"unsafe"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/registration"
	"github.com/grussorusso/serverledge/internal/node"
	"github.com/grussorusso/serverledge/internal/function"
	"github.com/grussorusso/serverledge/utils"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"

	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/net/context"
)

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

type FunctionAllocations map[string]FunctionAllocation

var (
    Allocation FunctionAllocations
    mu         sync.RWMutex
)

func SetAllocation(newAllocations FunctionAllocations) {
    mu.Lock()
    defer mu.Unlock()
    Allocation = newAllocations
}

func GetAllocation() FunctionAllocations {
    mu.RLock()
    defer mu.RUnlock()
    return Allocation
}

func InitNodeResources() {
	// Initialize node resources information
	cpuInfos, err := cpu.Info()
	if err != nil {
		log.Fatal(err)
	}

	vMemInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Fatal(err)
	}

	node.Resources.ComputationalCapacity = cpuInfos[0].Mhz * float64(len(cpuInfos))
	node.Resources.MaximumCapacity = cpuInfos[0].Mhz
	node.Resources.IPC = 1 // TODO
	node.Resources.PowerConsumption = 400 // TODO
	node.Resources.TotalMemoryMB = int64(vMemInfo.Total / 1e6)
}

func Run() {
	epochDuration := config.GetInt(config.EPOCH_DURATION, 30)
	solverTicker := time.NewTicker(time.Duration(epochDuration) * time.Second) // TODO: time.Minute
	defer solverTicker.Stop()

	log.Printf("Configured epoch duration: %d\n", epochDuration)

	for {
		select {
		case <-solverTicker.C:
			solve()
		}
	}
}

func allocateAndInitialize(data []int) *C.int {
	size := len(data)
	cArray := C.allocateMemory(C.int(size))
	for i := 0; i < size; i++ {
		cElement := (*C.int)(unsafe.Pointer(uintptr(unsafe.Pointer(cArray)) + uintptr(i)*unsafe.Sizeof(*cArray)))
		*cElement = C.int(data[i])
	}
	return cArray
}

func solve() {
	log.Println("Running solver")
	// Get all available servers
	serversMap := registration.GetServersMap()

	for _, value := range serversMap {
		log.Println("-----------------------------")
		log.Printf("URL: %s\n", value.Url)
		log.Printf("Available Warm Containers: %v\n", value.AvailableWarmContainers)
		log.Printf("Available Memory (MB): %d\n", value.AvailableMemMB)
		log.Printf("Available CPUs: %f\n", value.AvailableCPUs)
		log.Printf("Drop Count: %d\n", value.DropCount)
		log.Printf("Total Memory (MB): %v\n", value.TotalMemoryMB)
		log.Printf("Computational Capacity: %f\n", value.ComputationalCapacity)
		log.Printf("Maximum Capacity: %f\n", value.MaximumCapacity)
		log.Printf("IPC: %v\n", value.IPC)
		log.Printf("Power Consumption: %v\n", value.PowerConsumption)
		log.Println("-----------------------------")
	}

	functions, err := function.GetAll()
    if err != nil {
        log.Printf("Error:", err)
        return
    }

	for _, functionName := range functions {
		log.Println("-----------------------------")
		f, _ := function.GetFunction(functionName)
		log.Printf("Function name: %s\n", f.Name)
		log.Printf("Function Memory (MB): %v\n", f.MemoryMB)
		log.Printf("CPU Demand: %f\n", f.CPUDemand)
		log.Printf("Workload: %v\n", f.Workload)
		log.Printf("Deadline (ms): %v\n", f.Deadline)
		log.Printf("Invocations: %v\n", f.Invocations)
		log.Println("-----------------------------")
	}

	// Get data from registry
	var numberOfNodes int = len(serversMap) + 1
	var numberOfFunctions int = len(functions)

	if numberOfNodes == 0 || numberOfFunctions == 0 {
		return
	}

	var nodeMemory []int
	var nodeCapacity []int
	var maximumCapacity []int
	var nodeIpc []int
	var nodePowerConsumption []int
	
	for _, value := range serversMap {
		nodeMemory = append(nodeMemory, int(value.TotalMemoryMB)) // MB
		nodeCapacity = append(nodeCapacity, int(value.ComputationalCapacity)) // Mhz
		maximumCapacity = append(maximumCapacity, int(value.MaximumCapacity))
		nodeIpc = append(nodeIpc, int(value.IPC * 10))
		nodePowerConsumption = append(nodePowerConsumption, int(value.PowerConsumption))
	}

	nodeMemory = append(nodeMemory, int(node.Resources.TotalMemoryMB)) // MB
	nodeCapacity = append(nodeCapacity, int(node.Resources.ComputationalCapacity)) // Mhz
	maximumCapacity = append(maximumCapacity, int(node.Resources.MaximumCapacity))
	nodeIpc = append(nodeIpc, int(node.Resources.IPC * 10))
	nodePowerConsumption = append(nodePowerConsumption, int(node.Resources.PowerConsumption))

	var functionMemory []int
	var functionWorkload []int
	var functionDeadline []int
	var functionInvocations []int

	for _, functionName := range functions {
		f, _ := function.GetFunction(functionName)
		functionMemory = append(functionMemory, int(f.MemoryMB)) // MB
		functionWorkload = append(functionWorkload, int(f.Workload / 1e6)) // workload / 10**6 
		functionDeadline = append(functionDeadline, int(f.Deadline))
		functionInvocations = append(functionInvocations, int(f.Invocations))
	}

	C.initializePython()
	//defer C.finalizePython()

	cNodeMemory := allocateAndInitialize(nodeMemory)
	defer C.freeMemory(cNodeMemory)

	cNodeCapacity := allocateAndInitialize(nodeCapacity)
	defer C.freeMemory(cNodeCapacity)
	
	cMaximumCapacity := allocateAndInitialize(maximumCapacity)
	defer C.freeMemory(cMaximumCapacity)

	cNodeIpc := allocateAndInitialize(nodeIpc)
	defer C.freeMemory(cNodeIpc)

	cNodePowerConsumption := allocateAndInitialize(nodePowerConsumption)
	defer C.freeMemory(cNodePowerConsumption)

	cFunctionMemory := allocateAndInitialize(functionMemory)
	defer C.freeMemory(cFunctionMemory)
	
	cFunctionWorkload := allocateAndInitialize(functionWorkload)
	defer C.freeMemory(cFunctionWorkload)
	
	cFunctionDeadline := allocateAndInitialize(functionDeadline)
	defer C.freeMemory(cFunctionDeadline)

	cFunctionInvocations := allocateAndInitialize(functionInvocations)
	defer C.freeMemory(cFunctionInvocations)

	log.Printf("Started solver\n")
	cResults := C.startSolver(
		C.int(numberOfNodes),
		C.int(numberOfFunctions),
		cNodeMemory,
		cNodeCapacity,
		cMaximumCapacity,
		cNodeIpc,
		cNodePowerConsumption,
		cFunctionMemory,
		cFunctionWorkload,
		cFunctionDeadline,
		cFunctionInvocations,
	)

	jsonStr := C.GoString(cResults)

	var results SolverResults
	err = json.Unmarshal([]byte(jsonStr), &results)
	if err != nil {
		log.Fatal(err)
		return
	}

	log.Printf("Solver walltime: %f", results.SolverWalltime)
	log.Printf("Solver status: %s", results.SolverStatusName)
	log.Printf("Energy consumption: %f\n", results.ObjectiveValue)
	log.Printf("Active nodes: %v\n", results.ActiveNodesIndexes)
	log.Printf("Functions capacity: %v\n", results.FunctionsCapacity)
	for nodeID, instances := range results.NodesInstances {
		log.Printf("Node %d has instances: %v\n", nodeID, instances)
	}

	nodeIPs := []string{}
	for _, value := range serversMap {
		ip := value.Url[7 : len(value.Url) - 5]
		nodeIPs = append(nodeIPs, ip)
	}
	nodeIPs = append(nodeIPs, utils.GetIpAddress().String())

	log.Printf("node IPs: %v\n", nodeIPs)

	allocations := make(FunctionAllocations)

	for i, functionName := range functions {
		ipInstancesMap := make(map[string]int)
		for key, instances := range results.NodesInstances {
			if floatVal, ok := instances[i].(float64); ok {
				ipInstancesMap[nodeIPs[key]] = int(floatVal)
			} else {
				log.Printf("Expected float64 but found %T at index %d for nodeID %d", instances[i], i, key)
			}
		}

		allocations[functionName] = FunctionAllocation{
			Capacity: results.FunctionsCapacity[i],
			Instances: ipInstancesMap,
		}

		// Update CPU Demand
		f, _ := function.GetFunction(functionName)
		f.CPUDemand = math.Round((results.FunctionsCapacity[i] / node.Resources.MaximumCapacity) * 100) / 100
		// TODO: check if CPU demand > 1
		err = f.SaveToEtcd()
		if err != nil {
			log.Fatal(err)
		}
	}

	for functionName, functionAllocation := range allocations {
		log.Printf("----------------------\n")
		log.Printf("Function name: %s\n", functionName)
		log.Printf("Capacity needed (Mhz): %f", functionAllocation.Capacity)
		for node, instances := range functionAllocation.Instances {
			log.Printf("Node IP: %s\n", node)
			log.Printf("Instances: %v\n", instances)
		}
	}
	log.Printf("----------------------\n")

	// Save allocation to Etcd
	if err := saveAllocationsToEtcd(allocations); err != nil {
		log.Fatal(err)
	}
	log.Println("Solver terminated")
}

func WatchAllocation() {
	log.Println("Watching allocation...")
	etcdClient, err := utils.GetEtcdClient()
	if err != nil {
		log.Fatal(err)
		return
	}

    // Watcher
    watchChan := etcdClient.Watch(context.Background(), "allocation")
    for watchResp := range watchChan {
        for _, event := range watchResp.Events {
            log.Printf("Event received! Type: %s Key: %s Value: %s\n", event.Type, event.Kv.Key, event.Kv.Value)

			// Update functions allocation
			allocations, err := getAllocationsFromEtcd()
			if err != nil {
				log.Printf("Error retrieving allocations: %v\n", err)
				continue
			}

			SetAllocation(allocations)
			log.Printf("Updated Allocation: %v\n", Allocation)
        }
    }
}

func saveAllocationsToEtcd(allocations FunctionAllocations) error {
	etcdClient, err := utils.GetEtcdClient()
	if err != nil {
		log.Fatal(err)
		return err
	}

	payload, err := json.Marshal(allocations)
	if err != nil {
		return fmt.Errorf("Could not marshal allocations: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := etcdClient.Grant(ctx, 60) // TODO: lease time
	if err != nil {
		log.Fatal(err)
		return err
	}

	_, err = etcdClient.Put(ctx, "allocation", string(payload), clientv3.WithLease(resp.ID))
	if err != nil {
		log.Fatal(err)
		return err
	}

	return nil
}

func getAllocationsFromEtcd() (FunctionAllocations, error) {
	etcdClient, err := utils.GetEtcdClient()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    resp, err := etcdClient.Get(ctx, "allocation")
    if err != nil {
        return nil, fmt.Errorf("failed to get allocations from etcd: %v", err)
    }

    if len(resp.Kvs) == 0 {
        return nil, fmt.Errorf("no data found for key 'allocation'")
    }

    var allocations FunctionAllocations
    err = json.Unmarshal(resp.Kvs[0].Value, &allocations)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal allocations: %v", err)
    }

    return allocations, nil
}
