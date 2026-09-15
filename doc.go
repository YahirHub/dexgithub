// Package dexgithub provides GitHub App authentication and safe local Git
// repository management for single-host control panels and deployment tools.
//
// The package keeps GitHub installation tokens in memory, avoids embedding
// credentials in Git remotes and uses the host Git executable for repository
// operations.
package dexgithub
