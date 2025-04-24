package githubops

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v58/github"
)

type GitHubService interface {
	Search() *github.SearchService
	Repositories() *github.RepositoriesService
	Git() *github.GitService
	PullRequests() *github.PullRequestsService
}

type GitHubClient struct {
	client GitHubService
	ctx    context.Context
	logger *log.Logger
}

func NewGitHubClient(ctx context.Context, client GitHubService) *GitHubClient {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("Error creating logs directory: %v", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFile, err := os.Create(fmt.Sprintf("logs/renovate-updater_%s.log", timestamp))
	if err != nil {
		log.Fatalf("Error creating log file: %v", err)
	}

	// Create logger that writes to both file and console
	logger := log.New(logFile, "", log.LstdFlags)

	return &GitHubClient{
		client: client,
		ctx:    ctx,
		logger: logger,
	}
}

func (g *GitHubClient) Log(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	g.logger.Println(message)
	fmt.Println(message) // Also print to console
}

func (g *GitHubClient) FindReposWithRenovate(org string) ([]*github.Repository, error) {
	g.Log("Finding repositories with renovate.json for org: %s", org)

	var allRepos []*github.Repository
	opts := &github.RepositoryListByOrgOptions{
		Type: "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	for {
		repos, resp, err := g.client.Repositories().ListByOrg(g.ctx, org, opts)
		if err != nil {
			g.Log("Error listing repositories: %v", err)
			if resp != nil {
				g.Log("Response status: %s", resp.Status)
				g.Log("Response headers: %+v", resp.Header)
			}
			return nil, err
		}

		g.Log("Found %d repositories in org (page %d)", len(repos), opts.Page)
		if resp != nil {
			g.Log("Rate limit: %d/%d remaining", resp.Rate.Remaining, resp.Rate.Limit)

			// Check if we're close to the rate limit
			if resp.Rate.Remaining < 100 {
				g.Log("Approaching rate limit. Remaining: %d", resp.Rate.Remaining)
			}

			// If we hit the rate limit, wait until it resets
			if resp.Rate.Remaining == 0 {
				resetTime := resp.Rate.Reset.Time
				waitTime := time.Until(resetTime)
				if waitTime > 0 {
					g.Log("Rate limit reached. Waiting %v until reset at %v", waitTime, resetTime)
					time.Sleep(waitTime)
					continue // Retry the same page
				}
			}
		}

		// Check each repository for renovate.json
		for _, repo := range repos {
			// Try both root and .github directory
			paths := []string{"renovate.json", ".github/renovate.json"}
			for _, path := range paths {
				_, _, resp, err := g.client.Repositories().GetContents(g.ctx, org, repo.GetName(), path, nil)
				if err == nil {
					g.Log("Found renovate.json in %s at %s", repo.GetFullName(), path)
					allRepos = append(allRepos, repo)
					break
				}
				if resp != nil {
					if resp.StatusCode == 404 {
						continue // File not found, try next path
					}
					// Check rate limit for each request
					if resp.Rate.Remaining == 0 {
						resetTime := resp.Rate.Reset.Time
						waitTime := time.Until(resetTime)
						if waitTime > 0 {
							g.Log("Rate limit reached while checking files. Waiting %v until reset at %v", waitTime, resetTime)
							time.Sleep(waitTime)
							// Retry the same file check
							_, _, resp, err = g.client.Repositories().GetContents(g.ctx, org, repo.GetName(), path, nil)
							if err == nil {
								g.Log("Found renovate.json in %s at %s", repo.GetFullName(), path)
								allRepos = append(allRepos, repo)
								break
							}
						}
					}
				}
				if err != nil && !strings.Contains(err.Error(), "404") {
					g.Log("Error checking %s in %s: %v", path, repo.GetFullName(), err)
				}
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	g.Log("Total repositories with renovate.json found: %d", len(allRepos))
	return allRepos, nil
}

func (g *GitHubClient) createSignedCommit(repo *github.Repository, content []byte, gpgKeyID string) error {
	g.Log("Starting to create signed commit for repository: %s", repo.GetFullName())

	// Create a temporary directory for the git operations
	tempDir, err := os.MkdirTemp("", "github-renovate-updater-*")
	if err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	g.Log("Created temporary directory: %s", tempDir)

	// Clone the repository
	cloneURL := fmt.Sprintf("git@github.com:%s/%s.git", repo.GetOwner().GetLogin(), repo.GetName())
	g.Log("Cloning repository: %s", cloneURL)
	cmd := exec.Command("git", "clone", cloneURL, tempDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error cloning repository: %v, output: %s", err, string(output))
	}
	g.Log("Repository cloned successfully")

	// Create and checkout the new branch
	branchName := "update-renovate-config"
	g.Log("Creating and checking out branch: %s", branchName)
	cmd = exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = tempDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error creating branch: %v, output: %s", err, string(output))
	}
	g.Log("Branch created and checked out successfully")

	// Write the new content to renovate.json
	renovatePath := filepath.Join(tempDir, "renovate.json")
	g.Log("Writing renovate.json to: %s", renovatePath)
	if err := os.WriteFile(renovatePath, content, 0644); err != nil {
		return fmt.Errorf("error writing renovate.json: %v", err)
	}
	g.Log("renovate.json written successfully")

	// Add the file
	g.Log("Adding renovate.json to git")
	cmd = exec.Command("git", "add", "renovate.json")
	cmd.Dir = tempDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error adding file: %v, output: %s", err, string(output))
	}
	g.Log("File added to git successfully")

	// Configure git for signing
	if gpgKeyID != "" {
		g.Log("Configuring git for signing with key: %s", gpgKeyID)
		cmd = exec.Command("git", "config", "user.signingkey", gpgKeyID)
		cmd.Dir = tempDir
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("error configuring signing key: %v, output: %s", err, string(output))
		}
		g.Log("Git signing configured successfully")
	}

	// Create the commit
	commitCmd := []string{"commit", "-m", "Update renovate.json to use MyOtherOrg"}
	if gpgKeyID != "" {
		commitCmd = append(commitCmd, "-S")
	}
	g.Log("Creating commit with command: git %s", strings.Join(commitCmd, " "))
	cmd = exec.Command("git", commitCmd...)
	cmd.Dir = tempDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error creating commit: %v, output: %s", err, string(output))
	}
	g.Log("Commit created successfully")

	// Push the branch
	g.Log("Pushing branch to remote")
	cmd = exec.Command("git", "push", "origin", branchName)
	cmd.Dir = tempDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error pushing branch: %v, output: %s", err, string(output))
	}
	g.Log("Branch pushed successfully")

	return nil
}

