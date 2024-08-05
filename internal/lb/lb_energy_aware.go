package lb

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/solver"
	"github.com/grussorusso/serverledge/internal/registration"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type EnergyAwareProxyServer struct{}

func (energyAware *EnergyAwareProxyServer) newBalancer(targets []*middleware.ProxyTarget) middleware.ProxyBalancer {
	return middleware.NewRoundRobinBalancer(targets)
}

func (energyAware *EnergyAwareProxyServer) StartReverseProxy(e *echo.Echo, region string) {
	registry := &registration.Registry{Area: region}
	targets, err := getEdgeTargets(registry)
	if err != nil {
		log.Printf("Cannot connect to registry to retrieve targets: %v\n", err)
		os.Exit(2)
	}
	log.Printf("Initializing with %d targets\n", len(targets))
	balancer := energyAware.newBalancer(targets)
	currentTargets = targets
	
	e.Use(responseLogging)
	e.Use(dynamicTargetMiddleware(balancer, registry))
	e.Use(middleware.Proxy(balancer))

	go solver.WatchAllocation()

	portNumber := config.GetInt(config.API_PORT, 1323)
	if err := e.Start(fmt.Sprintf(":%d", portNumber)); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.Logger.Fatal("Shutting down the server")
	}
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

func responseLogging(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        // Create a custom recorder for the response
        rec := &responseRecorder{
            ResponseWriter: c.Response().Writer,
        }
        c.Response().Writer = rec

        err := next(c)
        if err != nil {
            return err
        }

        log.Printf("Response Status: %d", rec.status)
        log.Printf("Response Body: %s", rec.buffer.String())

		// TODO: if ok, update instances
		//if rec.status == 200 {
		//}

        return nil
    }
}

func dynamicTargetMiddleware(balancer middleware.ProxyBalancer, registry *registration.Registry) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
			// Remove prefix "/invoke/" from the URL
            urlPath := c.Request().URL.Path
            tokens := strings.Split(urlPath, "/")
            if len(tokens) > 2 {
                funcName := tokens[2]
                log.Printf("Request for function: %s\n", funcName)

				// Get function allocation
                allocation := solver.GetAllocation()	
                functionAllocation, ok := allocation[funcName]
                if !ok {
					log.Printf("No targets found for function %s\n", funcName)
					// Reset targets
					targets, err := getEdgeTargets(registry)
					if err != nil {
						log.Printf("Cannot connect to registry to retrieve targets: %v\n", err)
						os.Exit(2)
					}
					updateEdgeTargets(balancer, targets)
					log.Printf("Updated targets for %s\n", funcName)
                    return next(c)
                }
				log.Printf("Current allocation for %s: %v\n", funcName, allocation[funcName])

				var targets []*middleware.ProxyTarget
                for targetIp := range functionAllocation.Instances {
					if functionAllocation.Instances[targetIp] != 0 {
						addr := fmt.Sprintf("http://%s:%d", targetIp, config.GetInt(config.API_PORT, 1323))
						parsedUrl, err := url.Parse(addr)
						if err != nil {
							fmt.Printf("parsedUrl error")
							continue
						}
						targets = append(targets, &middleware.ProxyTarget{Name: addr, URL: parsedUrl})
					}
                }

				updateEdgeTargets(balancer, targets)
				log.Printf("Updated targets for %s\n", funcName)
				for i, target := range currentTargets {
					log.Printf("Target %d: URL = %s\n", i, target.URL)
				}
            } else {
                log.Printf("Invalid function name")
            }

            return next(c)
        }
    }
}

func getEdgeTargets(registry *registration.Registry) ([]*middleware.ProxyTarget, error) {
	edgeNodes, err := registry.GetAll(false)
	if err != nil {
		return nil, err
	}

	targets := make([]*middleware.ProxyTarget, 0, len(edgeNodes))
	for _, addr := range edgeNodes {
		log.Printf("Found target: %v\n", addr)
		parsedUrl, err := url.Parse(addr)
		if err != nil {
			return nil, err
		}
		targets = append(targets, &middleware.ProxyTarget{Name: addr, URL: parsedUrl})
	}

	log.Printf("Found %d targets\n", len(targets))
	return targets, nil
}

func updateEdgeTargets(balancer middleware.ProxyBalancer, targets []*middleware.ProxyTarget) {
	toKeep := make([]bool, len(currentTargets))
	for i := range currentTargets {
		toKeep[i] = false
	}
	for _, t := range targets {
		toAdd := true
		for i, curr := range currentTargets {
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
	for i, curr := range currentTargets {
		if !toKeep[i] {
			toRemove = append(toRemove, curr.Name)
		}
	}
	for _, curr := range toRemove {
		if balancer.RemoveTarget(curr) {
			log.Printf("Removing %s\n", curr)
		}
	}

	currentTargets = targets
}
