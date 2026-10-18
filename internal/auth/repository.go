package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/cache"
)

type Repository interface {
	ValidateKey(ctx context.Context, rawKey string) (*models.APIKey, error)
	CreateKey(ctx context.Context, projectID uuid.UUID, label, scope string) (string, *models.APIKey, error)
	RevokeKey(ctx context.Context, projectID, keyID uuid.UUID) error
	ListKeys(ctx context.Context, projectID uuid.UUID) ([]*models.APIKey, error)
}

type postgresRepository struct {
	pool      *pgxpool.Pool
	cache     *cache.RedisClient
	apiKeyTTL time.Duration
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
		`SELECT ak.id, ak.project_id, ak.key_prefix, ak.label, ak.created_at, ak.revoked_at, ak.scope
		 FROM api_keys ak WHERE ak.key_hash = $1`,
		hash,
	).Scan(&apiKey.ID, &apiKey.ProjectID, &apiKey.KeyPrefix, &apiKey.Label, &apiKey.CreatedAt, &apiKey.RevokedAt, &apiKey.Scope)

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

func (r *postgresRepository) CreateKey(ctx context.Context, projectID uuid.UUID, label, scope string) (string, *models.APIKey, error) {
	rawKey, err := generateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("generate key: %w", err)
	}

	hash := hashKey(rawKey)
	prefix := rawKey[:8]

	apiKey := &models.APIKey{}
	err = r.pool.QueryRow(ctx,
		`INSERT INTO api_keys (project_id, key_hash, key_prefix, label, scope, created_at)
		 VALUES ($1, $2, $3, $4, $5, NOW()) RETURNING id, created_at`,
		projectID, hash, prefix, label, scope,
	).Scan(&apiKey.ID, &apiKey.CreatedAt)

	if err != nil {
		return "", nil, err
	}

	apiKey.ProjectID = projectID
	apiKey.KeyPrefix = prefix
	apiKey.Label = label
	apiKey.Scope = scope

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
		return fmt.Errorf("active api key %w", models.ErrNotFound)
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
	rows, _ := r.pool.Query(ctx,
		`SELECT id, project_id, key_prefix, label, created_at, revoked_at, scope
		 FROM api_keys WHERE project_id = $1 ORDER BY created_at DESC`,
		projectID,
	)
	return pgx.CollectRows(rows, pgx.RowToAddrOfStructByPos[models.APIKey])
}

func generateAPIKey() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	key := make([]byte, 43)
	for i := range key {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		key[i] = charset[n.Int64()]
	}
	return "ot_" + string(key), nil
}

func hashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
