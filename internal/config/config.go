package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL          string
	DBMaxConns           int32
	DBMinConns           int32
	NatsURL              string
	NatsStreamName       string
	NatsMaxBytes         int64
	RedisURL             string
	RedisAPIKeyTTL       time.Duration
	RedisUserCacheTTL    time.Duration
	RedisSegmentCacheTTL time.Duration
	APIPort              int
	APIRateLimit         int
	APIRateWindow        time.Duration
	SegmentInterval      time.Duration
	SchemaFlushInterval  time.Duration
	DLQMaxRetries        int
	EventRetentionDays   int
}

func Load() (*Config, error) {
	cfg := &Config{}

	// Required
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	cfg.DBMaxConns = int32(getEnvInt("DB_MAX_CONNS", 25))
	cfg.DBMinConns = int32(getEnvInt("DB_MIN_CONNS", 5))
	cfg.NatsURL = getEnvStr("NATS_URL", "nats://localhost:4222")
	cfg.NatsStreamName = getEnvStr("NATS_STREAM_NAME", "EVENTS")
	cfg.NatsMaxBytes = int64(getEnvInt("NATS_MAX_BYTES", 1073741824))
	cfg.RedisURL = getEnvStr("REDIS_URL", "redis://localhost:6379/0")
	cfg.RedisAPIKeyTTL = getEnvDuration("REDIS_API_KEY_TTL", 5*time.Minute)
	cfg.RedisUserCacheTTL = getEnvDuration("REDIS_USER_CACHE_TTL", 60*time.Second)
	cfg.RedisSegmentCacheTTL = getEnvDuration("REDIS_SEGMENT_CACHE_TTL", 10*time.Minute)
	cfg.APIPort = getEnvInt("API_PORT", 8080)
	cfg.APIRateLimit = getEnvInt("API_RATE_LIMIT", 100)
	cfg.APIRateWindow = getEnvDuration("API_RATE_WINDOW", 60*time.Second)
	cfg.SegmentInterval = getEnvDuration("SEGMENT_INTERVAL", 5*time.Minute)
	cfg.SchemaFlushInterval = getEnvDuration("SCHEMA_FLUSH_INTERVAL", 30*time.Second)
	cfg.DLQMaxRetries = getEnvInt("DLQ_MAX_RETRIES", 5)
	cfg.EventRetentionDays = getEnvInt("EVENT_RETENTION_DAYS", 90)

	return cfg, nil
}

func getEnvStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
