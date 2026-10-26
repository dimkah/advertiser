package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func playFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/google-play-cropwise.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseGooglePlayPage(t *testing.T) {
	page := playFixture(t)
	info, err := parseGooglePlayPage(page, "com.nst.cropio")
	if err != nil {
		t.Fatal(err)
	}
	if info.Platform != "Android" || info.Name != "Cropwise Operations" || info.Version != "2.7.2" || info.BundleId != "com.nst.cropio" {
		t.Fatalf("unexpected app: %+v", info)
	}
	if !strings.Contains(info.ReleaseNotes, "інтерфейс\n◦ Виправлено") || strings.Contains(info.ReleaseNotes, "<br>") {
		t.Fatalf("unexpected release notes: %q", info.ReleaseNotes)
	}
	for name, page := range map[string]string{
		"missing version": strings.Replace(page, `"2.7.2"`, `null`, 1),
		"unknown version": strings.Replace(page, `"2.7.2"`, `"Varies with device"`, 1),
		"wrong package":   strings.ReplaceAll(page, "com.nst.cropio", "com.other.app"),
		"missing title":   strings.Replace(page, `"Cropwise Operations"`, `null`, 1),
		"malformed data":  `AF_initDataCallback({key: 'ds:5', data: [broken});`,
		"blocked page":    `<html>Please sign in</html>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseGooglePlayPage(page, "com.nst.cropio"); err == nil {
				t.Fatal("expected an error instead of invented release data")
			}
		})
	}
}

func TestParseGooglePlayIgnoresCallbackNumber(t *testing.T) {
	page := "AF_initDataCallback({key: 'ds:0', data: [null], sideChannel: {}});" +
		strings.Replace(playFixture(t), "ds:5", "ds:42", 1)
	if _, err := parseGooglePlayPage(page, "com.nst.cropio"); err != nil {
		t.Fatal(err)
	}
}

func TestParseGooglePlaySparseFields(t *testing.T) {
	page, err := os.ReadFile("testdata/google-play-weighings.html")
	if err != nil {
		t.Fatal(err)
	}
	info, err := parseGooglePlayPage(string(page), "com.nst.cropio.weighings")
	if err != nil {
		t.Fatal(err)
	}
	if info.Platform != "Android" || info.Name != "Cropio Weighings" || info.Version != "1.43.6" || info.BundleId != "com.nst.cropio.weighings" || info.ReleaseNotes != "" {
		t.Fatalf("unexpected app: %+v", info)
	}
	for _, replacement := range []string{`null`, `"Varies with device"`} {
		pageWithoutVersion := strings.Replace(string(page), `"1.43.6"`, replacement, 1)
		if _, err := parseGooglePlayPage(pageWithoutVersion, "com.nst.cropio.weighings"); err == nil {
			t.Fatal("expected an error for unavailable sparse version")
		}
	}
}

func TestGetAndroidAppInfo(t *testing.T) {
	page := playFixture(t)
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "com.nst.cropio" || r.URL.Query().Get("hl") != "uk" || r.URL.Query().Get("gl") != "UA" {
			t.Errorf("unexpected storefront query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, page)
	}))
	defer server.Close()
	previousURL := googlePlayURL
	googlePlayURL = server.URL
	t.Cleanup(func() { googlePlayURL = previousURL })
	if _, err := getAndroidAppInfo("com.nst.cropio"); err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{http.StatusNotFound, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		status = code
		if _, err := getAndroidAppInfo("com.nst.cropio"); err == nil {
			t.Fatalf("expected an error for HTTP %d", code)
		}
	}
}

func TestGooglePlayLive(t *testing.T) {
	if os.Getenv("LIVE_GOOGLE_PLAY") != "1" {
		t.Skip("set LIVE_GOOGLE_PLAY=1 for a read-only public storefront check")
	}
	for _, id := range []string{"com.nst.cropio", "com.nst.cropio.telematics", "com.nst.cropio.yield", "com.nst.cropio.weighings"} {
		info, err := getAndroidAppInfo(id)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %s, release notes present=%t", id, info.Version, info.ReleaseNotes != "")
	}
}
