package solver

import (
	"log"
	"fmt"
	"time"
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/registration"
	"github.com/grussorusso/serverledge/internal/node"
	"github.com/grussorusso/serverledge/internal/function"
	"github.com/grussorusso/serverledge/internal/cache"
	"github.com/grussorusso/serverledge/internal/metrics"
	"github.com/grussorusso/serverledge/utils"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"

	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/net/context"
)

var epoch int32

func getEpochDuration() time.Duration {
	epochDuration := config.GetInt(config.EPOCH_DURATION, 20)
	return time.Duration(epochDuration) * time.Second // TODO: fix to time.Minute
}

func Init() {
	isSolverNode := config.GetBool(config.IS_SOLVER_NODE, false)
	if isSolverNode {
		epoch = 0
		solverTicker := time.NewTicker(getEpochDuration())
		defer solverTicker.Stop()

		// Attempt to connect data exporter to Prometheus
		if metrics.Enabled {
			if err := metrics.ConnectToPrometheus(); !err {
				log.Printf("Failed to connect to Prometheus: %v", err)
			}
		}

		// TODO: remove
		time.Sleep(10 * time.Second)
		solve()

		for {
			select {
			case <-solverTicker.C:
				solve()
			}
		}
	} else {
		watchFunctionsAllocation()
	}
}

func watchFunctionsAllocation() {
	log.Println("Running function allocation watcher")
	
	etcdClient, err := utils.GetEtcdClient()
	if err != nil {
		log.Fatal(err)
		return
	}

    watchChan := etcdClient.Watch(context.Background(), "allocation")
    for watchResp := range watchChan {
        for _, event := range watchResp.Events {
			switch event.Type {
			case clientv3.EventTypePut:
				log.Println("Etcd Event Type: PUT")
				
				// Deserialize JSON to obtain the SystemFunctionsAllocation struct
				var allocation SystemFunctionsAllocation
				err := json.Unmarshal(event.Kv.Value, &allocation)
				if err != nil {
					log.Printf("Error unmarshalling allocation: %v", err)
					continue
				}

				// Update local cache
				allocation.saveToCache()
				log.Printf("Updated cache with new allocation: %v", allocation)

			case clientv3.EventTypeDelete:
				log.Println("Etcd Event Type: DELETE")

				// Delete allocation from the local cache
				deleteFromCache()

			default:
				log.Printf("Unhandled event type: %v\n", event.Type)
			}
        }
    }
}

func solve() {
	log.Println("Running solver")

	// Solver URL
	defaultHostport := fmt.Sprintf("%s:5000", utils.GetIpAddress().String())
	url := fmt.Sprintf("http://%s/solve_with_cp_sat", config.GetString(config.SOLVER_ADDRESS, defaultHostport))

	// Get all available servers and functions
	serversMap := registration.GetServersMap()
	functions, err := function.GetAll()
	if err != nil {
		log.Fatalf("Error retrieving functions: %v", err)
		return
	}

	// Log system information
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

	for _, functionName := range functions {
		log.Println("-----------------------------")
		f, _ := function.GetFunction(functionName)
		log.Printf("Function name: %s\n", f.Name)
		log.Printf("Function Memory (MB): %v\n", f.MemoryMB)
		log.Printf("Workload: %v\n", f.Workload)
		log.Printf("Deadline (ms): %v\n", f.Deadline)
		log.Printf("Invocations: %v\n", f.Invocations)
		log.Println("-----------------------------")
	}

	var numberOfNodes int = len(serversMap) + 1
	var numberOfFunctions int = len(functions)

	if numberOfFunctions == 0 {
		log.Printf("There are no registered functions")
		return
	}

	// Prepare data slices
	nodeInfo, nodeIp := prepareNodeInfo(serversMap)
	functionInfo := prepareFunctionInfo(functions)

    requestData := map[string]interface{}{
        "number_of_nodes":        numberOfNodes,
        "number_of_functions":    numberOfFunctions,
        "node_memory":            nodeInfo.TotalMemoryMB,
        "node_capacity":          nodeInfo.ComputationalCapacity,
        "maximum_capacity":       nodeInfo.MaximumCapacity,
        "node_ipc":               nodeInfo.IPC,
        "node_power_consumption": nodeInfo.PowerConsumption,
        "function_memory":        functionInfo.MemoryMB,
        "function_workload":      functionInfo.Workload,
        "function_deadline":      functionInfo.Deadline,
        "function_invocations":   functionInfo.Invocations,
    }

    requestBody, err := json.Marshal(requestData)
    if err != nil {
        log.Fatalf("Error marshalling request data: %v", err)
    }

    // Create a POST request
    response, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
    if err != nil {
        log.Printf("Error making request: %v", err)
		return
    }
    defer response.Body.Close()

	var results SolverResults
    err = json.NewDecoder(response.Body).Decode(&results)
    if err != nil {
        log.Fatalf("Error decoding response: %v", err)
    }

	// Log results
	log.Printf("Solver walltime: %f", results.SolverWalltime)
	log.Printf("Solver status: %s", results.SolverStatusName)
	log.Printf("Energy consumption: %f", results.ObjectiveValue)
	log.Printf("Active nodes: %v", results.ActiveNodesIndexes)

	for functionID, functionsCapacity := range results.FunctionsCapacity {
		log.Printf("Function %d computational capacity: %v", functionID, functionsCapacity)
	}

	for nodeID, instances := range results.NodesInstances {
		log.Printf("Node %d (%s) has instances: %v", nodeID, nodeIp[nodeID], instances)
	}

	// Retrive functions allocation
	functionsAllocation, err := computeFunctionsAllocation(results, functions, nodeIp)
	if err != nil {
		log.Fatalf("Error processing functions allocation: %v", err)
		return
	}

	// Save functions allocation to Etcd
	if err := functionsAllocation.saveToEtcd(); err != nil {
		log.Fatalf("Error saving functions allocation to Etcd: %v", err)
	}

	// Save the new allocation to the local cache
	functionsAllocation.saveToCache()

	if metrics.Enabled {
		var solverFails int = 0
		if results.SolverStatusName != "FEASIBLE" && results.SolverStatusName != "OPTIMAL" {
			solverFails = 1
		}
		metrics.RecordSolverMetrics(results.ActiveNodesIndexes, epoch, solverFails, nodeInfo.PowerConsumption)
		metrics.ResetCurrentFailures()

		// Save solver metrics through data exporter
		metrics.SaveMetrics(epoch)
		epoch++
	}
	
	log.Println("Solver terminated")
}

