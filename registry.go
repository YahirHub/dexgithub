package dexgithub

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Service) KnownRepositories() ([]RepositoryRecord, error) {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	records, err := s.store.LoadRegistry()
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Name == records[j].Name {
			return records[i].Path < records[j].Path
		}
		return strings.ToLower(records[i].Name) < strings.ToLower(records[j].Name)
	})
	return records, nil
}

func (s *Service) RegisterLocal(ctx context.Context, repoPath string) (RepositoryRecord, error) {
	root, err := s.Git.ResolveRepository(ctx, repoPath)
	if err != nil {
		return RepositoryRecord{}, err
	}
	remote, _ := s.Git.RemoteURL(ctx, root)
	record := RepositoryRecord{
		Kind:      "local",
		Path:      root,
		Name:      filepath.Base(root),
		RemoteURL: remote,
		AddedAt:   time.Now().UTC(),
	}
	if owner, name, ok := parseGitHubCloneURL(remote, s.cfg.WebBaseURL); ok {
		record.FullName = owner + "/" + name
		record.Name = name
	}
	return s.upsertRecord(record)
}

func (s *Service) CloneRepository(ctx context.Context, installationID, repositoryID int64, destination string) (RepositoryRecord, error) {
	if _, err := s.ValidateUserInstallation(ctx, installationID); err != nil {
		return RepositoryRecord{}, err
	}
	repos, err := s.RepositoriesForUserInstallation(ctx, installationID)
	if err != nil {
		return RepositoryRecord{}, err
	}
	var repo Repository
	found := false
	for _, candidate := range repos {
		if candidate.ID == repositoryID {
			repo = candidate
			found = true
			break
		}
	}
	if !found {
		return RepositoryRecord{}, errors.New("repositorio no accesible para el usuario en esa instalación")
	}
	if repo.Archived || repo.Disabled {
		return RepositoryRecord{}, errors.New("repositorio archivado o deshabilitado")
	}
	token, err := s.installationToken(ctx, installationID)
	if err != nil {
		return RepositoryRecord{}, err
	}
	path, err := s.Git.Clone(ctx, repo, destination, token)
	if err != nil {
		return RepositoryRecord{}, err
	}
	record := RepositoryRecord{
		Kind:           "clone",
		Path:           path,
		Name:           repo.Name,
		FullName:       repo.FullName,
		RemoteURL:      repo.CloneURL,
		RepositoryID:   repo.ID,
		InstallationID: installationID,
		DefaultBranch:  repo.DefaultBranch,
		AddedAt:        time.Now().UTC(),
	}
	persisted, err := s.upsertRecord(record)
	if err != nil {
		_ = os.RemoveAll(path)
		return RepositoryRecord{}, err
	}
	return persisted, nil
}

// ClonePublic clones a public repository without credentials. The URL must
// belong to the configured GitHub web host (github.com by default).
func (s *Service) ClonePublic(ctx context.Context, cloneURL, destination string) (RepositoryRecord, error) {
	owner, name, ok := parseGitHubCloneURL(cloneURL, s.cfg.WebBaseURL)
	if !ok {
		return RepositoryRecord{}, errors.New("URL de clone pública no pertenece al GitHub configurado")
	}
	repo := Repository{Name: name, Owner: owner, FullName: owner + "/" + name, CloneURL: cloneURL}
	path, err := s.Git.Clone(ctx, repo, destination, "")
	if err != nil {
		return RepositoryRecord{}, err
	}
	record := RepositoryRecord{Kind: "clone", Path: path, Name: name, FullName: repo.FullName, RemoteURL: cloneURL, AddedAt: time.Now().UTC()}
	persisted, err := s.upsertRecord(record)
	if err != nil {
		_ = os.RemoveAll(path)
		return RepositoryRecord{}, err
	}
	return persisted, nil
}

func parseGitHubCloneURL(raw, webBase string) (owner, name string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return "", "", false
	}
	base, err := url.Parse(webBase)
	if err != nil || !strings.EqualFold(u.Hostname(), base.Hostname()) {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner, err = safeRepoComponent(parts[0])
	if err != nil {
		return "", "", false
	}
	name, err = safeRepoComponent(parts[1])
	if err != nil {
		return "", "", false
	}
	return owner, name, true
}

