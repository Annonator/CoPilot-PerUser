# Dependabot Monthly Grouping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a monthly Dependabot configuration that groups all supported repository dependency updates into one multi-ecosystem group.

**Architecture:** This is a repository configuration change only. A single `.github/dependabot.yml` file will define the top-level multi-ecosystem group and attach each detected dependency ecosystem to it with `patterns: ["*"]`.

**Tech Stack:** GitHub Dependabot, YAML, npm, Go Modules, Docker, Docker Compose.

---

## File Structure

- Create: `.github/dependabot.yml`
  - Owns Dependabot version update configuration for npm, Go Modules, Docker, and Docker Compose.
- Existing reference: `docs/superpowers/specs/2026-06-21-dependabot-monthly-grouping-design.md`
  - Captures the approved design and repository dependency surfaces.

## Task 1: Add Dependabot Configuration

**Files:**
- Create: `.github/dependabot.yml`

- [ ] **Step 1: Create the Dependabot config directory and file**

Create `.github/dependabot.yml` with this exact content:

```yaml
version: 2

multi-ecosystem-groups:
  monthly-dependencies:
    schedule:
      interval: "monthly"

updates:
  - package-ecosystem: "npm"
    directory: "/src/web"
    patterns:
      - "*"
    multi-ecosystem-group: "monthly-dependencies"

  - package-ecosystem: "gomod"
    directory: "/src/api"
    patterns:
      - "*"
    multi-ecosystem-group: "monthly-dependencies"

  - package-ecosystem: "docker"
    directory: "/src/web"
    patterns:
      - "*"
    multi-ecosystem-group: "monthly-dependencies"

  - package-ecosystem: "docker"
    directory: "/src/api"
    patterns:
      - "*"
    multi-ecosystem-group: "monthly-dependencies"

  - package-ecosystem: "docker-compose"
    directory: "/"
    patterns:
      - "*"
    multi-ecosystem-group: "monthly-dependencies"
```

- [ ] **Step 2: Parse the YAML locally**

Run:

```bash
ruby -e 'require "yaml"; YAML.load_file(".github/dependabot.yml"); puts "valid YAML"'
```

Expected output:

```text
valid YAML
```

- [ ] **Step 3: Confirm configured dependency paths exist**

Run these checks:

```bash
test -f src/web/package.json
test -f src/web/package-lock.json
test -f src/api/go.mod
test -f src/web/Dockerfile
test -f src/api/Dockerfile
test -f docker-compose.yml
```

Expected: every command exits with status `0`.

- [ ] **Step 4: Inspect the final diff**

Run:

```bash
git diff -- .github/dependabot.yml
```

Expected: the diff shows only the new Dependabot configuration file.

- [ ] **Step 5: Leave implementation uncommitted unless the user asks for a commit**

Run:

```bash
git status --short
```

Expected: `.github/dependabot.yml` is untracked or modified, and unrelated pre-existing user changes remain untouched.
