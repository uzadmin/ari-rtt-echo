package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func main() {
	// ARI server details
	ariURL := "http://localhost:8088/ari"
	username := "asterisk"
	password := "asterisk"

	// Create HTTP client
	client := &http.Client{}

	// Originate a call to the echo extension
	originateURL := fmt.Sprintf("%s/channels", ariURL)

	// Create the request body
	requestBody := map[string]interface{}{
		"endpoint":    "PJSIP/echo-test",
		"extension":   "echo",
		"context":     "default",
		"priority":    1,
		"application": "ari-app",
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		log.Fatal("Failed to marshal request body:", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", originateURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Fatal("Failed to create request:", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(username, password)

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Failed to make request:", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Fatalf("ARI API error: %d - %s", resp.StatusCode, string(body))
	}

	// Parse response
	var channel map[string]interface{}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Failed to read response body:", err)
	}

	if err := json.Unmarshal(body, &channel); err != nil {
		log.Fatal("Failed to unmarshal response:", err)
	}

	channelID := channel["id"].(string)
	fmt.Printf("Call originated successfully: %s\n", channelID)

	// Wait for a bit to let the call run and generate metrics
	fmt.Println("Waiting for call to generate metrics...")
	time.Sleep(15 * time.Second)

	// Try to hang up the call
	hangupURL := fmt.Sprintf("%s/channels/%s", ariURL, channelID)
	hangupReq, err := http.NewRequest("DELETE", hangupURL, nil)
	if err != nil {
		log.Fatal("Failed to create hangup request:", err)
	}
	hangupReq.SetBasicAuth(username, password)

	hangupResp, err := client.Do(hangupReq)
	if err != nil {
		log.Printf("Failed to hangup call: %v", err)
	} else {
		hangupResp.Body.Close()
		if hangupResp.StatusCode < 200 || hangupResp.StatusCode >= 300 {
			fmt.Printf("Warning: Hangup returned status %d\n", hangupResp.StatusCode)
		} else {
			fmt.Println("Call hung up successfully")
		}
	}

	// Check metrics
	fmt.Println("Checking metrics...")
	metricsURL := "http://localhost:9090/metrics"
	metricsReq, err := http.NewRequest("GET", metricsURL, nil)
	if err != nil {
		log.Fatal("Failed to create metrics request:", err)
	}

	metricsResp, err := client.Do(metricsReq)
	if err != nil {
		log.Fatal("Failed to get metrics:", err)
	}
	defer metricsResp.Body.Close()

	metricsBody, err := ioutil.ReadAll(metricsResp.Body)
	if err != nil {
		log.Fatal("Failed to read metrics response:", err)
	}

	fmt.Printf("Metrics: %s\n", string(metricsBody))
}
