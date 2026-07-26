package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config agrupa la configuración que la aplicación necesita para arrancar.
// Se amplía en fases futuras (JWT) a medida que se usen.
type Config struct {
	Port        string
	DatabaseURL string

	// RedisAddr vacío significa "Redis no configurado": la app sigue
	// funcionando correctamente, solo sin el lock de idempotencia (ver
	// application.NoopIdempotencyLocker).
	RedisAddr     string
	RedisPassword string
	RedisDB       int
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

	redisDB := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("REDIS_DB inválido: %w", err)
		}
		redisDB = parsed
	}

	return Config{
		Port:          port,
		DatabaseURL:   databaseURL,
		RedisAddr:     os.Getenv("REDIS_ADDR"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDB,
	}, nil
}
