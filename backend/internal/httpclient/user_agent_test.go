package httpclient

import (
	"net/http"
	"testing"
)

func TestSetBrowserUserAgent(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	SetBrowserUserAgent(request)
	if got := request.Header.Get("User-Agent"); got != BrowserUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, BrowserUserAgent)
	}
}
