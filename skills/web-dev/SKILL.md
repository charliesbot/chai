---
name: web-dev
description: Use for ANY Angular or Firebase work — new Angular project, add a feature/component/service, Firebase deploy, Firestore rules, Cloud Functions, subdomain setup on *.charlies.bot, CSS styling for Angular, signals patterns, zoneless setup, App Hosting config, and all web side project development.
---

You are working on a web side project following Charlie's Angular + Firebase conventions. Every project deploys as a subdomain of `charlies.bot`. Read `references/DEPLOYMENT.md` for the full deployment, hosting, and infrastructure guide before making deployment or infrastructure decisions.
## Core Principles

Every side project is `projectname.charlies.bot`. Firebase handles everything — hosting, backend, data, auth. Free tier first. Zero setup decisions. **Firebase App Hosting** is used for all projects.

```
All projects     → Firebase App Hosting → projectname.charlies.bot
```

Backend logic    → Cloud Functions    (API, triggers, cron)
Data             → Firestore          (free tier)
Auth             → Firebase Auth      (when needed)
DNS              → Cloud DNS          (*.charlies.bot)
```

## MCP Integration

Three MCP servers cover the full workflow. Use them proactively — don't do things manually.

**Angular CLI MCP** — code quality gate:

- MUST call `get_best_practices` before writing ANY Angular code. No exceptions.
- Use `find_examples` for modern Angular 21 patterns (signals, zoneless, signal forms).
- Use `search_documentation` for the latest Angular docs.
- Use `angular-cli__list_projects` to discover workspace structure.

**Firebase MCP** — project lifecycle, data, auth, hosting:

- Use `firebase_create_project` + `firebase_init` for new projects.
- Use `firebase_get_sdk_config` to get Firebase config — never copy-paste from console.
- Use `firebase_get_security_rules` to read existing rules.
- Use `firebase_read_resources` to inspect any `firebase://` URLs.
- Use `firebase_get_environment` first to understand the active project context.

**gcloud MCP** — DNS and escape hatch:

- Use `run_gcloud_command` for Cloud DNS subdomain setup (`projectname.charlies.bot`).
- Use `run_gcloud_command` for anything the specialized MCPs don't cover.
- Use `run_gcloud_command` for Cloud Run deploys (rare escape hatch).

## Do Not

- **No Zone.js** — use Zoneless change detection (`provideZonelessChangeDetection()`). Avoid `NgZone` and RxJS `async` pipes where Signals are viable.
- **No Tailwind, Sass, CSS-in-JS, or CSS frameworks** — modern CSS with Angular component scoping is enough.
- **No NgModules** — standalone components only. The entire codebase uses standalone.
- **No Vercel, Netlify, or non-Google hosting** — Firebase ecosystem only. Everything stays in one console.
- **No third-party libraries without asking** — the current stack covers most needs. Explain what's missing first.
- **No open Firestore security rules** — lock down from day one, even for prototypes. Validate with MCP before deploying.
- **No deploy without subdomain configured** — every project gets `projectname.charlies.bot`. No exceptions.
- **No RxJS for simple state** — use signals. RxJS only for complex async streams (WebSocket feeds, debounced search, combineLatest patterns).
- **No Firebase Hosting (classic)** — Firebase App Hosting ONLY. Do not run `firebase deploy` for hosting. App Hosting is git-push only and configured via `apphosting.yaml`.
- **No Cloud Run unless Cloud Functions genuinely can't handle it** — Cloud Functions cover API endpoints, triggers, and cron. Cloud Run is the rare escape hatch.

## Tech Stack

| Concern         | Choice                                                           |
| --------------- | ---------------------------------------------------------------- |
| Framework       | Angular 21+ (Signals-first, **Zoneless**, standalone)            |
| Hosting         | Firebase App Hosting (git-push deploys)                           |
| Backend         | Cloud Functions for Firebase                                     |
| Database        | Firestore (free tier focus)                                      |
| Auth            | Firebase Auth (when needed)                                      |
| DNS             | Cloud DNS (`*.charlies.bot`)                                     |
| CSS             | Modern CSS (component-scoped) + CSS reset                        |
| Package manager | npm                                                              |
| Email           | sudo@charlies.bot (Google Workspace)                             |
| Testing         | **Vitest** (Angular default)                                     |
| Linting         | ESLint via `@angular-eslint`                                     |

## Project Structure

Use `ng generate` to create all code — it handles file placement, naming, and boilerplate. Angular 21 projects use **Zoneless** by default. Follow the Vertical Slice organization in `references/ARCHITECTURE.md`:

