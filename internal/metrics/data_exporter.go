package metrics

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/grussorusso/serverledge/internal/config"
)

var v1api v1.API

func queryPrometheus(query string, duration time.Duration) (model.Value, error) {
	client := v1api
	end := time.Now()
	start := end.Add(-duration)

	if start.After(end) {
		return nil, fmt.Errorf("Invalid time range: start time %v is after end time %v", start, end)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, warnings, err := client.QueryRange(ctx, query, v1.Range{
		Start: start,
		End:   end,
		Step:  time.Second,
	})
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	return result, nil
}

// Function to execute a current query on Prometheus
func queryPrometheusCurrent(query string) (model.Value, error) {
	client := v1api
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, warnings, err := client.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	return result, nil
}

// Function to calculate the average cpu/memory usage per node
func calculateAveragePerNode(data model.Value) map[string]float64 {
	if data == nil {
		return nil
	}

	matrix, ok := data.(model.Matrix)
	if !ok {
		return nil
	}

	nodeAverages := make(map[string]float64)
	for _, stream := range matrix {
		node := string(stream.Metric["instance"]) 
		var sum float64
		var count int

		for _, point := range stream.Values {
			sum += float64(point.Value)
			count++
		}

		if count > 0 {
			nodeAverages[node] = sum / float64(count)
		}
	}

	return nodeAverages
}

// Function to append data to a CSV file for CPU and memory usage
func appendToCSV(filename, timestamp string, epoch int32, cpuAvg, memAvg float64) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	record := []string{timestamp, fmt.Sprintf("%d", epoch), fmt.Sprintf("%f", cpuAvg), fmt.Sprintf("%f", memAvg)}
	return writer.Write(record)
}

// Function to append failure deadline data to a CSV file
func appendToCSVFailures(filename, timestamp string, epoch int32, data map[string]float64) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for functionName, value := range data {
		record := []string{timestamp, functionName, fmt.Sprintf("%d", epoch), fmt.Sprintf("%f", value)}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// Function to append solver results to a CSV file
func appendToCSVSolver(filename, timestamp string, epoch int32, numActiveNodes int, solverFails int, systemPowerConsumption int32) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	record := []string{timestamp, fmt.Sprintf("%d", epoch), fmt.Sprintf("%d", numActiveNodes), fmt.Sprintf("%d", solverFails), fmt.Sprintf("%d", systemPowerConsumption)}
	return writer.Write(record)
}

// Function to append function results to a CSV file
func appendToCSVFunction(filename, timestamp string, epoch int32, deadlineFails int) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	record := []string{timestamp, fmt.Sprintf("%d", epoch), fmt.Sprintf("%d", deadlineFails)}
	return writer.Write(record)
}

func ConnectToPrometheus() bool {
	client, err := api.NewClient(api.Config{
		Address: "http://172.16.2.182:9091",
	})
	if err != nil {
		log.Fatalf("Error creating Prometheus client: %v", err)
		return false
	}

	v1api = v1.NewAPI(client)
	return true
}

func SaveMetrics(epoch int32) {
	if v1api == nil {
		log.Printf("Prometheus client is nil")
		return
	}

	epochDuration := config.GetInt(config.EPOCH_DURATION, 20)

	cpuQuery := fmt.Sprintf(`avg(avg_over_time(sedge_node_cpu_usage[%ds])) by (instance)`, epochDuration)
	memQuery := fmt.Sprintf(`avg(avg_over_time(sedge_node_memory_usage[%ds])) by (instance)`, epochDuration)
	deadlineQuery := `sedge_deadline_failures`

	cpuUsage, err := queryPrometheus(cpuQuery, time.Duration(epochDuration)*time.Second)
	if err != nil {
		fmt.Errorf("Error querying Prometheus for CPU: %v", err)
	}

	nodeCpuAverages := calculateAveragePerNode(cpuUsage)

	memoryUsage, err := queryPrometheus(memQuery, time.Duration(epochDuration)*time.Second)
	if err != nil {
		fmt.Errorf("Error querying Prometheus for Memory: %v", err)
	}

	nodeMemoryAverages := calculateAveragePerNode(memoryUsage)

	deadlineFailures, err := queryPrometheusCurrent(deadlineQuery)
	if err != nil {
		fmt.Errorf("Error querying Prometheus for Deadline Failures: %v", err)
	}

	timestamp := time.Now().Format(time.RFC3339)

	for node, cpuAvg := range nodeCpuAverages {
		memAvg, exists := nodeMemoryAverages[node]
		if !exists {
			memAvg = 0
		}

		filename := fmt.Sprintf("exported_data/usage_%dmin_%dfunctions_node%s.csv", config.GetInt(config.EPOCH_DURATION, 20), config.GetInt(config.REGISTERED_FUNCTIONS, 0), strings.Split(node, ":")[0])
		if err := appendToCSV(filename, timestamp, epoch, cpuAvg, memAvg); err != nil {
			log.Fatalf("Error writing to CSV for node %s: %v", node, err)
		}
	}

	totalFailures := make(map[string]float64)
	if vector, ok := deadlineFailures.(model.Vector); ok {
		for _, sample := range vector {
			functionName := string(sample.Metric["function"])
			totalFailures[functionName] = float64(sample.Value)
		}
	} else {
		//fmt.Println("Expected vector type")
		return
	}

	filename := fmt.Sprintf("exported_data/deadline_failures_%dmin_%dfunctions.csv", config.GetInt(config.EPOCH_DURATION, 20), config.GetInt(config.REGISTERED_FUNCTIONS, 0))
	if err := appendToCSVFailures(filename, timestamp, epoch, totalFailures); err != nil {
		fmt.Println("Error appending to CSV file:", err)
		return
	}
}

// Function to record solver metrics
func RecordSolverMetrics(activeNodes []int32, epoch int32, solverFails int, nodePowerConsumption []int) {
	timestamp := time.Now().Format(time.RFC3339)

	var numActiveNodes int
	var systemPowerConsumption int32
	for i, value := range activeNodes {
		numActiveNodes += int(value) // Convert int32 to int
		systemPowerConsumption += int32(nodePowerConsumption[i]) * value
	}

	filename := fmt.Sprintf("exported_data/solver_%dmin_%dfunctions.csv", config.GetInt(config.EPOCH_DURATION, 20), config.GetInt(config.REGISTERED_FUNCTIONS, 0))
	if err := appendToCSVSolver(filename, timestamp, epoch, numActiveNodes, solverFails, systemPowerConsumption); err != nil {
		log.Fatalf("Error writing to CSV: %v", err)
	}
}

// Function to record function metrics
func RecordFunctionMetrics(epoch int32, deadlineFails int) {
	timestamp := time.Now().Format(time.RFC3339)

	filename := fmt.Sprintf("exported_data/deadline_failures_%dmin_%dfunctions.csv", config.GetInt(config.EPOCH_DURATION, 20), config.GetInt(config.REGISTERED_FUNCTIONS, 0))
	if err := appendToCSVFunction(filename, timestamp, epoch, deadlineFails); err != nil {
		log.Fatalf("Error writing to CSV: %v", err)
	}
}