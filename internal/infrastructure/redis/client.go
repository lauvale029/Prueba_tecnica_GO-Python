package redis

import "github.com/redis/go-redis/v9"

// NewClient crea un cliente de Redis. La conexión real ocurre de forma
// perezosa en el primer comando (igual que database/sql), así que esta
// función nunca falla por sí sola: si Redis no está disponible, el
// primer intento de usarlo simplemente devolverá un error, que
// IdempotencyLocker.Acquire trata como "no conseguí el lock".
func NewClient(addr, password string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
}