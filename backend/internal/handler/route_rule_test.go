package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

func TestWriteRouteRuleMutationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "missing", err: service.ErrRouteRuleNotFound, wantStatus: http.StatusNotFound, wantCode: "ROUTE_RULE_NOT_FOUND"},
		{name: "conflict", err: service.ErrRouteRuleNameConflict, wantStatus: http.StatusConflict, wantCode: "ROUTE_RULE_NAME_CONFLICT"},
		{name: "protected", err: service.ErrSystemRouteRuleProtected, wantStatus: http.StatusForbidden, wantCode: "SYSTEM_RULE_PROTECTED"},
		{name: "fallback", err: errors.New("invalid rule"), wantStatus: http.StatusBadRequest, wantCode: "ROUTE_RULE_UPDATE_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			writeRouteRuleMutationError(ctx, http.StatusBadRequest, "ROUTE_RULE_UPDATE_FAILED", test.err)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			var body model.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", body.Error.Code, test.wantCode)
			}
		})
	}
}
