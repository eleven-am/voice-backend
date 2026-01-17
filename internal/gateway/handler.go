package gateway

import (
	"github.com/labstack/echo/v4"
)

type Handler struct {
	wsServer *WSServer
}

func NewHandler(wsServer *WSServer) *Handler {
	return &Handler{wsServer: wsServer}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.GET("/ws", h.wsServer.HandleConnection)
}
