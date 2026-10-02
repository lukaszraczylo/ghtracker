package gh

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	jwtLifetime   = 9 * time.Minute // GitHub caps app JWTs at 10 minutes
	jwtClockSkew  = 60 * time.Second
	tokenSafety   = 5 * time.Minute
	maxAuthBody   = 1 << 20
	installLookup = "/repos/%s/installation"
)

// AppAuth mints GitHub App installation tokens and caches them until shortly before expiry.
type AppAuth struct {
	key          *rsa.PrivateKey
	hc           *http.Client
	now          func() time.Time
	tokens       map[int64]cachedToken
	repoInstalls map[string]int64
	base         string
	appID        int64
	fixedInstall int64
	mu           sync.Mutex
}

type cachedToken struct {
	expires time.Time
	value   string
}

func NewAppAuth(appID int64, keyPEM, baseURL string, installationID int64, hc *http.Client) (*AppAuth, error) {
	key, err := ParsePrivateKey([]byte(keyPEM))
	if err != nil {
		return nil, err
	}
	return &AppAuth{
		appID:        appID,
		key:          key,
		base:         strings.TrimRight(baseURL, "/"),
		hc:           hc,
		fixedInstall: installationID,
		now:          time.Now,
		tokens:       map[int64]cachedToken{},
		repoInstalls: map[string]int64{},
	}, nil
}

// ParsePrivateKey accepts PKCS#1 and PKCS#8 PEM RSA keys.
func ParsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("private key: no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key: unsupported format: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key: not an RSA key")
	}
	return rk, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (a *AppAuth) appJWT() (string, error) {
	now := a.now()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-jwtClockSkew).Unix(),
		"exp": now.Add(jwtLifetime).Unix(),
		"iss": fmt.Sprint(a.appID),
	})
	signing := b64(header) + "." + b64(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("sign app jwt: %w", err)
	}
	return signing + "." + b64(sig), nil
}

// Token returns an installation token valid for repo ("owner/name").
func (a *AppAuth) Token(ctx context.Context, repo string) (string, error) {
	id, err := a.installationID(ctx, repo)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	t, ok := a.tokens[id]
	a.mu.Unlock()
	if ok && a.now().Before(t.expires.Add(-tokenSafety)) {
		return t.value, nil
	}
	return a.refresh(ctx, id)
}

func (a *AppAuth) installationID(ctx context.Context, repo string) (int64, error) {
	if a.fixedInstall != 0 {
		return a.fixedInstall, nil
	}
	a.mu.Lock()
	id, ok := a.repoInstalls[repo]
	a.mu.Unlock()
	if ok {
		return id, nil
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := a.appRequest(ctx, http.MethodGet, fmt.Sprintf(installLookup, repo), &out); err != nil {
		return 0, fmt.Errorf("find installation for %s: %w", repo, err)
	}
	a.mu.Lock()
	a.repoInstalls[repo] = out.ID
	a.mu.Unlock()
	return out.ID, nil
}

func (a *AppAuth) refresh(ctx context.Context, id int64) (string, error) {
	var out struct {
		ExpiresAt time.Time `json:"expires_at"`
		Token     string    `json:"token"`
	}
	if err := a.appRequest(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", id), &out); err != nil {
		return "", fmt.Errorf("create installation token: %w", err)
	}
	a.mu.Lock()
	a.tokens[id] = cachedToken{value: out.Token, expires: out.ExpiresAt}
	a.mu.Unlock()
	return out.Token, nil
}

func (a *AppAuth) appRequest(ctx context.Context, method, path string, out any) error {
	jwt, err := a.appJWT()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAuthBody))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return &APIError{Status: resp.StatusCode, Message: apiMessage(body)}
	}
	return json.Unmarshal(body, out)
}
