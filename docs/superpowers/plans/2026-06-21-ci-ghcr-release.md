# CI, GHCR, and Release Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PR checks, tag-based GHCR image publishing, and GitHub Release creation for the backend and frontend containers.

**Architecture:** CI and release automation are separate GitHub Actions workflows. PRs and `main` pushes validate code and container builds without publishing; `v*` tags rerun validation, push versioned GHCR images, and create generated GitHub release notes.

**Tech Stack:** GitHub Actions, Docker Buildx, GHCR, Go 1.23, Node.js 22, Next.js 15, Vitest, ESLint.

---

## File Structure

- Create `.github/workflows/ci.yml` for PR and `main` push validation.
- Create `.github/workflows/release.yml` for tag-triggered GHCR publishing and GitHub Release creation.
- Create `src/web/eslint.config.mjs` for frontend lint configuration.
- Modify `src/web/package.json` to add a `lint` script and ESLint dev dependencies.
- Modify `src/web/package-lock.json` through `npm install --package-lock-only` so CI can install the new lint dependencies reproducibly.

### Task 1: Frontend Lint Setup

**Files:**
- Create: `src/web/eslint.config.mjs`
- Modify: `src/web/package.json`
- Modify: `src/web/package-lock.json`

- [ ] **Step 1: Add the desired lint script and ESLint config**

`src/web/package.json` should include this script:

```json
"lint": "eslint ."
```

`src/web/eslint.config.mjs` should extend Next.js TypeScript and core web vitals lint rules and ignore generated output:

```javascript
import { FlatCompat } from "@eslint/eslintrc";
import { dirname } from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const compat = new FlatCompat({
  baseDirectory: __dirname
});

const eslintConfig = [
  {
    ignores: [".next/**", "next-env.d.ts", "node_modules/**"]
  },
  ...compat.extends("next/core-web-vitals", "next/typescript")
];

export default eslintConfig;
```

- [ ] **Step 2: Update dependency metadata**

Run:

```bash
cd src/web
npm install --save-dev eslint eslint-config-next --package-lock-only
```

Expected: `package.json` and `package-lock.json` include the ESLint packages.

- [ ] **Step 3: Verify lint locally**

Run:

```bash
cd src/web
npm ci
npm run lint
```

Expected: lint exits with code 0.

### Task 2: Pull Request CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Add CI workflow**

Create a workflow that runs backend checks, frontend checks, and Docker image build checks on pull requests and `main` pushes.

- [ ] **Step 2: Verify locally equivalent commands**

Run:

```bash
cd src/api
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
cd ../web
npm run lint
npm run typecheck
npm run test
npm run build
cd ../..
docker build --file src/api/Dockerfile src/api
docker build --file src/web/Dockerfile src/web
```

Expected: every command exits with code 0 when Docker is available.

### Task 3: Tag Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Add release workflow**

Create a workflow that runs on `v*` tags, repeats validation, logs in to GHCR with `GITHUB_TOKEN`, publishes both service images, and creates a GitHub Release using generated notes.

- [ ] **Step 2: Verify workflow YAML parses**

Run:

```bash
ruby -e 'require "yaml"; Dir[".github/workflows/*.yml"].each { |file| YAML.load_file(file); puts file }'
```

Expected: both workflow file paths print and the command exits with code 0.

### Task 4: Commit, Push, and Initial Release

**Files:**
- Commit all changed files from Tasks 1-3.

- [ ] **Step 1: Commit workflow implementation**

Run:

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml src/web/eslint.config.mjs src/web/package.json src/web/package-lock.json docs/superpowers/plans/2026-06-21-ci-ghcr-release.md
git commit -m "ci: add ghcr release automation"
```

Expected: commit succeeds.

- [ ] **Step 2: Push main and tag**

Fast-forward `main` to the implementation commit, push it, create `v0.1.0`, and push the tag.

```bash
git checkout main
git merge --ff-only codex/ci-ghcr-release
git push origin main
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

Expected: GitHub receives the workflow commit and the tag-triggered release workflow starts.

- [ ] **Step 3: Confirm release run**

Run:

```bash
gh run list --workflow release.yml --limit 1
```

Expected: the latest `release.yml` run for `v0.1.0` completes successfully.
