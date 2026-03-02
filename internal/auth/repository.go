package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/cache"
)

type Repository interface {
	ValidateKey(ctx context.Context, rawKey string) (*models.APIKey, error)
	CreateKey(ctx context.Context, projectID uuid.UUID, label string) (string, *models.APIKey, error)
	RevokeKey(ctx context.Context, projectID, keyID uuid.UUID) error
	ListKeys(ctx context.Context, projectID uuid.UUID) ([]*models.APIKey, error)
}

type postgresRepository struct {
	pool       *pgxpool.Pool
	cache      *cache.RedisClient
	apiKeyTTL  time.Duration
}

func NewRepository(pool *pgxpool.Pool, cache *cache.RedisClient, apiKeyTTL time.Duration) Repository {
	return &postgresRepository{
		pool:      pool,
		cache:     cache,
		apiKeyTTL: apiKeyTTL,
	}
}

func (r *postgresRepository) ValidateKey(ctx context.Context, rawKey string) (*models.APIKey, error) {
	hash := hashKey(rawKey)

	// Check Redis first
	if r.cache != nil {
		cacheKey := fmt.Sprintf("apikey:%s", hash)
		cached, err := r.cache.Get(ctx, cacheKey)
		if err == nil && cached != "" {
			var apiKey models.APIKey
			if err := json.Unmarshal([]byte(cached), &apiKey); err == nil {
				return &apiKey, nil
			}
		}
	}

	// Fallback to DB
	apiKey := &models.APIKey{}
	err := r.pool.QueryRow(ctx,
		`SELECT ak.id, ak.project_id, ak.key_prefix, ak.label, ak.created_at, ak.revoked_at
		 FROM api_keys ak WHERE ak.key_hash = $1`,
		hash,
	).Scan(&apiKey.ID, &apiKey.ProjectID, &apiKey.KeyPrefix, &apiKey.Label, &apiKey.CreatedAt, &apiKey.RevokedAt)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Cache the result
	if r.cache != nil && apiKey.RevokedAt == nil {
		data, _ := json.Marshal(apiKey)
		cacheKey := fmt.Sprintf("apikey:%s", hash)
		r.cache.Set(ctx, cacheKey, string(data), r.apiKeyTTL)
	}

	return apiKey, nil
}

func (r *postgresRepository) CreateKey(ctx context.Context, projectID uuid.UUID, label string) (string, *models.APIKey, error) {
	// Generate random key: ot_ + 43 base62 chars
	rawKey, err := generateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("generate key: %w", err)
	}

	hash := hashKey(rawKey)
	prefix := rawKey[:8]

	apiKey := &models.APIKey{}
	err = r.pool.QueryRow(ctx,
		`INSERT INTO api_keys (project_id, key_hash, key_prefix, label, created_at)
		 VALUES ($1, $2, $3, $4, NOW()) RETURNING id, created_at`,
		projectID, hash, prefix, label,
	).Scan(&apiKey.ID, &apiKey.CreatedAt)

	if err != nil {
		return "", nil, err
	}

	apiKey.ProjectID = projectID
	apiKey.KeyPrefix = prefix
	apiKey.Label = label

	return rawKey, apiKey, nil
}

func (r *postgresRepository) RevokeKey(ctx context.Context, projectID, keyID uuid.UUID) error {
	// Get key hash before revoking (for cache invalidation)
	var keyHash string
	err := r.pool.QueryRow(ctx,
		`UPDATE api_keys SET revoked_at = NOW()
		 WHERE id = $1 AND project_id = $2 AND revoked_at IS NULL
		 RETURNING key_hash`,
		keyID, projectID,
	).Scan(&keyHash)

	if err == pgx.ErrNoRows {
		return fmt.Errorf("key not found or already revoked")
	}
	if err != nil {
		return err
	}

	// Remove from cache
	if r.cache != nil {
		cacheKey := fmt.Sprintf("apikey:%s", keyHash)
		r.cache.Delete(ctx, cacheKey)
	}

	return nil
}

func (r *postgresRepository) ListKeys(ctx context.Context, projectID uuid.UUID) ([]*models.APIKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, project_id, key_prefix, label, created_at, revoked_at
		 FROM api_keys WHERE project_id = $1 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		k := &models.APIKey{}
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.KeyPrefix, &k.Label, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func generateAPIKey() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Encode to base62-like string
	encoded := base64.RawURLEncoding.EncodeToString(b)
	// Trim to 43 chars and prepend prefix
	if len(encoded) > 43 {
		encoded = encoded[:43]
	}
	return "ot_" + encoded, nil
}

func hashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
