package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// --- fakes implementing the Sync* interfaces ---

type fakeSyncUser struct {
	id   uuid.UUID
	last *uuid.UUID
}

func (f fakeSyncUser) GetUserID() uuid.UUID       { return f.id }
func (f fakeSyncUser) GetLastScopeID() *uuid.UUID { return f.last }

type fakeMembership struct {
	scope uuid.UUID
	name  string
	role  string
}

func (m fakeMembership) GetScopeID() uuid.UUID { return m.scope }
func (m fakeMembership) GetScopeName() string  { return m.name }
func (m fakeMembership) GetRole() string       { return m.role }

// --- pure helpers ---

func TestBearerFrom(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := BearerFrom(c); err == nil {
		t.Error("expected error with no Authorization header")
	}
	c.Request.Header.Set("Authorization", "Bearer abc.def")
	if tok, err := BearerFrom(c); err != nil || tok != "abc.def" {
		t.Errorf("BearerFrom = %q, %v", tok, err)
	}
}

func TestComposeName(t *testing.T) {
	cases := []struct {
		id   IDPIdentity
		want string
	}{
		{IDPIdentity{Name: "Full Name"}, "Full Name"},
		{IDPIdentity{FirstName: "First", LastName: "Last"}, "First Last"},
		{IDPIdentity{FirstName: "First"}, "First"},
		{IDPIdentity{LastName: "Last"}, "Last"},
		{IDPIdentity{}, ""},
	}
	for _, c := range cases {
		if got := ComposeName(&c.id); got != c.want {
			t.Errorf("ComposeName(%+v) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestPtrIfNotEmpty(t *testing.T) {
	if PtrIfNotEmpty("") != nil {
		t.Error("empty should be nil")
	}
	if p := PtrIfNotEmpty("x"); p == nil || *p != "x" {
		t.Error("non-empty should point at the value")
	}
}

func TestPickCurrent(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	ms := []SyncMembership{fakeMembership{scope: a, role: "owner"}, fakeMembership{scope: b, role: "viewer"}}

	// override hit
	if got, err := PickCurrent(ms, &b, nil); err != nil || (*got).GetScopeID() != b {
		t.Errorf("override: got %v, %v", got, err)
	}
	// override miss → error
	other := uuid.New()
	if _, err := PickCurrent(ms, &other, nil); err == nil {
		t.Error("override miss should error")
	}
	// lastScope hit
	if got, err := PickCurrent(ms, nil, &b); err != nil || (*got).GetScopeID() != b {
		t.Errorf("lastScope: got %v, %v", got, err)
	}
	// fall back to first
	if got, _ := PickCurrent(ms, nil, nil); (*got).GetScopeID() != a {
		t.Error("expected first membership fallback")
	}
	// empty → nil, nil
	if got, err := PickCurrent(nil, nil, nil); got != nil || err != nil {
		t.Errorf("empty: got %v, %v", got, err)
	}
}

func TestToEntries(t *testing.T) {
	a := uuid.New()
	got := toEntries([]SyncMembership{fakeMembership{scope: a, name: "Acme", role: "owner"}})
	if len(got) != 1 || got[0].ScopeID != a || got[0].ScopeName != "Acme" || got[0].Role != "owner" {
		t.Errorf("toEntries = %+v", got)
	}
}

func TestClearSessionCookieHandler(t *testing.T) {
	r := gin.New()
	r.POST("/signout", ClearSessionCookieHandler(SessionCookie{Name: "iro_session", Domain: ".iro.local", HttpOnly: true}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/signout", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "iro_session" || cookies[0].MaxAge >= 0 {
		t.Errorf("expected a clearing cookie, got %+v", cookies)
	}
}

func TestClerkKeyfunc_URLNotSet(t *testing.T) {
	// Bypass the override and any cached instance so the env-missing branch runs.
	restore := SetClerkKeyfuncForTest(nil)
	defer restore()
	clerkKfMu.Lock()
	saved := clerkKfInstance
	clerkKfInstance = nil
	clerkKfMu.Unlock()
	defer func() {
		clerkKfMu.Lock()
		clerkKfInstance = saved
		clerkKfMu.Unlock()
	}()
	t.Setenv("CLERK_JWKS_URL", "")
	if _, err := clerkKeyfunc(context.Background()); err == nil {
		t.Error("expected error when CLERK_JWKS_URL is unset")
	}
}

// --- SyncHandler ---

func validHooks() SyncHooks {
	return SyncHooks{
		FindOrCreateUser: func(*gorm.DB, *IDPIdentity) (SyncUser, error) {
			return fakeSyncUser{id: uuid.New()}, nil
		},
		ListMemberships: func(*gorm.DB, uuid.UUID) ([]SyncMembership, error) {
			return []SyncMembership{fakeMembership{scope: uuid.New(), name: "Acme", role: "owner"}}, nil
		},
		UpdateLastScope: func(*gorm.DB, uuid.UUID, uuid.UUID) error { return nil },
		BuildClaims: func(SyncUser, SyncMembership) jwt.Claims {
			return &jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
		},
	}
}

func TestSyncHandler_Success(t *testing.T) {
	priv, pub := freshKeys(t)
	defer SetClerkKeyfuncForTest(keyfuncReturning(pub))()
	defer SetKeysForTest(priv, pub)()

	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, validHooks(),
		WithSessionCookie(SessionCookie{Name: "iro_session", Domain: ".iro.local", HttpOnly: true, MaxAge: 3600})))

	tok := fakeClerkToken(t, priv, "test-kid", jwt.MapClaims{
		"sub": "user_1", "email": "a@b.c", "exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/sync", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, body %s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) == 0 {
		t.Error("expected a session cookie to be set")
	}
}

func TestSyncHandler_MissingBearer(t *testing.T) {
	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, validHooks()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/auth/sync", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", w.Code)
	}
}

func TestSyncHandler_BadClerkToken(t *testing.T) {
	_, pub := freshKeys(t)
	defer SetClerkKeyfuncForTest(keyfuncReturning(pub))()

	r := gin.New()
	r.POST("/auth/sync", SyncHandler(nil, validHooks()))
	req := httptest.NewRequest(http.MethodPost, "/auth/sync", nil)
	req.Header.Set("Authorization", "Bearer garbage.token.here")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", w.Code)
	}
}
