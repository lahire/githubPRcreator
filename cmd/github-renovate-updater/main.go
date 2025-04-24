package main

import (
	"context"
	"flag"
	"log"

	"github.com/google/go-github/v58/github"
	"github.com/lahire/github-renovate-updater/internal/githubops" //TODO Changethis
	"golang.org/x/oauth2"
)

var (
	orgName     = flag.String("org", "MyOrg", "GitHub organization name")
	repoName    = flag.String("repo", "", "Specific repository name (optional, if not provided will scan entire org)")
	dryRun      = flag.Bool("dry-run", false, "Run in dry-run mode (no PRs will be created)")
	githubToken = flag.String("token", "", "GitHub personal access token (required)")
	gpgKeyID    = flag.String("gpg-key", "", "GPG key ID for signing commits (optional)")
)

func main() {
	flag.Parse()

	if *githubToken == "" {
		log.Fatal("GitHub token is required. Please provide it using the -token flag")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *githubToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	ghClient := githubops.NewGitHubClient(ctx, &githubops.GitHubClientWrapper{Client: client})

	if *repoName != "" {
		// Process single repository
		repo, _, err := client.Repositories.Get(ctx, *orgName, *repoName)
		if err != nil {
			log.Fatalf("Error getting repository %s: %v", *repoName, err)
		}

		ghClient.Log("Processing single repository: %s", repo.GetFullName())
		err = ghClient.CheckAndUpdateRenovateConfig(repo, *dryRun, *gpgKeyID)
		if err != nil {
			log.Fatalf("Error processing repository %s: %v", repo.GetFullName(), err)
		}
	} else {
		// Process entire organization
		repos, err := ghClient.FindReposWithRenovate(*orgName)
		if err != nil {
			log.Fatalf("Error finding repositories: %v", err)
		}

		ghClient.Log("Found %d repositories with renovate.json", len(repos))

		for _, repo := range repos {
			ghClient.Log("Processing repository: %s", repo.GetFullName())
			err := ghClient.CheckAndUpdateRenovateConfig(repo, *dryRun, *gpgKeyID)
			if err != nil {
				ghClient.Log("Error processing repository %s: %v", repo.GetFullName(), err)
				continue
			}
		}
	}
}