func prepareNodeInfo(serversMap map[string]*registration.StatusInformation) (NodeInformation, []string) {
	nodeIp := make([]string, len(serversMap) + 1)
	nodeInfo := NodeInformation{
		TotalMemoryMB:			make([]int, len(serversMap) + 1),
		ComputationalCapacity:	make([]int, len(serversMap) + 1),
		MaximumCapacity:      	make([]int, len(serversMap) + 1),
		IPC:              		make([]int, len(serversMap) + 1),
		PowerConsumption: 		make([]int, len(serversMap) + 1),
	}

	i := 0
	for _, server := range serversMap {
        nodeInfo.TotalMemoryMB[i] = int(server.TotalMemoryMB)
        nodeInfo.ComputationalCapacity[i] = int(server.ComputationalCapacity)
        nodeInfo.MaximumCapacity[i] = int(server.MaximumCapacity)
        nodeInfo.IPC[i] = int(server.IPC * 10)
        nodeInfo.PowerConsumption[i] = int(server.PowerConsumption)

        // Get node IP address
        nodeIp[i] = server.Url[7:len(server.Url) - 5]
		i++
    }

    nodeInfo.TotalMemoryMB[i] = int(node.Resources.TotalMemoryMB)
    nodeInfo.ComputationalCapacity[i] = int(node.Resources.ComputationalCapacity)
    nodeInfo.MaximumCapacity[i] = int(node.Resources.MaximumCapacity)
    nodeInfo.IPC[i] = int(node.Resources.IPC * 10)
    nodeInfo.PowerConsumption[i] = int(node.Resources.PowerConsumption)

	nodeIp[i] = utils.GetIpAddress().String()

	return nodeInfo, nodeIp
}

func prepareFunctionInfo(functions []string) FunctionInformation {
	functionInfo := FunctionInformation{
		MemoryMB:		make([]int, len(functions)),
		Workload:		make([]int, len(functions)),
		Deadline:		make([]int, len(functions)),
		Invocations:	make([]int, len(functions)),
	}

	for i, functionName := range functions {
		f, err := function.GetFunction(functionName)
		if !err {
			log.Printf("Error retrieving function %s: %v", functionName, err)
			continue
		}

		functionInfo.MemoryMB[i] = int(f.MemoryMB)
		functionInfo.Workload[i] = int(f.Workload / 1e6)
		functionInfo.Deadline[i] = int(f.Deadline)
		functionInfo.Invocations[i] = int(f.Invocations)
	}

	return functionInfo
}

func computeFunctionsAllocation(results SolverResults, functions []string, nodeIp []string) (SystemFunctionsAllocation, error) {
	functionsAllocation := make(SystemFunctionsAllocation)
	
	for i, functionName := range functions {
		nodesMap := make(map[string]NodeAllocationInfo)
		functionsCapacity := results.FunctionsCapacity[i]
		
		emptyAllocation := true
		for key, instances := range results.NodesInstances {
			if floatVal, ok := instances[i].(float64); ok && floatVal > 0 {
				// Type assertion
				if capacityAssigned, ok := functionsCapacity[key].(float64); ok {
					nodesMap[nodeIp[key]] = NodeAllocationInfo{
						Instances: 				int(floatVal),
						ComputationalCapacity:  capacityAssigned,
					}
					emptyAllocation = false
				}
			}
		}

		if !emptyAllocation {
			functionsAllocation[functionName] = FunctionNodeAllocation{
				NodeAllocations: nodesMap,
			}
		}
	}

	return functionsAllocation, nil
}

