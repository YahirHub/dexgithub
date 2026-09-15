package dexgithub

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxGitOutputBytes = 8 << 20

type GitConfig struct {
	Binary      string
	DefaultRoot string
	Timeout     time.Duration
}

// GitManager wraps the host Git executable. It never invokes a shell for Git
// operations and never embeds credentials in repository URLs.
type GitManager struct {
	binary      string
	defaultRoot string
	timeout     time.Duration
	mu          sync.RWMutex
}

func NewGitManager(cfg GitConfig) (*GitManager, error) {
	if strings.TrimSpace(cfg.Binary) == "" {
		cfg.Binary = "git"
	}
	path, err := exec.LookPath(cfg.Binary)
	if err != nil {
		return nil, fmt.Errorf("git no disponible: %w", err)
	}
	root, err := normalizeCloneRoot(cfg.DefaultRoot)
	if err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	return &GitManager{binary: path, defaultRoot: root, timeout: cfg.Timeout}, nil
}

func normalizeCloneRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(DefaultRootDir, "repos")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if abs == "/" || abs == "." {
		return "", errors.New("dexgithub: raíz de clones demasiado amplia")
	}
	return abs, nil
}

func ensureCloneRoot(root string) error {
	if info, err := os.Lstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("dexgithub: raíz de clones no es un directorio real")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(root, 0o700)
}

func (g *GitManager) DefaultRoot() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.defaultRoot
}

func (g *GitManager) run(ctx context.Context, dir string, credential string, args ...string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("dexgithub: context requerido")
	}
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		dir = filepath.Clean(abs)
	}

	cmd := exec.CommandContext(ctx, g.binary, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var cleanup func()
	if credential != "" {
		helper, err := createAskPassHelper()
		if err != nil {
			return nil, err
		}
		cleanup = func() { _ = os.Remove(helper) }
		defer cleanup()
		cmd.Env = executableEnv(map[string]string{
			"GIT_ASKPASS":         helper,
			"GIT_TERMINAL_PROMPT": "0",
			"DEXGITHUB_TOKEN":     credential,
			"DEXGITHUB_USERNAME":  "x-access-token",
		})
	} else {
		cmd.Env = executableEnv(map[string]string{"GIT_TERMINAL_PROMPT": "0"})
	}

	var stdout, stderr cappedBuffer
	stdout.max = maxGitOutputBytes
	stderr.max = maxGitOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git cancelado/timeout: %w", ctx.Err())
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 2048 {
			msg = msg[:2048]
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New("git: " + msg)
	}
	if stdout.overflow || stderr.overflow {
		return nil, errors.New("git: salida excede 8 MiB")
	}
	return stdout.Bytes(), nil
}

type cappedBuffer struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.overflow = true
		return original, nil
	}
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return original, nil
}

func (b *cappedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *cappedBuffer) String() string { return b.buf.String() }

func createAskPassHelper() (string, error) {
	f, err := os.CreateTemp("", "dexgithub-askpass-*.sh")
	if err != nil {
		return "", err
	}
	path := f.Name()
	content := `#!/bin/sh
case "$1" in
  *Username*|*username*) printf '%s\n' "$DEXGITHUB_USERNAME" ;;
  *Password*|*password*) printf '%s\n' "$DEXGITHUB_TOKEN" ;;
  *) exit 1 ;;
esac
`
	if err := f.Chmod(0o700); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if _, err := io.WriteString(f, content); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func safeRepoComponent(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\\\\x00\r\n") {
		return "", errors.New("componente de repositorio inválido")
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' && r != '.' {
			return "", errors.New("componente de repositorio contiene caracteres no admitidos")
		}
	}
	return value, nil
}