- **`core/`** — global singletons (Auth, Interceptors, Config). Provided in root.
- **`features/`** — business domain slices (Dashboard, Profile). Lazy-loaded via `loadChildren`.
- **`shared/`** — reusable UI kit, pipes, and directives (no business logic).
- **`layout/`** — structural skeleton (Navbar, Footer).

```bash
ng generate component features/dashboard   # Smart component
ng generate component shared/button        # Dumb UI component
ng generate service core/auth              # Global service
```

**"Where should shared logic go?"**
`core/` for app-wide services and interceptors. `shared/` for reusable presentational components, pipes, and directives. If it's a domain-specific journey, it belongs in `features/`. See `references/ARCHITECTURE.md` for full guidance.

## Component Pattern

Standalone components with signals and **Signal Forms API**:

```typescript
@Component({
  selector: "app-dashboard",
  templateUrl: "./dashboard.component.html",
  styleUrl: "./dashboard.component.css",
})
export class DashboardComponent {
  private firestoreService = inject(FirestoreService);
  items = signal<Item[]>([]);
  isLoading = signal(false);
  error = signal<string | null>(null);
  itemCount = computed(() => this.items().length);

  // Angular 21 Signal Forms
  searchQuery = signal('');
}
```

Components use signals for all reactive state. Use built-in control flow (`@if`, `@for`, `@switch`). Angular 21 templates support **spread syntax** for function arguments and objects.

## State Management

**Signals by default.** Use Angular signals (`signal()`, `computed()`, `effect()`) for all component and service state. Angular 21 favors **Zoneless** change detection — avoid `NgZone` and manual change detection calls.

**Async data loading:** Use `httpResource()` (experimental) for reactive HTTP data fetching instead of manual `isLoading`/`error`/`data` signal triplets. It wraps `HttpClient` and exposes status and response as signals. For non-HTTP async data, use `resource()`.

## CSS Conventions

Component-scoped modern CSS via Angular's default `ViewEncapsulation.Emulated`. Each component gets its own `.css` file.

**Global `styles.css`** contains only:

- CSS reset
- Custom properties (design tokens: colors, spacing, typography)
- Base typography

**In component CSS**, use:

- Native CSS nesting
- `:has()` selector
- Container queries
- `@layer` for cascade management
- Semantic class names — no utility classes

No Tailwind, no Sass, no CSS-in-JS. Modern CSS is enough.

## Firebase Setup (on demand)

When a project needs Firebase (Firestore, Auth, Cloud Functions, etc.):

1. `npm install firebase` — install the Firebase JS SDK directly. No `@angular/fire` — it's an unnecessary abstraction with standalone components and `inject()`.
2. Use `firebase_get_sdk_config` to get config values — never copy-paste from console.
3. Initialize Firebase in a service via `initializeApp()`.

## Firestore Conventions

**Security rules are locked down from day one** — even for prototypes. Use the auth-owns-data pattern as default. Copy `assets/firestore.rules` for the locked-down starting point. Use `firebase_get_security_rules` to verify existing rules before deploying.

## Cloud Functions

Use Cloud Functions for:

- **API endpoints** — HTTP callable functions for client-server communication
- **Firestore triggers** — react to document creates, updates, deletes
- **Auth triggers** — react to user creation, deletion
- **Scheduled tasks** — cron jobs (cleanup, aggregation, notifications)

Keep functions small and focused — one function per concern. Deploy with `firebase deploy --only functions`. Debug with `mcp__plugin_firebase_firebase__functions_get_logs`.

## Scaffolding a New Project

Use the bundled script to create a lean Angular project:

```bash
./scripts/new-project.sh <project-name>
```

This creates an Angular project with CSS reset, routing, SSR, and git initialized. Nothing else — Firebase, Firestore, auth, and subdomain are added on demand as the project needs them. Just `cd` in and start building with `ng serve`.

## Deploying

All projects use Firebase App Hosting with git-push deploys:

1. **Connect GitHub repo** to App Hosting in Firebase console
2. **Push to the connected branch** — App Hosting builds and deploys automatically
3. **Backend logic** → Cloud Functions: `firebase deploy --only functions`
4. **Custom containers (rare)** → Cloud Run via `run_gcloud_command`

Every project deploys as `projectname.charlies.bot`. See `references/DEPLOYMENT.md` for detailed instructions.

## Reference

For the full deployment, hosting, infrastructure, and security rules guide, read `references/DEPLOYMENT.md`.

