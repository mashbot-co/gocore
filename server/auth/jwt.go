// Package auth handles outbound JWT signing for consumer-issued tokens and
// inbound verification of external IDP tokens (Clerk, WorkOS, Auth0, etc).
//
// The split:
//
//   - SignToken / VerifyToken: round-trip our OWN tokens. The consumer
//     defines whatever claims struct it wants (user_id, project_id, role,
//     tenant_id, workspace_id — whatever) and passes it as a jwt.Claims
//     value. gocore signs with an RSA private key and verifies with the
//     matching public key.
//
//   - VerifyClerkToken (in clerk.go): validates an inbound Clerk session
//     token against Clerk's JWKS. Returns an IDPIdentity (Subject, Email,
//     OrgID) — never our domain user_id, because Clerk doesn't know it.
//     The /auth/sync handler is responsible for materializing the IDP
//     identity into a domain User row.
//
// Key material:
//
//   - JWT_PRIVATE_KEY / JWT_PUBLIC_KEY: base64-encoded PEM blocks. Call
//     Init() at boot to load them. If either is unset, Init() generates an
//     ephemeral RSA-2048 keypair — convenient for local dev but tokens
//     don't survive process restarts.
//   - JWT_ISSUER: optional, defaults to "gocore". Stamped into iss claim.
package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/golang-jwt/jwt/v5"
)

// IDPIdentity is what falls out of an inbound external IDP token (e.g.
// Clerk). It carries identity, not authorization — there is no UserID
// because gocore doesn't know the consumer's user model. The consumer maps
// Subject (and/or Email) onto its own User row in /auth/sync.
//
// Optional profile fields (Name / FirstName / LastName / ImageURL) are
// populated only when the IDP's JWT template includes them; consumers
// should treat them as best-effort and fall back gracefully when empty.
type IDPIdentity struct {
	Subject   string // raw provider user identifier ("user_xxx")
	Email     string
	OrgID     string // raw provider org identifier ("org_xxx"), if present
	Name      string // display name from the `name` claim, if present
	FirstName string // from `first_name`, if present
	LastName  string // from `last_name`, if present
	ImageURL  string // from `image_url` / `picture`, if present
}

var (
	keyMu      sync.RWMutex
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
)

// Init prepares the RSA keypair used to sign and verify outbound consumer
// tokens. Reads JWT_PRIVATE_KEY and JWT_PUBLIC_KEY (base64 PEM) when both
// are set; otherwise generates an ephemeral RSA-2048 keypair so local dev
// can boot without manual key setup. Logs a warning on the ephemeral path.
func Init() error {
	keyMu.Lock()
	defer keyMu.Unlock()

	priv := os.Getenv("JWT_PRIVATE_KEY")
	pub := os.Getenv("JWT_PUBLIC_KEY")

	if priv == "" || pub == "" {
		log.Println("[auth] JWT_PRIVATE_KEY/JWT_PUBLIC_KEY unset — generating ephemeral RSA-2048 keypair; tokens will be invalidated on restart")
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return fmt.Errorf("generate ephemeral key: %w", err)
		}
		privateKey = k
		publicKey = &k.PublicKey
		return nil
	}

	privPEM, err := decodeBase64(priv)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	pubPEM, err := decodeBase64(pub)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}

	privateKey, err = parsePrivateKey(privPEM)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}
	publicKey, err = parsePublicKey(pubPEM)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	return nil
}

// InitKeys is a backwards-compatible alias preserved so existing callers
// (e.g. studio's main.go) don't have to change. Prefer Init() in new code.
func InitKeys() error { return Init() }

// SignToken signs the supplied claims with the configured private key,
// using RS256. The caller chooses the claims shape; embedding
// jwt.RegisteredClaims is recommended so iss/aud/exp/iat are present.
func SignToken(claims jwt.Claims) (string, error) {
	keyMu.RLock()
	pk := privateKey
	keyMu.RUnlock()
	if pk == nil {
		return "", fmt.Errorf("auth: private key not initialized — call auth.Init()")
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(pk)
}

// VerifyToken parses and validates the token against the configured public
// key, decoding into the supplied claims pointer. dest must be a pointer
// to a jwt.Claims implementation matching what was signed; standard
// JWT validation (exp, nbf) runs automatically.
func VerifyToken(tokenString string, dest jwt.Claims) (*jwt.Token, error) {
	keyMu.RLock()
	pk := publicKey
	keyMu.RUnlock()
	if pk == nil {
		return nil, fmt.Errorf("auth: public key not initialized — call auth.Init()")
	}
	tok, err := jwt.ParseWithClaims(tokenString, dest, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pk, nil
	})
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return tok, nil
}

// PublicKey returns the loaded RSA public key. Useful for middleware that
// validates tokens without going through VerifyToken (e.g. when the claim
// type isn't known statically). Returns nil if Init() hasn't run.
func PublicKey() *rsa.PublicKey {
	keyMu.RLock()
	defer keyMu.RUnlock()
	return publicKey
}

// SetKeysForTest installs the supplied keypair, bypassing env loading. Test
// helper. Returns a cleanup func that restores previous state.
func SetKeysForTest(priv *rsa.PrivateKey, pub *rsa.PublicKey) func() {
	keyMu.Lock()
	prevPriv, prevPub := privateKey, publicKey
	privateKey, publicKey = priv, pub
	keyMu.Unlock()
	return func() {
		keyMu.Lock()
		privateKey, publicKey = prevPriv, prevPub
		keyMu.Unlock()
	}
}

// decodeBase64 accepts either standard base64 (with padding) or raw
// (no padding) — both forms appear in the wild for env-supplied PEM.
func decodeBase64(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func parsePrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("not an RSA private key")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func parsePublicKey(pemData []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaKey, nil
}
