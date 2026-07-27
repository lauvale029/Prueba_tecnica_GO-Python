package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
)

// summaryCacheTTL es cuánto tiempo vive un resumen guardado antes de
// expirar solo. La invalidación explícita (Invalidate) cubre el caso de
// que un pago cambie de estado antes de que el TTL expire.
const summaryCacheTTL = 30 * time.Second

// SummaryCache implementa application.SummaryCache con Redis: no es la
// fuente de verdad (Postgres lo es), solo evita recalcular el resumen en
// cada petición si nada cambió recientemente.
type SummaryCache struct {
	client *redis.Client
}

func NewSummaryCache(client *redis.Client) *SummaryCache {
	return &SummaryCache{client: client}
}

var _ application.SummaryCache = (*SummaryCache)(nil)

func summaryCacheKey(merchantID string) string {
	return "merchant-summary:" + merchantID
}

func (c *SummaryCache) Get(ctx context.Context, merchantID string) (application.MerchantSummary, bool) {
	data, err := c.client.Get(ctx, summaryCacheKey(merchantID)).Bytes()
	if err != nil {
		// redis.Nil (no había nada guardado) o cualquier otro error de
		// Redis: en ambos casos, es un MISS — quien llama recalculará
		// contra Postgres.
		return application.MerchantSummary{}, false
	}

	var summary application.MerchantSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return application.MerchantSummary{}, false
	}
	return summary, true
}

func (c *SummaryCache) Set(ctx context.Context, merchantID string, summary application.MerchantSummary) {
	data, err := json.Marshal(summary)
	if err != nil {
		return
	}
	c.client.Set(ctx, summaryCacheKey(merchantID), data, summaryCacheTTL)
}

func (c *SummaryCache) Invalidate(ctx context.Context, merchantID string) {
	c.client.Del(ctx, summaryCacheKey(merchantID))
}