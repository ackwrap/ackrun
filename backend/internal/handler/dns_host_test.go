package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

func TestWriteDNSHostError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid", err: service.ErrDNSHostInvalid, wantStatus: http.StatusBadRequest, wantCode: "DNS_HOST_INVALID"},
		{name: "missing", err: service.ErrDNSHostNotFound, wantStatus: http.StatusNotFound, wantCode: "DNS_HOST_NOT_FOUND"},
		{name: "conflict", err: service.ErrDNSHostDomainConflict, wantStatus: http.StatusConflict, wantCode: "DNS_HOST_DOMAIN_CONFLICT"},
		{name: "internal", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantCode: "DNS_HOST_UPDATE_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			writeDNSHostError(context, "DNS_HOST_UPDATE_FAILED", test.err)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if body := response.Body.String(); !strings.Contains(body, `"code":"`+test.wantCode+`"`) {
				t.Fatalf("body = %s, want code %s", body, test.wantCode)
			}
		})
	}
}
