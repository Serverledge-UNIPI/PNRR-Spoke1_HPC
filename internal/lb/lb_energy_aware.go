package lb

import (
	"log"
	"fmt"
	"time"
	"errors"
	"sync"
	"strings"
	"bytes"
	
	"net/http"
	"net/url"
	"net"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/solver"
	"github.com/grussorusso/serverledge/internal/registration"
	
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type CustomTransport struct {
	Transport http.RoundTripper
}

// Custom structure for recording the response
type responseRecorder struct {
    http.ResponseWriter
    buffer bytes.Buffer
    status int
}

type EnergyAwareProxyServer struct{}

var (
    currentDefaultCloudTargets = []*middleware.ProxyTarget{}	// List of default cloud targets (updated at regular intervals)
    currentDefaultEdgeTargets  = []*middleware.ProxyTarget{}	// List of default edge targets (updated at regular intervals)
    targetsMutex               sync.Mutex
)

var currentBalancerTargets = []*middleware.ProxyTarget{}	// Current active targets in the balancer
var balancer middleware.ProxyBalancer

func (energyAware *EnergyAwareProxyServer) newBalancer(targets []*middleware.ProxyTarget) middleware.ProxyBalancer {
	return middleware.NewRoundRobinBalancer(targets)
}

func (energyAware *EnergyAwareProxyServer) StartReverseProxy(e *echo.Echo, region string) {
	registry := &registration.Registry{Area: region}

	if err := updateDefaultCloudTargets(region); err != nil {
		log.Printf("Cannot update cloud default targets: %v\n", err)
	}

	if err := updateDefaultEdgeTargets(registry); err != nil {
		log.Printf("Cannot update edge default targets: %v\n", err)
	}
	
	balancer = energyAware.newBalancer(currentBalancerTargets)
	
	e.Use(responseMiddleware)
	e.Use(dynamicTargetMiddleware(registry))
	e.Use(middleware.ProxyWithConfig(
		middleware.ProxyConfig{
			Balancer:	balancer,
			Transport:	&CustomTransport{
				Transport: http.DefaultTransport,
			},
		},
	))
	
	go solver.Init()
	go updateDefaultTargets(registry, region)

	portNumber := config.GetInt(config.API_PORT, 1323)
	if err := e.Start(fmt.Sprintf(":%d", portNumber)); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.Logger.Fatal("Shutting down the server")
	}
}

func splitReqUrl(reqUrl string) (string, string, string, error) {
	parsedURL, err := url.Parse(reqUrl)
	if err != nil {
		log.Printf("Error parsing URL: %v\n", err)
		return "", "", "", err
	}

	ip, _, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		log.Printf("Error splitting host and port: %v\n", err)
		return "", "", "", err
	}

	// Remove prefix "/invoke/" from the URL
	tokens := strings.Split(parsedURL.Path, "/")
    reqType := tokens[1]

    if reqType == "invoke" {
        if len(tokens) < 2 {
            log.Printf("Missing function name")
            return "", "", "", errors.New("Missing function name")
        }

        funcName := tokens[2]
        return ip, reqType, funcName, nil
    }

    // Return the request type only if not "invoke"
    return ip, reqType, "", nil
}

// Execute HTTP request and log additional information
func (c *CustomTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := c.Transport.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		log.Printf("Request to %s failed (%v): %v", req.URL, resp.StatusCode, err)
		return nil, err
	}
	log.Printf("Request to %s took %v with response status code %v", req.URL, duration, resp.StatusCode)

	// Decrement instances if invoke request succeeded
	ip, reqType, funcName, err := splitReqUrl(req.URL.String())
	if err != nil {
		log.Printf("Error while splitting %v", req.URL)
		return nil, err
	}

	if reqType == "invoke" && resp.StatusCode == 200 {
		allocation, found := solver.GetAllocationFromCache()
		if found {
			solver.DecrementInstances(allocation, funcName, ip)
		}
	}

	return resp, nil
}

// Write the data into the buffer and into the ResponseWriter
func (rec *responseRecorder) Write(b []byte) (int, error) {
    rec.buffer.Write(b)
    return rec.ResponseWriter.Write(b)
}

// Store the status of the response
func (rec *responseRecorder) WriteHeader(statusCode int) {
    rec.status = statusCode
    rec.ResponseWriter.WriteHeader(statusCode)
}

// Middleware for logging response information
func responseMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
        // Create a custom recorder for the response
		rec := &responseRecorder{
			ResponseWriter: c.Response().Writer,
		}
		c.Response().Writer = rec

		err := next(c)
		if err != nil {
			// Log the error with details
			if httpError, ok := err.(*echo.HTTPError); ok {
				log.Printf("Error: Code=%d, Message=%s, RequestPath=%s, RequestMethod=%s",
					httpError.Code,
					httpError.Message,
					c.Request().URL.Path,
					c.Request().Method,
				)
			} else {
				log.Printf("Error: %v, RequestPath=%s, RequestMethod=%s",
					err,
					c.Request().URL.Path,
					c.Request().Method,
				)
			}
			// Return a generic error response
			return echo.NewHTTPError(http.StatusInternalServerError, "An error occurred")
		}

		log.Printf("Response Status: %d, Response Body: %s, RequestPath=%s, RequestMethod=%s",
			rec.status,
			rec.buffer.String(),
			c.Request().URL.Path,
			c.Request().Method,
		)

		return nil
	}
}

