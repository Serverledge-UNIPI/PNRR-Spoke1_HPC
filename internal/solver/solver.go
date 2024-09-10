package solver

import (
	"log"
	"fmt"
	"time"
	"bytes"
	"os"
	"sync"
	"syscall"
	"encoding/json"
	"net/http"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/registration"
	"github.com/grussorusso/serverledge/internal/node"
	"github.com/grussorusso/serverledge/internal/function"
	"github.com/grussorusso/serverledge/internal/metrics"
	"github.com/grussorusso/serverledge/utils"

	"github.com/shirou/gopsutil/cpu"

	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/net/context"
)

var epoch int32
var (
	SolverStatus string
	StatusMu sync.Mutex
)
var (
	functionsPeakInvocationsMap map[string][]int // contains peak invocations read from a json file (should be predicted)
	mu sync.Mutex
)

func getEpochDuration() time.Duration {
	epochDuration := config.GetInt(config.EPOCH_DURATION, 15)
	return time.Duration(epochDuration) * time.Minute
}

func createMapPeakInvocations() {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile("solver_inputs.json")
	if err != nil {
		fmt.Println("Error while reading functions data file: ", err)
	}

	var functionsPeakInvocations []FunctionPeakInvocations
	err = json.Unmarshal([]byte(data), &functionsPeakInvocations)
	if err != nil {
		fmt.Println("Error while decoding JSON: ", err)
	}

	// Map creation with function name as key and peak invocations list as value
	functionsPeakInvocationsMap = make(map[string][]int)
	for _, function := range functionsPeakInvocations {
		functionsPeakInvocationsMap[function.Name] = function.EpochPeakInvocations
	}

	if len(functionsPeakInvocationsMap) == 0 {
		log.Fatalf("Error: peak invocations simulation map is empty")
	}
}

