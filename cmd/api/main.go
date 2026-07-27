package main

import (
	"log"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/config"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres"
	redisinfra "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/redis"
	transporthttp "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/transport/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error de configuración: %v", err)
	}

	db, err := postgres.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("error conectando a la base de datos: %v", err)
	}
	defer db.Close()

	var locker application.IdempotencyLocker = application.NoopIdempotencyLocker{}
	var summaryCache application.SummaryCache = application.NoopSummaryCache{}
	if cfg.RedisAddr != "" {
		redisClient := redisinfra.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		locker = redisinfra.NewIdempotencyLocker(redisClient)
		summaryCache = redisinfra.NewSummaryCache(redisClient)
		log.Printf("idempotency lock + summary cache: usando Redis en %s", cfg.RedisAddr)
	} else {
		log.Println("Redis no configurado (REDIS_ADDR vacío): sin lock de idempotencia ni cache de resumen")
	}

	merchantRepo := postgres.NewMerchantRepository(db)
	paymentRepo := postgres.NewPaymentRepository(db)
	paymentHistoryRepo := postgres.NewPaymentStatusHistoryRepository(db)
	paymentService := application.NewPaymentService(paymentRepo, merchantRepo, paymentHistoryRepo, locker, summaryCache)
	paymentHandler := transporthttp.NewPaymentHandler(paymentService)

	merchantService := application.NewMerchantService(merchantRepo)
	merchantHandler := transporthttp.NewMerchantHandler(merchantService, paymentService)

	app := transporthttp.NewRouter(merchantHandler, paymentHandler)

	log.Printf("MOVA payments API listening on port %s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("error del servidor: %v", err)
	}
}