package converter

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/healthexporter/collector"
	"github.com/leptonai/gpud/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CSVFiles represents the collection of CSV files that can be generated
type CSVFiles struct {
	MetricsFile     string
	EventsFile      string
	ComponentsFile  string
	MachineInfoFile string
}

// CSVConverter defines the interface for converting health data to CSV format
type CSVConverter interface {
	Convert(data *collector.HealthData, outputDir, timestamp string) (*CSVFiles, error)
}

// csvConverter implements the CSVConverter interface
type csvConverter struct{}

// NewCSVConverter creates a new CSV converter
func NewCSVConverter() CSVConverter {
	return &csvConverter{}
}

// Convert converts health data to CSV format and writes to files
func (c *csvConverter) Convert(data *collector.HealthData, outputDir, timestamp string) (*CSVFiles, error) {
	files := &CSVFiles{}

	// Write metrics CSV
	if len(data.Metrics) > 0 {
		filename := fmt.Sprintf("gpuhealth_metrics_%s.csv", timestamp)
		files.MetricsFile = filename
		if err := c.writeMetricsCSV(outputDir, filename, data); err != nil {
			return nil, fmt.Errorf("failed to write metrics CSV: %w", err)
		}
	}

	// Write events CSV
	if len(data.Events) > 0 {
		filename := fmt.Sprintf("gpuhealth_events_%s.csv", timestamp)
		files.EventsFile = filename
		if err := c.writeEventsCSV(outputDir, filename, data); err != nil {
			return nil, fmt.Errorf("failed to write events CSV: %w", err)
		}
	}

	// Write component health CSV
	if len(data.ComponentData) > 0 {
		filename := fmt.Sprintf("gpuhealth_component_health_%s.csv", timestamp)
		files.ComponentsFile = filename
		if err := c.writeComponentHealthCSV(outputDir, filename, data); err != nil {
			return nil, fmt.Errorf("failed to write component health CSV: %w", err)
		}
	}

	// Write machine info CSV
	if data.MachineInfo != nil {
		filename := fmt.Sprintf("gpuhealth_machine_info_%s.csv", timestamp)
		files.MachineInfoFile = filename
		if err := c.writeMachineInfoCSV(outputDir, filename, data); err != nil {
			return nil, fmt.Errorf("failed to write machine info CSV: %w", err)
		}
	}

	return files, nil
}

