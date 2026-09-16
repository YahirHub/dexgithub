package dexgithub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileStorePermissionsAndRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permisos POSIX")
	}
	root := filepath.Join(t.TempDir(), "github")
	store := NewFileStore(root)
	app := AppCredentials{AppID: 1, ClientID: "client", ClientSecret: "secret", PrivateKeyPEM: "pem"}
	if err := store.SaveApp(app); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadApp()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ClientSecret != "secret" {
		t.Fatalf("roundtrip: %+v", loaded)
	}
	baseInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if baseInfo.Mode().Perm() != 0o700 {
		t.Fatalf("root mode=%o", baseInfo.Mode().Perm())
	}
	secretInfo, err := os.Stat(filepath.Join(root, ".dexgithub", "app.json"))
	if err != nil {
		t.Fatal(err)
	}
	if secretInfo.Mode().Perm() != 0o600 {
		t.Fatalf("app mode=%o", secretInfo.Mode().Perm())
	}
}

func TestFileStoreRejectsSymlinkedSecretDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink POSIX")
	}
	base := filepath.Join(t.TempDir(), "github")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(base, ".dexgithub")); err != nil {
		t.Fatal(err)
	}
	store := NewFileStore(base)
	err := store.SaveSettings(Settings{CloneRoot: filepath.Join(base, "repos")})
	if err == nil {
		t.Fatal("esperaba rechazo de symlink")
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	body := []byte(`{"action":"push"}`)
	secret := "hook-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifyWebhookSignature(secret, body, signature) {
		t.Fatal("firma válida rechazada")
	}
	if VerifyWebhookSignature(secret, append(body, '!'), signature) {
		t.Fatal("firma inválida aceptada")
	}
}

func TestFileStoreAtUsesExactDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permisos POSIX")
	}
	dir := filepath.Join(t.TempDir(), ".dex", "dexgithub")
	store := NewFileStoreAt(dir)
	if err := store.SaveSettings(Settings{CloneRoot: filepath.Join(t.TempDir(), "repos")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatalf("StateDir exacto no contiene settings.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".dexgithub")); !os.IsNotExist(err) {
		t.Fatalf("NewFileStoreAt no debe añadir un subdirectorio .dexgithub, err=%v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("state dir mode=%o", info.Mode().Perm())
	}
}

func TestConfigStateDirUsesExactDirectory(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), ".dex", "dexgithub")
	service, err := New(Config{RootDir: t.TempDir(), StateDir: stateDir, CloneRoot: filepath.Join(t.TempDir(), "repos")})
	if err != nil {
		t.Fatal(err)
	}
	cfg := service.Config()
	if cfg.StateDir != filepath.Clean(stateDir) {
		t.Fatalf("StateDir=%q; esperaba %q", cfg.StateDir, filepath.Clean(stateDir))
	}
	if err := service.store.SaveSettings(Settings{CloneRoot: cfg.CloneRoot}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "settings.json")); err != nil {
		t.Fatalf("estado no se guardó en StateDir exacto: %v", err)
	}
}
