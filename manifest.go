package dexgithub

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ManifestOptions controls creation of a GitHub App registration form.
type ManifestOptions struct {
	Name          string
	HomepageURL   string
	RedirectURL   string
	CallbackURLs  []string
	SetupURL      string
	SetupOnUpdate bool
	Description   string
	Owner         string // empty = personal account; otherwise organization login
	RequestOAuth  bool
	WebhookURL    string
	WebhookActive bool
	Events        []string
	Permissions   map[string]string
	State         string
}

// ManifestForm is rendered by a host application as a POST form to GitHub.
// The Manifest value must be sent as the `manifest` form field.
type ManifestForm struct {
	ActionURL string
	Manifest  string
	State     string
}

func RandomState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Service) ManifestForm(options ManifestOptions) (ManifestForm, error) {
	if err := requireHTTPSURL(options.HomepageURL, true); err != nil {
		return ManifestForm{}, fmt.Errorf("HomepageURL inválida: %w", err)
	}
	if err := requireHTTPSURL(options.RedirectURL, true); err != nil {
		return ManifestForm{}, fmt.Errorf("RedirectURL inválida: %w", err)
	}
	if len(options.CallbackURLs) > 10 {
		return ManifestForm{}, errors.New("GitHub admite máximo 10 callback URLs")
	}
	for _, callback := range options.CallbackURLs {
		if err := requireHTTPSURL(callback, true); err != nil {
			return ManifestForm{}, fmt.Errorf("callback URL inválida: %w", err)
		}
	}
	if options.SetupURL != "" {
		if options.RequestOAuth {
			return ManifestForm{}, errors.New("SetupURL no puede combinarse con RequestOAuth")
		}
		if err := requireHTTPSURL(options.SetupURL, true); err != nil {
			return ManifestForm{}, fmt.Errorf("SetupURL inválida: %w", err)
		}
	}
	if options.WebhookURL != "" {
		if err := requireHTTPSURL(options.WebhookURL, false); err != nil {
			return ManifestForm{}, fmt.Errorf("webhook URL inválida: %w", err)
		}
	}
	if options.State == "" {
		state, err := RandomState()
		if err != nil {
			return ManifestForm{}, err
		}
		options.State = state
	}
	permissions := options.Permissions
	if len(permissions) == 0 {
		permissions = map[string]string{"contents": "read"}
	}
	for name, level := range permissions {
		if strings.TrimSpace(name) == "" {
			return ManifestForm{}, errors.New("permiso GitHub vacío")
		}
		switch level {
		case "read", "write":
		default:
			return ManifestForm{}, fmt.Errorf("nivel inválido para permiso %q", name)
		}
	}
	events := options.Events
	if events == nil {
		events = []string{}
	}
	manifest := map[string]any{
		"url":                      options.HomepageURL,
		"redirect_url":             options.RedirectURL,
		"callback_urls":            options.CallbackURLs,
		"description":              options.Description,
		"public":                   false,
		"default_permissions":      permissions,
		"default_events":           events,
		"request_oauth_on_install": options.RequestOAuth,
	}
	if strings.TrimSpace(options.Name) != "" {
		manifest["name"] = strings.TrimSpace(options.Name)
	}
	if options.SetupURL != "" {
		manifest["setup_url"] = options.SetupURL
		manifest["setup_on_update"] = options.SetupOnUpdate
	}
	if options.WebhookURL != "" {
		manifest["hook_attributes"] = map[string]any{
			"url":    options.WebhookURL,
			"active": options.WebhookActive,
		}
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return ManifestForm{}, err
	}
	endpoint := "/settings/apps/new"
	if strings.TrimSpace(options.Owner) != "" {
		owner := url.PathEscape(strings.TrimSpace(options.Owner))
		endpoint = "/organizations/" + owner + "/settings/apps/new"
	}
	action, err := s.api.webURL(endpoint)
	if err != nil {
		return ManifestForm{}, err
	}
	u, err := url.Parse(action)
	if err != nil {
		return ManifestForm{}, err
	}
	q := u.Query()
	q.Set("state", options.State)
	u.RawQuery = q.Encode()
	return ManifestForm{ActionURL: u.String(), Manifest: string(encoded), State: options.State}, nil
}

// CompleteManifest exchanges GitHub's one-hour manifest code for the app's
// private credentials and persists them using the configured Store.
func (s *Service) CompleteManifest(ctx context.Context, code string) (AppCredentials, error) {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 256 {
		return AppCredentials{}, errors.New("código de manifest inválido")
	}
	endpoint, err := s.api.apiURL("/app-manifests/" + url.PathEscape(code) + "/conversions")
	if err != nil {
		return AppCredentials{}, err
	}
	var response struct {
		ID            int64  `json:"id"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		WebhookSecret string `json:"webhook_secret"`
		PEM           string `json:"pem"`
		Name          string `json:"name"`
		Slug          string `json:"slug"`
		HTMLURL       string `json:"html_url"`
		CreatedAt     string `json:"created_at"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	if err := s.api.requestJSON(ctx, http.MethodPost, endpoint, "", nil, &response); err != nil {
		return AppCredentials{}, err
	}
	credentials := AppCredentials{
		AppID:         response.ID,
		ClientID:      response.ClientID,
		ClientSecret:  response.ClientSecret,
		WebhookSecret: response.WebhookSecret,
		PrivateKeyPEM: response.PEM,
		Name:          response.Name,
		Slug:          response.Slug,
		HTMLURL:       response.HTMLURL,
		OwnerLogin:    response.Owner.Login,
	}
	if !credentials.Valid() {
		return AppCredentials{}, errors.New("GitHub devolvió credenciales incompletas para la app")
	}
	if _, err := parseRSAPrivateKey([]byte(credentials.PrivateKeyPEM)); err != nil {
		return AppCredentials{}, err
	}
	if err := s.store.SaveApp(credentials); err != nil {
		return AppCredentials{}, fmt.Errorf("guardar credenciales GitHub App: %w", err)
	}
	s.setApp(credentials)
	return credentials, nil
}

func requireHTTPSURL(raw string, allowLocalHTTP bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if u.Host == "" || u.User != nil || u.Fragment != "" {
		return errors.New("URL absoluta esperada")
	}
	if u.Scheme == "https" {
		return nil
	}
	if allowLocalHTTP && u.Scheme == "http" && isTrustedLocalHTTPHost(u.Hostname()) {
		return nil
	}
	return errors.New("se requiere HTTPS salvo host local/LAN permitido")
}

func isTrustedLocalHTTPHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if isLoopbackHost(host) {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	return strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".home.arpa")
}
