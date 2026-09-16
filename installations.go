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

func (s *Service) InstallURL() (string, error) {
	app, err := s.requireApp()
	if err != nil {
		return "", err
	}
	if app.Slug == "" {
		return "", errors.New("GitHub App sin slug; no se puede construir URL de instalación")
	}
	return s.api.webURL("/apps/" + url.PathEscape(app.Slug) + "/installations/new")
}

// UserInstallations returns only installations explicitly accessible to the
// connected GitHub user, including organization installations available via
// that user's organization membership.
func (s *Service) UserInstallations(ctx context.Context) ([]Installation, error) {
	token, err := s.ensureUserToken(ctx)
	if err != nil {
		return nil, err
	}
	var result []Installation
	for page := 1; page <= s.cfg.MaxAPIPages; page++ {
		raw, err := s.api.apiURL("/user/installations")
		if err != nil {
			return nil, err
		}
		u, _ := url.Parse(raw)
		q := u.Query()
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()
		var response struct {
			Installations []installationAPI `json:"installations"`
		}
		if err := s.api.requestJSON(ctx, http.MethodGet, u.String(), token.AccessToken, nil, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Installations {
			result = append(result, item.model())
		}
		if len(response.Installations) < 100 {
			return result, nil
		}
	}
	return nil, errors.New("GitHub devolvió más páginas de instalaciones que el límite configurado")
}

// AppInstallations lists all installations belonging to this GitHub App and
// therefore authenticates as the app with a short-lived JWT.
func (s *Service) AppInstallations(ctx context.Context) ([]Installation, error) {
	jwt, err := s.AppJWT(time.Now())
	if err != nil {
		return nil, err
	}
	var result []Installation
	for page := 1; page <= s.cfg.MaxAPIPages; page++ {
		raw, err := s.api.apiURL("/app/installations")
		if err != nil {
			return nil, err
		}
		u, _ := url.Parse(raw)
		q := u.Query()
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()
		var response []installationAPI
		if err := s.api.requestJSON(ctx, http.MethodGet, u.String(), jwt, nil, &response); err != nil {
			return nil, err
		}
		for _, item := range response {
			result = append(result, item.model())
		}
		if len(response) < 100 {
			return result, nil
		}
	}
	return nil, errors.New("GitHub devolvió más páginas de instalaciones que el límite configurado")
}

type installationAPI struct {
	ID                  int64             `json:"id"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
	SuspendedAt         *time.Time        `json:"suspended_at"`
	Account             struct {
		Login string `json:"login"`
		Type  string `json:"type"`
		ID    int64  `json:"id"`
	} `json:"account"`
}

func (i installationAPI) model() Installation {
	return Installation{
		ID:                  i.ID,
		AccountLogin:        i.Account.Login,
		AccountType:         i.Account.Type,
		AccountID:           i.Account.ID,
		RepositorySelection: i.RepositorySelection,
		Permissions:         i.Permissions,
		SuspendedAt:         i.SuspendedAt,
	}
}

func (s *Service) ValidateUserInstallation(ctx context.Context, installationID int64) (Installation, error) {
	if installationID <= 0 {
		return Installation{}, errors.New("installation ID inválido")
	}
	installations, err := s.UserInstallations(ctx)
	if err != nil {
		return Installation{}, err
	}
	for _, installation := range installations {
		if installation.ID == installationID {
			if installation.SuspendedAt != nil {
				return Installation{}, errors.New("la instalación de GitHub App está suspendida")
			}
			return installation, nil
		}
	}
	return Installation{}, errors.New("la instalación no pertenece a las instalaciones accesibles del usuario")
}

func (s *Service) RepositoriesForUserInstallation(ctx context.Context, installationID int64) ([]Repository, error) {
	if _, err := s.ValidateUserInstallation(ctx, installationID); err != nil {
		return nil, err
	}
	token, err := s.ensureUserToken(ctx)
	if err != nil {
		return nil, err
	}
	return s.listRepositories(ctx, "/user/installations/"+strconv.FormatInt(installationID, 10)+"/repositories", token.AccessToken, installationID)
}

func (s *Service) RepositoriesForInstallation(ctx context.Context, installationID int64) ([]Repository, error) {
	if installationID <= 0 {
		return nil, errors.New("installation ID inválido")
	}
	token, err := s.installationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	return s.listRepositories(ctx, "/installation/repositories", token, installationID)
}

// AccessibleRepositories aggregates repositories from every GitHub App
// installation accessible to the connected user (personal account and orgs).
func (s *Service) AccessibleRepositories(ctx context.Context) ([]Repository, error) {
	installations, err := s.UserInstallations(ctx)
	if err != nil {
		return nil, err
	}
	token, err := s.ensureUserToken(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{})
	var result []Repository
	for _, installation := range installations {
		if installation.SuspendedAt != nil {
			continue
		}
		repos, err := s.listRepositories(ctx, "/user/installations/"+strconv.FormatInt(installation.ID, 10)+"/repositories", token.AccessToken, installation.ID)
		if err != nil {
			return nil, fmt.Errorf("repositorios de %s: %w", installation.AccountLogin, err)
		}
		for _, repo := range repos {
			if _, exists := seen[repo.ID]; exists {
				continue
			}
			seen[repo.ID] = struct{}{}
			result = append(result, repo)
		}
	}
	return result, nil
}

func (s *Service) listRepositories(ctx context.Context, endpoint, bearer string, installationID int64) ([]Repository, error) {
	var result []Repository
	for page := 1; page <= s.cfg.MaxAPIPages; page++ {
		raw, err := s.api.apiURL(endpoint)
		if err != nil {
			return nil, err
		}
		u, _ := url.Parse(raw)
		q := u.Query()
		q.Set("per_page", "100")
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()
		var response struct {
			TotalCount   int `json:"total_count"`
			Repositories []struct {
				ID            int64           `json:"id"`
				NodeID        string          `json:"node_id"`
				Name          string          `json:"name"`
				FullName      string          `json:"full_name"`
				Private       bool            `json:"private"`
				DefaultBranch string          `json:"default_branch"`
				CloneURL      string          `json:"clone_url"`
				SSHURL        string          `json:"ssh_url"`
				HTMLURL       string          `json:"html_url"`
				Archived      bool            `json:"archived"`
				Disabled      bool            `json:"disabled"`
				CreatedAt     time.Time       `json:"created_at"`
				UpdatedAt     time.Time       `json:"updated_at"`
				PushedAt      time.Time       `json:"pushed_at"`
				Permissions   map[string]bool `json:"permissions"`
				Owner         struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"repositories"`
		}
		if err := s.api.requestJSON(ctx, http.MethodGet, u.String(), bearer, nil, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Repositories {
			result = append(result, Repository{
				ID: item.ID, NodeID: item.NodeID, Name: item.Name, FullName: item.FullName,
				Owner: item.Owner.Login, Private: item.Private, DefaultBranch: item.DefaultBranch,
				CloneURL: item.CloneURL, SSHURL: item.SSHURL, HTMLURL: item.HTMLURL,
				Archived: item.Archived, Disabled: item.Disabled,
				CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, PushedAt: item.PushedAt,
				Permissions: item.Permissions, InstallationID: installationID,
			})
		}
		if len(response.Repositories) < 100 {
			return result, nil
		}
	}
	return nil, errors.New("GitHub devolvió más páginas de repositorios que el límite configurado")
}

