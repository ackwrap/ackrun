package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
	"github.com/gin-gonic/gin"
)

func TestNodeShareResponseDisablesCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	nodeService := service.NewNodeService(db)
	imported, err := nodeService.Import(model.NodeImportRequest{Content: "vless://66666666-6666-4666-8666-666666666666@proxy.example:443#Share-Test"})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListNodesBySubscription(imported.SubscriptionID)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("unexpected imported nodes: count=%d err=%v", len(nodes), err)
	}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(nodes[0].ID, 10)}}
	NewNodeHandler(nodeService).Share(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
}
