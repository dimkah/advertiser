package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var googlePlayURL = "https://play.google.com/store/apps/details"

var playDataStart = regexp.MustCompile(`AF_initDataCallback\(\s*\{[^}]*?\bdata:\s*`)
var notesBreaks = regexp.MustCompile(`(?i)<br\s*/?>|</(?:p|div|li)\s*>`)
var notesTags = regexp.MustCompile(`<[^>]*>`)

func getAndroidAppInfo(packageName string) (AppInfo, error) {
	endpoint, err := url.Parse(googlePlayURL)
	if err != nil {
		return AppInfo{}, err
	}
	query := endpoint.Query()
	query.Set("id", packageName)
	query.Set("hl", "uk")
	query.Set("gl", "UA")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return AppInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := httpClient.Do(req)
	if err != nil {
		return AppInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AppInfo{}, fmt.Errorf("Google Play returned HTTP %d for %s", resp.StatusCode, packageName)
	}
	const maxPageSize = 8 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageSize+1))
	if err != nil {
		return AppInfo{}, err
	}
	if len(body) > maxPageSize {
		return AppInfo{}, fmt.Errorf("Google Play page exceeds size limit")
	}
	return parseGooglePlayPage(string(body), packageName)
}

func parseGooglePlayPage(page, packageName string) (AppInfo, error) {
	// Google embeds JSON arrays in script callbacks. Decode the JSON only; never
	// execute JavaScript. Match the package ID instead of relying on a ds:N key.
	for _, match := range playDataStart.FindAllStringIndex(page, -1) {
		var data interface{}
		if err := json.NewDecoder(strings.NewReader(page[match[1]:])).Decode(&data); err != nil {
			continue
		}
		if playString(data, 1, 2, 77, 0) != packageName {
			continue
		}
		info := AppInfo{
			Platform: "Android",
			BundleId: packageName,
			Name:     playString(data, 1, 2, 0, 0),
			Version:  playString(data, 1, 2, 140, 0, 0, 0),
		}
		if info.Name == "" || info.Version == "" || strings.EqualFold(info.Version, "Varies with device") {
			return AppInfo{}, fmt.Errorf("Google Play has no usable name/version for %s", packageName)
		}
		// An app may legitimately have no release notes. Never substitute the
		// update date for missing notes or use it as a release identifier.
		notes := playString(data, 1, 2, 144, 1, 1)
		notes = notesBreaks.ReplaceAllString(notes, "\n")
		notes = notesTags.ReplaceAllString(notes, "")
		info.ReleaseNotes = strings.TrimSpace(html.UnescapeString(notes))
		return info, nil
	}
	return AppInfo{}, fmt.Errorf("Google Play app data not found for %s (page unavailable or format changed)", packageName)
}

func playString(value interface{}, path ...int) string {
	for _, index := range path {
		items, ok := value.([]interface{})
		if !ok || index < 0 || len(items) == 0 {
			return ""
		}
		if index < len(items) {
			value = items[index]
			continue
		}
		// Some responses put higher fields in a trailing sparse object.
		// Its keys are one-based field numbers, unlike the array indexes.
		fields, ok := items[len(items)-1].(map[string]interface{})
		if !ok {
			return ""
		}
		value = fields[strconv.Itoa(index+1)]
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
