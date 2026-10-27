package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chat/backend/internal/auth"
	"chat/backend/internal/chat"
	"chat/backend/internal/config"
	"chat/backend/internal/database/migrations"
	"chat/backend/internal/httpapi"
	"chat/backend/internal/observability"
	"chat/backend/internal/provider"
	"chat/backend/internal/repository"
	"chat/backend/internal/storage"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	logger := observability.NewLogger()
	logger.Info("starting_chat_api_service")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration_load_failed", "error", err.Error())
		os.Exit(1)
	}

	// Connect MySQL
	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		logger.Error("mysql_open_failed", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Ping DB with retry for docker startup
	var pingErr error
	for i := 0; i < 5; i++ {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		pingErr = db.PingContext(pingCtx)
		pingCancel()
		if pingErr == nil {
			break
		}
		logger.Warn("waiting_for_mysql", "attempt", i+1, "error", pingErr.Error())
		time.Sleep(2 * time.Second)
	}

	var repo repository.Repository
	if pingErr != nil {
		logger.Warn("mysql_connection_failed_using_memory_repo_fallback", "error", pingErr.Error())
		repo = repository.NewMemoryRepo()
	} else {
		// Run migrations
		migCtx, migCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := migrations.Run(migCtx, db); err != nil {
			logger.Error("migrations_failed", "error", err.Error())
			migCancel()
			os.Exit(1)
		}
		migCancel()
		logger.Info("database_migrations_applied_successfully")
		repo = repository.NewMySQLRepo(db)
	}

	store, err := storage.NewLocalStorage(cfg.StoragePath)
	if err != nil {
		logger.Error("local_storage_init_failed", "error", err.Error())
		os.Exit(1)
	}

	// Provider client
	prov := provider.NewOpenAIProvider(cfg.LLMBaseURL, cfg.LLMAPIKey)

	// Auth & Chat services
	authService := auth.NewAuthService(repo, cfg)
	chatService := chat.NewService(repo, store, prov, cfg, logger)

	// Background worker
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go chatService.RunCleanerWorker(ctx)

	// HTTP Handlers & Router
	handlers := httpapi.NewHandlers(authService, chatService, db, store, logger)
	router := httpapi.NewRouter(handlers, authService, cfg, logger)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Streaming responses manage their own lifecycle
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("server_listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_listen_failed", "error", err.Error())
		}
	}()

	<-ctx.Done()
	logger.Info("shutting_down_gracefully")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server_shutdown_failed", "error", err.Error())
	}
	logger.Info("server_stopped")
}