func (g *GitHubClient) CheckAndUpdateRenovateConfig(repo *github.Repository, dryRun bool, gpgKeyID string) error {
	g.Log("Starting to check and update renovate config for repository: %s", repo.GetFullName())

	// Get the renovate.json file content
	g.Log("Getting renovate.json content")
	content, _, _, err := g.client.Repositories().GetContents(g.ctx, repo.GetOwner().GetLogin(), repo.GetName(), "renovate.json", nil)
	if err != nil {
		return fmt.Errorf("error getting renovate.json content: %v", err)
	}
	g.Log("Successfully retrieved renovate.json content")

	contentStr, err := content.GetContent()
	if err != nil {
		return fmt.Errorf("error decoding content: %v", err)
	}
	g.Log("Content decoded successfully")

	// Check if the content contains "github>MyOrg/"
	if !strings.Contains(contentStr, "github>MyOrg/") {
		g.Log("No need to update - content does not contain 'github>MyOrg/'")
		return nil // No need to update
	}
	g.Log("Content contains 'github>MyOrg/' - proceeding with update")

	// Replace the string
	newContent := strings.ReplaceAll(contentStr, "github>MyOrg/", "github>MyOtherOrg/")
	g.Log("Content updated successfully")

	if dryRun {
		g.Log("[DRY RUN] Would update renovate.json in %s", repo.GetFullName())
		return nil
	}

	// Create signed commit using Git CLI
	g.Log("Creating signed commit")
	if err := g.createSignedCommit(repo, []byte(newContent), gpgKeyID); err != nil {
		return fmt.Errorf("error creating signed commit: %v", err)
	}
	g.Log("Signed commit created successfully")

	// Get the default branch name
	g.Log("Getting default branch name")
	repoInfo, _, err := g.client.Repositories().Get(g.ctx, repo.GetOwner().GetLogin(), repo.GetName())
	if err != nil {
		return fmt.Errorf("error getting repository info: %v", err)
	}
	defaultBranch := repoInfo.GetDefaultBranch()
	g.Log("Default branch is: %s", defaultBranch)

	// Create PR
	g.Log("Creating pull request")
	_, _, err = g.client.PullRequests().Create(g.ctx, repo.GetOwner().GetLogin(), repo.GetName(), &github.NewPullRequest{
		Title: github.String("@JiraIssue-xx | Update renovate.json to use MyOtherOrg"),
		Body:  github.String("This PR updates the renovate.json configuration to use the MyOtherOrg organization instead of MyOrg. (changes `github>MyOrg` to `github>MyOtherOrg`)"),
		Head:  github.String("update-renovate-config"),
		Base:  github.String(defaultBranch),
	})
	if err != nil {
		return fmt.Errorf("error creating PR: %v", err)
	}
	g.Log("Pull request created successfully")

	return nil
}

type GitHubClientWrapper struct {
	Client *github.Client
}

func (w *GitHubClientWrapper) Search() *github.SearchService {
	return w.Client.Search
}

func (w *GitHubClientWrapper) Repositories() *github.RepositoriesService {
	return w.Client.Repositories
}

func (w *GitHubClientWrapper) Git() *github.GitService {
	return w.Client.Git
}

func (w *GitHubClientWrapper) PullRequests() *github.PullRequestsService {
	return w.Client.PullRequests
}
