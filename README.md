# GitHub Renovate Updater

A Go tool that helps update renovate.json configurations across multiple repositories in a GitHub organization.

## Features

- Searches for repositories in a GitHub organization that have a renovate.json file
- Updates renovate.json files that reference "github>MyOrg/" to use "github>MyOtherOrg/" instead
- Creates pull requests with the changes
- Supports dry-run mode for testing changes without making them

## Prerequisites

- Go 1.21 or later
- A GitHub personal access token with appropriate permissions (repo access)

## Installation

```bash
git clone https://github.com/yourusername/github-renovate-updater.git
cd github-renovate-updater
go mod download
```

## Usage

```bash
go run cmd/github-renovate-updater/main.go -token YOUR_GITHUB_TOKEN [-org MyOrg] [-dry-run]
```

### Command Line Arguments

- `-token`: GitHub personal access token (required)
- `-org`: GitHub organization name (default: "MyOrg")
- `-dry-run`: Run in dry-run mode (no PRs will be created)

## Example

```bash
# Run in dry-run mode to see what changes would be made
go run cmd/github-renovate-updater/main.go -token YOUR_GITHUB_TOKEN -dry-run

# Run for real to create PRs
go run cmd/github-renovate-updater/main.go -token YOUR_GITHUB_TOKEN
```

## Testing

```bash
go test ./...
```

## License

MIT 
