package lb

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"bytes"
	"net" 
	"net/http"
	"net/url"

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

var resetTargets bool = false

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
	
	e.Use(responseMiddleware)
	e.Use(dynamicTargetMiddleware(balancer, registry))
	e.Use(middleware.ProxyWithConfig(
		middleware.ProxyConfig{
			Balancer:    balancer,
			Transport: &CustomTransport{
				Transport: http.DefaultTransport,
			},
		},
	))
	
	go solver.WatchAllocation()

	portNumber := config.GetInt(config.API_PORT, 1323)
	if err := e.Start(fmt.Sprintf(":%d", portNumber)); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.Logger.Fatal("Shutting down the server")
	}
}

func splitReqUrl(reqUrl string) (string, string, error) {
	parsedURL, err := url.Parse(reqUrl)
	if err != nil {
		log.Printf("Error parsing URL: %v\n", err)
		return "", "", err
	}

	ip, _, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		log.Printf("Error splitting host and port: %v\n", err)
		return "", "", err
	}

	// Remove prefix "/invoke/" from the URL
	tokens := strings.Split(parsedURL.Path, "/")
	if len(tokens) <= 2 {
		log.Printf("Invalid function name")
		return "", "", errors.New("Invalid function name")
	}
	funcName := tokens[2]
	
	return ip, funcName, nil
}

// Executes HTTP request and log additional information
func (c *CustomTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := c.Transport.RoundTrip(req)
	duration := time.Since(start)

	ip, funcName, err := splitReqUrl(req.URL.String())
	if err != nil {
		log.Printf("Error while extracting ip address from %v", req.URL)
		return nil, err
	}

	if err != nil {
		log.Printf("Request to %s failed: %v", req.URL, err)
		return nil, err
	}
	log.Printf("Request to %s took %v", req.URL, duration)

	// Decrement instances if request succeeded
	if len(solver.Allocation) != 0 {
		solver.DecrementInstances(funcName, ip)
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

func dynamicTargetMiddleware(balancer middleware.ProxyBalancer, registry *registration.Registry) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
			// Remove prefix "/invoke/" from the URL
            urlPath := c.Request().URL.Path
            tokens := strings.Split(urlPath, "/")
            if len(tokens) > 2 {
                funcName := tokens[2]
                log.Printf("Request for function: %s\n", funcName)

                functionAllocation, ok := solver.Allocation[funcName]
                if !ok {
					log.Printf("No allocation found for function %s\n", funcName)

					// Reset targets
					isAllocationEmpty := (len(solver.Allocation) == 0)
					if resetTargets != isAllocationEmpty {
						targets, err := getEdgeTargets(registry)
						if err != nil {
							log.Printf("Cannot connect to registry to retrieve targets: %v\n", err)
							os.Exit(2)
						}
						
						if len(targets) != 0 {
							updateEdgeTargets(balancer, targets)
							log.Printf("Updated targets for %s\n", funcName)
							resetTargets = isAllocationEmpty
						}
					}

                    return next(c)
                }
				log.Printf("Current allocation for %s: %v\n", funcName, solver.Allocation[funcName])

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

// -------------------------- TARGET FUNCTIONS --------------------------

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