func (g *GitManager) Clone(ctx context.Context, repo Repository, destination, credential string) (string, error) {
	if strings.TrimSpace(repo.CloneURL) == "" {
		return "", errors.New("repositorio sin clone_url HTTPS")
	}
	owner, err := safeRepoComponent(repo.Owner)
	if err != nil {
		return "", err
	}
	name, err := safeRepoComponent(repo.Name)
	if err != nil {
		return "", err
	}
	root := g.DefaultRoot()
	if err := ensureCloneRoot(root); err != nil {
		return "", err
	}
	if destination == "" {
		destination = filepath.Join(root, owner, name)
	} else if !filepath.IsAbs(destination) {
		destination = filepath.Join(root, destination)
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if !withinPath(abs, root) {
		return "", errors.New("destino de clone fuera de la raíz configurada")
	}
	if _, err := os.Lstat(abs); err == nil {
		return "", errors.New("el destino del clone ya existe")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return "", err
	}
	_, err = g.run(ctx, "", credential, "clone", "--origin", "origin", "--", repo.CloneURL, abs)
	if err != nil {
		_ = os.RemoveAll(abs)
		return "", err
	}
	return abs, nil
}

func withinPath(target, base string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (g *GitManager) ResolveRepository(ctx context.Context, repoPath string) (string, error) {
	if strings.TrimSpace(repoPath) == "" {
		return "", errors.New("ruta de repositorio vacía")
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	out, err := g.run(ctx, resolved, "", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	top := strings.TrimSpace(string(out))
	if top == "" {
		return "", errors.New("git no devolvió raíz del repositorio")
	}
	return filepath.Clean(top), nil
}

func (g *GitManager) Status(ctx context.Context, repoPath string) (WorkingTreeStatus, error) {
	root, err := g.ResolveRepository(ctx, repoPath)
	if err != nil {
		return WorkingTreeStatus{}, err
	}
	out, err := g.run(ctx, root, "", "status", "--porcelain=v1", "--branch", "-z", "--untracked-files=normal")
	if err != nil {
		return WorkingTreeStatus{}, err
	}
	parts := bytes.Split(out, []byte{0})
	status := WorkingTreeStatus{}
	for _, raw := range parts {
		if len(raw) == 0 {
			continue
		}
		line := string(raw)
		if strings.HasPrefix(line, "## ") {
			branch := strings.TrimPrefix(line, "## ")
			if idx := strings.Index(branch, "..."); idx >= 0 {
				branch = branch[:idx]
			}
			status.Branch = strings.TrimSpace(branch)
			continue
		}
		if len(line) < 3 {
			continue
		}
		entry := StatusEntry{IndexCode: line[:1], WorkCode: line[1:2], Path: strings.TrimSpace(line[3:])}
		status.Entries = append(status.Entries, entry)
	}
	status.Dirty = len(status.Entries) > 0
	return status, nil
}

func (g *GitManager) Branches(ctx context.Context, repoPath string) ([]Branch, error) {
	root, err := g.ResolveRepository(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	format := "%(refname)%09%(objectname)%09%(HEAD)%09%(upstream:short)"
	out, err := g.run(ctx, root, "", "for-each-ref", "--format="+format, "refs/heads/", "refs/remotes/")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	branches := make([]Branch, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 3 {
			continue
		}
		full := fields[0]
		branch := Branch{FullRef: full, Commit: fields[1], Current: fields[2] == "*"}
		if len(fields) == 4 {
			branch.Upstream = fields[3]
		}
		switch {
		case strings.HasPrefix(full, "refs/heads/"):
			branch.Name = strings.TrimPrefix(full, "refs/heads/")
		case strings.HasPrefix(full, "refs/remotes/"):
			branch.Name = strings.TrimPrefix(full, "refs/remotes/")
			branch.Remote = true
		default:
			continue
		}
		branches = append(branches, branch)
	}
	return branches, nil
}

func validateBranchName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "-") || len(name) > 255 || strings.ContainsAny(name, "\r\n\x00") {
		return errors.New("nombre de rama inválido")
	}
	return nil
}

func (g *GitManager) CheckoutBranch(ctx context.Context, repoPath, branch string) error {
	if err := validateBranchName(branch); err != nil {
		return err
	}
	root, err := g.ResolveRepository(ctx, repoPath)
	if err != nil {
		return err
	}
	if _, err := g.run(ctx, root, "", "check-ref-format", "--branch", branch); err != nil {
		return errors.New("nombre de rama no aceptado por Git")
	}
	if _, err := g.run(ctx, root, "", "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err = g.run(ctx, root, "", "switch", branch)
		return err
	}
	remoteRef := "refs/remotes/origin/" + branch
	if _, err := g.run(ctx, root, "", "show-ref", "--verify", "--quiet", remoteRef); err == nil {
		_, err = g.run(ctx, root, "", "switch", "--track", "-c", branch, "origin/"+branch)
		return err
	}
	return errors.New("rama no encontrada localmente ni en origin")
}

func validateRevision(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "-") || len(ref) > 255 || strings.ContainsAny(ref, "\r\n\x00") {
		return errors.New("referencia Git inválida")
	}
	return nil
}

func (g *GitManager) Commits(ctx context.Context, repoPath, ref string, limit int) ([]Commit, error) {
	if ref == "" {
		ref = "HEAD"
	}
	if err := validateRevision(ref); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		return nil, errors.New("máximo 500 commits por consulta")
	}
	root, err := g.ResolveRepository(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	format := "%H%x00%P%x00%an%x00%ae%x00%aI%x00%s%x00"
	out, err := g.run(ctx, root, "", "log", "-n", strconv.Itoa(limit), "--format="+format, ref, "--")
	if err != nil {
		return nil, err
	}
	fields := bytes.Split(out, []byte{0})
	commits := make([]Commit, 0, limit)
	for i := 0; i+5 < len(fields); i += 6 {
		if len(fields[i]) == 0 {
			continue
		}
		when, parseErr := time.Parse(time.RFC3339, string(fields[i+4]))
		if parseErr != nil {
			return nil, fmt.Errorf("fecha de commit inválida: %w", parseErr)
		}
		commit := Commit{
			Hash:        string(fields[i]),
			AuthorName:  string(fields[i+2]),
			AuthorEmail: string(fields[i+3]),
			AuthoredAt:  when,
			Subject:     string(fields[i+5]),
		}
		if parents := strings.Fields(string(fields[i+1])); len(parents) > 0 {
			commit.Parents = parents
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

func (g *GitManager) Fetch(ctx context.Context, repoPath, credential string) error {
	root, err := g.ResolveRepository(ctx, repoPath)
	if err != nil {
		return err
	}
	_, err = g.run(ctx, root, credential, "fetch", "--prune", "--tags", "origin")
	return err
}

func (g *GitManager) PullFastForward(ctx context.Context, repoPath, credential string) error {
	root, err := g.ResolveRepository(ctx, repoPath)
	if err != nil {
		return err
	}
	_, err = g.run(ctx, root, credential, "pull", "--ff-only")
	return err
}

func (g *GitManager) RemoteURL(ctx context.Context, repoPath string) (string, error) {
	root, err := g.ResolveRepository(ctx, repoPath)
	if err != nil {
		return "", err
	}
	out, err := g.run(ctx, root, "", "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
