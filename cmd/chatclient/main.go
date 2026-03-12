package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	// Import dto to use type definitions
	"github.com/john/botsapp/internal/api/dto"
)

var (
	apiURL = "http://localhost:8080"
	wsURL  = "ws://localhost:8080/ws/feed"
	token  string
	myInfo dto.UserInfo
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== BotsApp Terminal Client ===")

	// 1. Auth Flow
	fmt.Print("Enter your phone number (e.g., +919999900001): ")
	phone, _ := reader.ReadString('\n')
	phone = strings.TrimSpace(phone)

	if err := requestOTP(phone); err != nil {
		log.Fatalf("OTP Request failed: %v", err)
	}
	fmt.Println("OTP requested. Check API server logs for the code.")

	fmt.Print("Enter OTP code: ")
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	if err := verifyOTP(phone, code); err != nil {
		log.Fatalf("OTP Verify failed: %v", err)
	}
	fmt.Printf("Authenticated successfully as %s (ID: %d)\n\n", myInfo.PhoneNumber, myInfo.ID)

	// 2. Select Contact
	fmt.Print("Enter the phone number of the person you want to chat with: ")
	targetPhone, _ := reader.ReadString('\n')
	targetPhone = strings.TrimSpace(targetPhone)

	targetContact, err := syncContact(targetPhone)
	if err != nil {
		log.Fatalf("Contact sync failed: %v", err)
	}

	fmt.Printf("Found contact %s (ID: %d). Starting chat...\n", targetContact.PhoneNumber, targetContact.UserID)
	fmt.Println("--------------------------------------------------")

	// 3. Connect to WebSocket
	stopWS := make(chan struct{})
	go connectWebSocket(stopWS)

	// Wait briefly for WS to connect
	time.Sleep(500 * time.Millisecond)

	// 4. Input Loop
	fmt.Println("Type your message and press Enter to send. Type /quit to exit.")
	for {
		// No prompt symbol to avoid messing with async incoming messages, or just a simple ">"
		msg, _ := reader.ReadString('\n')
		msg = strings.TrimSpace(msg)

		if msg == "/quit" {
			close(stopWS)
			break
		}

		if msg == "" {
			continue
		}

		sendMessage(targetContact.PhoneNumber, msg)
	}
}

// --- API Helpers ---

func requestOTP(phone string) error {
	reqBody, _ := json.Marshal(dto.OTPRequestReq{PhoneNumber: phone})
	resp, err := http.Post(apiURL+"/auth/otp/request", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func verifyOTP(phone, code string) error {
	reqBody, _ := json.Marshal(dto.OTPVerifyReq{PhoneNumber: phone, Code: code})
	resp, err := http.Post(apiURL+"/auth/otp/verify", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var res dto.OTPVerifyRes
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	token = res.Token
	myInfo = res.User
	return nil
}

func syncContact(phone string) (dto.ContactInfo, error) {
	reqBody, _ := json.Marshal(dto.ContactSyncReq{PhoneNumbers: []string{phone}})

	req, _ := http.NewRequest("POST", apiURL+"/contacts/sync", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return dto.ContactInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return dto.ContactInfo{}, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var res dto.ContactSyncRes
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return dto.ContactInfo{}, err
	}

	if len(res.Found) == 0 {
		return dto.ContactInfo{}, fmt.Errorf("contact not found on network")
	}

	return res.Found[0], nil
}

func sendMessage(toPhone, text string) {
	// Our message payload format expects a raw JSON payload that the bot understands.
	// For simple testing, we'll wrap text in {"text": "..."}.
	payloadBytes, _ := json.Marshal(map[string]string{"text": text})

	reqMsg := dto.SendMessageReq{
		To:      toPhone,
		Intent:  "text_message",
		Payload: payloadBytes,
	}
	reqBody, _ := json.Marshal(reqMsg)

	req, _ := http.NewRequest("POST", apiURL+"/messages", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[Error] Failed to send msg: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[Error] Send status %d: %s\n", resp.StatusCode, string(body))
		return
	}

	var res dto.SendMessageRes
	_ = json.NewDecoder(resp.Body).Decode(&res)
	fmt.Printf("[Sent] -> MsgID: %d | Status: %s\n", res.MessageID, res.Status)
}

func connectWebSocket(stop <-chan struct{}) {
	u, _ := url.Parse(wsURL)
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("WebSocket dialect err: %v", err)
	}
	defer c.Close()

	// Handle graceful shutdown
	go func() {
		<-stop
		c.Close()
	}()

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Printf("WS closed: %v", err)
			return
		}

		var event dto.FeedEvent
		if err := json.Unmarshal(message, &event); err != nil {
			fmt.Printf("\n[Raw WS]: %s\n", string(message))
			continue
		}

		switch event.Type {
		case "new_message":
			// It comes in as map[string]interface{} under Data because of FeedEvent definition
			dataBytes, _ := json.Marshal(event.Data)
			var msgInfo dto.MessageInfo
			_ = json.Unmarshal(dataBytes, &msgInfo)

			if msgInfo.FromUserID == myInfo.ID {
				// We sent this. Might be an echo or real-time feed of our bot speaking.
				fmt.Printf("\n[My Bot/System]: %s\n", string(msgInfo.Payload))
			} else {
				// Someone sent us a message!
				fmt.Printf("\n[Contact]: %s\n", string(msgInfo.Payload))
			}

		case "status_update":
			dataBytes, _ := json.Marshal(event.Data)
			var statusUpdate dto.StatusUpdateEvent
			_ = json.Unmarshal(dataBytes, &statusUpdate)
			fmt.Printf("\n[Status] MsgID %d -> %s\n", statusUpdate.MessageID, statusUpdate.Status)

		default:
			fmt.Printf("\n[WS %s]: %v\n", event.Type, event.Data)
		}
	}
}
