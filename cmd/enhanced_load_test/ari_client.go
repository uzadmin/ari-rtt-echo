package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// ARIClient handles communication with the ARI API
type ARIClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

// Channel represents an ARI channel
type Channel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// OriginateRequest represents a request to originate a call
type OriginateRequest struct {
	Endpoint  string            `json:"endpoint"`
	App       string            `json:"app"`
	AppArgs   []string          `json:"appArgs,omitempty"`
	CallerId  string            `json:"callerId,omitempty"`
	Timeout   int               `json:"timeout,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
	Context   string            `json:"context,omitempty"`
	Extension string            `json:"extension,omitempty"`
	Priority  int               `json:"priority,omitempty"`
}

// NewARIClient creates a new ARI client
func NewARIClient(baseURL, username, password string) *ARIClient {
	return &ARIClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Originate creates a new channel
func (c *ARIClient) Originate(request OriginateRequest) (*Channel, error) {
	url := fmt.Sprintf("http://%s/ari/channels", c.baseURL)

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	var channel Channel
	err = c.makeRequest("POST", url, body, &channel)
	if err != nil {
		return nil, err
	}

	return &channel, nil
}

// HangupChannel hangs up a channel
func (c *ARIClient) HangupChannel(channelID string) error {
	url := fmt.Sprintf("http://%s/ari/channels/%s", c.baseURL, channelID)
	return c.makeRequest("DELETE", url, nil, nil)
}

// GetMetrics retrieves metrics from the ARI service
func (c *ARIClient) GetMetrics() (map[string]interface{}, error) {
	// Point to the metrics service
	metricsURL := fmt.Sprintf("http://localhost:9090/metrics")

	var metrics map[string]interface{}
	err := c.makeRequest("GET", metricsURL, nil, &metrics)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// makeRequest makes an HTTP request to the ARI API
func (c *ARIClient) makeRequest(method, url string, body []byte, result interface{}) error {
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return err
		}
	}

	// Set basic auth
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("ARI API error: %d - %s", resp.StatusCode, string(respBody))
	}

	// Parse response if needed
	if result != nil {
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		return json.Unmarshal(respBody, result)
	}

	return nil
}
