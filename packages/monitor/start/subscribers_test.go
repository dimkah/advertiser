package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestGetSubscribers(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{"strings", `["12345","-1001234567890"]`, []string{"12345", "-1001234567890"}},
		{"numbers", `[12345,-10012345678900001]`, []string{"12345", "-10012345678900001"}},
		{"mixed", `["123450000", -9007199254740993, "-10012345678900001", 67890]`, []string{"123450000", "-9007199254740993", "-10012345678900001", "67890"}},
		{"empty", `[]`, []string{}},
		{"missing", `null`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodGet || r.URL.Path != "/subscribers.json" || r.URL.Query().Get("access_token") != "test-token" {
					t.Errorf("unexpected subscriber request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			oldDB := dbURL
			dbURL = server.URL
			t.Cleanup(func() { dbURL = oldDB })

			if got := getSubscribers("test-token"); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("subscribers = %v, want %v", got, tc.want)
			}
			if requests != 1 {
				t.Fatalf("expected exactly one read, got %d requests", requests)
			}
		})
	}
}

func TestDecodeSubscribersRejectsInvalidValues(t *testing.T) {
	for _, body := range []string{
		`{}`, `[12345,`, `[true]`, `[null]`, `[{}]`, `[[]]`,
		`[1.5]`, `[1e5]`, `[9223372036854775808]`,
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := decodeSubscribers([]byte(body)); err == nil {
				t.Fatal("expected invalid subscriber data to be rejected")
			}
		})
	}
}
