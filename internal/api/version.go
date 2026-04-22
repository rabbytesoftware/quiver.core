package api

import "github.com/gin-gonic/gin"

// Version is the contract every API version must implement.
// Prefix returns the route group prefix (e.g. "/v0").
// Register mounts all REST and WebSocket routes for the version.
// WSHandler returns the version's WS handler so the hub can fan out domain events.
type Version interface {
	Prefix() string
	Register(rg *gin.RouterGroup)
	WSHandler() WSVersion
}
