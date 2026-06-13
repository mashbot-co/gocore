// Package server is the opinionated "one-call-and-you're-running" entry
// point for gocore-based APIs. Consumers write a tiny main() that fills in
// a Config struct and calls Run; everything else — env loading, SSM,
// gocore.Init, database, JWT key init, gin router assembly, middleware
// wiring, GraphQL handler, authz field middleware, dev-vs-release mode,
// Lambda detection — happens inside Run.
//
// The intent is: every iro/mashbot/<future> API runs the same way. If you
// need to customise something Server.Run doesn't expose, the right path is
// to add a hook to this Config and have every consumer benefit — not to
// fork main.go.
//
// Minimal consumer:
//
//	func main() {
//	    server.Run(server.Config{
//	        Name:               "iro-studio",
//	        ScopeName:          "project",
//	        RoutePrefix:        "/v1/core",
//	        BuildSchema:        buildSchema,
//	        AuthCatalog:        policy.MustLoad(),
//	        EndpointPolicyYAML: authYAML,
//	        NewRoleResolver:    iroapi.MakeRoleResolver,
//	        NewClaims:          func() jwt.Claims { return &iroapi.Claims{} },
//	        LiftClaims:         liftClaims,
//	        RegisterREST:       auth.RegisterRoutes,
//	    })
//	}
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"github.com/mashbot-co/gocore"
	gocoreauth "github.com/mashbot-co/gocore/server/auth"
	"github.com/mashbot-co/gocore/server/authz"
	"github.com/mashbot-co/gocore/config"
	"github.com/mashbot-co/gocore/db/connection"
	"github.com/mashbot-co/gocore/server/middleware"
)

// Config fully describes a gocore-based API. Most fields are required;
// defaults are documented where they apply.
type Config struct {
	// ── Identity (passed through to gocore.Init) ─────────────────────────

	// Name is the consumer's short identifier (e.g. "iro-studio"). Required.
	Name string

	// StubHeaderPrefix overrides the default "X-Stub-" used in dev-mode
	// stub-auth headers. Optional.
	StubHeaderPrefix string

	// JWTIssuer overrides the `iss` claim default (Name). Optional.
	JWTIssuer string

	// ScopeName names the per-request scope concept ("project", "tenant").
	// Drives derived JWT claim names and arg names. Optional — leave empty
	// for user-only APIs with no scope context.
	ScopeName string

	// ── Network ──────────────────────────────────────────────────────────

	// Port defaults to "8002" when empty. Ignored under Lambda.
	Port string

	// RoutePrefix is the mount point for both REST and GraphQL groups
	// (e.g. "/v1/core" → /v1/core/rest/health, /v1/core/graphql). Required.
	RoutePrefix string

	// ── GraphQL ──────────────────────────────────────────────────────────

	// BuildSchema constructs the gqlgen executable schema given the
	// initialised database. The consumer wires its Resolver type here.
	// Required.
	BuildSchema func(db *gorm.DB) graphql.ExecutableSchema

	// ── Authorization ────────────────────────────────────────────────────

	// AuthCatalog is the shared scope catalog (typically loaded via
	// iro/shared/policy or its consumer equivalent). Required when the
	// API exposes any authenticated endpoints (most cases).
	AuthCatalog authz.Policy

	// EndpointPolicyYAML is the raw auth.yml content. Required when
	// AuthCatalog is set.
	EndpointPolicyYAML []byte

	// NewRoleResolver constructs the per-request role-resolver closure
	// given the DB. The role resolver queries the consumer's Membership /
	// TenantUser / equivalent join table. Required when AuthCatalog is set.
	NewRoleResolver func(db *gorm.DB) authz.RoleResolver

	// ── Claims (production JWT auth) ─────────────────────────────────────

	// NewClaims returns a freshly-allocated zero claims pointer per
	// request. gocore can't statically know the consumer's Claims type;
	// the factory lets it allocate without reflection. Required.
	NewClaims func() jwt.Claims

	// LiftClaims is called after gocore verifies the token and parses into
	// the claims allocated by NewClaims. It maps the consumer's specific
	// claim fields onto gin context keys (user_id, project_id, is_admin,
	// etc.) so downstream middleware can read them. Required.
	LiftClaims func(c *gin.Context, parsed jwt.Claims)

	// ── Custom endpoints ─────────────────────────────────────────────────

	// RegisterREST is an optional hook to attach custom REST endpoints
	// (e.g. /auth/sync). Mounted under <RoutePrefix>/rest and runs in the
	// unauthenticated group — handlers are responsible for any in-handler
	// auth they need (sync verifies its own Clerk JWT, for instance).
	RegisterREST func(r *gin.RouterGroup, db *gorm.DB)

	// EnrichUserContext is an optional hook that runs after authentication
	// (JWTAuth in release, OptionalAuth in dev) and before InjectDBContext.
	// Use it to fill in context attributes that the JWT path provided but
	// the stub path didn't — e.g. fetching the user's IsAdmin flag from
	// the database when only a user_id stub header is present. In release
	// mode, where liftClaims has already populated everything from the
	// token, this should typically be a no-op.
	EnrichUserContext func(c *gin.Context, db *gorm.DB)

	// PostEnvLoad runs after .env and SSM have populated the process
	// environment but before any middleware is built. Use it to derive
	// env vars from already-loaded values — for example, composing
	// CORS_ALLOWED_ORIGINS from a product-specific subdomain layout
	// (www., app., …) without duplicating the .env-walking logic. An
	// explicit env value should win; the hook should no-op when the
	// derived target is already set.
	PostEnvLoad func()
}