func (s *Service) installationToken(ctx context.Context, installationID int64) (string, error) {
	if installationID <= 0 {
		return "", errors.New("installation ID inválido")
	}
	now := time.Now()
	s.mu.RLock()
	cached, ok := s.installToken[installationID]
	s.mu.RUnlock()
	if ok && cached.token != "" && now.Add(5*time.Minute).Before(cached.expiresAt) {
		return cached.token, nil
	}
	jwt, err := s.AppJWT(now)
	if err != nil {
		return "", err
	}
	endpoint, err := s.api.apiURL("/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens")
	if err != nil {
		return "", err
	}
	var response struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := s.api.requestJSON(ctx, http.MethodPost, endpoint, jwt, map[string]any{}, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Token) == "" || response.ExpiresAt.IsZero() {
		return "", errors.New("GitHub no devolvió un installation token válido")
	}
	s.mu.Lock()
	s.installToken[installationID] = cachedInstallationToken{token: response.Token, expiresAt: response.ExpiresAt}
	s.mu.Unlock()
	return response.Token, nil
}

func (s *Service) RepositoryByID(ctx context.Context, installationID, repositoryID int64) (Repository, error) {
	repos, err := s.RepositoriesForInstallation(ctx, installationID)
	if err != nil {
		return Repository{}, err
	}
	for _, repo := range repos {
		if repo.ID == repositoryID {
			return repo, nil
		}
	}
	return Repository{}, fmt.Errorf("repositorio %d no accesible por la instalación %d", repositoryID, installationID)
}
