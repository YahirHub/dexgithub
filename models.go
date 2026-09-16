package dexgithub

import "time"

// AppCredentials are the sensitive credentials of a GitHub App.
// PrivateKeyPEM, ClientSecret and WebhookSecret must be treated as secrets.
type AppCredentials struct {
	AppID         int64     `json:"app_id"`
	ClientID      string    `json:"client_id"`
	ClientSecret  string    `json:"client_secret"`
	WebhookSecret string    `json:"webhook_secret,omitempty"`
	PrivateKeyPEM string    `json:"private_key_pem"`
	Name          string    `json:"name,omitempty"`
	Slug          string    `json:"slug,omitempty"`
	HTMLURL       string    `json:"html_url,omitempty"`
	OwnerLogin    string    `json:"owner_login,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

func (c AppCredentials) Valid() bool {
	return c.AppID > 0 && c.ClientID != "" && c.ClientSecret != "" && c.PrivateKeyPEM != ""
}

// UserToken is a GitHub App user access token and optional refresh token.
type UserToken struct {
	AccessToken      string    `json:"access_token"`
	TokenType        string    `json:"token_type,omitempty"`
	Scope            string    `json:"scope,omitempty"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	RefreshToken     string    `json:"refresh_token,omitempty"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitempty"`
	Login            string    `json:"login,omitempty"`
	UserID           int64     `json:"user_id,omitempty"`
}

func (t UserToken) ExpiringSoon(now time.Time) bool {
	return !t.ExpiresAt.IsZero() && !now.Add(2*time.Minute).Before(t.ExpiresAt)
}

// Installation describes a GitHub App installation visible to a user or app.
type Installation struct {
	ID                  int64             `json:"id"`
	AccountLogin        string            `json:"account_login"`
	AccountType         string            `json:"account_type"`
	AccountID           int64             `json:"account_id"`
	RepositorySelection string            `json:"repository_selection,omitempty"`
	Permissions         map[string]string `json:"permissions,omitempty"`
	SuspendedAt         *time.Time        `json:"suspended_at,omitempty"`
}

// Repository describes a repository exposed by a GitHub App installation.
type Repository struct {
	ID             int64           `json:"id"`
	NodeID         string          `json:"node_id,omitempty"`
	Name           string          `json:"name"`
	FullName       string          `json:"full_name"`
	Owner          string          `json:"owner"`
	Private        bool            `json:"private"`
	DefaultBranch  string          `json:"default_branch,omitempty"`
	CloneURL       string          `json:"clone_url"`
	SSHURL         string          `json:"ssh_url,omitempty"`
	HTMLURL        string          `json:"html_url,omitempty"`
	Archived       bool            `json:"archived,omitempty"`
	Disabled       bool            `json:"disabled,omitempty"`
	CreatedAt      time.Time       `json:"created_at,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at,omitempty"`
	PushedAt       time.Time       `json:"pushed_at,omitempty"`
	Permissions    map[string]bool `json:"permissions,omitempty"`
	InstallationID int64           `json:"installation_id,omitempty"`
}

// GitHubUser identifies the connected GitHub account.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
}

// RepositoryRecord persists a local repository known by the library.
type RepositoryRecord struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"` // clone or local
	Path           string    `json:"path"`
	Name           string    `json:"name"`
	FullName       string    `json:"full_name,omitempty"`
	RemoteURL      string    `json:"remote_url,omitempty"`
	RepositoryID   int64     `json:"repository_id,omitempty"`
	InstallationID int64     `json:"installation_id,omitempty"`
	DefaultBranch  string    `json:"default_branch,omitempty"`
	AddedAt        time.Time `json:"added_at"`
}

// Settings are persisted non-secret library settings.
type Settings struct {
	CloneRoot string `json:"clone_root"`
}

// Branch is a local or remote branch reference.
type Branch struct {
	Name     string `json:"name"`
	FullRef  string `json:"full_ref"`
	Commit   string `json:"commit"`
	Current  bool   `json:"current"`
	Remote   bool   `json:"remote"`
	Upstream string `json:"upstream,omitempty"`
}

// Commit is a compact immutable view of a Git commit.
type Commit struct {
	Hash        string    `json:"hash"`
	Parents     []string  `json:"parents,omitempty"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	AuthoredAt  time.Time `json:"authored_at"`
	Subject     string    `json:"subject"`
}

// WorkingTreeStatus summarizes porcelain v1 status entries.
type WorkingTreeStatus struct {
	Branch  string        `json:"branch,omitempty"`
	Dirty   bool          `json:"dirty"`
	Entries []StatusEntry `json:"entries,omitempty"`
}

type StatusEntry struct {
	IndexCode string `json:"index_code"`
	WorkCode  string `json:"work_code"`
	Path      string `json:"path"`
}