// Run is the single entry point — handles every step the API needs to
// stand up. Call from main() and don't return. log.Fatalf on any boot
// failure (missing required config, DB connection, key init, etc.).
func Run(cfg Config) {
	cfg.validate()
	cfg.applyDefaults()

	loadEnvFile()
	config.MustLoadFromSSM(context.Background())

	if cfg.PostEnvLoad != nil {
		cfg.PostEnvLoad()
	}

	gocore.Init(gocore.Config{
		Name:             cfg.Name,
		StubHeaderPrefix: cfg.StubHeaderPrefix,
		JWTIssuer:        cfg.JWTIssuer,
		ScopeName:        cfg.ScopeName,
	})

	gormDB, err := connection.Setup()
	if err != nil {
		log.Fatalf("[%s] database failed: %v", cfg.Name, err)
	}
	log.Printf("[%s] database connected", cfg.Name)

	if err := gocoreauth.Init(); err != nil {
		log.Fatalf("[%s] auth keys failed to initialize: %v", cfg.Name, err)
	}

	router := buildRouter(cfg, gormDB)

	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambdaStart(router)
		return
	}
	runLocal(cfg, router)
}

// validate ensures required Config fields are set. log.Fatalf on missing
// requirements — a misconfigured server should never reach the network.
func (cfg *Config) validate() {
	missing := []string{}
	if cfg.Name == "" {
		missing = append(missing, "Name")
	}
	if cfg.RoutePrefix == "" {
		missing = append(missing, "RoutePrefix")
	}
	if cfg.BuildSchema == nil {
		missing = append(missing, "BuildSchema")
	}
	if cfg.NewClaims == nil {
		missing = append(missing, "NewClaims")
	}
	if cfg.LiftClaims == nil {
		missing = append(missing, "LiftClaims")
	}
	// AuthCatalog + EndpointPolicyYAML + NewRoleResolver are intertwined;
	// require all-or-none.
	authPieces := 0
	if cfg.AuthCatalog != nil {
		authPieces++
	}
	if len(cfg.EndpointPolicyYAML) > 0 {
		authPieces++
	}
	if cfg.NewRoleResolver != nil {
		authPieces++
	}
	if authPieces != 0 && authPieces != 3 {
		missing = append(missing, "AuthCatalog+EndpointPolicyYAML+NewRoleResolver (must be set together or all empty)")
	}
	if len(missing) > 0 {
		log.Fatalf("server.Run: missing required Config fields: %v", missing)
	}
}

func (cfg *Config) applyDefaults() {
	if cfg.Port == "" {
		cfg.Port = "8002"
	}
}