func InitNodeResources() error {
	// Initialize node resources information
	cpuInfo, err := cpu.Info()
	if err != nil {
		log.Fatal(err)
		return err
	}

	vMemInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Fatal(err)
		return err
	}

	node.Resources.ComputationalCapacity = cpuInfo[0].Mhz * float64(len(cpuInfo))
	node.Resources.MaximumCapacity = cpuInfo[0].Mhz
	node.Resources.IPC = 1 // TODO
	node.Resources.PowerConsumption = int32(config.GetInt(config.NODE_POWER_CONSUMPTION, 0))
	node.Resources.TotalMemoryMB = int64(vMemInfo.Total / 1e6)

	return nil
}

func (functionsAllocation *SystemFunctionsAllocation) saveToCache () {
	cache.GetCacheInstance().Set("allocation", functionsAllocation, getEpochDuration())
}

func deleteFromCache () {
	cache.GetCacheInstance().Delete("allocation")
}

func GetAllocationFromCache() (*SystemFunctionsAllocation, bool) {
	systemFunctionsAllocation, found := cache.GetCacheInstance().Get("allocation")
	if !found {
		// Cache miss
		return &SystemFunctionsAllocation{}, false
	}

	// Cache hit
	functionsAllocation, ok := systemFunctionsAllocation.(*SystemFunctionsAllocation)
	if !ok {
		// Type assertion failed
		log.Println("Type assertion failed: expected *SystemFunctionsAllocation")
		return &SystemFunctionsAllocation{}, false
	}

	functionsAllocationCopy := *functionsAllocation
	return &functionsAllocationCopy, true
}

func (functionsAllocation *SystemFunctionsAllocation) saveToEtcd() error {
	etcdClient, err := utils.GetEtcdClient()
	if err != nil {
		log.Fatal(err)
		return err
	}

	payload, err := json.Marshal(functionsAllocation)
	if err != nil {
		return fmt.Errorf("Could not marshal functions allocation: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := etcdClient.Grant(ctx, int64(getEpochDuration() / time.Second)) // TODO: fix to time.Minute
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

func getFromEtcd() (*SystemFunctionsAllocation, error) {
	etcdClient, err := utils.GetEtcdClient()
	if err != nil {
		log.Fatal(err)
		return &SystemFunctionsAllocation{}, err
	}

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    resp, err := etcdClient.Get(ctx, "allocation")
    if err != nil {
        return &SystemFunctionsAllocation{}, fmt.Errorf("Failed to get functions allocation from etcd: %v", err)
    }

    if len(resp.Kvs) == 0 {
        return &SystemFunctionsAllocation{}, fmt.Errorf("No data found for key 'allocation'")
    }

    var functionsAllocation SystemFunctionsAllocation
    err = json.Unmarshal(resp.Kvs[0].Value, &functionsAllocation)
    if err != nil {
        return &SystemFunctionsAllocation{}, fmt.Errorf("Failed to unmarshal functions allocation: %v", err)
    }

    return &functionsAllocation, nil
}


func DecrementInstances(allocation *SystemFunctionsAllocation, functionName string, nodeIp string) bool {
	// Check if the function allocation exists
	functionAllocation, exists := (*allocation)[functionName]
	if !exists {
		log.Printf("Function '%s' not found in allocation", functionName)
		return false
	}

	// Check if node allocation exists
	nodeAllocation, ok := functionAllocation.NodeAllocations[nodeIp]
	if !ok {
		log.Printf("Node %s not found in function %s allocation", nodeIp, functionName)
		return false
	}
		
	// Update the value
	newValue := nodeAllocation.Instances - 1
	if newValue == 0 {
		// Remove node from the map if it handled all the requests
		delete(functionAllocation.NodeAllocations, nodeIp)

		// Remove function from the map if function has no more node allocation
		if len(functionAllocation.NodeAllocations) == 0 {
			delete(*allocation, functionName)
			if len(*allocation) == 0 {
				deleteFromCache()
				return true
			}
		}
	} else if newValue > 0 {
		// Update node allocation info
		nodeAllocation.Instances = newValue
		functionAllocation.NodeAllocations[nodeIp] = nodeAllocation
		(*allocation)[functionName] = functionAllocation
	} else {
		log.Println("Invalid number of instances")
		return false
	}

	// Put the updated allocation back into the cache
	localCache := cache.GetCacheInstance()
	remainingExpiration := localCache.GetExpiration("allocation")
	localCache.Set("allocation", allocation, remainingExpiration)
	return true
}
