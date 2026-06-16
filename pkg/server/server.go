package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/solo/fulltext-search/pkg/index"
)

type Server struct {
	engine       *gin.Engine
	handler      *Handler
	indexManager *index.IndexManager
	port         int
}

func NewServer(im *index.IndexManager, port int) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	handler := NewHandler(im)

	s := &Server{
		engine:       r,
		handler:      handler,
		indexManager: im,
		port:         port,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	v1 := s.engine.Group("/v1")
	{
		health := v1.Group("")
		{
			health.GET("/healthz", s.handler.HealthCheck)
			health.GET("/readyz", s.handler.ReadyCheck)
		}

		indexes := v1.Group("/indexes")
		{
			indexes.GET("", s.handler.ListIndexes)
			indexes.POST("/:name", s.handler.CreateIndex)
			indexes.GET("/:name", s.handler.GetIndex)
			indexes.DELETE("/:name", s.handler.DeleteIndex)
			indexes.POST("/:name/_refresh", s.handler.RefreshIndex)
			indexes.POST("/:name/_forcemerge", s.handler.ForceMerge)

			docs := indexes.Group("/:name/docs")
			{
				docs.GET("/:id", s.handler.GetDocument)
				docs.PUT("/:id", s.handler.PutDocument)
				docs.DELETE("/:id", s.handler.DeleteDocument)
			}

			indexes.POST("/:name/_bulk", s.handler.Bulk)
			indexes.POST("/:name/_search", s.handler.Search)
			indexes.GET("/:name/_search", s.handler.Search)
			indexes.GET("/:name/_suggest", s.handler.Suggest)
		}
	}

	s.engine.GET("/healthz", s.handler.HealthCheck)
	s.engine.GET("/readyz", s.handler.ReadyCheck)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.engine,
	}
	return srv.ListenAndServe()
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}
