package dexgithub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrNotConfigured = errors.New("dexgithub: configuración no disponible")

// Store is the persistence boundary for credentials, the connected user,
// settings and the local repository registry. Implementations must protect
// AppCredentials and UserToken as secrets.
type Store interface {
	LoadApp() (AppCredentials, error)
	SaveApp(AppCredentials) error
	DeleteApp() error
	LoadUserToken() (UserToken, error)
	SaveUserToken(UserToken) error
	DeleteUserToken() error
	LoadSettings() (Settings, error)
	SaveSettings(Settings) error
	LoadRegistry() ([]RepositoryRecord, error)
	SaveRegistry([]RepositoryRecord) error
}

// FileStore stores state in one private directory using root-only permissions.
// It intentionally does not encrypt at rest: on a single-host root daemon the
// decryption key would live on the same host. Callers that have a vault/KMS can
// inject another Store implementation.
type FileStore struct {
	root string
	mu   sync.Mutex
}

func NewFileStore(root string) *FileStore {
	if root == "" {
		root = DefaultRootDir
	}
	return NewFileStoreAt(filepath.Join(filepath.Clean(root), ".dexgithub"))
}

// NewFileStoreAt stores DexGitHub state exactly in dir. This lets a consumer
// define its own private layout without inheriting the legacy .dexgithub suffix.
func NewFileStoreAt(dir string) *FileStore {
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(DefaultRootDir, ".dexgithub")
	}
	return &FileStore{root: filepath.Clean(dir)}
}

func (s *FileStore) LoadApp() (AppCredentials, error) {
	var v AppCredentials
	if err := s.loadJSON("app.json", &v); err != nil {
		return AppCredentials{}, err
	}
	if !v.Valid() {
		return AppCredentials{}, errors.New("dexgithub: credenciales de GitHub App incompletas")
	}
	return v, nil
}

func (s *FileStore) SaveApp(v AppCredentials) error {
	if !v.Valid() {
		return errors.New("dexgithub: credenciales de GitHub App incompletas")
	}
	return s.saveJSON("app.json", v)
}

func (s *FileStore) DeleteApp() error { return s.remove("app.json") }

func (s *FileStore) LoadUserToken() (UserToken, error) {
	var v UserToken
	if err := s.loadJSON("user.json", &v); err != nil {
		return UserToken{}, err
	}
	if v.AccessToken == "" {
		return UserToken{}, ErrNotConfigured
	}
	return v, nil
}

func (s *FileStore) SaveUserToken(v UserToken) error {
	if v.AccessToken == "" {
		return errors.New("dexgithub: token de usuario vacío")
	}
	return s.saveJSON("user.json", v)
}

func (s *FileStore) DeleteUserToken() error { return s.remove("user.json") }

func (s *FileStore) LoadSettings() (Settings, error) {
	var v Settings
	if err := s.loadJSON("settings.json", &v); err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return Settings{}, nil
		}
		return Settings{}, err
	}
	return v, nil
}

func (s *FileStore) SaveSettings(v Settings) error { return s.saveJSON("settings.json", v) }

func (s *FileStore) LoadRegistry() ([]RepositoryRecord, error) {
	var v []RepositoryRecord
	if err := s.loadJSON("registry.json", &v); err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func (s *FileStore) SaveRegistry(v []RepositoryRecord) error {
	if v == nil {
		v = []RepositoryRecord{}
	}
	return s.saveJSON("registry.json", v)
}

func (s *FileStore) path(name string) string { return filepath.Join(s.root, name) }

func (s *FileStore) ensureRoot() error {
	base := filepath.Dir(s.root)
	if err := ensureSecureDir(base, 0o700); err != nil {
		return fmt.Errorf("directorio raíz de DexGitHub: %w", err)
	}
	if err := ensureSecureDir(s.root, 0o700); err != nil {
		return fmt.Errorf("directorio privado de DexGitHub: %w", err)
	}
	return nil
}

func ensureSecureDir(path string, mode fs.FileMode) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("la ruta existente no es un directorio real")
		}
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	info, err = os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("el directorio creado no es una ruta real")
	}
	return os.Chmod(path, mode)
}

func (s *FileStore) loadJSON(name string, out any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRoot(); err != nil {
		return err
	}
	path := s.path(name)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotConfigured
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("dexgithub: archivo de estado inseguro")
	}
	if info.Size() > 4<<20 {
		return errors.New("dexgithub: archivo de estado excede 4 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decodificar %s: %w", name, err)
	}
	return nil
}

func (s *FileStore) saveJSON(name string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRoot(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWritePrivate(s.path(name), data)
}

func atomicWritePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".dexgithub-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	df, err := os.Open(dir)
	if err == nil {
		_ = df.Sync()
		_ = df.Close()
	}
	ok = true
	return nil
}

func (s *FileStore) remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.path(name)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("dexgithub: archivo de estado inseguro")
	}
	return os.Remove(path)
}
