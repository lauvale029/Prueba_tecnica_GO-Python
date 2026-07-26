package main

import (
	"log"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/config"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/postgres"
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

	merchantRepo := postgres.NewMerchantRepository(db)
	merchantService := application.NewMerchantService(merchantRepo)
	merchantHandler := transporthttp.NewMerchantHandler(merchantService)

	app := transporthttp.NewRouter(merchantHandler)

	log.Printf("MOVA payments API listening on port %s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("error del servidor: %v", err)
	}
}