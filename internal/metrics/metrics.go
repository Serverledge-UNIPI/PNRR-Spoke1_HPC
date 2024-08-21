package metrics

import (
	"log"
	"fmt"
	"time"

	"net/http"

	"github.com/grussorusso/serverledge/internal/config"
	"github.com/grussorusso/serverledge/internal/node"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

var Enabled bool
var registry = prometheus.NewRegistry()
var nodeIdentifier string

func Init() {
	if config.GetBool(config.METRICS_ENABLED, false) {
		log.Println("Metrics enabled.")
		Enabled = true
	} else {
		Enabled = false
		return
	}

	nodeIdentifier = node.NodeIdentifier
	registerGlobalMetrics()
	RecordNodeMetrics()

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true})
	http.Handle("/metrics", handler)
	err := http.ListenAndServe(":2112", nil)
	if err != nil {
		log.Printf("Listen and serve terminated with error: %s\n", err)
		return
	}
}

// Global metrics
var (
	CompletedInvocations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sedge_completed_total",
		Help: "The total number of completed function invocations",
	}, []string{"node", "function"})
	ExecutionTimes = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sedge_exectime",
		Help:    "Function duration",
		Buckets: durationBuckets,
	}, []string{"node", "function"})
)

// TODO: Node metrics
var (
    CpuUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sedge_node_cpu_usage",
			Help: "Total CPU usage",
		}, 
		[]string{"node"},
	)
	MemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sedge_node_memory_usage",
			Help: "Total memory usage",
		}, 
		[]string{"node"},
	)
	PowerConsumption = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sedge_node_power_usage",
			Help: "Total power consumption",
		}, 
		[]string{"node"},
	)
)
// --------------------------

// TODO: Solver metrics
var (
	SolverFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sedge_solver_failures",
			Help: "The total number of solver failures",
		}, 
		[]string{"epoch"},
	)
	NodesStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "sedge_node_status",
			Help: "Indicates whether a node should be on or off for a specific epoch",
		}, 
		[]string{"node", "epoch"},
	)
)
// --------------------------

// TODO: Function metrics
var DeadlineFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
		Name: "sedge_deadline_failures",
		Help: "The total number of function deadline failures",
	}, 
	[]string{"function"},
)
// --------------------------

var durationBuckets = []float64{0.002, 0.005, 0.010, 0.02, 0.03, 0.05, 0.1, 0.15, 0.3, 0.6, 1.0}

func AddCompletedInvocation(funcName string) {
	CompletedInvocations.With(prometheus.Labels{"function": funcName, "node": nodeIdentifier}).Inc()
}
func AddFunctionDurationValue(funcName string, duration float64) {
	ExecutionTimes.With(prometheus.Labels{"function": funcName, "node": nodeIdentifier}).Observe(duration)
}

// TODO
func AddNodeUsage(cpuUsage float64, memUsage float64) {
	CpuUsage.WithLabelValues(nodeIdentifier).Set(cpuUsage)
	MemoryUsage.WithLabelValues(nodeIdentifier).Set(memUsage)
}

func AddSolverFailure(epoch int32) {
	epochStr := fmt.Sprintf("%d", epoch)
	SolverFailures.WithLabelValues(epochStr).Inc()
}

func AddNodesStatus(activeNodes []int32, nodeIp []string, epoch int32) {
	epochStr := fmt.Sprintf("%d", epoch)
	for i, value := range activeNodes {
		log.Printf("[Epoch %d] Node with IP %s is set to %b", epoch, nodeIp[i], value)
		NodesStatus.WithLabelValues(nodeIp[i], epochStr).Set(float64(value))
	}
}

func AddDeadlineFailures(funcName string) {
	DeadlineFailures.WithLabelValues(funcName).Inc()
}
// --------------------------

func registerGlobalMetrics() {
	registry.MustRegister(CompletedInvocations)
	registry.MustRegister(ExecutionTimes)

	registry.MustRegister(SolverFailures)
	registry.MustRegister(NodesStatus)

	registry.MustRegister(CpuUsage)
	registry.MustRegister(MemoryUsage)
	registry.MustRegister(PowerConsumption)

	registry.MustRegister(DeadlineFailures)
}

// TODO
func RecordNodeMetrics() {
    go func() {
        for {
			cpuUsage, memUsage := GetResourcesUsage()
			AddNodeUsage(cpuUsage, memUsage)
            time.Sleep(10 * time.Second)
        }
    }()
}

func GetResourcesUsage() (float64, float64) {
	cpuPercent, err := cpu.Percent(0, false)
    if err != nil {
		log.Printf("Error in retrieving CPU information: %v\n", err)
        return 0.0, 0.0
    }
	
	log.Printf("CPU usage: %f%%\n", cpuPercent[0])

	vMemInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Fatalf("Error in retrieving memory information: %v", err)
		return 0.0, 0.0
	}
	log.Printf("Memory usage: %f%%\n", vMemInfo.UsedPercent)

    return cpuPercent[0], vMemInfo.UsedPercent
}
// --------------------------
