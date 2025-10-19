package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"hfbridge/internal/config"
	"hfbridge/internal/logdb"
)

func ListenAndServe(cfg config.Config, store *logdb.Store) error {
	srv := New(cfg, store)
	log.Printf("[hf-proxy] listen %s, defaultUpstream=%s, proxyOrigin=%s\n",
		cfg.BindAddr, cfg.DefaultUpstream, cfg.ProxyOrigin)
	return srv.ListenAndServe()
}

func New(cfg config.Config, store *logdb.Store) *http.Server {
	gin.SetMode(gin.ReleaseMode)

	var initialBytes int64
	var initialCount int64
	if store != nil {
		if summary, err := store.UsageSummary(context.Background()); err != nil {
			log.Printf("[usage-log] preload summary failed: %v", err)
		} else {
			initialBytes = summary.TotalBytes
			initialCount = summary.TotalCount
		}
	}
	stats := newStatsCollector(0, initialBytes, initialCount)

	proxy := newProxyHandler(cfg, store, stats)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			if serveStaticIfExists(c) {
				c.Abort()
				return
			}
		}
		c.Next()
	})

	router.GET("/v1/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.Any("/lfs/content", func(c *gin.Context) {
		proxy.handleLFSContent(c.Writer, c.Request)
		c.Abort()
	})
	router.GET("/v1/usage", usageHandler(store, stats))
	router.GET("/v1/usage/stream", usageStreamHandler(stats, store))

		router.NoRoute(func(c *gin.Context) {
			if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
				if strings.HasPrefix(c.Request.URL.Path, "/_next/image") {
					if serveOptimizedImage(c) {
						return
					}
				}
			}

			proxy.handleReverseProxy(c.Writer, c.Request)
			c.Abort()
		})
	router.NoMethod(func(c *gin.Context) {
		proxy.handleReverseProxy(c.Writer, c.Request)
		c.Abort()
	})

	return &http.Server{
		Addr:              cfg.BindAddr,
		Handler:           router,
		ReadTimeout:       time.Hour,
		WriteTimeout:      time.Hour,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}
}
