package web

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bismigel/tools-site/internal/tool"
)

const (
	testTeam = "team.test"
	testAud  = "aud-123"
)

type jwtSigner struct {
	key *rsa.PrivateKey
	kid string
}

func newSigner(t *testing.T, kid string) *jwtSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &jwtSigner{key: key, kid: kid}
}

func (s *jwtSigner) jwks() []byte {
	pub := s.key.PublicKey
	body := map[string]any{"keys": []map[string]string{{
		"kty": "RSA",
		"kid": s.kid,
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
	out, _ := json.Marshal(body)
	return out
}

func (s *jwtSigner) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "typ": "JWT", "kid": s.kid}
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signing := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func validClaims(now time.Time) map[string]any {
	return map[string]any{
		"aud":   testAud,
		"iss":   "https://" + testTeam,
		"sub":   "user-1",
		"email": "owner@example.com",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
}

type gatedFixture struct {
	ts       *httptest.Server
	signer   *jwtSigner
	hits     *int32
	clock    *time.Time
	rotate   func(*jwtSigner)
	jwksURL  string
	verifier *Verifier
}

func newGatedFixture(t *testing.T) *gatedFixture {
	t.Helper()

	css, err := os.ReadFile("../../theme/tools.css")
	if err != nil {
		t.Fatalf("theme/tools.css missing — run `make css`: %v", err)
	}

	now := time.Now().UTC()
	clock := &now
	signer := newSigner(t, "kid-1")

	var current atomic.Pointer[jwtSigner]
	current.Store(signer)
	hits := new(int32)

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(current.Load().jwks())
	}))
	t.Cleanup(jwks.Close)

	verifier, err := NewVerifier(AccessConfig{
		TeamDomain: testTeam,
		Audience:   testAud,
		JWKSURL:    jwks.URL,
		Now:        func() time.Time { return *clock },
	})
	if err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(reg.Register(textTool()))
	must(reg.Register(tool.Tool{
		Name:       "secret_tool",
		Summary:    "A gated tool",
		Visibility: tool.Gated,
		Inputs:     []tool.Input{{Name: "text", Label: "Text", Kind: tool.Text}},
		Run: func(_ context.Context, in tool.Values) (tool.Result, error) {
			return tool.Result{
				Title: "Secret",
				Lines: []tool.Line{{Kind: tool.LineText, Value: "classified " + in.Get("text")}},
			}, nil
		},
	}))

	srv, err := New(Config{
		Registry:  reg,
		BaseURL:   "https://tools.example",
		Templates: os.DirFS("../.."),
		Static:    os.DirFS("../../web/static"),
		CSS:       css,
		Access:    verifier,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	return &gatedFixture{
		ts: ts, signer: signer, hits: hits, clock: clock, jwksURL: jwks.URL, verifier: verifier,
		rotate: func(s *jwtSigner) { current.Store(s) },
	}
}

func (f *gatedFixture) get(t *testing.T, path, jwt string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if jwt != "" {
		req.Header.Set(accessHeader, jwt)
	}
	res, err := f.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(body)
}

var csrfRE = regexp.MustCompile(`name="_csrf" value="([^"]+)"`)

func TestAccessRequiresAValidJWT(t *testing.T) {
	f := newGatedFixture(t)
	valid := f.signer.sign(t, validClaims(*f.clock))

	t.Run("valid token reaches the gated tool", func(t *testing.T) {
		res, body := f.get(t, "/lab/secret_tool", valid)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if !strings.Contains(body, "secret_tool") {
			t.Error("gated page did not render")
		}
	})

	t.Run("no header is 401", func(t *testing.T) {
		res, _ := f.get(t, "/lab/secret_tool", "")
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("a forged email header is not an identity", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, f.ts.URL+"/lab/secret_tool", nil)
		req.Header.Set("Cf-Access-Authenticated-User-Email", "owner@example.com")
		res, err := f.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d (the email header must never authorise)", res.StatusCode)
		}
	})

	t.Run("the public zone cannot reach a gated tool", func(t *testing.T) {
		res, _ := f.get(t, "/t/secret_tool", valid)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("gated tools stay out of the public manifest", func(t *testing.T) {
		res, body := f.get(t, "/api/tools", valid)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		if strings.Contains(body, "secret_tool") {
			t.Error("gated tool leaked into the public manifest")
		}
	})
}

func TestAccessRejectsBadClaims(t *testing.T) {
	f := newGatedFixture(t)
	now := *f.clock

	cases := map[string]map[string]any{
		"expired": func() map[string]any {
			c := validClaims(now)
			c["exp"] = now.Add(-2 * time.Minute).Unix()
			return c
		}(),
		"wrong audience": func() map[string]any { c := validClaims(now); c["aud"] = "other"; return c }(),
		"wrong issuer":   func() map[string]any { c := validClaims(now); c["iss"] = "https://evil.test"; return c }(),
		"issued later":   func() map[string]any { c := validClaims(now); c["iat"] = now.Add(10 * time.Minute).Unix(); return c }(),
		"no expiry":      func() map[string]any { c := validClaims(now); delete(c, "exp"); return c }(),
	}
	for name, claims := range cases {
		t.Run(name, func(t *testing.T) {
			res, _ := f.get(t, "/lab/secret_tool", f.signer.sign(t, claims))
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d", res.StatusCode)
			}
		})
	}

	t.Run("tampered payload", func(t *testing.T) {
		token := f.signer.sign(t, validClaims(now))
		parts := strings.Split(token, ".")
		payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
		payload = []byte(strings.Replace(string(payload), "user-1", "user-2", 1))
		parts[1] = base64.RawURLEncoding.EncodeToString(payload)
		res, _ := f.get(t, "/lab/secret_tool", strings.Join(parts, "."))
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d", res.StatusCode)
		}
	})

	t.Run("unknown kid is 401 and does not hammer JWKS", func(t *testing.T) {
		before := atomic.LoadInt32(f.hits)
		other := newSigner(t, "kid-unknown")
		res, _ := f.get(t, "/lab/secret_tool", other.sign(t, validClaims(now.Add(time.Minute))))
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d", res.StatusCode)
		}
		after := atomic.LoadInt32(f.hits)
		if after-before > 1 {
			t.Errorf("JWKS fetched %d times for one unknown kid", after-before)
		}
	})

	t.Run("key rotation is picked up after the cache floor", func(t *testing.T) {
		rotated := newSigner(t, "kid-2")
		f.rotate(rotated)
		// The old key is still cached; the new kid needs a refresh, which is
		// rate-limited to once a minute, so move the clock past that.
		*f.clock = f.clock.Add(2 * time.Minute)

		res, _ := f.get(t, "/lab/secret_tool", rotated.sign(t, validClaims(*f.clock)))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d after rotation", res.StatusCode)
		}
	})
}