func (s *Service) upsertRecord(record RepositoryRecord) (RepositoryRecord, error) {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	if record.Path == "" {
		return RepositoryRecord{}, errors.New("ruta de repositorio vacía")
	}
	abs, err := filepath.Abs(record.Path)
	if err != nil {
		return RepositoryRecord{}, err
	}
	record.Path = filepath.Clean(abs)
	records, err := s.store.LoadRegistry()
	if err != nil {
		return RepositoryRecord{}, err
	}
	for i := range records {
		if records[i].Path == record.Path {
			record.ID = records[i].ID
			if record.AddedAt.IsZero() {
				record.AddedAt = records[i].AddedAt
			}
			records[i] = record
			if err := s.store.SaveRegistry(records); err != nil {
				return RepositoryRecord{}, err
			}
			return record, nil
		}
	}
	if record.ID == "" {
		record.ID, err = randomID()
		if err != nil {
			return RepositoryRecord{}, err
		}
	}
	if record.AddedAt.IsZero() {
		record.AddedAt = time.Now().UTC()
	}
	records = append(records, record)
	if err := s.store.SaveRegistry(records); err != nil {
		return RepositoryRecord{}, err
	}
	return record, nil
}

func (s *Service) ForgetRepository(id string) error {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	records, err := s.store.LoadRegistry()
	if err != nil {
		return err
	}
	filtered := records[:0]
	found := false
	for _, record := range records {
		if record.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, record)
	}
	if !found {
		return errors.New("repositorio registrado no encontrado")
	}
	return s.store.SaveRegistry(filtered)
}

func (s *Service) repositoryRecord(id string) (RepositoryRecord, error) {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	records, err := s.store.LoadRegistry()
	if err != nil {
		return RepositoryRecord{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return RepositoryRecord{}, errors.New("repositorio registrado no encontrado")
}

func (s *Service) FetchRepository(ctx context.Context, id string) error {
	record, err := s.repositoryRecord(id)
	if err != nil {
		return err
	}
	credential := ""
	if record.InstallationID > 0 {
		if _, err := s.ValidateUserInstallation(ctx, record.InstallationID); err != nil {
			return err
		}
		credential, err = s.installationToken(ctx, record.InstallationID)
		if err != nil {
			return err
		}
	}
	return s.Git.Fetch(ctx, record.Path, credential)
}

func (s *Service) PullRepository(ctx context.Context, id string) error {
	record, err := s.repositoryRecord(id)
	if err != nil {
		return err
	}
	credential := ""
	if record.InstallationID > 0 {
		if _, err := s.ValidateUserInstallation(ctx, record.InstallationID); err != nil {
			return err
		}
		credential, err = s.installationToken(ctx, record.InstallationID)
		if err != nil {
			return err
		}
	}
	return s.Git.PullFastForward(ctx, record.Path, credential)
}

func (s *Service) RepositoryStatus(ctx context.Context, id string) (WorkingTreeStatus, error) {
	record, err := s.repositoryRecord(id)
	if err != nil {
		return WorkingTreeStatus{}, err
	}
	return s.Git.Status(ctx, record.Path)
}

func (s *Service) RepositoryBranches(ctx context.Context, id string) ([]Branch, error) {
	record, err := s.repositoryRecord(id)
	if err != nil {
		return nil, err
	}
	return s.Git.Branches(ctx, record.Path)
}

func (s *Service) CheckoutRepositoryBranch(ctx context.Context, id, branch string) error {
	record, err := s.repositoryRecord(id)
	if err != nil {
		return err
	}
	return s.Git.CheckoutBranch(ctx, record.Path, branch)
}

func (s *Service) RepositoryCommits(ctx context.Context, id, ref string, limit int) ([]Commit, error) {
	record, err := s.repositoryRecord(id)
	if err != nil {
		return nil, err
	}
	return s.Git.Commits(ctx, record.Path, ref, limit)
}

func (s *Service) DescribeRepository(id string) (RepositoryRecord, error) {
	return s.repositoryRecord(id)
}

func (s *Service) RemoveClonedRepository(ctx context.Context, id string) error {
	if ctx == nil {
		return errors.New("dexgithub: context requerido")
	}
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	records, err := s.store.LoadRegistry()
	if err != nil {
		return err
	}
	index := -1
	var record RepositoryRecord
	for i, candidate := range records {
		if candidate.ID == id {
			index = i
			record = candidate
			break
		}
	}
	if index < 0 {
		return errors.New("repositorio registrado no encontrado")
	}
	if record.Kind != "clone" {
		return errors.New("los repositorios locales sólo pueden olvidarse, no eliminarse")
	}
	root := s.CloneRoot()
	abs, err := filepath.Abs(record.Path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if !withinPath(abs, root) || abs == root {
		return errors.New("ruta clonada fuera de la raíz administrada")
	}
	if _, err := s.Git.ResolveRepository(ctx, abs); err != nil {
		return fmt.Errorf("la ruta ya no es un repositorio Git válido: %w", err)
	}
	if err := os.RemoveAll(abs); err != nil {
		return err
	}
	records = append(records[:index], records[index+1:]...)
	return s.store.SaveRegistry(records)
}
