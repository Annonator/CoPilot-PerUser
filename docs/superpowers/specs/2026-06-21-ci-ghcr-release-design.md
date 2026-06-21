# CI, GHCR, and Release Automation Design

## Goal

Add GitHub Actions automation that validates pull requests, builds both application containers, publishes tagged container images to GitHub Container Registry, and creates GitHub Releases from version tags.

## Repository Context

The repository contains two independently buildable services:

- `src/api` is a Go backend module with `go test ./...` coverage and `src/api/Dockerfile`.
- `src/web` is a Next.js frontend with `npm run test`, `npm run typecheck`, `npm run build`, and `src/web/Dockerfile`.
- `docker-compose.yml` builds both local containers directly from `src/api` and `src/web`.

There are no existing GitHub Actions workflows and no existing local or remote Git tags. The first release tag will therefore cover the full repository history.

## Selected Approach

Use two workflows:

- `.github/workflows/ci.yml` validates pull requests and pushes to `main`.
- `.github/workflows/release.yml` validates version tags, publishes images to GHCR, and creates a GitHub Release.

This keeps regular PR feedback separate from release publishing. Pull requests and `main` pushes should prove that tests, linting, typechecking, builds, and container image builds work without pushing images. Only pushed tags matching `v*` should publish packages or create GitHub Releases.

## Pull Request Checks

The CI workflow will run on pull requests and `main` pushes.

Backend checks:

- Run `gofmt` in check mode against all Go files.
- Run `go vet ./...`.
- Run `go test ./...`.

Frontend checks:

- Run `npm ci`.
- Run `npm run lint`.
- Run `npm run typecheck`.
- Run `npm run test`.
- Run `npm run build`.

Container checks:

- Build the API image from `src/api/Dockerfile` without pushing it.
- Build the web image from `src/web/Dockerfile` without pushing it.

The frontend does not currently define a lint script, so this change will add an ESLint setup compatible with the existing Next.js and TypeScript project.

## Release Publishing

The release workflow will run only for pushed tags matching `v*`.

It will:

- Rerun backend and frontend checks before publishing.
- Build and push `ghcr.io/annonator/copilot-peruser-api`.
- Build and push `ghcr.io/annonator/copilot-peruser-web`.
- Apply image tags derived from the Git tag, `latest`, and the commit SHA.
- Create a GitHub Release with generated release notes.

The initial release tag will be `v0.1.0`, matching the current `src/web/package.json` version. Because no earlier tag exists, GitHub's generated notes for `v0.1.0` will summarize all history reachable from the tag.

## Permissions and Secrets

The workflows will use the repository-provided `GITHUB_TOKEN`.

Required permissions:

- `contents: read` for CI.
- `contents: write` in the release workflow to create GitHub Releases.
- `packages: write` in the release workflow to push GHCR images.

No application runtime secrets, GitHub enterprise billing credentials, OAuth secrets, or internal app token secrets will be added to workflows. Container builds must succeed with build-time-only configuration and must not embed deployment secrets.

## Verification

Local verification before pushing will include:

- `go test ./...` in `src/api`.
- `go vet ./...` in `src/api`.
- `gofmt` check for backend files.
- `npm ci` in `src/web` if dependency metadata changes.
- `npm run lint`, `npm run typecheck`, `npm run test`, and `npm run build` in `src/web`.
- Docker image builds for both service Dockerfiles when Docker is available locally.
- Static YAML parsing for the new workflow files.