func handleInvoke(funcName string, registry *registration.Registry) {
	// Retrieve the allocation from the cache
	functionsAllocation, found := solver.GetAllocationFromCache()
	if !found {
		log.Printf("Functions allocation not found in cache")
		resetBalancerTargets()
		return
	}

	// Retrieve the allocation for the specified function
	functionAllocation, ok := (*functionsAllocation)[funcName]
	if !ok {
		log.Printf("No allocation found for function %s\n", funcName)
		resetBalancerTargets()
		return
	}

	// Log the current allocation
	log.Printf("Current allocation for %s: %v\n", funcName, functionAllocation)

	// Create a list of targets for the load balancer
	var targets []*middleware.ProxyTarget
	for targetIP, nodeAllocationInfo := range functionAllocation.NodeAllocations {
		if nodeAllocationInfo.Instances > 0 {
			addr := fmt.Sprintf("http://%s:%d", targetIP, config.GetInt(config.API_PORT, 1323))
			parsedURL, err := url.Parse(addr)
			if err != nil {
				log.Printf("Error parsing URL %s: %v\n", addr, err)
				continue
			}
			targets = append(targets, &middleware.ProxyTarget{Name: addr, URL: parsedURL})
		}
	}

	updateBalancerTargets(targets)
}

// Function to reset the load balancer targets
func resetBalancerTargets() {
	if len(currentDefaultEdgeTargets) > 0 {
		updateBalancerTargets(currentDefaultEdgeTargets)
	} else {
		log.Printf("No edge nodes available")
		updateBalancerTargets([]*middleware.ProxyTarget{})
	}
}

func dynamicTargetMiddleware(registry *registration.Registry) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
			urlPath := c.Request().URL.Path
            tokens := strings.Split(urlPath, "/")
            if len(tokens) < 2 {
				log.Printf("Error while splitting %v", c.Request().URL.Path)
				// abort
				return next(c)
			}    
			reqType := tokens[1]
        
			if reqType == "invoke" {
				handleInvoke(tokens[2], registry)
			} else {
				// Other request type (to be managed by the cloud)
				updateBalancerTargets(currentDefaultCloudTargets)
			}

			log.Printf("Current targets (%d)", len(currentBalancerTargets))
			for i, target := range currentBalancerTargets {
				log.Printf("Target %d: URL = %s\n", i, target.URL)
			}

            return next(c)
        }
    }
}

// -------------------------- TARGETS HANDLER FUNCTIONS --------------------------

// Update balancer targets
func updateBalancerTargets(newTargets []*middleware.ProxyTarget) {
	toKeep := make([]bool, len(currentBalancerTargets))
	for i := range currentBalancerTargets {
		toKeep[i] = false
	}
	for _, t := range newTargets {
		toAdd := true
		for i, curr := range currentBalancerTargets {
			if curr.Name == t.Name {
				toKeep[i] = true
				toAdd = false
			}
		}
		if toAdd {
			if balancer.AddTarget(t) {
				log.Printf("Adding %s\n", t.Name)
			}
		}
	}

	toRemove := make([]string, 0)
	for i, curr := range currentBalancerTargets {
		if !toKeep[i] {
			toRemove = append(toRemove, curr.Name)
		}
	}
	for _, curr := range toRemove {
		if balancer.RemoveTarget(curr) {
			log.Printf("Removing %s\n", curr)
		}
	}

	currentBalancerTargets = newTargets
}

func updateDefaultCloudTargets(region string) error {
    // Update default cloud nodes
    cloudNodes, err := registration.GetCloudNodes(region)
    if err != nil {
        return err
    }

    cloudTargets := make([]*middleware.ProxyTarget, 0, len(cloudNodes))
    for _, addr := range cloudNodes {
        log.Printf("Found cloud server at: %v\n", addr)
        parsedUrl, err := url.Parse(addr)
        if err != nil {
            log.Printf("Error parsing address: %v\n", err)
            continue
        }
        cloudTargets = append(cloudTargets, &middleware.ProxyTarget{Name: addr, URL: parsedUrl})
    }

    targetsMutex.Lock()
    defer targetsMutex.Unlock()
    currentDefaultCloudTargets = cloudTargets

    return nil
}

func updateDefaultEdgeTargets(registry *registration.Registry) error {
    // Update default edge nodes
    edgeNodes, err := registry.GetAll(false)
    if err != nil {
        return err
    }

    edgeTargets := make([]*middleware.ProxyTarget, 0, len(edgeNodes))
    for _, addr := range edgeNodes {
        parsedUrl, err := url.Parse(addr)
        if err != nil {
            log.Printf("Error parsing address: %v\n", err)
            continue
        }
        edgeTargets = append(edgeTargets, &middleware.ProxyTarget{Name: addr, URL: parsedUrl})
    }

    targetsMutex.Lock()
    defer targetsMutex.Unlock()
    currentDefaultEdgeTargets = edgeTargets

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