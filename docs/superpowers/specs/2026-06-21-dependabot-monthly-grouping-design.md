# Dependabot Monthly Grouping Design

## Goal

Configure Dependabot for this repository so routine dependency version checks run monthly and create as few pull requests as GitHub supports.

## Repository Context

The repository currently has these dependency surfaces:

- `src/web/package.json` and `src/web/package-lock.json` for the Next.js frontend npm dependencies.
- `src/api/go.mod` for the Go backend module.
- `src/web/Dockerfile` for the frontend container base image.
- `src/api/Dockerfile` for the backend container base image.
- `docker-compose.yml` for local multi-service container startup.

There are no GitHub Actions workflows in the repository at the time of this design, so the Dependabot configuration will not include a `github-actions` ecosystem entry.

## Selected Approach

Use one monthly multi-ecosystem Dependabot group for all supported dependency ecosystems in this repository.

Each update entry will use `patterns: ["*"]` and `multi-ecosystem-group: "monthly-dependencies"` so Dependabot can consolidate npm, Go Modules, Docker, and Docker Compose updates into one monthly pull request when compatible updates are available.

## Dependabot Entries

The configuration will add `.github/dependabot.yml` with:

- A top-level `multi-ecosystem-groups.monthly-dependencies.schedule.interval` set to `monthly`.
- An npm update entry for `/src/web`.
- A Go Modules update entry for `/src/api`.
- Docker update entries for `/src/web` and `/src/api`.
- A Docker Compose update entry for `/`.

No assignees, labels, private registries, ignore rules, or dependency allowlists will be added in this change. Those require a separate scoped change when the repository has a review ownership policy or private package registry requirement.

## Security and Operations

The config will not expose or reference secrets. It will only describe public dependency manifests and package ecosystems.

Dependabot security update behavior remains governed by GitHub repository security settings. This design focuses on scheduled version update grouping.

## Verification

Verification will be static and local:

- Confirm `.github/dependabot.yml` exists and is valid YAML.
- Confirm every configured directory corresponds to an existing manifest or Docker/Compose file in the repository.
- Confirm all grouped entries include `patterns: ["*"]`, because GitHub requires patterns when assigning entries to a multi-ecosystem group.
