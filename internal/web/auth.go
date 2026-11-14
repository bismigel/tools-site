package web

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	accessHeader   = "Cf-Access-Jwt-Assertion"
	jwksCacheTTL   = time.Hour
	csrfBucketSecs = 600 // 10 minutes, two buckets accepted
	clockLeeway    = 60 * time.Second
)

// AccessConfig is what main reads from the environment. TeamDomain and Audience
// come from the ConfigMap; neither is a secret.
type AccessConfig struct {
	TeamDomain string
	Audience   string
	JWKSURL    string // default https://<team>/cdn-cgi/access/certs
	HTTPClient *http.Client
	Now        func() time.Time
	Bypass     bool // TOOLS_DEV_INSECURE=1; refuses to start in-cluster
}

type identity struct {
	Sub   string
	Email string
	JWT   string
}

type identityKey struct{}

func identityFrom(ctx context.Context) (identity, bool) {
	id, ok := ctx.Value(identityKey{}).(identity)
	return id, ok
}

// Verifier checks Cloudflare Access JWTs in-process. The unsigned
// Cf-Access-Authenticated-User-Email header is never consulted: any pod in the
// cluster can set it (spec §7.2).
type Verifier struct {
	cfg    AccessConfig
	client *http.Client
	now    func() time.Time

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	fetched     time.Time
	lastAttempt time.Time
	fetches     int
}

// NewVerifier validates configuration and returns a ready verifier.
func NewVerifier(cfg AccessConfig) (*Verifier, error) {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	if cfg.Bypass {
		// A dev build must never be deployable by accident (NFR-8.2 / FR-4.4).
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			return nil, errors.New("TOOLS_DEV_INSECURE is set inside Kubernetes; refusing to start")
		}
		return &Verifier{cfg: cfg, client: client, now: now}, nil
	}

	if cfg.TeamDomain == "" || cfg.Audience == "" {
		return nil, errors.New("access: team domain and audience are required")
	}
	if cfg.JWKSURL == "" {
		cfg.JWKSURL = "https://" + cfg.TeamDomain + "/cdn-cgi/access/certs"
	}
	return &Verifier{cfg: cfg, client: client, now: now, keys: make(map[string]*rsa.PublicKey)}, nil
}

func (v *Verifier) issuer() string { return "https://" + v.cfg.TeamDomain }

// Middleware rejects anything without a valid Access JWT.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v.cfg.Bypass {
			ctx := context.WithValue(r.Context(), identityKey{}, identity{
				Sub: "dev", Email: "dev@localhost",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		raw := r.Header.Get(accessHeader)
		if raw == "" {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		claims, err := v.verify(r.Context(), raw)
		if err != nil {
			http.Error(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), identityKey{}, identity{
			Sub: claims.Sub, Email: claims.Email, JWT: raw,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type accessClaims struct {
	Aud   json.RawMessage `json:"aud"`
	Exp   int64           `json:"exp"`
	Iat   int64           `json:"iat"`
	Iss   string          `json:"iss"`
	Sub   string          `json:"sub"`
	Email string          `json:"email"`
}

func (v *Verifier) verify(ctx context.Context, raw string) (accessClaims, error) {
	var claims accessClaims

	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return claims, errors.New("malformed token")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, errors.New("bad header encoding")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return claims, errors.New("bad header")
	}
	if header.Alg != "RS256" {
		return claims, fmt.Errorf("unsupported alg %q", header.Alg)
	}

	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return claims, err
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return claims, errors.New("bad signature encoding")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return claims, errors.New("bad signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, errors.New("bad payload encoding")
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, errors.New("bad claims")
	}

	now := v.now()
	if claims.Exp == 0 || now.After(time.Unix(claims.Exp, 0).Add(clockLeeway)) {
		return claims, errors.New("expired")
	}
	if claims.Iat != 0 && time.Unix(claims.Iat, 0).After(now.Add(clockLeeway)) {
		return claims, errors.New("issued in the future")
	}
	if claims.Iss != v.issuer() {
		return claims, fmt.Errorf("wrong issuer %q", claims.Iss)
	}
	if !audienceContains(claims.Aud, v.cfg.Audience) {
		return claims, errors.New("wrong audience")
	}
	return claims, nil
}

func audienceContains(raw json.RawMessage, want string) bool {
	if len(raw) == 0 {
		return false
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == want
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return false
	}
	for _, a := range many {
		if a == want {
			return true
		}
	}
	return false
}

// key returns a public key, refreshing JWKS when the kid is unknown (at most
// once per minute, so a hostile token cannot turn us into a JWKS hammer).
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	if v.fetched.IsZero() || v.now().Sub(v.fetched) > jwksCacheTTL || v.fetches == 0 {
		if err := v.fetchLocked(ctx); err != nil {
			return nil, err
		}
		if k, ok := v.keys[kid]; ok {
			return k, nil
		}
	}
	if v.now().Sub(v.lastAttempt) < time.Minute {
		return nil, fmt.Errorf("unknown key %q", kid)
	}
	if err := v.fetchLocked(ctx); err != nil {
		return nil, err
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown key %q", kid)
}

func (v *Verifier) fetchLocked(ctx context.Context) error {
	v.lastAttempt = v.now()
	v.fetches++

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: status %d", res.StatusCode)
	}

	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}
	if len(keys) == 0 {
		return errors.New("jwks: no usable keys")
	}
	v.keys = keys
	v.fetched = v.now()
	return nil
}

// CSRF ----------------------------------------------------------------
//
// The token is HMAC-SHA256 keyed by the Access JWT itself, over a 10-minute
// bucket. Two consequences, both wanted:
//
//   - no per-process secret, so any replica can verify a form rendered by any
//     other replica (a random process secret would break behind a Service);
//   - a cross-site attacker cannot compute it, because it cannot read the JWT:
//     Cloudflare adds it as a request header, never as something page script can
//     see.
//
// This replaces the "HMAC(session id, per-process secret)" sketch in the spec.
func (v *Verifier) csrfToken(jwt string, bucket int64) string {
	mac := hmac.New(sha256.New, jwtKey(jwt))
	mac.Write([]byte("csrf|" + strconv.FormatInt(bucket, 10)))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

func jwtKey(jwt string) []byte {
	sum := sha256.Sum256([]byte(jwt))
	return sum[:]
}

// CSRFFor returns the token to render into a gated page.
func (v *Verifier) CSRFFor(r *http.Request) string {
	id, ok := identityFrom(r.Context())
	if !ok || id.JWT == "" {
		return ""
	}
	return v.csrfToken(id.JWT, v.now().Unix()/csrfBucketSecs)
}

// CheckCSRF validates the header or the hidden form field, accepting the current
// and previous bucket so a form does not expire mid-fill.
func (v *Verifier) CheckCSRF(r *http.Request, formValue string) bool {
	id, ok := identityFrom(r.Context())
	if !ok {
		return false
	}
	if id.JWT == "" {
		return true // dev bypass: no credential to forge
	}

	got := r.Header.Get("X-Tools-CSRF")
	if got == "" {
		got = formValue
	}
	if got == "" {
		return false
	}

	bucket := v.now().Unix() / csrfBucketSecs
	for _, b := range []int64{bucket, bucket - 1} {
		want := v.csrfToken(id.JWT, b)
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
			return true
		}
	}
	return false
}
