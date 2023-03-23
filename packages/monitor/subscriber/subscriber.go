package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/oauth2/google"
)

// func main() {

// 	args := map[string]interface{}{
// 		"str": "hello",
// 		"int": 42,
// 	}

// 	parse(args)
// }

func Main(args map[string]interface{}) map[string]interface{} {

	parse(args)

	msg := make(map[string]interface{})
	msg["body"] = "ok"
	return msg
}

func parse(args map[string]interface{}) {

	jsonBytes, err := json.Marshal(args)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	jsonString := string(jsonBytes)

	update, err := parseTelegramRequest(jsonString)

	if err != nil {
		log.Println(err)
	}

	if update.Message.Chat.Id != 0 && update.Message.Text != "" {
		perform(*update)
	} else {
		fmt.Println("Unknown args: ", update)
	}
}

func perform(update Update) {

	fmt.Println("Chat ID: ", update.Message.Chat.Id)
	fmt.Println("Message: ", update.Message.Text)

	chatId := strconv.Itoa(update.Message.Chat.Id)
	messageString := update.Message.Text

	if strings.Contains(messageString, "/subscribe") {
		// Subscribe
		token := setupFirestore()
		subscribers := getSubscribers(token)
		log.Println(subscribers)
		addSubscriber(chatId, subscribers, token)

	} else if strings.Contains(messageString, "/unsubscribe") {
		// Unsubscribe
		token := setupFirestore()
		subscribers := getSubscribers(token)
		log.Println(subscribers)
		removeSubscriber(chatId, subscribers, token)

	} else {
		// Unknown command
		fmt.Println("Unknown command: ", messageString)
		panic("Unknown command: " + messageString)
	}
}

func removeSubscriber(subscriber string, currentSubscribers []string, token string) {

	// Remove duplicates
	currentSubscribers = removeDuplicateStr(currentSubscribers)

	// Remove subscriber
	for i, v := range currentSubscribers {
		if v == subscriber {
			currentSubscribers = append(currentSubscribers[:i], currentSubscribers[i+1:]...)
		}
	}

	updateSubscribers(token, currentSubscribers)
}

func addSubscriber(subscriber string, currentSubscribers []string, token string) {

	// Append new subscriber
	currentSubscribers = append(currentSubscribers, subscriber)

	// Remove duplicates
	currentSubscribers = removeDuplicateStr(currentSubscribers)

	updateSubscribers(token, currentSubscribers)
}

// Telegram

// Update is a Telegram object that the handler receives every time an user interacts with the bot.
type Update struct {
	UpdateId int     `json:"update_id"`
	Message  Message `json:"message"`
}

// Message is a Telegram object that can be found in an update.
type Message struct {
	Text string `json:"text"`
	Chat Chat   `json:"chat"`
}

// A Telegram Chat indicates the conversation to which the message belongs.
type Chat struct {
	Id int `json:"id"`
}

// parseTelegramRequest handles incoming update from the Telegram web hook
func parseTelegramRequest(message string) (*Update, error) {

	if message == "" {
		return nil, fmt.Errorf("empty message")
	}

	var update Update
	err := json.Unmarshal([]byte(message), &update)
	if err != nil {
		fmt.Println("resultString: ", message)
		panic(err)
	}
	return &update, nil
}

// Firestore data access

var dbURL string = "https://cropiotest-default-rtdb.europe-west1.firebasedatabase.app"
var subscribersPath string = "/subscribers"

func updateSubscribers(accessToken string, subscribers []string) {

	result := put(dbURL, subscribersPath, subscribers, accessToken)

	fmt.Println("updateAppInfo: ", result)
}

func getSubscribers(accessToken string) []string {

	jsonString := get(dbURL, subscribersPath, accessToken)

	var stringArray []string
	err := json.Unmarshal([]byte(jsonString), &stringArray)
	if err != nil {
		fmt.Println("resultString: ", jsonString)
		panic(err)
	}

	return stringArray
}

func get(dbURL string, dataPath string, accessToken string) string {

	url := dbURL + dataPath + ".json?access_token=" + accessToken

	resp, errR := http.Get(url)
	if errR != nil {
		panic(errR)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	resultString := string(bodyBytes)

	return resultString
}

func put(dbURL string, dataPath string, data []string, accessToken string) string {

	url := dbURL + dataPath + ".json?access_token=" + accessToken

	// marshal the data to JSON format
	payload, err := json.Marshal(data)
	if err != nil {
		fmt.Println("error marshaling data:", err)
		return ""
	}

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	// read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error reading Firestore API response body:", err)
		return ""
	}

	resultString := string(respBody)

	return resultString
}

// Fierstore connection and authorisation

func setupFirestore() string {

	ctx := context.Background()
	serviceAccountJSON, _ := base64.StdEncoding.DecodeString(os.Getenv("GCP_CREDS_JSON_BASE64"))
	firebaseProjectID := "cropiotest"

	accessToken, err := generateAccessToken(ctx, firebaseProjectID, serviceAccountJSON)
	if err != nil {
		log.Println(err)
	}

	if len(accessToken) == 0 {
		panic("No access token")
	}

	return accessToken
}

func generateAccessToken(ctx context.Context, firebaseProjectID string, serviceAccountJSON []byte) (string, error) {

	creds, err := google.CredentialsFromJSON(ctx, serviceAccountJSON, "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/firebase.database")
	if err != nil {
		fmt.Println("Error getting credentials: ", err)
		return "", err
	}

	tokenSource := creds.TokenSource
	token, err := tokenSource.Token()
	if err != nil {
		fmt.Println("Error getting token: ", err)
		return "", err
	}

	fmt.Println("Authorised with service account.")
	fmt.Println("Token expiry: ", token.Expiry)

	return token.AccessToken, nil
}

// Helpers

func removeDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range strSlice {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}
