# CoPilot-PerUser

Self-service dashboard for GitHub Copilot AI credit usage.

Users authenticate with Google. The Go API resolves the Google email to a GitHub Enterprise SAML-linked login and returns only that user's AI credit billing usage.

## Project Layout

```text
src/web   Next.js frontend
src/api   Go backend
docs      specs and implementation plans
```

## Local Configuration

Create local environment files from `.env.example`.

```bash
cp .env.example .env
cp .env.example src/web/.env.local
```

Never commit real Google OAuth secrets or GitHub enterprise admin tokens.

## Host Development

Run the API:

```bash
cd src/api
go test ./...
go run ./cmd/server
```

Run the web app:

```bash
cd src/web
npm install
npm run dev
```

Open `http://localhost:3000`.

## Docker Development

```bash
docker compose up --build
```

The web service runs on `http://localhost:3000`. The API service runs on `http://localhost:8080`.
