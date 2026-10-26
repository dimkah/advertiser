package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver"
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
	subscribers := getSubscribers(token)

	monitorApps(token, getApps(token, appsPath), getVersions(token, versionsPath), versionsPath, subscribers, getAppInfo)
	monitorApps(token, getApps(token, androidAppsPath), getVersions(token, androidVersionsPath), androidVersionsPath, subscribers, getAndroidAppInfo)
}

func monitorApps(token string, apps []string, versions map[string]interface{}, path string, subscribers []string, fetch func(string) (AppInfo, error)) {
	for _, app := range apps {
		appInfo, err := fetch(app)
		if err != nil {
			log.Printf("Error checking %s: %v", app, err)
			continue
		}

		current, err := semver.NewVersion(appInfo.Version)
		if err != nil {
			log.Printf("Invalid store version for %s: %q", app, appInfo.Version)
			continue
		}

		key := appInfo.bundleIdString()
		savedValue, exists := versions[key]
		if !exists || savedValue == nil {
			// Establish a baseline for newly monitored apps without announcing an old release.
			if err := updateAppInfo(token, path, appInfo); err != nil {
				log.Printf("Error saving baseline for %s: %v", app, err)
			}
			continue
		}

		savedVersion, ok := savedValue.(string)
		if !ok {
			log.Printf("Invalid saved version for %s", app)
			continue
		}
		saved, err := semver.NewVersion(savedVersion)
		if err != nil {
			log.Printf("Invalid saved version for %s: %q", app, savedVersion)
			continue
		}
		if !current.GreaterThan(saved) {
			continue
		}
		// Record the release before sending, so a failed recipient does not
		// cause the same release to be sent to every chat on the next run.
		if err := updateAppInfo(token, path, appInfo); err != nil {
			log.Printf("Error saving version for %s: %v", app, err)
			continue
		}
		if err := notify(appInfo, subscribers); err != nil {
			log.Printf("Error notifying about %s: %v", app, err)
		}
	}
}

// Notify about app release

func notify(appInfo AppInfo, subscribers []string) error {
	msg := fmt.Sprintf("Доступне оновлення (%s),\n<b>%s</b>, версія <b>%s</b>:\n%s",
		html.EscapeString(appInfo.Platform), html.EscapeString(appInfo.Name),
		html.EscapeString(appInfo.Version), html.EscapeString(appInfo.ReleaseNotes))
	var lastError error
	for _, subscriber := range subscribers {
		if err := sendToTelegram(msg, subscriber, os.Getenv("TLGRM_BOT_TOKEN")); err != nil {
			lastError = err
		}
	}
	return lastError
}

var telegramURL = "https://api.telegram.org"
var httpClient = &http.Client{Timeout: 20 * time.Second}

func sendToTelegram(message string, chatId string, botToken string) error {
	endpoint := telegramURL + "/bot" + botToken + "/sendMessage"
	resp, err := httpClient.PostForm(endpoint, url.Values{
		"chat_id": {chatId}, "text": {message}, "parse_mode": {"HTML"},
	})
	if err != nil {
		// net/http errors contain the request URL, including the bot token.
		return fmt.Errorf("Telegram request failed for chat %s", chatId)
	}
	defer resp.Body.Close()
	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("invalid Telegram response for chat %s", chatId)
	}
	if resp.StatusCode != http.StatusOK || !result.OK {
		return fmt.Errorf("Telegram rejected message for chat %s (HTTP %d)", chatId, resp.StatusCode)
	}
	return nil
}

// Fetch app info
var itcURL = "https://itunes.apple.com/lookup" // /? &bundleId=com.xxx.xx  &country=ua"

type AppInfo struct {
	Platform     string `json:"-"`
	Name         string `json:"trackName"` //"trackName":"Cropwise Operations"
	BundleId     string //"bundleId":"com.nst.cropio"
	Version      string //"version":"4.11.1"
	ReleaseNotes string //"releaseNotes":"- Updated localisations\n- Fixed issues"
}

func (info AppInfo) bundleIdString() string {
	// Preserve the existing Firebase keys, which omit dots.
	return strings.ReplaceAll(info.BundleId, ".", "")
}

type Result struct {
	Results []AppInfo `json:"results"`
}

