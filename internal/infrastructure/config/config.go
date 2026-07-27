package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config agrupa la configuración que la aplicación necesita para arrancar.
type Config struct {
	Port        string
	DatabaseURL string

	// RedisAddr vacío significa "Redis no configurado": la app sigue
	// funcionando correctamente, solo sin el lock de idempotencia (ver
	// application.NoopIdempotencyLocker).
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// JWTSecret firma y verifica los tokens emitidos por /auth/login.
	// AuthUsername/AuthPassword son la única credencial de servicio
	// aceptada (no hay tabla de usuarios, ver README).
	JWTSecret     string
	JWTExpiration time.Duration
	AuthUsername  string
	AuthPassword  string
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

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET no está definido")
	}

	authUsername := os.Getenv("AUTH_USERNAME")
	if authUsername == "" {
		return Config{}, fmt.Errorf("AUTH_USERNAME no está definido")
	}

	authPassword := os.Getenv("AUTH_PASSWORD")
	if authPassword == "" {
		return Config{}, fmt.Errorf("AUTH_PASSWORD no está definido")
	}

	jwtExpirationMinutes := 60
	if v := os.Getenv("JWT_EXPIRATION_MINUTES"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("JWT_EXPIRATION_MINUTES inválido: %w", err)
		}
		jwtExpirationMinutes = parsed
	}

	return Config{
		Port:          port,
		DatabaseURL:   databaseURL,
		RedisAddr:     os.Getenv("REDIS_ADDR"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       redisDB,
		JWTSecret:     jwtSecret,
		JWTExpiration: time.Duration(jwtExpirationMinutes) * time.Minute,
		AuthUsername:  authUsername,
		AuthPassword:  authPassword,
	}, nil
}
