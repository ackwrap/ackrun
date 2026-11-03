package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestControlSummaryAPIWorksWithoutInstalledCoreAndOmitsCredentials(t *testing.T) {
	app := testApplication(t)
	defer app.Close()
	if _, err := app.store.CreateSubscription(&model.SubscriptionRequest{Name: "synthetic", URL: "https://example.invalid/private-token", UserAgent: "synthetic-user-agent"}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/runtime/summary", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("summary HTTP status = %d", recorder.Code)
	}
	var summary model.ControlSummary
	if err := json.Unmarshal(recorder.Body.Bytes(), &summary); err != nil || summary.SubscriptionCount != 1 || summary.MonthlyTraffic.Month == "" {
		t.Fatalf("summary decode/count failed: %v", err)
	}
	if strings.Contains(recorder.Body.String(), "private-token") || strings.Contains(recorder.Body.String(), "synthetic-user-agent") {
		t.Fatal("summary leaked subscription credentials")
	}
}