func Init() {
	epoch = 0
	solverTicker := time.NewTicker(getEpochDuration())
	defer solverTicker.Stop()
	
	StatusMu.Lock()
	SolverStatus = "UNKNOWN"
	StatusMu.Unlock()
	
	isSolverNode := config.GetBool(config.IS_SOLVER_NODE, false)
	if isSolverNode {
		// Attempt to connect data exporter to Prometheus
		if metrics.Enabled {
			if err := metrics.ConnectToPrometheus(); !err {
				log.Printf("Failed to connect to Prometheus: %v", err)
			}
		}

		// Peak invocations prediction
		createMapPeakInvocations()

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
    log.Println("Running functions allocation watcher")

    etcdClient, err := utils.GetEtcdClient()
    if err != nil {
        log.Fatalf("Error getting etcd client: %v", err)
    }

    watchChan := etcdClient.Watch(context.Background(), "allocation")
    for watchResp := range watchChan {
        for _, event := range watchResp.Events {
            switch event.Type {
            case clientv3.EventTypePut:
                handlePutEvent(event)

            case clientv3.EventTypeDelete:
                handleDeleteEvent()

            default:
                log.Printf("Unhandled event type: %v", event.Type)
            }
        }
    }
}

func handlePutEvent(event *clientv3.Event) {
    log.Println("Etcd Event Type: PUT")

    // Deserialize JSON to obtain the SystemFunctionsAllocation struct
    var allocation SystemFunctionsAllocation
    if err := json.Unmarshal(event.Kv.Value, &allocation); err != nil {
        log.Printf("Error unmarshalling allocation: %v", err)
        return
    }

    // Update solver status
    StatusMu.Lock()
	if len(allocation) == 0 {
		SolverStatus = "INFEASIBLE"
	} else {
		SolverStatus = "FEASIBLE"
	}
	log.Printf("Solver status: %s", SolverStatus)
    StatusMu.Unlock()

    epoch++
}

func handleDeleteEvent() {
    log.Println("Etcd Event Type: DELETE")
    
    // Reset solver status
    StatusMu.Lock()
    SolverStatus = "UNKNOWN"
	log.Printf("Solver status: %s", SolverStatus)
    StatusMu.Unlock()
}

func solve() {
	log.Println("Running solver")

	if metrics.Enabled && epoch > 0 {
		// Save node metrics through data exporter
		metrics.SaveNodeMetrics(epoch - 1)
	}

	// Solver URL
	defaultHostport := fmt.Sprintf("%s:5000", utils.GetIpAddress().String())
	url := fmt.Sprintf("http://%s/solve_%s", config.GetString(config.SOLVER_ADDRESS, defaultHostport), config.GetString(config.SOLVER_TYPE, "milp"))

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

	//for _, functionName := range functions {
	//	log.Println("-----------------------------")
	//	f, _ := function.GetFunction(functionName)
	//	log.Printf("Function Name: %s\n", f.Name)
	//	log.Printf("Function Memory (MB): %v\n", f.MemoryMB)
	//	log.Printf("Workload: %v\n", f.Workload)
	//	log.Printf("Deadline (ms): %v\n", f.Deadline)
	//	log.Println("-----------------------------")
	//}

	var numberOfNodes int = len(serversMap)
	var numberOfFunctions int = len(functions)

	if numberOfFunctions == 0 {
		log.Printf("There are no registered functions")
		return
	}

	// Prepare data slices
	nodeInfo, nodeIp := prepareNodeInfo(serversMap)
	functionInfo := prepareFunctionInfo(functions)

    requestData := map[string]interface{}{
        "number_of_nodes":				numberOfNodes,
        "number_of_functions":			numberOfFunctions,
        "node_memory":					nodeInfo.TotalMemoryMB,
        "node_capacity":				nodeInfo.ComputationalCapacity,
        "maximum_capacity":				nodeInfo.MaximumCapacity,
        "node_ipc":						nodeInfo.IPC,
        "node_power_consumption":		nodeInfo.PowerConsumption,
        "function_memory":				functionInfo.MemoryMB,
        "function_workload":			functionInfo.Workload,
        "function_deadline":			functionInfo.Deadline,
        "function_peak_invocations":	functionInfo.PeakInvocations,
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

	for functionId, functionCapacity := range results.FunctionCapacities {
		log.Printf("Function '%s' computational capacities: %v", functions[functionId], functionCapacity)
	}

	for nodeId, instances := range results.NodeInstances {
		log.Printf("Node %d (%s) has instances: %v", nodeId, nodeIp[nodeId], instances)
	}

    StatusMu.Lock()
	SolverStatus = results.SolverStatusName
    StatusMu.Unlock()

	var functionsAllocation = make(SystemFunctionsAllocation)
	if results.SolverStatusName == "FEASIBLE" || results.SolverStatusName == "OPTIMAL" {
		// Retrive functions allocation
		functionsAllocation, err = computeFunctionsAllocation(results, functions, nodeIp)
		if err != nil {
			log.Fatalf("Error processing functions allocation: %v", err)
			return
		}
	}

	// Save functions allocation to Etcd
	if err := functionsAllocation.saveToEtcd(); err != nil {
		log.Fatalf("Error saving functions allocation to Etcd: %v", err)
	}

	if metrics.Enabled {
		var solverFails int = 0
		if results.SolverStatusName != "FEASIBLE" && results.SolverStatusName != "OPTIMAL" {
			solverFails = 1
		}
		
		// Save solver metrics through data exporter
		metrics.SaveSolverMetrics(results.ActiveNodesIndexes, epoch, solverFails, results.ObjectiveValue)
	}

	epoch++
}

func prepareNodeInfo(serversMap map[string]*registration.StatusInformation) (NodeInformation, []string) {
	nodeIp := make([]string, len(serversMap))
	nodeInfo := NodeInformation{
		TotalMemoryMB:			make([]int, len(serversMap)),
		ComputationalCapacity:	make([]int, len(serversMap)),
		MaximumCapacity:      	make([]int, len(serversMap)),
		IPC:              		make([]int, len(serversMap)),
		PowerConsumption: 		make([]int, len(serversMap)),
	}

	i := 0
	for _, server := range serversMap {
        nodeInfo.TotalMemoryMB[i] = int(server.TotalMemoryMB)
        nodeInfo.ComputationalCapacity[i] = int(server.ComputationalCapacity)
        nodeInfo.MaximumCapacity[i] = int(server.MaximumCapacity)
        nodeInfo.IPC[i] = int(server.IPC * 10)
        nodeInfo.PowerConsumption[i] = int(server.PowerConsumption)

        // Get node IP address
        nodeIp[i] = server.Url
		i++
    }

	return nodeInfo, nodeIp
}

func prepareFunctionInfo(functions []string) FunctionInformation {
	mu.Lock()
	defer mu.Unlock()

	functionInfo := FunctionInformation{
		MemoryMB:			make([]int, len(functions)),
		Workload:			make([]int, len(functions)),
		Deadline:			make([]int, len(functions)),
		PeakInvocations:	make([]int, len(functions)),
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
		functionInfo.PeakInvocations[i] = int(functionsPeakInvocationsMap[functionName][epoch])
	}

	return functionInfo
}

func computeFunctionsAllocation(results SolverResults, functions []string, nodeIp []string) (SystemFunctionsAllocation, error) {
	functionsAllocation := make(SystemFunctionsAllocation)
	
	for i, functionName := range functions {
		nodesMap := make(map[string]NodeAllocationInfo)
		functionCapacities := results.FunctionCapacities[i]
		
		emptyAllocation := true
		for key, instances := range results.NodeInstances {
			if floatVal, ok := instances[i].(float64); ok && floatVal > 0 {
				// Type assertion
				if capacityAssigned, ok := functionCapacities[key].(float64); ok {
					nodesMap[nodeIp[key]] = NodeAllocationInfo{
						PrewarmContainers:		int(floatVal),
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

	var mem syscall.Sysinfo_t
    err = syscall.Sysinfo(&mem)
    if err != nil {
        fmt.Println("Error:", err)
        return err
    }

    totalMemory := mem.Totalram * uint64(mem.Unit)
    totalMemoryMB := totalMemory / (1024 * 1024)

	node.Resources.ComputationalCapacity = cpuInfo[0].Mhz * float64(len(cpuInfo))
	node.Resources.MaximumCapacity = cpuInfo[0].Mhz
	node.Resources.IPC = float32(config.GetFloat(config.NODE_IPC, 0))
	node.Resources.PowerConsumption = int32(config.GetInt(config.NODE_POWER_CONSUMPTION, 0))
	node.Resources.TotalMemoryMB = int64(totalMemoryMB)

	return nil
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
	resp, err := etcdClient.Grant(ctx, int64(config.GetInt(config.EPOCH_DURATION, 15) * 60 ) + 20) // TODO: remove +20
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
