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
func appendToCSVUsage(filename, timestamp string, epoch int32, cpuAvg, memAvg float64) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	fileInfo, err := file.Stat()
	if fileInfo.Size() == 0 {
		header := []string{"Timestamp", "Epoch", "AverageCpuUsage", "AverageMemUsage"}
		if err := writer.Write(header); err != nil {
			return err
		}
	}

	record := []string{timestamp, fmt.Sprintf("%d", epoch), fmt.Sprintf("%f", cpuAvg), fmt.Sprintf("%f", memAvg)}
	return writer.Write(record)
}

// Function to append solver results to a CSV file
func appendToCSVSolver(filename, timestamp string, epoch int32, numActiveNodes int, solverFails int, systemPowerConsumption float64) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	fileInfo, err := file.Stat()
	if fileInfo.Size() == 0 {
		header := []string{"Timestamp", "Epoch", "ActiveNodes", "SolverFails", "SystemPowerConsumption"}
		if err := writer.Write(header); err != nil {
			return err
		}
	}

	record := []string{timestamp, fmt.Sprintf("%d", epoch), fmt.Sprintf("%d", numActiveNodes), fmt.Sprintf("%d", solverFails), fmt.Sprintf("%f", systemPowerConsumption)}
	return writer.Write(record)
}

// Function to append function results to a CSV file
func appendToCSVFunction(filename, timestamp string, epoch int, functionName string, executionTime float64, failed int) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	fileInfo, err := file.Stat()
	if fileInfo.Size() == 0 {
		header := []string{"Timestamp", "Epoch", "FunctionName", "ExecutionTime", "ExecutionFailed"}
		if err := writer.Write(header); err != nil {
			return err
		}
	}

	record := []string{timestamp, fmt.Sprintf("%d", epoch), functionName, fmt.Sprintf("%f", executionTime), fmt.Sprintf("%d", failed)}
	return writer.Write(record)
}

func ConnectToPrometheus() bool {
	client, err := api.NewClient(api.Config{
		Address: "http://172.16.5.198:9091",
	})
	if err != nil {
		log.Fatalf("Error creating Prometheus client: %v", err)
		return false
	}

	v1api = v1.NewAPI(client)
	return true
}

func RetryConnectToPrometheus(maxRetries int, retryInterval time.Duration) bool {
	for i := 0; i < maxRetries; i++ {
		log.Printf("Prometheus client is nil. Attempting to reconnect... (attempt %d of %d)", i + 1, maxRetries)
		ConnectToPrometheus()
		if v1api != nil {
			log.Println("Successfully connected to Prometheus")
			return true
		}
		time.Sleep(retryInterval) // Wait before the next attempt
	}

	log.Println("Failed to connect to Prometheus after multiple attempts")
	return false
}

func SaveNodeMetrics(epoch int32) {
	if v1api == nil {
		success := RetryConnectToPrometheus(3, 1*time.Second)
		if !success {
			return
		}
	}

	epochDuration := config.GetInt(config.EPOCH_DURATION, 15)

	cpuQuery := fmt.Sprintf(`avg(avg_over_time(sedge_node_cpu_usage[%dm])) by (instance)`, epochDuration)
	memQuery := fmt.Sprintf(`avg(avg_over_time(sedge_node_memory_usage[%dm])) by (instance)`, epochDuration)

	cpuUsage, err := queryPrometheus(cpuQuery, time.Duration(epochDuration)*time.Minute)
	if err != nil {
		fmt.Errorf("Error querying Prometheus for CPU usage: %v", err)
	}

	nodeCpuAverages := calculateAveragePerNode(cpuUsage)

	memoryUsage, err := queryPrometheus(memQuery, time.Duration(epochDuration)*time.Minute)
	if err != nil {
		fmt.Errorf("Error querying Prometheus for memory usage: %v", err)
	}

	nodeMemoryAverages := calculateAveragePerNode(memoryUsage)

	timestamp := time.Now().Format(time.RFC3339)
	for node, cpuAvg := range nodeCpuAverages {
		memAvg, exists := nodeMemoryAverages[node]
		if !exists {
			memAvg = 0
		}

		filename := fmt.Sprintf("exported_data/usage_%dmin_%s_node%s.csv", config.GetInt(config.EPOCH_DURATION, 15), config.GetString(config.SOLVER_TYPE, "milp"), strings.Split(node, ":")[0])
		if err := appendToCSVUsage(filename, timestamp, epoch, cpuAvg, memAvg); err != nil {
			log.Fatalf("Error writing to CSV for node %s: %v", node, err)
		}
	}
}

// Function to record solver metrics
func SaveSolverMetrics(activeNodes []int32, epoch int32, solverFails int, objectiveValue float64) {
	timestamp := time.Now().Format(time.RFC3339)

	var numActiveNodes int
	for _, value := range activeNodes {
		numActiveNodes += int(value) // Convert int32 to int
	}

	filename := fmt.Sprintf("exported_data/solver_%dmin_%s.csv", config.GetInt(config.EPOCH_DURATION, 15), config.GetString(config.SOLVER_TYPE, "milp"))
	if err := appendToCSVSolver(filename, timestamp, epoch, numActiveNodes, solverFails, objectiveValue); err != nil {
		log.Fatalf("Error writing to CSV: %v", err)
	}
}

// Function to record execution time
func RecordFunctionMetrics(epoch int, functionName string, executionTime float64, failed int) {
	timestamp := time.Now().Format(time.RFC3339)

	filename := fmt.Sprintf("exported_data/functions_execution_time_%dmin_%s.csv", config.GetInt(config.EPOCH_DURATION, 15), config.GetString(config.SOLVER_TYPE, "milp"))
	if err := appendToCSVFunction(filename, timestamp, epoch, functionName, executionTime, failed); err != nil {
		log.Fatalf("Error writing to CSV: %v", err)
	}
}