package lb

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"
	"encoding/json"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/function"
	"github.com/grussorusso/serverledge/internal/registration"
	"github.com/labstack/echo/v4"
)

var proxyMap FunctionProxyMap

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
	// Select the target node
	functionName := strings.TrimPrefix(c.Request().RequestURI, "/invoke/")
	if functionName == c.Request().RequestURI {
		functionName = "cloud"
	}

	target := proxyMap.getNextTarget(functionName)
	if target == nil {
		return c.String(http.StatusInternalServerError, "No available targets")
	}
	log.Printf("Selected target: %v", target)

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

	if strings.HasPrefix(c.Request().RequestURI, "/invoke/") {
		// Check the status code
		if resp.StatusCode == http.StatusOK {
			var executionReport function.ExecutionReport

			// Decode the JSON into an ExecutionReport structure
			err = json.Unmarshal(body, &executionReport)
			if err != nil {
				log.Fatalf("Error while decoding JSON: %v", err)
			}

			// TODO: Write "DEADLINE FAILURES" metric based on the executionReport
		} else {
			// TODO: Write "FAILED INVOCATIONS" metric based on the executionReport
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
		proxyMap.addProxy("edge", "random", edgeTargets, []int{})
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