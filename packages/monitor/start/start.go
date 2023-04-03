package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
)

// func main() {
// 	perform()
// }

func Main(args map[string]interface{}) map[string]interface{} {

	perform()

	msg := make(map[string]interface{})
	msg["body"] = "ok"
	return msg
}

func perform() {

	token := setupFirestore()

	apps := getApps(token)
	versions := getVersions(token)
	subscribers := getSubscribers(token)

	log.Println(apps)
	log.Println(subscribers)
	log.Println(versions)

	for _, app := range apps {
		fmt.Println("------------ Checking app: " + app)

		appInfo := getAppInfo(app)
		bundleIdString := appInfo.bundleIdString()

		var needNotify = true

		fmt.Println("Latest version: ", versions[bundleIdString])

		if appInfo.Version != versions[bundleIdString] {
			fmt.Println("New version: " + appInfo.Version + " for " + appInfo.Name)
			needNotify = true
		} else {
			fmt.Println("No new version for " + appInfo.Name + " (" + appInfo.Version + ")")
			needNotify = false
		}

		if needNotify {
			updateAppInfo(token, appInfo)
			notify(appInfo, subscribers)
		} else {
			fmt.Println("No need to notify and update")
		}
	}
}

// Notify about app release

func notify(appInfo AppInfo, subscribers []string) {

	msg := appInfo.Name + ", version: " + appInfo.Version + "\n" + appInfo.ReleaseNotes

	for _, subscriber := range subscribers {
		sendToTelegram(msg, subscriber, os.Getenv("TLGRM_BOT_TOKEN"))
	}
}

func encodeParam(s string) string {
	return url.QueryEscape(s)
}

func sendToTelegram(message string, chatId string, botToken string) {

	fmt.Println("Sending message to telegram: " + message)

	tlgMsgUrl := "https://api.telegram.org/" + botToken + "/sendMessage?chat_id=" + chatId + "&text=" + encodeParam(message)

	respTlgr, errTlgr := http.Get(tlgMsgUrl)
	if errTlgr != nil {
		panic(errTlgr)
	}
	defer respTlgr.Body.Close()

	bodyBytesTlgr, err := ioutil.ReadAll(respTlgr.Body)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Telegram respoinse: ", string(bodyBytesTlgr))
}

// Fetch app info

var itcURL = "https://itunes.apple.com/lookup?bundleId=" // com.xxx.xx  &country=ua"

type AppInfo struct {
	Name         string `json:"trackName"` //"trackName":"Cropwise Operations"
	BundleId     string //"bundleId":"com.nst.cropio"
	Version      string //"version":"4.11.1"
	ReleaseNotes string //"releaseNotes":"- Updated localisations\n- Fixed issues"
}

func (info AppInfo) bundleIdString() string {
	// replace all dots with underscores to avoid problems with firestore
	return strings.ReplaceAll(info.BundleId, ".", "")
}

type Result struct {
	Results []AppInfo `json:"results"`
}

func getAppInfo(app string) AppInfo {

	url := itcURL + app

	req, err := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Cache-Control", "no-cache")
	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{}
	resp, errR := client.Do(req)
	if err != nil {
		log.Fatal(errR)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	resultString := string(bodyBytes)

	var data *Result = &Result{}
	errj := json.Unmarshal([]byte(resultString), data)
	if errj != nil {
		fmt.Println(errj.Error())
	}

	appInfo := data.Results[0]

	return appInfo
}

// Firestore data access

var dbURL string = "https://cropiotest-default-rtdb.europe-west1.firebasedatabase.app"
var appsPath string = "/apps"
var subscribersPath string = "/subscribers"
var versionsPath string = "/versions"

func updateAppInfo(accessToken string, appInfo AppInfo) {

	data := map[string]string{
		appInfo.bundleIdString(): appInfo.Version,
	}

	result := put(dbURL, versionsPath, data, accessToken)

	fmt.Println("updateAppInfo: ", result)
}

func getApps(accessToken string) []string {

	jsonString := get(dbURL, appsPath, accessToken)

	var stringArray []string
	err := json.Unmarshal([]byte(jsonString), &stringArray)
	if err != nil {
		fmt.Println("resultString: ", jsonString)
		panic(err)
	}

	return stringArray
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

func getVersions(accessToken string) map[string]interface{} {

	jsonString := get(dbURL, versionsPath, accessToken)

	var versions map[string]interface{}
	err := json.Unmarshal([]byte(jsonString), &versions)
	if err != nil {
		fmt.Println("resultString: ", jsonString)
		panic(err)
	}

	return versions
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

func put(dbURL string, dataPath string, data map[string]string, accessToken string) string {

	url := dbURL + dataPath + ".json?access_token=" + accessToken

	// marshal the data to JSON format
	payload, err := json.Marshal(data)
	if err != nil {
		fmt.Println("error marshaling data:", err)
		return ""
	}

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(payload))
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
