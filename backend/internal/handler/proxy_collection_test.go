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

func TestWriteProxyCollectionMutationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid", err: service.ErrProxyCollectionRuleBindingInvalid, wantStatus: http.StatusBadRequest, wantCode: "INVALID_ROUTE_RULE_BINDING"},
		{name: "missing", err: service.ErrProxyCollectionRuleNotFound, wantStatus: http.StatusNotFound, wantCode: "ROUTE_RULE_NOT_FOUND"},
		{name: "conflict", err: service.ErrProxyCollectionRuleBindingConflict, wantStatus: http.StatusConflict, wantCode: "ROUTE_RULE_BINDING_CONFLICT"},
		{name: "collection missing", err: service.ErrProxyCollectionNotFound, wantStatus: http.StatusNotFound, wantCode: "PROXY_COLLECTION_NOT_FOUND"},
		{name: "protected", err: service.ErrSystemProxyCollectionProtected, wantStatus: http.StatusForbidden, wantCode: "SYSTEM_COLLECTION_PROTECTED"},
		{name: "internal", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantCode: "CREATE_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			writeProxyCollectionMutationError(ctx, "CREATE_FAILED", test.err)
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
