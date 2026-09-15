package dexgithub

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu       sync.Mutex
	app      AppCredentials
	user     UserToken
	settings Settings
	registry []RepositoryRecord
}

func (m *memoryStore) LoadApp() (AppCredentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.app.Valid() {
		return AppCredentials{}, ErrNotConfigured
	}
	return m.app, nil
}
func (m *memoryStore) SaveApp(v AppCredentials) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.app = v
	return nil
}
func (m *memoryStore) DeleteApp() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.app = AppCredentials{}
	return nil
}
func (m *memoryStore) LoadUserToken() (UserToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.user.AccessToken == "" {
		return UserToken{}, ErrNotConfigured
	}
	return m.user, nil
}
func (m *memoryStore) SaveUserToken(v UserToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user = v
	return nil
}
func (m *memoryStore) DeleteUserToken() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user = UserToken{}
	return nil
}
func (m *memoryStore) LoadSettings() (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings, nil
}
func (m *memoryStore) SaveSettings(v Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = v
	return nil
}
func (m *memoryStore) LoadRegistry() ([]RepositoryRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]RepositoryRecord(nil), m.registry...), nil
}
func (m *memoryStore) SaveRegistry(v []RepositoryRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry = append([]RepositoryRecord(nil), v...)
	return nil
}

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func TestManifestFormAndCompleteManifest(t *testing.T) {
	pemKey := testPrivateKeyPEM(t)
	var sawVersion bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app-manifests/code-123/conversions" {
			http.NotFound(w, r)
			return
		}
		sawVersion = r.Header.Get("X-GitHub-Api-Version") == DefaultAPIVersion
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": int64(55), "client_id": "Iv1.client", "client_secret": "secret",
			"webhook_secret": "hook", "pem": pemKey, "name": "Dex Test", "slug": "dex-test",
			"html_url": "https://github.com/apps/dex-test", "owner": map[string]any{"login": "owner"},
		})
	}))
	defer server.Close()
	store := &memoryStore{}
	service, err := New(Config{RootDir: t.TempDir(), CloneRoot: filepath.Join(t.TempDir(), "repos"), WebBaseURL: server.URL, APIBaseURL: server.URL, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	form, err := service.ManifestForm(ManifestOptions{
		Name: "Dex GitHub", HomepageURL: "https://dex.example", RedirectURL: "https://dex.example/github/manifest/callback",
		CallbackURLs: []string{"https://dex.example/github/oauth/callback"}, RequestOAuth: true, State: "state-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(form.ActionURL, "/settings/apps/new") || !strings.Contains(form.ActionURL, "state=state-123") {
		t.Fatalf("action inesperada: %s", form.ActionURL)
	}
	var manifest map[string]any
	if err := json.Unmarshal([]byte(form.Manifest), &manifest); err != nil {
		t.Fatal(err)
	}
	perms := manifest["default_permissions"].(map[string]any)
	if perms["contents"] != "read" {
		t.Fatalf("permisos inesperados: %#v", perms)
	}

	credentials, err := service.CompleteManifest(context.Background(), "code-123")
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AppID != 55 || credentials.Slug != "dex-test" || !sawVersion {
		t.Fatalf("credenciales inesperadas: %+v version=%v", credentials, sawVersion)
	}
	stored, err := store.LoadApp()
	if err != nil || stored.ClientSecret != "secret" {
		t.Fatalf("store: %+v %v", stored, err)
	}
}

func TestUserOAuthInstallationsRepositoriesAndInstallationToken(t *testing.T) {
	pemKey := testPrivateKeyPEM(t)
	store := &memoryStore{app: AppCredentials{AppID: 77, ClientID: "Iv1.test", ClientSecret: "client-secret", PrivateKeyPEM: pemKey, Slug: "dex-test"}}
	var installationTokenCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "user-token", "token_type": "bearer", "expires_in": 28800, "refresh_token": "refresh-token", "refresh_token_expires_in": 100000})
		case "/user":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Errorf("auth user=%q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 9, "login": "octo", "name": "Octo"})
		case "/user/installations":
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "installations": []any{map[string]any{"id": 101, "repository_selection": "selected", "account": map[string]any{"id": 9, "login": "octo", "type": "User"}, "permissions": map[string]any{"contents": "read"}}}})
		case "/user/installations/101/repositories":
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "repositories": []any{map[string]any{"id": 501, "name": "private-repo", "full_name": "octo/private-repo", "private": true, "default_branch": "main", "clone_url": "https://github.com/octo/private-repo.git", "owner": map[string]any{"login": "octo"}}}})
		case "/app/installations/101/access_tokens":
			installationTokenCalls++
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
				t.Errorf("JWT ausente: %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_installation", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
		case "/installation/repositories":
			if r.Header.Get("Authorization") != "Bearer ghs_installation" {
				t.Errorf("auth installation=%q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 1, "repositories": []any{map[string]any{"id": 501, "name": "private-repo", "full_name": "octo/private-repo", "private": true, "default_branch": "main", "clone_url": "https://github.com/octo/private-repo.git", "owner": map[string]any{"login": "octo"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service, err := New(Config{RootDir: t.TempDir(), CloneRoot: filepath.Join(t.TempDir(), "repos"), WebBaseURL: server.URL, APIBaseURL: server.URL, Store: store})
	if err != nil {
		t.Fatal(err)
	}

	authURL, err := service.UserAuthorizationURL(AuthorizeOptions{RedirectURI: server.URL + "/callback", State: "state-abc"})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	if u.Query().Get("client_id") != "Iv1.test" || u.Query().Get("state") != "state-abc" {
		t.Fatalf("auth URL: %s", authURL)
	}

	token, err := service.ExchangeUserCode(context.Background(), "code", server.URL+"/callback")
	if err != nil {
		t.Fatal(err)
	}
	if token.Login != "octo" || token.UserID != 9 {
		t.Fatalf("token: %+v", token)
	}
	installations, err := service.UserInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 1 || installations[0].ID != 101 {
		t.Fatalf("installations: %+v", installations)
	}
	repos, err := service.RepositoriesForUserInstallation(context.Background(), 101)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].ID != 501 || !repos[0].Private {
		t.Fatalf("repos: %+v", repos)
	}

	if _, err := service.RepositoriesForInstallation(context.Background(), 101); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RepositoriesForInstallation(context.Background(), 101); err != nil {
		t.Fatal(err)
	}
	if installationTokenCalls != 1 {
		t.Fatalf("installation token pidió %d veces", installationTokenCalls)
	}
}

func TestAppInstallationsUsesArrayResponse(t *testing.T) {
	store := &memoryStore{app: AppCredentials{AppID: 88, ClientID: "c", ClientSecret: "s", PrivateKeyPEM: testPrivateKeyPEM(t)}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"id": 22, "repository_selection": "all", "account": map[string]any{"id": 33, "login": "org", "type": "Organization"}, "permissions": map[string]any{"contents": "read"}}})
	}))
	defer server.Close()
	service, err := New(Config{RootDir: t.TempDir(), CloneRoot: filepath.Join(t.TempDir(), "repos"), WebBaseURL: server.URL, APIBaseURL: server.URL, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.AppInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AccountType != "Organization" {
		t.Fatalf("items: %+v", items)
	}
}

func TestValidateUserInstallationRejectsSpoofedID(t *testing.T) {
	store := &memoryStore{app: AppCredentials{AppID: 1, ClientID: "c", ClientSecret: "s", PrivateKeyPEM: testPrivateKeyPEM(t)}, user: UserToken{AccessToken: "u"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/installations" {
			_ = json.NewEncoder(w).Encode(map[string]any{"total_count": 0, "installations": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	service, err := New(Config{RootDir: t.TempDir(), CloneRoot: filepath.Join(t.TempDir(), "repos"), WebBaseURL: server.URL, APIBaseURL: server.URL, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ValidateUserInstallation(context.Background(), 999)
	if err == nil || !strings.Contains(err.Error(), "no pertenece") {
		t.Fatalf("error: %v", err)
	}
}

func TestAPIErrorIsSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-GitHub-Request-Id", "req-1")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()
	cfg, err := (Config{RootDir: t.TempDir(), CloneRoot: filepath.Join(t.TempDir(), "repos"), WebBaseURL: server.URL, APIBaseURL: server.URL, Store: &memoryStore{}}).normalized()
	if err != nil {
		t.Fatal(err)
	}
	err = newAPIClient(cfg).requestJSON(context.Background(), http.MethodGet, server.URL, "super-secret", nil, &map[string]any{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RequestID != "req-1" {
		t.Fatalf("error: %#v", err)
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("token filtrado: %v", err)
	}
}
