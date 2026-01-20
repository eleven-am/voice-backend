package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Payload   any    `json:"payload,omitempty"`
}

var wsConn *websocket.Conn

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY env required")
	}

	gwURL := os.Getenv("GATEWAY_URL")
	if gwURL == "" {
		gwURL = "ws://localhost:8081/v1/agents/connect"
	}

	u, _ := url.Parse(gwURL)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+apiKey)

	fmt.Println("[ECHO] Starting echo agent...")
	fmt.Printf("[ECHO] Connecting to %s\n", u.String())

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("[ECHO] Dial failed: %v, status=%d, body=%s\n", err, resp.StatusCode, string(body))
		}
		log.Fatal("dial:", err)
	}
	wsConn = conn
	defer conn.Close()

	fmt.Println("[ECHO] Connected to gateway!")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("[ECHO] Shutting down...")
		conn.Close()
		os.Exit(0)
	}()

	fmt.Println("[ECHO] Waiting for messages...")

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			fmt.Printf("[ECHO] Read error: %v\n", err)
			return
		}

		fmt.Printf("[ECHO] Raw message: %s\n", string(data))

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			fmt.Printf("[ECHO] Unmarshal error: %v\n", err)
			continue
		}

		fmt.Printf("[ECHO] Parsed: type=%s session=%s request=%s\n", msg.Type, msg.SessionID, msg.RequestID)

		if msg.Type == "utterance" {
			text := extractText(msg.Payload)
			fmt.Printf("[ECHO] Utterance text: %q\n", text)
			if text != "" {
				sendResponse(msg.SessionID, "You said: "+text)
			} else {
				fmt.Println("[ECHO] Empty text, not responding")
			}
		} else {
			fmt.Printf("[ECHO] Ignoring message type: %s\n", msg.Type)
		}
	}
}

func extractText(payload any) string {
	fmt.Printf("[ECHO] extractText payload type: %T\n", payload)
	if m, ok := payload.(map[string]any); ok {
		fmt.Printf("[ECHO] extractText map: %+v\n", m)
		if t, ok := m["text"].(string); ok {
			return t
		}
	}
	return ""
}

func sendResponse(sessionID, text string) {
	fmt.Printf("[ECHO] Sending response via WebSocket: session=%s text=%q\n", sessionID, text)

	response := Message{
		Type:      "response",
		SessionID: sessionID,
		Payload:   map[string]any{"text": text},
	}

	data, err := json.Marshal(response)
	if err != nil {
		fmt.Printf("[ECHO] Marshal error: %v\n", err)
		return
	}

	fmt.Printf("[ECHO] Sending: %s\n", string(data))

	if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
		fmt.Printf("[ECHO] WriteMessage error: %v\n", err)
		return
	}

	fmt.Println("[ECHO] Response sent successfully!")
}