func TestCSRFOnGatedPosts(t *testing.T) {
	f := newGatedFixture(t)
	valid := f.signer.sign(t, validClaims(*f.clock))

	res, page := f.get(t, "/lab/secret_tool", valid)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("page status = %d", res.StatusCode)
	}
	m := csrfRE.FindStringSubmatch(page)
	if m == nil {
		t.Fatal("gated page has no CSRF token")
	}
	token := m[1]

	post := func(field, header string, jwt string) *http.Response {
		form := url.Values{}
		if field != "" {
			form.Set("_csrf", field)
		}
		form.Set("text", "hello")
		req, err := http.NewRequest(http.MethodPost, f.ts.URL+"/lab/secret_tool",
			strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if header != "" {
			req.Header.Set("X-Tools-CSRF", header)
		}
		if jwt != "" {
			req.Header.Set(accessHeader, jwt)
		}
		res, err := f.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	if got := post("", "", valid).StatusCode; got != http.StatusForbidden {
		t.Errorf("no token = %d, want 403", got)
	}
	if got := post("", "wrong-token", valid).StatusCode; got != http.StatusForbidden {
		t.Errorf("bad header token = %d, want 403", got)
	}
	if got := post(token, "", valid).StatusCode; got != http.StatusOK {
		t.Errorf("form token = %d, want 200", got)
	}
	if got := post("", token, valid).StatusCode; got != http.StatusOK {
		t.Errorf("header token = %d, want 200", got)
	}

	// A token minted for a different session must not work.
	other := f.signer.sign(t, func() map[string]any {
		c := validClaims(*f.clock)
		c["sub"] = "someone-else"
		return c
	}())
	if got := post(token, "", other).StatusCode; got != http.StatusForbidden {
		t.Errorf("token from another session = %d, want 403", got)
	}
}

func TestAccessBypassOnlyOutsideCluster(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	if _, err := NewVerifier(AccessConfig{Bypass: true}); err == nil {
		t.Fatal("bypass must refuse to start inside Kubernetes")
	}

	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	v, err := NewVerifier(AccessConfig{Bypass: true})
	if err != nil {
		t.Fatal(err)
	} // Outside the cluster it is allowed, and it does not verify anything.
	if v.cfg.Bypass != true {
		t.Fatal("bypass not recorded")
	}
}

func TestAccessConfigValidation(t *testing.T) {
	if _, err := NewVerifier(AccessConfig{TeamDomain: "", Audience: testAud}); err == nil {
		t.Fatal("expected an error without a team domain")
	}
	if _, err := NewVerifier(AccessConfig{TeamDomain: testTeam, Audience: ""}); err == nil {
		t.Fatal("expected an error without an audience")
	}
	cfg := AccessConfig{TeamDomain: testTeam, Audience: testAud}
	v, err := NewVerifier(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://" + testTeam + "/cdn-cgi/access/certs"; v.cfg.JWKSURL != want {
		t.Fatalf("JWKSURL = %q, want %q", v.cfg.JWKSURL, want)
	}
	if got := v.issuer(); got != "https://"+testTeam {
		t.Fatalf("issuer = %q", got)
	}
}
