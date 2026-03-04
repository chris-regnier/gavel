package harness

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// RepositoryManager handles cloning and caching of external repositories
type RepositoryManager struct {
	cacheDir string
	repos    map[string]string // name -> local path
	mu       sync.RWMutex      // protects repos map
}

// NewRepositoryManager creates a new repository manager
func NewRepositoryManager(cacheDir string) *RepositoryManager {
	if cacheDir == "" {
		cacheDir = ".gavel/cache"
	}
	return &RepositoryManager{
		cacheDir: cacheDir,
		repos:    make(map[string]string),
	}
}

// Prepare clones all repositories and returns a map of name -> local path
func (rm *RepositoryManager) Prepare(ctx context.Context, repos []RepositoryConfig) (map[string]string, error) {
	if err := os.MkdirAll(rm.cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	for _, repo := range repos {
		path, err := rm.cloneRepo(ctx, repo)
		if err != nil {
			return nil, fmt.Errorf("cloning %s: %w", repo.Name, err)
		}
		rm.mu.Lock()
		rm.repos[repo.Name] = path
		rm.mu.Unlock()
	}

	rm.mu.RLock()
	result := make(map[string]string, len(rm.repos))
	for k, v := range rm.repos {
		result[k] = v
	}
	rm.mu.RUnlock()

	return result, nil
}

// GetPath returns the local path for a cloned repository
func (rm *RepositoryManager) GetPath(name string) (string, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	path, ok := rm.repos[name]
	return path, ok
}

// Cleanup removes all cached repositories
func (rm *RepositoryManager) Cleanup() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for name, path := range rm.repos {
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("Failed to clean up cached repo", "name", name, "error", err)
		}
	}
	rm.repos = make(map[string]string)

	return os.RemoveAll(rm.cacheDir)
}

// cloneRepo clones a repository to the cache directory
func (rm *RepositoryManager) cloneRepo(ctx context.Context, repo RepositoryConfig) (string, error) {
	// Create a safe directory name from the repo name
	safeName := sanitizeRepoName(repo.Name)
	repoPath := filepath.Join(rm.cacheDir, safeName)

	// Check if already cloned
	if info, err := os.Stat(repoPath); err == nil && info.IsDir() {
		// Check if it's a git repo
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
			slog.Info("Repository already cached", "name", repo.Name, "path", repoPath)
			
			// If a specific commit/tag is requested, check it out
			if repo.Commit != "" || repo.Tag != "" {
				if err := rm.checkoutRef(ctx, repoPath, repo); err != nil {
					return "", fmt.Errorf("checking out ref: %w", err)
				}
			}
			return repoPath, nil
		}
	}

	// Remove if exists but not a git repo
	os.RemoveAll(repoPath)

	slog.Info("Cloning repository", "name", repo.Name, "url", repo.URL)

	// Build git clone command
	args := []string{"clone"}
	
	// Add depth for shallow clone
	depth := repo.Depth
	if depth == 0 {
		depth = 1 // Default to shallow clone
	}
	args = append(args, "--depth", fmt.Sprintf("%d", depth))

	// Add branch if specified
	if repo.Branch != "" {
		args = append(args, "--branch", repo.Branch)
	}

	args = append(args, repo.URL, repoPath)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0") // Don't prompt for credentials

	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone failed: %w\n%s", err, output)
	}

	// Checkout specific commit or tag if specified
	if repo.Commit != "" || repo.Tag != "" {
		if err := rm.checkoutRef(ctx, repoPath, repo); err != nil {
			return "", fmt.Errorf("checking out ref: %w", err)
		}
	}

	return repoPath, nil
}

// checkoutRef checks out a specific commit or tag
func (rm *RepositoryManager) checkoutRef(ctx context.Context, repoPath string, repo RepositoryConfig) error {
	ref := repo.Commit
	if ref == "" {
		ref = repo.Tag
	}
	if ref == "" {
		return nil
	}

	slog.Info("Checking out ref", "repo", repo.Name, "ref", ref)

	cmd := exec.CommandContext(ctx, "git", "checkout", ref)
	cmd.Dir = repoPath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout failed: %w\n%s", err, output)
	}

	return nil
}

// ResolveTargets converts TargetConfig entries to actual filesystem paths
func (rm *RepositoryManager) ResolveTargets(targets []TargetConfig, packages []string) ([]string, error) {
	var paths []string

	// Handle deprecated packages field
	for _, pkg := range packages {
		paths = append(paths, pkg)
	}

	// Handle new targets field
	for _, target := range targets {
		if target.Path != "" {
			// Local path
			paths = append(paths, target.Path)
		} else if target.Repo != "" {
			// External repo
			repoPath, ok := rm.GetPath(target.Repo)
			if !ok {
				return nil, fmt.Errorf("unknown repo: %s", target.Repo)
			}

			if len(target.Paths) > 0 {
				// Specific subdirectories within the repo
				for _, subPath := range target.Paths {
					fullPath := filepath.Join(repoPath, subPath)
					paths = append(paths, fullPath)
				}
			} else {
				// Analyze entire repo
				paths = append(paths, repoPath)
			}
		}
	}

	return paths, nil
}
