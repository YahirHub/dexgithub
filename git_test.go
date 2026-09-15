package dexgithub

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func makeTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "Test User")
	run("config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("uno\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "Primer commit")
	run("switch", "-c", "feature/demo")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("uno\ndos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("commit", "-am", "Segundo commit")
	return dir
}

func TestGitManagerStatusBranchesCheckoutAndCommits(t *testing.T) {
	repo := makeTestRepo(t)
	manager, err := NewGitManager(GitConfig{DefaultRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	status, err := manager.Status(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch != "feature/demo" || status.Dirty {
		t.Fatalf("estado inesperado: %+v", status)
	}

	branches, err := manager.Branches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 {
		t.Fatalf("esperaba 2 ramas locales, obtuvo %d: %+v", len(branches), branches)
	}

	commits, err := manager.Commits(ctx, repo, "HEAD", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("esperaba 2 commits, obtuvo %d: %+v", len(commits), commits)
	}
	if commits[0].Subject != "Segundo commit" || commits[1].Subject != "Primer commit" {
		t.Fatalf("orden/asunto inesperado: %+v", commits)
	}
	if strings.TrimSpace(commits[0].Hash) != commits[0].Hash {
		t.Fatalf("hash contiene whitespace: %q", commits[0].Hash)
	}

	if err := manager.CheckoutBranch(ctx, repo, "main"); err != nil {
		t.Fatal(err)
	}
	status, err = manager.Status(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch != "main" {
		t.Fatalf("rama actual = %q", status.Branch)
	}
}

func TestGitManagerCloneKeepsCredentialOutOfRemote(t *testing.T) {
	source := makeTestRepo(t)
	root := t.TempDir()
	manager, err := NewGitManager(GitConfig{DefaultRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	cloned, err := manager.Clone(context.Background(), Repository{Owner: "owner", Name: "repo", CloneURL: source}, "", "ghs_secret_value")
	if err != nil {
		t.Fatal(err)
	}
	remote, err := manager.RemoteURL(context.Background(), cloned)
	if err != nil {
		t.Fatal(err)
	}
	if remote != source {
		t.Fatalf("remote alterado: %q", remote)
	}
	gitConfig, err := os.ReadFile(filepath.Join(cloned, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gitConfig), "ghs_secret_value") || strings.Contains(string(gitConfig), "x-access-token") {
		t.Fatalf("credencial persistida en .git/config: %s", gitConfig)
	}
}

func TestGitManagerRejectsCloneOutsideRoot(t *testing.T) {
	root := t.TempDir()
	manager, err := NewGitManager(GitConfig{DefaultRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Clone(context.Background(), Repository{Owner: "owner", Name: "repo", CloneURL: "https://github.com/owner/repo.git"}, filepath.Join(filepath.Dir(root), "escape"), "")
	if err == nil || !strings.Contains(err.Error(), "fuera de la raíz") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestRegisterLocalRepository(t *testing.T) {
	repo := makeTestRepo(t)
	root := t.TempDir()
	service, err := New(Config{RootDir: root, CloneRoot: filepath.Join(root, "repos")})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.RegisterLocal(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != "local" || record.ID == "" || record.Path != repo {
		t.Fatalf("registro inesperado: %+v", record)
	}
	known, err := service.KnownRepositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(known) != 1 || known[0].ID != record.ID {
		t.Fatalf("registry inesperado: %+v", known)
	}
}
