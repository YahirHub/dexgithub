package dexgithub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type AuthorizeOptions struct {
	RedirectURI string
	State       string
	Login       string
	Prompt      string
}

func (s *Service) UserAuthorizationURL(options AuthorizeOptions) (string, error) {
	app, err := s.requireApp()
	if err != nil {
		return "", err
	}
	if options.RedirectURI != "" {
		if err := requireHTTPSURL(options.RedirectURI, true); err != nil {
			return "", fmt.Errorf("redirect URI inválida: %w", err)
		}
	}
	if options.State == "" {
		return "", errors.New("state OAuth requerido")
	}
	endpoint, err := s.api.webURL("/login/oauth/authorize")
	if err != nil {
		return "", err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", app.ClientID)
	q.Set("state", options.State)
	if options.RedirectURI != "" {
		q.Set("redirect_uri", options.RedirectURI)
	}
	if strings.TrimSpace(options.Login) != "" {
		q.Set("login", strings.TrimSpace(options.Login))
	}
	if options.Prompt != "" {
		if options.Prompt != "select_account" {
			return "", errors.New("prompt OAuth no soportado")
		}
		q.Set("prompt", options.Prompt)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Service) ExchangeUserCode(ctx context.Context, code, redirectURI string) (UserToken, error) {
	app, err := s.requireApp()
	if err != nil {
		return UserToken{}, err
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 512 {
		return UserToken{}, errors.New("código OAuth inválido")
	}
	if redirectURI != "" {
		if err := requireHTTPSURL(redirectURI, true); err != nil {
			return UserToken{}, err
		}
	}
	endpoint, err := s.api.webURL("/login/oauth/access_token")
	if err != nil {
		return UserToken{}, err
	}
	payload := map[string]string{
		"client_id":     app.ClientID,
		"client_secret": app.ClientSecret,
		"code":          code,
	}
	if redirectURI != "" {
		payload["redirect_uri"] = redirectURI
	}
	var response oauthTokenResponse
	if err := s.api.requestJSON(ctx, http.MethodPost, endpoint, "", payload, &response); err != nil {
		return UserToken{}, err
	}
	token, err := response.token(time.Now())
	if err != nil {
		return UserToken{}, err
	}
	user, err := s.userWithToken(ctx, token.AccessToken)
	if err != nil {
		return UserToken{}, fmt.Errorf("validar usuario GitHub: %w", err)
	}
	token.Login = user.Login
	token.UserID = user.ID
	if err := s.store.SaveUserToken(token); err != nil {
		return UserToken{}, err
	}
	return token, nil
}

type oauthTokenResponse struct {
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	ExpiresIn             int64  `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
}

func (r oauthTokenResponse) token(now time.Time) (UserToken, error) {
	if r.Error != "" {
		message := r.Error
		if r.ErrorDescription != "" {
			message += ": " + r.ErrorDescription
		}
		return UserToken{}, errors.New("GitHub OAuth: " + message)
	}
	if r.AccessToken == "" {
		return UserToken{}, errors.New("GitHub OAuth no devolvió access_token")
	}
	t := UserToken{AccessToken: r.AccessToken, TokenType: r.TokenType, Scope: r.Scope, RefreshToken: r.RefreshToken}
	if r.ExpiresIn > 0 {
		t.ExpiresAt = now.Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	if r.RefreshTokenExpiresIn > 0 {
		t.RefreshExpiresAt = now.Add(time.Duration(r.RefreshTokenExpiresIn) * time.Second)
	}
	return t, nil
}

func (s *Service) ConnectedUser(ctx context.Context) (GitHubUser, error) {
	token, err := s.ensureUserToken(ctx)
	if err != nil {
		return GitHubUser{}, err
	}
	return s.userWithToken(ctx, token.AccessToken)
}

func (s *Service) userWithToken(ctx context.Context, accessToken string) (GitHubUser, error) {
	endpoint, err := s.api.apiURL("/user")
	if err != nil {
		return GitHubUser{}, err
	}
	var response struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		HTMLURL   string `json:"html_url"`
	}
	if err := s.api.requestJSON(ctx, http.MethodGet, endpoint, accessToken, nil, &response); err != nil {
		return GitHubUser{}, err
	}
	if response.ID <= 0 || response.Login == "" {
		return GitHubUser{}, errors.New("GitHub devolvió un usuario inválido")
	}
	return GitHubUser{ID: response.ID, Login: response.Login, Name: response.Name, AvatarURL: response.AvatarURL, HTMLURL: response.HTMLURL}, nil
}

func (s *Service) ensureUserToken(ctx context.Context) (UserToken, error) {
	token, err := s.store.LoadUserToken()
	if err != nil {
		return UserToken{}, err
	}
	if !token.ExpiringSoon(time.Now()) {
		return token, nil
	}
	if token.RefreshToken == "" || (!token.RefreshExpiresAt.IsZero() && !time.Now().Before(token.RefreshExpiresAt)) {
		return UserToken{}, errors.New("sesión GitHub expirada; se requiere autorizar de nuevo")
	}
	return s.refreshUserToken(ctx, token)
}

func (s *Service) refreshUserToken(ctx context.Context, current UserToken) (UserToken, error) {
	app, err := s.requireApp()
	if err != nil {
		return UserToken{}, err
	}
	endpoint, err := s.api.webURL("/login/oauth/access_token")
	if err != nil {
		return UserToken{}, err
	}
	payload := map[string]string{
		"client_id":     app.ClientID,
		"client_secret": app.ClientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": current.RefreshToken,
	}
	var response oauthTokenResponse
	if err := s.api.requestJSON(ctx, http.MethodPost, endpoint, "", payload, &response); err != nil {
		return UserToken{}, err
	}
	fresh, err := response.token(time.Now())
	if err != nil {
		return UserToken{}, err
	}
	fresh.Login = current.Login
	fresh.UserID = current.UserID
	if err := s.store.SaveUserToken(fresh); err != nil {
		return UserToken{}, err
	}
	return fresh, nil
}

func (s *Service) LogoutGitHub() error {
	return s.store.DeleteUserToken()
}

func secondsString(v int64) string { return strconv.FormatInt(v, 10) }
