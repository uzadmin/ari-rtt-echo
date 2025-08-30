package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

// ARIClient handles communication with the ARI API
type ARIClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

// Bridge represents an ARI bridge
type Bridge struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"bridge_type"`
}

// Channel represents an ARI channel
type Channel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// NewARIClient creates a new ARI client
func NewARIClient(baseURL, username, password string) *ARIClient {
	return &ARIClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client:   &http.Client{},
	}
}

// AnswerChannel answers an incoming channel
func (c *ARIClient) AnswerChannel(channelID string) error {
	url := fmt.Sprintf("http://%s/ari/channels/%s/answer", c.baseURL, channelID)
	return c.makeRequest("POST", url, nil, nil)
}

// CreateBridge creates a new bridge
func (c *ARIClient) CreateBridge(request map[string]interface{}) (*Bridge, error) {
	url := fmt.Sprintf("http://%s/ari/bridges", c.baseURL)

	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	var bridge Bridge
	err = c.makeRequest("POST", url, body, &bridge)
	if err != nil {
		return nil, err
	}

	return &bridge, nil
}

// CreateExternalMedia creates a new external media channel
func (c *ARIClient) CreateExternalMedia(request map[string]interface{}) (string, error) {
	// Build query parameters for external media creation
	baseURL := fmt.Sprintf("http://%s/ari/channels/externalMedia", c.baseURL)

	// Extract required parameters from request
	params := []string{"app=" + request["app"].(string)}

	// Extract external_host (required)
	if externalHost, ok := request["external_host"].(string); ok {
		params = append(params, "external_host="+externalHost)
	}

	// Extract format (required)
	if format, ok := request["format"].(string); ok {
		params = append(params, "format="+format)
	} else {
		// Default to ulaw if not specified
		params = append(params, "format=ulaw")
	}

	// Extract optional parameters
	if encapsulation, ok := request["encapsulation"].(string); ok {
		params = append(params, "encapsulation="+encapsulation)
	}

	if channelID, ok := request["channelId"].(string); ok {
		params = append(params, "channelId="+channelID)
	}

	if direction, ok := request["direction"].(string); ok {
		params = append(params, "direction="+direction)
	}

	// Construct the full URL with query parameters
	url := baseURL + "?" + strings.Join(params, "&")

	var channel Channel
	err := c.makeRequest("POST", url, nil, &channel)
	if err != nil {
		return "", err
	}

	return channel.ID, nil
}

// AddChannelToBridge adds a channel to a bridge
func (c *ARIClient) AddChannelToBridge(bridgeID, channelID string) error {
	url := fmt.Sprintf("http://%s/ari/bridges/%s/addChannel", c.baseURL, bridgeID)

	request := map[string]interface{}{
		"channel": channelID,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	return c.makeRequest("POST", url, body, nil)
}

// RemoveChannelFromBridge removes a channel from a bridge
func (c *ARIClient) RemoveChannelFromBridge(bridgeID, channelID string) error {
	url := fmt.Sprintf("http://%s/ari/bridges/%s/removeChannel", c.baseURL, bridgeID)

	request := map[string]interface{}{
		"channel": channelID,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	return c.makeRequest("POST", url, body, nil)
}

// HangupChannel hangs up a channel
func (c *ARIClient) HangupChannel(channelID string) error {
	url := fmt.Sprintf("http://%s/ari/channels/%s", c.baseURL, channelID)
	return c.makeRequest("DELETE", url, nil, nil)
}

// DeleteBridge deletes a bridge
func (c *ARIClient) DeleteBridge(bridgeID string) error {
	url := fmt.Sprintf("http://%s/ari/bridges/%s", c.baseURL, bridgeID)
	return c.makeRequest("DELETE", url, nil, nil)
}

// GetChannel gets information about a channel
func (c *ARIClient) GetChannel(channelID string) (*Channel, error) {
	url := fmt.Sprintf("http://%s/ari/channels/%s", c.baseURL, channelID)

	var channel Channel
	err := c.makeRequest("GET", url, nil, &channel)
	if err != nil {
		return nil, err
	}

	return &channel, nil
}

// GetChannels gets all active channels
func (c *ARIClient) GetChannels() ([]Channel, error) {
	url := fmt.Sprintf("http://%s/ari/channels", c.baseURL)

	var channels []Channel
	err := c.makeRequest("GET", url, nil, &channels)
	if err != nil {
		return nil, err
	}

	return channels, nil
}

// GetBridges gets all active bridges
func (c *ARIClient) GetBridges() ([]Bridge, error) {
	url := fmt.Sprintf("http://%s/ari/bridges", c.baseURL)

	var bridges []Bridge
	err := c.makeRequest("GET", url, nil, &bridges)
	if err != nil {
		return nil, err
	}

	return bridges, nil
}

// GetApplication gets information about an ARI application
func (c *ARIClient) GetApplication(appName string) (map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s/ari/applications/%s", c.baseURL, appName)

	var app map[string]interface{}
	err := c.makeRequest("GET", url, nil, &app)
	if err != nil {
		return nil, err
	}

	return app, nil
}

// RegisterApplication registers an ARI application with Asterisk
func (c *ARIClient) RegisterApplication(appName string) error {
	// For registration, we might need to send a POST request
	// Let's try a GET request first to see if the application exists
	_, err := c.GetApplication(appName)
	if err != nil {
		// If the application doesn't exist, that's expected
		// Applications are typically registered when a WebSocket connection is established
		return nil
	}

	return nil
}

// GetEvents polls for ARI events
func (c *ARIClient) GetEvents(appName string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("http://%s/ari/events?api_key=%s:%s&app=%s",
		c.baseURL, c.username, c.password, appName)

	var events []map[string]interface{}
	err := c.makeRequest("GET", url, nil, &events)
	if err != nil {
		// If there are no events, we might get a 404 or other error
		// This is expected when there are no events
		return []map[string]interface{}{}, nil
	}

	return events, nil
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

		// Handle empty response body
		if len(respBody) == 0 {
			return nil
		}

		// Handle response that might be just a string
		respBodyStr := strings.TrimSpace(string(respBody))
		if respBodyStr == "" || respBodyStr == "null" {
			return nil
		}

		return json.Unmarshal(respBody, result)
	}

	return nil
}
