package inject

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
)

func Command(c *cli.Context) error {
	component := c.String("component")
	if component == "" {
		return fmt.Errorf("component name is required")
	}

	faultType := c.String("fault-type")
	if faultType == "" {
		faultType = "event"
	}

	faultMessage := c.String("fault-message")
	if faultMessage == "" {
		faultMessage = fmt.Sprintf("Injected fault for testing %s component", component)
	}

	// Get the server address
	address := c.String("address")
	if address == "" {
		address = fmt.Sprintf("http://localhost:%d", gpuhealthconfig.DefaultHealthPort)
	}

	// Create the injection request
	var requestBody map[string]interface{}
	switch faultType {
	case "component-error":
		requestBody = map[string]interface{}{
			"component_error": map[string]string{
				"component": component,
				"message":   faultMessage,
			},
		}

	case "event":
		requestBody = map[string]interface{}{
			"event": map[string]string{
				"component": component,
				"name":      component,
				"type":      "test",
				"message":   faultMessage,
			},
		}

	default:
		return fmt.Errorf("invalid fault type: %s", faultType)
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make the POST request to the inject-fault endpoint
	url := fmt.Sprintf("%s/inject-fault", address)
	fmt.Printf("Injecting fault into %s component at %s...\n", component, url)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to make request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error status %d", resp.StatusCode)
	}

	fmt.Printf("Successfully injected fault into %s component\n", component)
	return nil
}
