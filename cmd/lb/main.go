package main

import (
	"fmt"
	"log"
	"os"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/lb"
	"github.com/grussorusso/serverledge/internal/registration"
	"github.com/grussorusso/serverledge/utils"
)

func main() {
	configFileName := ""
	if len(os.Args) > 1 {
		configFileName = os.Args[1]
	}
	config.ReadConfiguration(configFileName)

	// TODO: split Area in Region + Type (e.g., cloud/lb/edge)
	region := config.GetString(config.REGISTRY_AREA, "ROME")
	registry := &registration.Registry{Area: "lb/" + region}
	hostport := fmt.Sprintf("http://%s:%d", utils.GetIpAddress().String(), config.GetInt(config.API_PORT, 1323))
	if _, err := registry.RegisterToEtcd(hostport); err != nil {
		log.Printf("Could not register to Etcd: %v", err)
	}

	log.Println("Load Balancer registered:", hostport)
	lb.StartReverseProxy(registry, region)
}