// loadEnvFile walks up from the working directory looking for `.env` and
// loads it via godotenv. No-op when no .env is present (Lambda case).
func loadEnvFile() {
	dir, _ := os.Getwd()
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			godotenv.Load(path)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// buildRouter assembles the gin engine: global middleware, REST group,
// GraphQL handler with authz field middleware, and the dev/release auth
// chain on the GraphQL group.
func buildRouter(cfg Config, gormDB *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.SetTrustedProxies(nil)
	r.Use(middleware.CORS())
	r.Use(middleware.RequestID())

	restGroup := r.Group(cfg.RoutePrefix + "/rest")
	registerHealthRoutes(restGroup)
	if cfg.RegisterREST != nil {
		cfg.RegisterREST(restGroup, gormDB)
	}

	// JWKS at the standards-compliant well-known path (RFC 5785), unversioned
	// so verifiers like jose's createRemoteJWKSet can discover it without
	// knowing the consumer's RoutePrefix.
	r.GET("/.well-known/jwks.json", gocoreauth.JWKSHandler)

	srv := handler.NewDefaultServer(cfg.BuildSchema(gormDB))

	// Field middleware enforces auth.yml against the catalog. Skip if
	// the consumer hasn't configured authz (rare — mostly tests).
	if cfg.AuthCatalog != nil {
		endpointPolicy, err := authz.ParseEndpointPolicy(cfg.EndpointPolicyYAML)
		if err != nil {
			log.Fatalf("[%s] parse endpoint policy: %v", cfg.Name, err)
		}
		srv.AroundFields(authz.AuthorizeField(cfg.AuthCatalog, endpointPolicy, cfg.NewRoleResolver(gormDB)))
	}

	gqlGroup := r.Group(cfg.RoutePrefix)
	if os.Getenv("GIN_MODE") == "release" {
		if cfg.AuthCatalog != nil {
			// A field-level AuthorizeField gate is configured, so it governs
			// per-field access (including the policy's `public:` fields). The
			// route only needs to verify + lift a token when one is present and
			// let anonymous requests through — required auth here would 401
			// anonymous callers before public fields (e.g. public blog reads)
			// could ever resolve.
			gqlGroup.Use(middleware.JWTAuthOptional(cfg.NewClaims, cfg.LiftClaims))
		} else {
			// No field-level gate — keep the whole endpoint closed (secure by
			// default) rather than exposing it to anonymous callers.
			gqlGroup.Use(middleware.JWTAuth(cfg.NewClaims, cfg.LiftClaims))
		}
	} else {
		gqlGroup.Use(middleware.OptionalAuth())
	}
	if cfg.EnrichUserContext != nil {
		gqlGroup.Use(enrichMiddleware(cfg.EnrichUserContext, gormDB))
	}
	gqlGroup.Use(middleware.InjectDBContext(), middleware.ScopeContext())

	gqlGroup.POST("/graphql", gin.WrapH(srv))
	if os.Getenv("GIN_MODE") != "release" {
		gqlGroup.GET("/graphql", gin.WrapH(srv))
		r.GET(cfg.RoutePrefix+"/playground", gin.WrapH(playgroundHandler(cfg.RoutePrefix+"/graphql")))
	}

	return r
}

// enrichMiddleware wraps the consumer's EnrichUserContext hook in a gin
// middleware so it slots into the chain alongside the gocore middlewares.
func enrichMiddleware(hook func(*gin.Context, *gorm.DB), db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		hook(c, db)
		c.Next()
	}
}

// registerHealthRoutes mounts a trivial /health endpoint. Consumers that
// want different liveness/readiness shapes can register their own under
// RegisterREST (it runs after this, so it can override).
func registerHealthRoutes(r *gin.RouterGroup) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}

// lambdaStart wraps the router for AWS Lambda's request/response shape
// and hands control to the Lambda runtime, which blocks forever.
func lambdaStart(router *gin.Engine) {
	adapter := ginadapter.NewV2(router)
	lambda.Start(func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		return adapter.ProxyWithContext(ctx, req)
	})
}

// runLocal starts a long-lived HTTP server on the configured port, useful
// for local dev and any non-Lambda deployment. Blocks until the server
// errors out.
func runLocal(cfg Config, router *gin.Engine) {
	port := cfg.Port
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}
	addr := ":" + port

	fmt.Printf("[%s] starting at http://localhost%s\n", cfg.Name, addr)
	fmt.Printf("  Health:     http://localhost%s%s/rest/health\n", addr, cfg.RoutePrefix)
	if os.Getenv("GIN_MODE") != "release" {
		fmt.Printf("  Playground: http://localhost%s%s/playground\n", addr, cfg.RoutePrefix)
	}
	fmt.Println("Press Ctrl+C to stop")

	if err := router.Run(addr); err != nil {
		log.Fatalf("[%s] server failed: %v", cfg.Name, err)
	}
}
