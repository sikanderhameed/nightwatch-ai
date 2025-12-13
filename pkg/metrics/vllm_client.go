package metrics

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FetchVLLMQueueDepth fetches the current queue depth from vLLM metrics endpoint
func FetchVLLMQueueDepth(url string) (int, error) {
	// Create HTTP client with 5-second timeout (production resilience)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Make GET request to metrics endpoint
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch metrics from %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse Prometheus format to extract queue depth
	return parseQueueDepth(string(body))
}

// parseQueueDepth extracts queue depth value from Prometheus metrics text
// Looks for: vllm_num_requests_waiting{model_name="phi-3"} 15
func parseQueueDepth(metrics string) (int, error) {
	lines := strings.Split(metrics, "\n")

	for _, line := range lines {
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Look for the queue depth metric
		if strings.HasPrefix(line, "vllm_num_requests_waiting") {
			// Extract the number at the end of the line
			// Format: vllm_num_requests_waiting{model_name="phi-3"} 15
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// Last field should be the numeric value
				value, err := strconv.Atoi(fields[len(fields)-1])
				if err != nil {
					return 0, fmt.Errorf("failed to parse queue depth value: %w", err)
				}
				return value, nil
			}
		}
	}

	return 0, fmt.Errorf("vllm_num_requests_waiting metric not found in response")
}