func getAppInfo(app string) (AppInfo, error) {

	// Construct the URL with query parameters
	url, err := url.Parse(itcURL)
	if err != nil {
		return AppInfo{}, fmt.Errorf("failed to parse URL: %w", err)
	}

	query := url.Query()
	query.Set("bundleId", app)
	query.Set("t", strconv.FormatInt(time.Now().Unix(), 10)) // add current time in seconds since the epoch to avoid caching
	query.Set("country", "UA")
	query.Set("lang", "uk")
	url.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return AppInfo{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Cache-Control", "private, no-cache, no-store, no-transform, must-revalidate, max-age=0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return AppInfo{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Перевірка статусного коду відповіді
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return AppInfo{}, fmt.Errorf("received non-200 response: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return AppInfo{}, fmt.Errorf("failed to read response body: %w", err)
	}
	resultString := string(bodyBytes)

	var data *Result = &Result{}
	errj := json.Unmarshal([]byte(resultString), data)
	if errj != nil {
		return AppInfo{}, fmt.Errorf("failed to Unmarshal result string: %w", errj)
	}

	if len(data.Results) == 0 {
		return AppInfo{}, fmt.Errorf("app %s not found in the Ukrainian App Store", app)
	}
	appInfo := data.Results[0]
	appInfo.Platform = "iOS"

	return appInfo, nil
}

// Firestore data access

var dbURL = strings.TrimRight(strings.TrimSpace(os.Getenv("FIREBASE_DATABASE_URL")), "/")
var appsPath string = "/apps"
var subscribersPath string = "/subscribers"
var versionsPath string = "/versions"
var androidAppsPath = "/androidApps"
var androidVersionsPath = "/androidVersions"

func updateAppInfo(accessToken string, path string, appInfo AppInfo) error {

	data := map[string]string{
		appInfo.bundleIdString(): appInfo.Version,
	}

	return put(dbURL, path, data, accessToken)
}

func getApps(accessToken string, path string) []string {

	jsonString := get(dbURL, path, accessToken)

	var stringArray []string
	err := json.Unmarshal([]byte(jsonString), &stringArray)
	if err != nil {
		fmt.Println("resultString: ", jsonString)
		panic(err)
	}

	return stringArray
}

func getSubscribers(accessToken string) []string {
	subscribers, err := decodeSubscribers([]byte(get(dbURL, subscribersPath, accessToken)))
	if err != nil {
		panic(err)
	}
	return subscribers
}

func decodeSubscribers(data []byte) ([]string, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("invalid subscribers list: expected a JSON array")
	}
	if values == nil {
		return nil, nil
	}

	subscribers := make([]string, len(values))
	for i, value := range values {
		if len(value) > 0 && value[0] == '"' {
			if err := json.Unmarshal(value, &subscribers[i]); err != nil {
				return nil, fmt.Errorf("invalid subscriber at index %d", i)
			}
			continue
		}
		// Firebase may store IDs as numbers. Keep their exact digits without
		// passing through float64, which loses precision above 2^53.
		if _, err := strconv.ParseInt(string(value), 10, 64); err != nil {
			return nil, fmt.Errorf("invalid subscriber at index %d: expected a string or integer", i)
		}
		subscribers[i] = string(value)
	}
	return subscribers, nil
}

func getVersions(accessToken string, path string) map[string]interface{} {

	jsonString := get(dbURL, path, accessToken)

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

func put(dbURL string, dataPath string, data map[string]string, accessToken string) error {

	url := dbURL + dataPath + ".json?access_token=" + accessToken

	// marshal the data to JSON format
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("invalid Firebase request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Firebase update failed for %s", dataPath)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Firebase update failed for %s (HTTP %d)", dataPath, resp.StatusCode)
	}
	return nil
}

// Fierstore connection and authorisation

func setupFirestore() string {
	if dbURL == "" {
		panic("Set FIREBASE_DATABASE_URL")
	}

	ctx := context.Background()
	serviceAccountJSON, _ := base64.StdEncoding.DecodeString(os.Getenv("GCP_CREDS_JSON_BASE64"))

	accessToken, err := generateAccessToken(ctx, serviceAccountJSON)
	if err != nil {
		log.Println(err)
	}

	if len(accessToken) == 0 {
		panic("No access token")
	}

	return accessToken
}

func generateAccessToken(ctx context.Context, serviceAccountJSON []byte) (string, error) {

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
