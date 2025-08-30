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
	username := "ari"
	password := "ari"

	// Create HTTP client
	client := &http.Client{}

	// Create a channel using the echo-test PJSIP endpoint
	fmt.Println("Creating SIP call using echo-test PJSIP endpoint...")

	originateURL := fmt.Sprintf("%s/channels", ariURL)

	// Create the request body for originating a call to the echo-test endpoint
	requestBody := map[string]interface{}{
		"endpoint":    "PJSIP/echo-test", // Use the echo-test PJSIP endpoint
		"extension":   "echo",
		"context":     "ari-context",
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

	// Read response body
	body, _ := ioutil.ReadAll(resp.Body)

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("ARI API error: %d - %s", resp.StatusCode, string(body))

		// Try alternative approach with minimal parameters
		altRequestBody := map[string]interface{}{
			"endpoint":    "PJSIP/echo-test",
			"application": "ari-app",
			"context":     "ari-context",
			"extension":   "echo",
		}

		altJSONBody, err := json.Marshal(altRequestBody)
		if err != nil {
			log.Fatal("Failed to marshal alternative request body:", err)
		}

		altReq, err := http.NewRequest("POST", originateURL, bytes.NewBuffer(altJSONBody))
		if err != nil {
			log.Fatal("Failed to create alternative request:", err)
		}

		altReq.Header.Set("Content-Type", "application/json")
		altReq.SetBasicAuth(username, password)

		altResp, err := client.Do(altReq)
		if err != nil {
			log.Fatal("Failed to make alternative request:", err)
		}
		defer altResp.Body.Close()

		altBody, _ := ioutil.ReadAll(altResp.Body)
		if altResp.StatusCode < 200 || altResp.StatusCode >= 300 {
			log.Fatalf("Alternative ARI API error: %d - %s", altResp.StatusCode, string(altBody))
		}

		body = altBody
		resp = altResp
	}

	// Parse response
	var channel map[string]interface{}
	if err := json.Unmarshal(body, &channel); err != nil {
		log.Fatal("Failed to unmarshal response:", err)
	}

	channelID := channel["id"].(string)
	fmt.Printf("Call originated successfully: %s\n", channelID)

	// Wait to let the call run and generate RTP traffic
	fmt.Println("Waiting for call to generate RTP traffic...")
	time.Sleep(20 * time.Second)

	// Check echo server metrics before hangup
	fmt.Println("Checking echo server metrics before hangup...")
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

	fmt.Printf("Echo Server Metrics: %s\n", string(metricsBody))

	// Try to hang up the call
	fmt.Println("Hanging up the call...")
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

	// Wait a bit and check final metrics
	fmt.Println("Waiting for final metrics...")
	time.Sleep(5 * time.Second)

	finalMetricsReq, err := http.NewRequest("GET", metricsURL, nil)
	if err != nil {
		log.Fatal("Failed to create final metrics request:", err)
	}

	finalMetricsResp, err := client.Do(finalMetricsReq)
	if err != nil {
		log.Fatal("Failed to get final metrics:", err)
	}
	defer finalMetricsResp.Body.Close()

	finalMetricsBody, err := ioutil.ReadAll(finalMetricsResp.Body)
	if err != nil {
		log.Fatal("Failed to read final metrics response:", err)
	}

	fmt.Printf("Final Echo Server Metrics: %s\n", string(finalMetricsBody))
}
