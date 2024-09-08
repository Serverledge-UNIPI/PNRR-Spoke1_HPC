package lb

import (
	"fmt"
	"log"
	"time"
	"strings"
	"sync"
	"os"
	"os/signal"
	"io/ioutil"
	"encoding/json"
	"net/http"
	"net/url"
	
	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/function"
	"github.com/grussorusso/serverledge/internal/registration"
	"github.com/grussorusso/serverledge/internal/solver"

	"github.com/grussorusso/serverledge/internal/metrics"

	"github.com/labstack/echo/v4"
	"context"
)

// proxyMap can contain both proxies associated with specific functions and default proxies
var proxyMap FunctionProxyMap

var (
    localAllocation solver.SystemFunctionsAllocation
    mu sync.Mutex
)

func registerTerminationHandler(r *registration.Registry, e *echo.Echo) {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)

	go func() {
		select {
		case sig := <-c:
			fmt.Printf("Got %s signal. Terminating...\n", sig)

			// deregister from etcd; server should be unreachable
			err := r.Deregister()
			if err != nil {
				log.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := e.Shutdown(ctx); err != nil {
				e.Logger.Fatal(err)
			}

			os.Exit(0)
		}
	}()
}

func handleRequest(c echo.Context) error {
    var proxyIdentifier string
    
    // Get the request URI
    requestURI := c.Request().RequestURI

    if strings.HasPrefix(requestURI, "/invoke/") {
		// Check if allocation is in the local cache
		mu.Lock()
		if len(localAllocation) == 0 || localAllocation == nil {
            // Use default edge nodes if allocation is not found
            proxyIdentifier = "edge"
			log.Printf("Allocation empty or not found")
        } else {
			// Extract the function name by removing the "/invoke/" prefix
            proxyIdentifier = strings.TrimPrefix(requestURI, "/invoke/")
			log.Printf("Allocation found")
		}
		mu.Unlock()
    } else {
        // Use default cloud nodes if the request is not an invoke one
        proxyIdentifier = "cloud"
    }
			
	// Select the target node
	target := proxyMap.getNextTarget(proxyIdentifier)
	if target == nil {
		log.Printf("[%s] No available targets", proxyIdentifier)
		return fmt.Errorf("[%s] No available targets", proxyIdentifier)
	}
	log.Printf("[%s] Selected target: %v", proxyIdentifier, target)

	// Create an HTTP client to forward the request to the backend and create a new request
	client := &http.Client{}
	req, err := http.NewRequest(c.Request().Method, target.String() + c.Request().RequestURI, c.Request().Body)
	if err != nil {
		return err
	}
	// Copy request headers
	req.Header = c.Request().Header

	// Send the request to the backend
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error while reading response body: %v", err)
	}

	if strings.HasPrefix(requestURI, "/invoke/") {
		functionName := strings.TrimPrefix(requestURI, "/invoke/")
		
		// Check the status code
		if resp.StatusCode == http.StatusOK {
			var executionReport function.ExecutionReport

			// Decode the JSON into an ExecutionReport structure
			err = json.Unmarshal(body, &executionReport)
			if err != nil {
				log.Fatalf("[%s] Error while decoding JSON: %v", functionName, err)
			}
			
			// Record function execution time metric based on the executionReport
			if metrics.Enabled {
				metrics.RecordFunctionMetrics(epoch, functionName, executionReport.Duration, 0)
			}
		} else {
			// Record failed invocations
			if metrics.Enabled {
				metrics.RecordFunctionMetrics(epoch, functionName, 0, 1)
			}
		}
	}

	// Generate the response for the client using the information from the backend response
	for k, v := range resp.Header {
		c.Response().Header().Set(k, v[0])
	}
	c.Response().WriteHeader(resp.StatusCode)
	_, err = c.Response().Writer.Write(body)

	return err
}

// StartReverseProxy initializes and starts a reverse proxy server with load balancing capabilities
func StartReverseProxy(r *registration.Registry, region string) {
	registry := &registration.Registry{Area: region}

	go metrics.Init()

	proxyMap = newFunctionProxyMap()
	if err := updateDefaultCloudTargets(region); err != nil {
		log.Printf("Cannot update cloud default targets: %v\n", err)
	}

	if err := updateDefaultEdgeTargets(registry); err != nil {
		log.Printf("Cannot update edge default targets: %v\n", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.Any("/*", handleRequest)
	
	registerTerminationHandler(r, e)

	// Start the etcd watcher to get allocation updates
	go watchFunctionsAllocation()
	
	// Periodically retrieve the available default targets
	go updateDefaultTargets(registry, region)

	portNumber := config.GetInt(config.API_PORT, 1323)
	log.Printf("Starting reverse proxy server on port %d", portNumber)
	if err := e.Start(fmt.Sprintf(":%d", portNumber)); err != nil && err != http.ErrServerClosed {
		e.Logger.Fatal("Shutting down the server")
	}
}

// -------------------------- TARGETS HANDLERS --------------------------

func updateDefaultCloudTargets(region string) error {
    // Update default cloud nodes
    cloudNodes, err := registration.GetCloudNodes(region)
    if err != nil {
        return err
    }

    cloudTargets := make([]*url.URL, 0, len(cloudNodes))
    for _, addr := range cloudNodes {
        log.Printf("Found cloud server at: %v\n", addr)
        parsedUrl, err := url.Parse(addr)
        if err != nil {
            log.Printf("Error parsing address: %v\n", err)
            continue
        }
        cloudTargets = append(cloudTargets, parsedUrl)
    }

	// Add/update proxy map for cloud requests
	_, exists := proxyMap["cloud"]
	if !exists {
		proxyMap.addProxy("cloud", "random", cloudTargets, []int{})
	} else {
		if len(cloudTargets) != 0 {
			proxyMap.updateProxy("cloud", cloudTargets, []int{})
		} else {
			proxyMap.deleteProxy("cloud")
		}
	}

    return nil
}

func updateDefaultEdgeTargets(registry *registration.Registry) error {
    // Update default edge nodes
    edgeNodes, err := registry.GetAll(false)
    if err != nil {
        return err
    }

    edgeTargets := make([]*url.URL, 0, len(edgeNodes))
    for _, addr := range edgeNodes {
        parsedUrl, err := url.Parse(addr)
        if err != nil {
            log.Printf("Error parsing address: %v\n", err)
            continue
        }
        edgeTargets = append(edgeTargets, parsedUrl)
    }

	// Add/update proxy map
	_, exists := proxyMap["edge"]
	if !exists {
		proxyMap.addProxy("edge", "roundrobin", edgeTargets, []int{})
	} else {
		if len(edgeTargets) != 0 {
			proxyMap.updateProxy("edge", edgeTargets, []int{})
		} else {
			proxyMap.deleteProxy("edge")
		}
	}

    return nil
}

// Update default targets data structures
func updateDefaultTargets(registry *registration.Registry, region string) {
	checkInterval := config.GetInt(config.LB_CHECK_INTERVAL, 30)
	checkTicker := time.NewTicker(time.Duration(checkInterval) * time.Second)
	defer checkTicker.Stop()

    for {
        select {
        case <-checkTicker.C:
            if err := updateDefaultCloudTargets(region); err != nil {
                log.Printf("Cannot update cloud default targets: %v\n", err)
            }

            if err := updateDefaultEdgeTargets(registry); err != nil {
                log.Printf("Cannot update edge default targets: %v\n", err)
            }
        }
    }
}