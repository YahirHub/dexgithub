package dexgithub

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultRootDir    = "/root/.github"
	DefaultAPIVersion = "2026-03-10"
)

// Config controls GitHub API access, local persistence and Git execution.
type Config struct {
	RootDir        string
	StateDir       string
	CloneRoot      string
	WebBaseURL     string
	APIBaseURL     string
	APIVersion     string
	GitBinary      string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
	GitTimeout     time.Duration
	MaxAPIPages    int
	Store          Store
}

func (c Config) normalized() (Config, error) {
	if strings.TrimSpace(c.RootDir) == "" {
		c.RootDir = DefaultRootDir
	}
	root, err := filepath.Abs(c.RootDir)
	if err != nil {
		return Config{}, err
	}
	c.RootDir = filepath.Clean(root)

	if strings.TrimSpace(c.StateDir) == "" {
		c.StateDir = filepath.Join(c.RootDir, ".dexgithub")
	}
	stateDir, err := filepath.Abs(c.StateDir)
	if err != nil {
		return Config{}, err
	}
	c.StateDir = filepath.Clean(stateDir)

	if strings.TrimSpace(c.CloneRoot) == "" {
		c.CloneRoot = filepath.Join(c.RootDir, "repos")
	}
	cloneRoot, err := filepath.Abs(c.CloneRoot)
	if err != nil {
		return Config{}, err
	}
	c.CloneRoot = filepath.Clean(cloneRoot)

	if strings.TrimSpace(c.WebBaseURL) == "" {
		c.WebBaseURL = "https://github.com"
	}
	if strings.TrimSpace(c.APIBaseURL) == "" {
		c.APIBaseURL = "https://api.github.com"
	}
	if err := validateBaseURL(c.WebBaseURL); err != nil {
		return Config{}, errors.New("web base URL inválida: " + err.Error())
	}
	if err := validateBaseURL(c.APIBaseURL); err != nil {
		return Config{}, errors.New("API base URL inválida: " + err.Error())
	}
	c.WebBaseURL = strings.TrimRight(c.WebBaseURL, "/")
	c.APIBaseURL = strings.TrimRight(c.APIBaseURL, "/")

	if strings.TrimSpace(c.APIVersion) == "" {
		c.APIVersion = DefaultAPIVersion
	}
	if strings.TrimSpace(c.GitBinary) == "" {
		c.GitBinary = "git"
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = 20 * time.Second
	}
	if c.GitTimeout <= 0 {
		c.GitTimeout = 2 * time.Minute
	}
	if c.MaxAPIPages <= 0 {
		c.MaxAPIPages = 100
	}
	if c.MaxAPIPages > 1000 {
		return Config{}, errors.New("MaxAPIPages no puede exceder 1000")
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: c.RequestTimeout}
	}
	if c.Store == nil {
		c.Store = NewFileStoreAt(c.StateDir)
	}
	return c, nil
}

func validateBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return errors.New("se requiere HTTPS salvo loopback")
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("URL base debe contener sólo esquema, host y ruta opcional")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func executableEnv(extra map[string]string) []string {
	env := os.Environ()
	for key, value := range extra {
		prefix := key + "="
		filtered := env[:0]
		for _, entry := range env {
			if !strings.HasPrefix(entry, prefix) {
				filtered = append(filtered, entry)
			}
		}
		env = append(filtered, prefix+value)
	}
	return env
}
