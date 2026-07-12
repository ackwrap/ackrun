package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackwrap/internal/service"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type RealtimeHandler struct {
	svc       *service.RealtimeService
	runtime   *service.RuntimeService
	installer *service.InstallerService
	config    *service.ConfigService
	singbox   *service.SingboxService
}

func NewRealtimeHandler(
	svc *service.RealtimeService,
	rt *service.RuntimeService,
	inst *service.InstallerService,
	cfg *service.ConfigService,
	sb *service.SingboxService,
) *RealtimeHandler {
	return &RealtimeHandler{
		svc:       svc,
		runtime:   rt,
		installer: inst,
		config:    cfg,
		singbox:   sb,
	}
}

func (h *RealtimeHandler) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	h.svc.AddClient(conn)
	defer h.svc.RemoveClient(conn)

	h.pushInitialState(conn)

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *RealtimeHandler) pushInitialState(conn *websocket.Conn) {
	var runtimeStatus string
	if rt, err := h.runtime.GetStatus(); err == nil {
		runtimeStatus = string(rt.Status)
		conn.WriteJSON(map[string]any{
			"type": "runtime.status",
			"time": 0,
			"data": rt,
		})
	}
	if inst, err := h.installer.GetStatus(); err == nil {
		conn.WriteJSON(map[string]any{
			"type": "installer.status",
			"time": 0,
			"data": inst,
		})
	}
	if runtimeStatus != "not_installed" {
		if status, err := h.config.GetConfigStatus(); err == nil {
			conn.WriteJSON(map[string]any{
				"type": "config.status",
				"time": 0,
				"data": status,
			})
		}
	}
	pid := h.singbox.GetPID()
	if pid > 0 {
		conn.WriteJSON(map[string]any{
			"type": "core.status",
			"time": 0,
			"data": map[string]any{
				"status": "running",
				"pid":    pid,
			},
		})
	} else if runtimeStatus != "not_installed" && runtimeStatus != "no_config" {
		conn.WriteJSON(map[string]any{
			"type": "core.status",
			"time": 0,
			"data": map[string]any{
				"status": "stopped",
				"pid":    0,
			},
		})
	}
}
