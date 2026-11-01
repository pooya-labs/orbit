# Local Development

## Requirements

- Windows 10/11, macOS, or Linux
- Node.js 20+
- npm (included with Node.js)
- Git

## First run (Windows PowerShell)

```powershell
git clone https://github.com/pooya16pg-spec/orbit.git
cd orbit
npm install
Copy-Item .env.example .env.local
npm run dev
```

Then open http://localhost:3000.

## Daily workflow

```powershell
git pull
npm run dev
```

Before pushing:

```powershell
npm run typecheck
npm run build
```

## Environment

The current dashboard does not require a backend. `.env.example` documents the future API endpoint. Do not place production credentials in `.env.example` or commit `.env.local`.

## Troubleshooting

### Port 3000 is already in use

Stop the other development server, or run:

```powershell
npm run dev -- --port 3001
```

### Dependency installation is in a bad state

Close the dev server, then run:

```powershell
Remove-Item -Recurse -Force node_modules
Remove-Item -Force package-lock.json -ErrorAction SilentlyContinue
npm install
```

### TypeScript or Next.js generated types look stale

```powershell
Remove-Item -Recurse -Force .next -ErrorAction SilentlyContinue
npm run typecheck
```
