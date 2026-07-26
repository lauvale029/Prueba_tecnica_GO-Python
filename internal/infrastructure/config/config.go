package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config agrupa la configuración que la aplicación necesita para arrancar.
// Se amplía en fases futuras (Redis, JWT) a medida que se usen.
type Config struct {
	Port        string
	DatabaseURL string
}

// Load lee las variables de entorno necesarias. Si existe un archivo
// .env en el directorio actual, lo carga primero
func Load() (Config, error) {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL no está definido")
	}

	return Config{
		Port:        port,
		DatabaseURL: databaseURL,
	}, nil
}
