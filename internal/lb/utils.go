package lb

import (
	"encoding/json"
	"sync"
	"log"
	"net/url"
	"fmt"

	"github.com/grussorusso/serverledge/internal/solver"
	"github.com/grussorusso/serverledge/internal/client"
	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/utils"

	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/net/context"
)

var epoch = -1

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
    var allocation solver.SystemFunctionsAllocation
    if err := json.Unmarshal(event.Kv.Value, &allocation); err != nil {
        log.Printf("Error unmarshalling allocation: %v", err)
        return
    }

    // Update local allocation
    mu.Lock()
    localAllocation = allocation
    log.Printf("Updated local allocation with: %v", localAllocation)
    mu.Unlock()

    epoch++

    // Process the allocation and send prewarming requests
    processAllocation(allocation)
}

func handleDeleteEvent() {
    log.Println("Etcd Event Type: DELETE")
    
    // Delete local allocation
    mu.Lock()
    localAllocation = nil
    mu.Unlock()

    // Clear proxy map
    for proxyIdentifier := range proxyMap {
        if proxyIdentifier != "edge" && proxyIdentifier != "cloud" {
            proxyMap.deleteProxy(proxyIdentifier)
        }
    }
}

func processAllocation(allocation solver.SystemFunctionsAllocation) {
    var wg sync.WaitGroup

    for functionName, functionNodeAllocation := range allocation {
        targets := []*url.URL{}
        weights := []int{}

        for nodeIp, nodeAllocation := range functionNodeAllocation.NodeAllocations {
            wg.Add(1)

            //nodeUrl := fmt.Sprintf("http://%s:%d", nodeIp, config.GetInt(config.API_PORT, 1323))
            nodeUrl, err := url.Parse(nodeIp)
            if err != nil {
                log.Printf("Error parsing node URL %s: %v", nodeUrl, err)
                wg.Done()
                continue
            }
            targets = append(targets, nodeUrl)
            weights = append(weights, nodeAllocation.PrewarmContainers)

            // Send prewarming requests
            go func(functionName string, nodeIp string, nodeAllocation solver.NodeAllocationInfo) {
                defer wg.Done()
                sendPrewarmRequest(functionName, nodeIp, nodeAllocation)
            }(functionName, nodeIp, nodeAllocation)
        }

        // Add a new proxy or update an existing one
        if _, exists := proxyMap[functionName]; !exists {
            lbPolicyName := config.GetString(config.LOAD_BALANCING_POLICY, "random")
            proxyMap.addProxy(functionName, lbPolicyName, targets, weights)
        } else {
            proxyMap.updateProxy(functionName, targets, weights)
        }
    }

    // Wait for all goroutines to finish
    wg.Wait()
}

func sendPrewarmRequest(functionName string, nodeUrl string, nodeAllocation solver.NodeAllocationInfo) {
    request := client.PrewarmingRequest{
        Function:       functionName,
        CPUDemand:      nodeAllocation.ComputationalCapacity,
        Instances:      int64(nodeAllocation.PrewarmContainers),
        ForceImagePull: true,
    }
    prewarmingBody, err := json.Marshal(request)
    if err != nil {
        log.Printf("Error while creating JSON: %v", err)
        return
    }

    prewarmUrl := fmt.Sprintf("%s/prewarm", nodeUrl)
    log.Printf("[%s] Sending prewarm to %s for %v instances", functionName, prewarmUrl, nodeAllocation.PrewarmContainers)
    resp, err := utils.PostJson(prewarmUrl, prewarmingBody)
    if err != nil {
        log.Printf("[%s] Prewarming failed: %v", functionName, err)
        return
    }
    utils.PrintJsonResponse(resp.Body)
}

// getLBPolicy selects and returns a load balancing policy based on the policy type and proxy
func getLBPolicy(policyName string, lbProxy interface{}) LBPolicy {
	switch policyName {
	case "energyaware":
		// Check if lbProxy is of type LBWeightedProxy
		if weightedProxy, ok := lbProxy.(*LBWeightedProxy); ok {
			return newWeightedPolicy(weightedProxy)
		}
		return nil
	case "roundrobin":
 		// The round robin policy works with LBProxy
         if proxy, ok := lbProxy.(*LBProxy); ok {
			return newRoundRobinPolicy(proxy)
		}
		return nil   
    default:
		// The default policy works with LBProxy
		if proxy, ok := lbProxy.(*LBProxy); ok {
			return newRandomPolicy(proxy)
		}
		return nil
	}
}

// ------------------- PROXY MAP HANDLERS -------------------

// newFunctionProxyMap creares a new FunctionProxyMap
func newFunctionProxyMap() FunctionProxyMap {
	return make(FunctionProxyMap)
}

// createLBProxy initializes a Proxy based on the policyName
func createLBProxy(lbPolicyName string, targets []*url.URL, weights []int) Proxy {
	var proxy Proxy

    switch lbPolicyName {
    case "energyaware":
        totalWeight := 0
        for _, value := range weights {
            totalWeight += value
        }

        proxy = &LBWeightedProxy{
            targets:     targets,
            weights:     weights,
            totalWeight: totalWeight,
        }
    default:
        proxy = &LBProxy{
            targets: targets,
        }
    }

    // Update lbPolicy using the created proxy
    proxy.setPolicy(getLBPolicy(lbPolicyName, proxy))

    return proxy
}

// addProxy adds a new proxy with load balancing capabilities for a specific function
func (proxyMap FunctionProxyMap) addProxy(proxyIdentifier string, lbPolicyName string, targets []*url.URL, weights []int) {
	if len(targets) == 0 {
		return
	}

	log.Printf("Adding a new lb proxy with identifier '%s' and policy '%s'", proxyIdentifier, lbPolicyName)
	proxyMap[proxyIdentifier] = createLBProxy(lbPolicyName, targets, weights)

	for i, target := range targets {
		log.Printf("[%s] Added target %d: %v", proxyIdentifier, i, target)
	}
}

// updateProxy updates an existing proxy 
func (proxyMap FunctionProxyMap) updateProxy(proxyIdentifier string, newTargets []*url.URL, newWeights []int) {
	log.Printf("Updating lb proxy with identifier '%s'", proxyIdentifier)
	lbProxy := proxyMap[proxyIdentifier]

	//log.Printf("Previous targets: %v", lbProxy.getTargets())
	lbProxy.setTargets(newTargets)
	//log.Printf("Targets updated: %v", lbProxy.getTargets())

	if weightedProxy, ok := lbProxy.(*LBWeightedProxy); ok {
		//log.Printf("Previous weights: %v", weightedProxy.weights)
		
		totalWeight := 0
		for _, value := range newWeights {
			totalWeight += value
		}

		weightedProxy.weights = newWeights
		weightedProxy.totalWeight = totalWeight

		//log.Printf("Weights updated: %v", weightedProxy.weights)
		//log.Printf("Total weight updated: %d", weightedProxy.totalWeight)
	}
}

func (proxyMap FunctionProxyMap) deleteProxy(proxyIdentifier string) {
    delete(proxyMap, proxyIdentifier)
}

// getNextTarget per una specifica funzione
func (proxyMap FunctionProxyMap) getNextTarget(proxyIdentifier string) *url.URL {
	lbProxy, exists := proxyMap[proxyIdentifier]
	if !exists {
		return nil
	}

	return lbProxy.getPolicy().selectTarget()
}
