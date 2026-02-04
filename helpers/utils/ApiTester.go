package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// TestApiConnection attempts to connect to the API and verify it's working
func TestApiConnection(baseUrl string) error {
	// Test the root endpoint
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	log.Printf("Testing connection to %s", baseUrl)
	
	resp, err := client.Get(baseUrl)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()
	
	log.Printf("Connection test successful! Status code: %d", resp.StatusCode)
	
	// Test creating a patient
	testPatient := map[string]interface{}{
		"name": "Test Patient",
		"data_count": 1,
	}
	
	jsonData, err := json.Marshal(testPatient)
	if err != nil {
		return fmt.Errorf("failed to marshal test patient: %w", err)
	}
	
	log.Printf("Testing POST to %s/api/patients with data: %s", baseUrl, string(jsonData))
	
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/patients", baseUrl), bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	postResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("patient creation test failed: %w", err)
	}
	defer postResp.Body.Close()
	
	log.Printf("POST test complete with status code: %d", postResp.StatusCode)
	
	return nil
}
