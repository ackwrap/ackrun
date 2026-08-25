package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

func TestWriteSSHErrorMapsSingboxDeploymentFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := map[string]int{
		"SSH_SINGBOX_CONFIG_INVALID":  http.StatusBadRequest,
		"SSH_SINGBOX_ROOT_REQUIRED":   http.StatusBadRequest,
		"SSH_SINGBOX_CONFIG_CONFLICT": http.StatusConflict,
		"SSH_SINGBOX_DEPLOY_BUSY":     http.StatusTooManyRequests,
		"SSH_SINGBOX_DEPLOY_FAILED":   http.StatusBadGateway,
		"SSH_SINGBOX_DEPLOY_TIMEOUT":  http.StatusGatewayTimeout,
	}
	for code, expectedStatus := range tests {
		t.Run(code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			writeSSHError(context, &service.SSHServiceError{Code: code, Message: "deployment failed"})
			if recorder.Code != expectedStatus {
				t.Fatalf("unexpected HTTP status: got=%d want=%d", recorder.Code, expectedStatus)
			}
			var response model.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Error.Code != code {
				t.Fatalf("unexpected API error code: %s", response.Error.Code)
			}
		})
	}
}
