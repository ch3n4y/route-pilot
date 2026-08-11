package server

import (
	"github.com/gin-gonic/gin"

	"route-manager/internal/routecmd"
)

func (s *Server) hRouteStatus(c *gin.Context) {
	ok(c, s.eng.Status())
}

func (s *Server) hRouteSync(c *gin.Context) {
	ok(c, s.eng.Reconcile())
}

func (s *Server) hRouteActual(c *gin.Context) {
	active, persistent, err := routecmd.ReadRoutes()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, gin.H{"active": active, "persistent": persistent})
}
