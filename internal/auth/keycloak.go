package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type KeycloakVerifier struct {
	issuer     string
	jwksURL    string
	audience   string
	httpClient *http.Client
	mu         sync.RWMutex
	cache      jwksCache
}

type jwksCache struct {
	keys      map[string]*rsa.PublicKey
	expiresAt time.Time
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

func NewKeycloakVerifierFromEnv() (*KeycloakVerifier, error) {
	issuer := strings.TrimSpace(os.Getenv("KEYCLOAK_ISSUER"))
	if issuer == "" {
		return nil, fmt.Errorf("KEYCLOAK_ISSUER not set")
	}

	jwksURL := strings.TrimSpace(os.Getenv("KEYCLOAK_JWKS_URL"))
	if jwksURL == "" {
		jwksURL = strings.TrimRight(issuer, "/") + "/protocol/openid-connect/certs"
	}

	audience := strings.TrimSpace(os.Getenv("KEYCLOAK_AUDIENCE"))

	return &KeycloakVerifier{
		issuer:     issuer,
		jwksURL:    jwksURL,
		audience:   audience,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		cache: jwksCache{
			keys: make(map[string]*rsa.PublicKey),
		},
	}, nil
}

func (v *KeycloakVerifier) VerifyToken(ctx context.Context, tokenStr string) (jwt.MapClaims, error) {
	if strings.TrimSpace(tokenStr) == "" {
		return nil, errors.New("token is empty")
	}

	parserOptions := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"RS256"}),
	}
	if v.issuer != "" {
		parserOptions = append(parserOptions, jwt.WithIssuer(v.issuer))
	}
	if v.audience != "" {
		parserOptions = append(parserOptions, jwt.WithAudience(v.audience))
	}

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token header missing kid")
		}
		return v.getKey(ctx, kid)
	}, parserOptions...)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}

func (v *KeycloakVerifier) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	now := time.Now()

	v.mu.RLock()
	key := v.cache.keys[kid]
	cacheValid := now.Before(v.cache.expiresAt)
	v.mu.RUnlock()

	if key != nil && cacheValid {
		return key, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	key = v.cache.keys[kid]
	if key == nil {
		return nil, fmt.Errorf("no matching jwk for kid %s", kid)
	}
	return key, nil
}

func (v *KeycloakVerifier) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("build jwks request: %w", err)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetch jwks returned status %d", resp.StatusCode)
	}

	var payload jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, key := range payload.Keys {
		if key.Kty != "RSA" || key.Kid == "" || key.N == "" || key.E == "" {
			continue
		}
		pubKey, err := parseRSAPublicKey(key.N, key.E)
		if err != nil {
			continue
		}
		keys[key.Kid] = pubKey
	}

	if len(keys) == 0 {
		return errors.New("no valid RSA keys found in jwks")
	}

	v.mu.Lock()
	v.cache.keys = keys
	v.cache.expiresAt = time.Now().Add(10 * time.Minute)
	v.mu.Unlock()

	return nil
}

func parseRSAPublicKey(n, e string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}

	eInt := 0
	for _, b := range eBytes {
		eInt = eInt*256 + int(b)
	}
	if eInt == 0 {
		return nil, errors.New("invalid exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: eInt,
	}, nil
}