// writeMetricsCSV writes metrics data to CSV format
func (c *csvConverter) writeMetricsCSV(outputDir, filename string, data *collector.HealthData) error {
	fullPath := filepath.Join(outputDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"timestamp", "component", "metric_name", "value", "labels"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write metrics data
	for _, metric := range data.Metrics {
		// Convert labels map to JSON string for CSV storage
		labelsJSON := ""
		if len(metric.Labels) > 0 {
			if labelsBytes, err := json.Marshal(metric.Labels); err == nil {
				labelsJSON = string(labelsBytes)
			}
		}

		// Convert Unix milliseconds to readable timestamp
		timestamp := time.Unix(metric.UnixMilliseconds/1000, (metric.UnixMilliseconds%1000)*1000000).UTC().Format(time.RFC3339)

		record := []string{
			timestamp,
			metric.Component,
			metric.Name,
			fmt.Sprintf("%v", metric.Value),
			labelsJSON,
		}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return nil
}

// writeEventsCSV writes events data to CSV format
func (c *csvConverter) writeEventsCSV(outputDir, filename string, data *collector.HealthData) error {
	fullPath := filepath.Join(outputDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"timestamp", "component", "event_name", "event_type", "message"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write events data
	for _, event := range data.Events {
		// Format event timestamp
		timestamp := event.Time.Format(time.RFC3339)

		record := []string{
			timestamp,
			event.Component,
			event.Name,
			event.Type,
			event.Message,
		}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return nil
}

// writeComponentHealthCSV writes component health data to CSV format
func (c *csvConverter) writeComponentHealthCSV(outputDir, filename string, data *collector.HealthData) error {
	fullPath := filepath.Join(outputDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"timestamp", "component_name", "health", "reason"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write component health data
	for componentName, componentResult := range data.ComponentData {
		componentInfo, ok := componentResult.(map[string]any)
		if !ok {
			log.Logger.Warnw("Unexpected component data format for CSV export", "component", componentName)
			continue
		}

		// Extract values safely, handling nil cases
		timestampStr := "N/A"
		if timeValue := componentInfo["time"]; timeValue != nil {
			// Handle different timestamp formats (metav1.Time, time.Time, etc.)
			switch t := timeValue.(type) {
			case time.Time:
				// Check for zero time (uninitialized timestamp)
				if !t.IsZero() {
					timestampStr = t.Format(time.RFC3339)
				}
			default:
				// Handle metav1.Time and other time types by extracting the underlying time.Time
				// metav1.Time has a Time field that contains the actual time.Time
				if timeVal := extractTimeFromInterface(t); timeVal != nil && !timeVal.IsZero() {
					timestampStr = timeVal.Format(time.RFC3339)
				} else {
					// Fallback: try to parse as string for other formats
					timeStr := fmt.Sprintf("%v", t)
					// Don't include the problematic zero timestamp
					if !strings.Contains(timeStr, "0001-01-01") {
						timestampStr = timeStr
					}
				}
			}
		}

		componentNameStr := ""
		if name := componentInfo["component_name"]; name != nil {
			componentNameStr = fmt.Sprintf("%v", name)
		}

		healthStr := ""
		if health := componentInfo["health"]; health != nil {
			healthStr = fmt.Sprintf("%v", health)
		}

		reasonStr := ""
		if reason := componentInfo["reason"]; reason != nil {
			reasonStr = fmt.Sprintf("%v", reason)
		}

		record := []string{timestampStr, componentNameStr, healthStr, reasonStr}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return nil
}

// writeMachineInfoCSV writes machine info to CSV format using the same structure as RenderTable
func (c *csvConverter) writeMachineInfoCSV(outputDir, filename string, data *collector.HealthData) error {
	fullPath := filepath.Join(outputDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header - same format as machine-info command table
	header := []string{"attribute_name", "attribute_value"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Extract machine info attributes following the same structure as RenderTable method
	i := data.MachineInfo
	if i == nil {
		return nil
	}

	// Basic system info
	c.writeCSVRecord(writer, "GPU Health Version", i.GPUHealthVersion)
	c.writeCSVRecord(writer, "Container Runtime Version", i.ContainerRuntimeVersion)
	c.writeCSVRecord(writer, "OS Image", i.OSImage)
	c.writeCSVRecord(writer, "Kernel Version", i.KernelVersion)

	// CPU info
	if i.CPUInfo != nil {
		c.writeCSVRecord(writer, "CPU Type", i.CPUInfo.Type)
		c.writeCSVRecord(writer, "CPU Manufacturer", i.CPUInfo.Manufacturer)
		c.writeCSVRecord(writer, "CPU Architecture", i.CPUInfo.Architecture)
		c.writeCSVRecord(writer, "CPU Logical Cores", fmt.Sprintf("%d", i.CPUInfo.LogicalCores))
	}

	// Memory info
	if i.MemoryInfo != nil {
		c.writeCSVRecord(writer, "Memory Total", fmt.Sprintf("%d", i.MemoryInfo.TotalBytes))
	}

	// GPU info
	c.writeCSVRecord(writer, "CUDA Version", i.CUDAVersion)
	c.writeCSVRecord(writer, "VBIOS Version", i.VBIOSVersion)
	if i.GPUInfo != nil {
		c.writeCSVRecord(writer, "GPU Driver Version", i.GPUDriverVersion)
		c.writeCSVRecord(writer, "GPU Product", i.GPUInfo.Product)
		c.writeCSVRecord(writer, "GPU Manufacturer", i.GPUInfo.Manufacturer)
		c.writeCSVRecord(writer, "GPU Architecture", i.GPUInfo.Architecture)
		c.writeCSVRecord(writer, "GPU Memory", i.GPUInfo.Memory)
	}

	// Location info
	if i.Location != nil {
		c.writeCSVRecord(writer, "Location Country", i.Location.Country)
		c.writeCSVRecord(writer, "Location Country Code", i.Location.CountryCode)
		c.writeCSVRecord(writer, "Location City", i.Location.City)
		c.writeCSVRecord(writer, "Location Region", i.Location.Region)
		c.writeCSVRecord(writer, "Location Zone", i.Location.Zone)
		c.writeCSVRecord(writer, "Location Latitude", fmt.Sprintf("%.6f", i.Location.Latitude))
		c.writeCSVRecord(writer, "Location Longitude", fmt.Sprintf("%.6f", i.Location.Longitude))
		c.writeCSVRecord(writer, "Location Timezone", i.Location.Timezone)
		c.writeCSVRecord(writer, "Location Source", i.Location.Source)
	}

	// Network info
	if i.NICInfo != nil {
		for idx, nic := range i.NICInfo.PrivateIPInterfaces {
			c.writeCSVRecord(writer, fmt.Sprintf("Private IP Interface %d", idx+1),
				fmt.Sprintf("%s (%s, %s)", nic.Interface, nic.MAC, nic.IP))
		}
	}

	// Disk info
	if i.DiskInfo != nil {
		c.writeCSVRecord(writer, "Container Root Disk", i.DiskInfo.ContainerRootDisk)

		// Write block devices info
		for idx, device := range i.DiskInfo.BlockDevices {
			prefix := fmt.Sprintf("Block Device %d", idx+1)
			c.writeCSVRecord(writer, prefix+" Name", device.Name)
			c.writeCSVRecord(writer, prefix+" Type", device.Type)
			c.writeCSVRecord(writer, prefix+" Size", fmt.Sprintf("%d", device.Size))
			c.writeCSVRecord(writer, prefix+" Used", fmt.Sprintf("%d", device.Used))
			c.writeCSVRecord(writer, prefix+" Mount Point", device.MountPoint)
			c.writeCSVRecord(writer, prefix+" FS Type", device.FSType)
			if device.Serial != "" {
				c.writeCSVRecord(writer, prefix+" Serial", device.Serial)
			}
			if device.Model != "" {
				c.writeCSVRecord(writer, prefix+" Model", device.Model)
			}
		}
	}

	// GPU instances (individual GPUs)
	if i.GPUInfo != nil && len(i.GPUInfo.GPUs) > 0 {
		for idx, gpu := range i.GPUInfo.GPUs {
			prefix := fmt.Sprintf("GPU %d", idx+1)
			c.writeCSVRecord(writer, prefix+" UUID", gpu.UUID)
			c.writeCSVRecord(writer, prefix+" Bus ID", gpu.BusID)
			c.writeCSVRecord(writer, prefix+" SN", gpu.SN)
			c.writeCSVRecord(writer, prefix+" Minor ID", gpu.MinorID)
			c.writeCSVRecord(writer, prefix+" Board ID", fmt.Sprintf("%d", gpu.BoardID))
		}
	}

	return nil
}

// writeCSVRecord is a helper function to write a CSV record
func (c *csvConverter) writeCSVRecord(writer *csv.Writer, name, value string) error {
	record := []string{name, value}
	return writer.Write(record)
}

// extractTimeFromInterface extracts time.Time from various time types including metav1.Time
func extractTimeFromInterface(timeValue any) *time.Time {
	// Handle metav1.Time specifically
	if metaTime, ok := timeValue.(metav1.Time); ok {
		return &metaTime.Time
	}

	// Use reflection to check if the value has a Time field (like metav1.Time)
	val := reflect.ValueOf(timeValue)
	if val.Kind() == reflect.Struct {
		timeField := val.FieldByName("Time")
		if timeField.IsValid() && timeField.Type() == reflect.TypeOf(time.Time{}) {
			timeVal := timeField.Interface().(time.Time)
			return &timeVal
		}
	}

	// If it's already a time.Time pointer
	if timePtr, ok := timeValue.(*time.Time); ok {
		return timePtr
	}

	return nil
}
