package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMonitorApps(t *testing.T) {
	for _, tc := range []struct {
		name       string
		saved      interface{}
		current    string
		telegramOK bool
		databaseOK bool
		wantSends  int
		wantWrites int
	}{
		{"baseline", nil, "2.7.2", true, true, 0, 1},
		{"same version", "2.7.2", "2.7.2", true, true, 0, 0},
		{"older store version", "2.7.3", "2.7.2", true, true, 0, 0},
		{"new version", "2.7.1", "2.7.2", true, true, 1, 1},
		{"Telegram rejects message", "2.7.1", "2.7.2", false, true, 1, 1},
		{"version save fails", "2.7.1", "2.7.2", true, false, 0, 1},
		{"unavailable version", "2.7.1", "Varies with device", true, true, 0, 0},
		{"unavailable baseline", nil, "Varies with device", true, true, 0, 0},
		{"invalid saved type", 123, "2.7.2", true, true, 0, 0},
		{"invalid saved version", "unknown", "2.7.2", true, true, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sends, writes int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/bottest/sendMessage":
					sends++
					if writes != 1 {
						t.Error("notification sent before saving the version")
					}
					if r.Method != http.MethodPost {
						t.Errorf("unexpected Telegram method: %s", r.Method)
					}
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("parse_mode") != "HTML" || !strings.Contains(r.Form.Get("text"), "(Android)") || !strings.Contains(r.Form.Get("text"), "&lt;field&gt; &amp; fixes_under_score") {
						t.Errorf("incorrect platform or escaping: %v", r.Form)
					}
					fmt.Fprintf(w, `{"ok":%t}`, tc.telegramOK)
				case "/androidVersions.json":
					writes++
					if r.Method != http.MethodPatch || r.URL.Query().Get("access_token") != "test-token" {
						t.Errorf("unexpected Firebase request: %s %s", r.Method, r.URL)
					}
					if sends != 0 {
						t.Error("version saved after notification")
					}
					if !tc.databaseOK {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["comnstcropio"] != tc.current {
						t.Errorf("incorrect saved version: %v", body)
					}
					fmt.Fprint(w, `{}`)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			oldDB, oldTelegram := dbURL, telegramURL
			dbURL, telegramURL = server.URL, server.URL
			t.Cleanup(func() { dbURL, telegramURL = oldDB, oldTelegram })
			t.Setenv("TLGRM_BOT_TOKEN", "test")
			versions := map[string]interface{}{}
			if tc.saved != nil {
				versions["comnstcropio"] = tc.saved
			}
			// A failed app must not prevent the next app from being checked.
			fetch := func(id string) (AppInfo, error) {
				if id == "broken.app" {
					return AppInfo{}, fmt.Errorf("store unavailable")
				}
				return AppInfo{"Android", "Cropwise Operations", id, tc.current, "<field> & fixes_under_score"}, nil
			}
			monitorApps("test-token", []string{"broken.app", "com.nst.cropio"}, versions, androidVersionsPath, []string{"123"}, fetch)
			if sends != tc.wantSends || writes != tc.wantWrites {
				t.Fatalf("sends/writes = %d/%d, want %d/%d", sends, writes, tc.wantSends, tc.wantWrites)
			}
		})
	}
}

func TestRecipientFailureDoesNotRepeatBroadcast(t *testing.T) {
	for _, path := range []string{versionsPath, androidVersionsPath} {
		t.Run(path, func(t *testing.T) {
			saved := map[string]interface{}{"comexampleapp": "1.0"}
			sends := map[string]int{}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case path + ".json":
					if r.Method == http.MethodPatch {
						writes++
						if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
							t.Error(err)
						}
					} else if r.Method != http.MethodGet {
						t.Errorf("unexpected database method: %s", r.Method)
					}
					json.NewEncoder(w).Encode(saved)
				case "/bottest/sendMessage":
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					chat := r.Form.Get("chat_id")
					sends[chat]++
					if saved["comexampleapp"] != "2.0" {
						t.Error("notification sent before recording the release")
					}
					if chat == "blocked" {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `{"ok":false}`)
					} else {
						fmt.Fprint(w, `{"ok":true}`)
					}
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			oldDB, oldTelegram := dbURL, telegramURL
			dbURL, telegramURL = server.URL, server.URL
			t.Cleanup(func() { dbURL, telegramURL = oldDB, oldTelegram })
			t.Setenv("TLGRM_BOT_TOKEN", "test")
			fetch := func(id string) (AppInfo, error) {
				return AppInfo{Platform: "Android", Name: "Example", BundleId: id, Version: "2.0"}, nil
			}
			for run := 0; run < 2; run++ {
				monitorApps("test-token", []string{"com.example.app"}, getVersions("test-token", path), path, []string{"blocked", "active"}, fetch)
			}
			if writes != 1 || sends["blocked"] != 1 || sends["active"] != 1 || len(sends) != 2 {
				t.Fatalf("expected one attempt per chat across two runs: writes=%d, sends=%v", writes, sends)
			}
		})
	}
}

func TestIOSLookupAndStorageIsolation(t *testing.T) {
	lookup := `{"results":[{"trackName":"Cropwise Operations","bundleId":"com.nst.cropio","version":"5.18.0","releaseNotes":"Updated"}]}`
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lookup" {
			fmt.Fprint(w, lookup)
			return
		}
		paths = append(paths, r.URL.Path)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	oldDB, oldITC := dbURL, itcURL
	dbURL, itcURL = server.URL, server.URL+"/lookup"
	t.Cleanup(func() { dbURL, itcURL = oldDB, oldITC })
	info, err := getAppInfo("com.nst.cropio")
	if err != nil || info.Platform != "iOS" || info.Version != "5.18.0" {
		t.Fatalf("unexpected iOS lookup: %+v, %v", info, err)
	}
	for _, path := range []string{versionsPath, androidVersionsPath} {
		if err := updateAppInfo("test-token", path, info); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(paths, ",") != "/versions.json,/androidVersions.json" {
		t.Fatalf("platform storage is not separate: %v", paths)
	}
	lookup = `{"results":[]}`
	if _, err := getAppInfo("com.missing.app"); err == nil {
		t.Fatal("expected empty lookup error")
	}
}
