package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestAlertHandlerErrorsAndSecretRedaction(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := service.NewAlertService(db, &paths.Paths{DataDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	channel, err := svc.CreateChannel(model.AlertChannelRequest{
		Name: "Failure webhook", Type: model.AlertChannelWebhook, Enabled: true,
		Secret: upstream.URL + "/secret-token", Config: model.AlertChannelConfig{WebhookAllowPrivate: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	handler := NewAlertHandler(svc)
	router := gin.New()
	router.GET("/channels", handler.ListChannels)
	router.POST("/channels", handler.CreateChannel)
	router.POST("/channels/:id/test", handler.TestChannel)
	router.DELETE("/channels/:id", handler.DeleteChannel)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/channels", nil))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "secret-token") || !strings.Contains(list.Body.String(), `"has_secret":true`) {
		t.Fatalf("channel list response = %d %s", list.Code, list.Body.String())
	}
	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/channels", strings.NewReader(`{"name":"","type":"webhook"}`)))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "ALERT_INVALID") {
		t.Fatalf("invalid channel response = %d %s", invalid.Code, invalid.Body.String())
	}
	testSend := httptest.NewRecorder()
	router.ServeHTTP(testSend, httptest.NewRequest(http.MethodPost, "/channels/"+strconv.FormatInt(channel.ID, 10)+"/test", nil))
	if testSend.Code != http.StatusBadGateway || !strings.Contains(testSend.Body.String(), "ALERT_DELIVERY_FAILED") {
		t.Fatalf("test send response = %d %s", testSend.Code, testSend.Body.String())
	}
	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodDelete, "/channels/99999", nil))
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "ALERT_NOT_FOUND") {
		t.Fatalf("missing channel response = %d %s", missing.Code, missing.Body.String())
	}
}
