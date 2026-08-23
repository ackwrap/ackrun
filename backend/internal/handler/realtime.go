package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
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

	if !h.svc.AddClientWithInitialState(conn, h.initialState) {
		return
	}
	defer h.svc.RemoveClient(conn)

	for {
		var command model.WSCommand
		if err := conn.ReadJSON(&command); err != nil {
			break
		}
		h.svc.HandleCommand(conn, command)
	}
}

func (h *RealtimeHandler) initialState() []model.WSEvent {
	events := make([]model.WSEvent, 0, 4)
	var runtimeStatus string
	if rt, err := h.runtime.GetStatus(); err == nil {
		runtimeStatus = string(rt.Status)
		events = append(events, model.WSEvent{Type: "runtime.status", Time: 0, Data: rt})
	}
	if inst, err := h.installer.GetStatus(); err == nil {
		events = append(events, model.WSEvent{Type: "installer.status", Time: 0, Data: inst})
	}
	if runtimeStatus != "not_installed" {
		if status, err := h.config.GetConfigStatus(); err == nil {
			events = append(events, model.WSEvent{Type: "config.status", Time: 0, Data: status})
		}
	}
	pid := h.singbox.GetPID()
	if pid > 0 {
		events = append(events, model.WSEvent{
			Type: "core.status",
			Time: 0,
			Data: map[string]any{
				"status": "running",
				"pid":    pid,
			},
		})
	} else if runtimeStatus != "not_installed" && runtimeStatus != "no_config" {
		events = append(events, model.WSEvent{
			Type: "core.status",
			Time: 0,
			Data: map[string]any{
				"status": "stopped",
				"pid":    0,
			},
		})
	}
	return events
}
