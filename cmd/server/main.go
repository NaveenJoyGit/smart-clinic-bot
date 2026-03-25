package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/naveenjoy/smart-clinic-bot/internal/admin"
	"github.com/naveenjoy/smart-clinic-bot/internal/ai"
	"github.com/naveenjoy/smart-clinic-bot/internal/dashboard"
	"github.com/naveenjoy/smart-clinic-bot/internal/config"
	"github.com/naveenjoy/smart-clinic-bot/internal/conversation"
	"github.com/naveenjoy/smart-clinic-bot/internal/db"
	"github.com/naveenjoy/smart-clinic-bot/internal/engine"
	"github.com/naveenjoy/smart-clinic-bot/internal/messaging"
	"github.com/naveenjoy/smart-clinic-bot/internal/notifications"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers/telegram"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers/whatsapp"
	"github.com/naveenjoy/smart-clinic-bot/internal/rag"
	"github.com/redis/go-redis/v9"
)

func main() {
	// 1. Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 2. Config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// 3. Database
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.RunMigrations(ctx, pool); err != nil {
		logger.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	// 4. Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()

	// 5. AI + RAG + Conversation
	aiClient, closeAI, err := ai.New(ctx, cfg)
	if err != nil {
		logger.Error("ai client init failed", "error", err)
		os.Exit(1)
	}
	defer closeAI()

	embedder, closeEmb, err := rag.NewEmbedder(ctx, cfg)
	if err != nil {
		logger.Error("embedder init failed", "error", err)
		os.Exit(1)
	}
	defer closeEmb()

	retriever := rag.NewRetriever(pool, embedder)
	indexer := rag.NewIndexer(pool, embedder)
	convManager := conversation.NewManager(pool, rdb)

	// 6. Bootstrap super_admin if env vars are set
	if cfg.AdminBootstrapEmail != "" && cfg.AdminBootstrapPassword != "" {
		if err := admin.BootstrapSuperAdmin(ctx, pool, cfg.AdminBootstrapEmail, cfg.AdminBootstrapPassword, logger); err != nil {
			logger.Warn("admin bootstrap failed", "error", err)
		}
	}
	if cfg.AdminJWTSecret == "" {
		logger.Warn("ADMIN_JWT_SECRET is not set; admin API will not work correctly")
	}

	// 7. Providers
	tgProvider := telegram.New(cfg.TelegramToken, cfg.DefaultTenantID)
	waProvider := whatsapp.New(cfg.WhatsAppToken, cfg.WhatsAppPhoneID, cfg.WhatsAppVerifyToken, cfg.DefaultTenantID)

	// 8. Messaging handler + notifier
	notifier := notifications.NewNotifier(logger, pool, cfg.TelegramToken, http.DefaultClient, waProvider)
	eng := engine.New(pool, convManager, aiClient, retriever, notifier, logger)
	handler := messaging.NewHandler(convManager, eng, notifier, logger)

	// 9. Admin sub-router
	adminRouter := admin.NewRouter(pool, indexer, cfg.AdminJWTSecret, logger)

	// 10. Dashboard sub-router
	dashboardRouter, err := dashboard.NewRouter(pool, indexer, cfg.AdminJWTSecret, logger)
	if err != nil {
		logger.Error("dashboard router init failed", "error", err)
		os.Exit(1)
	}

	// 11. Router + HTTP server
	router := messaging.NewRouter(handler, pool, tgProvider, waProvider, adminRouter, dashboardRouter, logger)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 11. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	logger.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}
