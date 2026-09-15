package dexgithub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type cachedInstallationToken struct {
	token     string
	expiresAt time.Time
}

// Service is the main reusable entrypoint. It coordinates GitHub App
// authentication, persisted connection state and local Git repositories.
type Service struct {
	cfg   Config
	store Store
	api   *apiClient
	Git   *GitManager

	mu           sync.RWMutex
	registryMu   sync.Mutex
	app          AppCredentials
	installToken map[int64]cachedInstallationToken
}

func New(config Config) (*Service, error) {
	cfg, err := config.normalized()
	if err != nil {
		return nil, err
	}
	settings, err := cfg.Store.LoadSettings()
	if err != nil {
		return nil, fmt.Errorf("cargar ajustes DexGitHub: %w", err)
	}
	if settings.CloneRoot != "" {
		cfg.CloneRoot, err = normalizeCloneRoot(settings.CloneRoot)
		if err != nil {
			return nil, fmt.Errorf("clone root persistido inválido: %w", err)
		}
	}
	gitManager, err := NewGitManager(GitConfig{
		Binary:      cfg.GitBinary,
		DefaultRoot: cfg.CloneRoot,
		Timeout:     cfg.GitTimeout,
	})
	if err != nil {
		return nil, err
	}
	s := &Service{
		cfg:          cfg,
		store:        cfg.Store,
		api:          newAPIClient(cfg),
		Git:          gitManager,
		installToken: make(map[int64]cachedInstallationToken),
	}
	if app, err := cfg.Store.LoadApp(); err == nil {
		s.app = app
	} else if !errors.Is(err, ErrNotConfigured) {
		return nil, fmt.Errorf("cargar GitHub App: %w", err)
	}
	return s, nil
}

func (s *Service) Config() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg := s.cfg
	cfg.Store = nil
	cfg.HTTPClient = nil
	return cfg
}

func (s *Service) AppCredentials() (AppCredentials, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.app.Valid() {
		return AppCredentials{}, false
	}
	return s.app, true
}

func (s *Service) setApp(app AppCredentials) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.app = app
	s.installToken = make(map[int64]cachedInstallationToken)
}

func (s *Service) requireApp() (AppCredentials, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.app.Valid() {
		return AppCredentials{}, ErrNotConfigured
	}
	return s.app, nil
}

// Disconnect removes locally stored GitHub App/user credentials. It does not
// delete the GitHub App registration or its installations on GitHub.
func (s *Service) Disconnect() error {
	if err := s.store.DeleteUserToken(); err != nil {
		return err
	}
	if err := s.store.DeleteApp(); err != nil {
		return err
	}
	s.mu.Lock()
	s.app = AppCredentials{}
	s.installToken = make(map[int64]cachedInstallationToken)
	s.mu.Unlock()
	return nil
}

func (s *Service) AppJWT(now time.Time) (string, error) {
	app, err := s.requireApp()
	if err != nil {
		return "", err
	}
	return appJWT(app, now)
}

func (s *Service) SetCloneRoot(ctx context.Context, root string) error {
	if ctx == nil {
		return errors.New("dexgithub: context requerido")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	clean, err := normalizeCloneRoot(root)
	if err != nil {
		return err
	}
	if err := ensureCloneRoot(clean); err != nil {
		return err
	}
	if err := s.store.SaveSettings(Settings{CloneRoot: clean}); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg.CloneRoot = clean
	s.mu.Unlock()
	s.Git.mu.Lock()
	s.Git.defaultRoot = clean
	s.Git.mu.Unlock()
	return nil
}

func (s *Service) CloneRoot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.CloneRoot
}
