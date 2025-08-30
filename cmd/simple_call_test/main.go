package main
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ari"
)

func main() {
	// Connect to ARI
	client, err := ari.NewClient(ari.ClientOptions{
		Application:  "ari-app",
		Username:     "asterisk",
		Password:     "asterisk",
		URL:          "http://localhost:8088/ari",
		WebsocketURL: "ws://localhost:8088/ari/events",
	})
	if err != nil {
		log.Fatal("Failed to create ARI client:", err)
	}

	// Start the client
	if err := client.Start(); err != nil {
		log.Fatal("Failed to start ARI client:", err)
	}
	defer client.Stop()

	fmt.Println("ARI client connected")

	// Create a channel to receive events
	eventChannel := client.OnEvent("*")

	// Listen for events
	go func() {
		for event := range eventChannel {
			fmt.Printf("Received event: %T\n", event)
		}
	}()

	// Originate a call
	originiateRequest := ari.OriginateRequest{
		Endpoint:    "PJSIP/echo",
		Context:     "default",
		Exten:       "echo",
		Priority:    1,
		Application: "ari-app",
	}

	channel, err := client.Channel().Originate(originiateRequest)
	if err != nil {
		log.Fatal("Failed to originate call:", err)
	}

	fmt.Printf("Call originated: %s\n", channel.ID)

	// Wait for a bit to let the call run
	time.Sleep(10 * time.Second)

	// Hang up the call
	err = client.Channel().Hangup(channel.ID, "normal")
	if err != nil {
		log.Printf("Failed to hangup call: %v", err)
	}

	fmt.Println("Call completed")
}