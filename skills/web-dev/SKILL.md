---
name: web-dev
description: Use for ANY Angular or Firebase work — new Angular project, add a feature/component/service/route, Firebase deploy, Firestore rules, Cloud Functions, subdomain setup on *.charlies.bot, CSS styling for Angular, signals patterns, zoneless setup, App Hosting config, Vitest testing, ESLint/Prettier setup, ng generate scaffolding, and all web side project development.
---

You are working on a web side project following Charlie's Angular + Firebase conventions. Every project deploys as a subdomain of `charlies.bot`.

- Read `references/ARCHITECTURE.md` when deciding where to place new files or creating a new feature folder.
- Read `references/DEPLOYMENT.md` before any deploy, DNS, or Cloud Functions work.
- Read `references/AUTH.md` when adding Firebase Auth to a project for the first time.

## Core Principles

Every side project is `projectname.charlies.bot`. Firebase handles everything — hosting, backend, data, auth. Free tier first. Zero setup decisions. **Firebase App Hosting** is used for all projects.

```
All projects     → Firebase App Hosting → projectname.charlies.bot
Backend logic    → Cloud Functions    (API, triggers, cron)
Data             → Firestore          (free tier)
Auth             → Firebase Auth      (when needed)
DNS              → Cloud DNS          (*.charlies.bot)
```

## MCP Integration

Three MCP servers cover the full workflow. Use them proactively — don't do things manually.

**Angular CLI MCP** — for patterns this skill doesn't cover:

- Use `get_best_practices` when working with Angular APIs not covered in this skill (e.g., animations, i18n, service workers). Skip it for signals, OnPush, standalone, routing — those are already defined here.
- Use `find_examples` when unsure about a specific modern Angular pattern.
- Use `search_documentation` for Angular API details.
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

| Concern         | Choice                                                |
| --------------- | ----------------------------------------------------- |
| Framework       | Angular 21+ (Signals-first, **Zoneless**, standalone) |
| Hosting         | Firebase App Hosting (git-push deploys)               |
| Backend         | Cloud Functions for Firebase                          |
| Database        | Firestore (free tier focus)                           |
| Auth            | Firebase Auth (when needed)                           |
| DNS             | Cloud DNS (`*.charlies.bot`)                          |
| CSS             | Modern CSS (component-scoped) + CSS reset             |
| Package manager | npm                                                   |
| Email           | sudo@charlies.bot (Google Workspace)                  |
| Testing         | **Vitest** (Angular default)                          |
| Linting         | ESLint via `angular-eslint`                           |
| Formatting      | Prettier                                              |

## Project Structure

Use `ng generate` to create all code — it handles file placement, naming, and boilerplate. Angular 21 projects use **Zoneless** by default and the **2025 file naming convention** (`dashboard.ts` not `dashboard.component.ts`).

- **`core/`** — app-wide infrastructure (auth, layout, interceptors). Not a feature.
- **Features at top level** — each feature is its own directory (`dashboard/`, `profile/`). Self-contained with own routes, components, and services.
- **`ui/`** — reusable dumb components, created on demand when shared across 2+ features.

```bash
ng generate component dashboard            # Feature component
ng generate component ui/button            # Reusable UI component
ng generate service core/auth              # Global service
```

**"Where does this go?"**
`core/` for infrastructure (auth, layout, interceptors). Top-level folder for features. `ui/` for components reused across features. Start in the feature, extract to `ui/` when reused.

## Component Pattern

**Inline templates for small components** (Angular best practice), external files for large ones:

```typescript
// Small component — inline template + styles (single file)
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: "app-dashboard-stats",
  template: `
    <div class="stats">
      @for (stat of stats(); track stat.label) {
        <span>{{ stat.label }}: {{ stat.value }}</span>
      }
    </div>
  `,
  styles: `
    .stats {
      display: flex;
      gap: 1rem;
    }
  `,
})
export class DashboardStats {
  readonly stats = input.required<Stat[]>();
}

// Large component — external template + styles
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: "app-dashboard",
  templateUrl: "./dashboard.html",
  styleUrl: "./dashboard.css",
})
export class Dashboard {
  private router = inject(Router);

  items = httpResource<Item[]>(() => "/api/items");
  itemCount = computed(() => this.items.value()?.length ?? 0);
  searchQuery = signal("");
  selectedCategory = linkedSignal(
    () => this.items.value()?.[0]?.category ?? "all",
  );
}
```

Components use signals for all reactive state. Use built-in control flow (`@if`, `@for`, `@switch`).

## State Management

**Signals by default.** Use Angular signals (`signal()`, `computed()`, `linkedSignal()`, `effect()`) for all component and service state. Use `computed()` for read-only derived state, `linkedSignal()` for writable derived state (e.g., a selection that resets when its source changes), and `effect()` sparingly as a last resort for side effects. Angular 21 favors **Zoneless** change detection — avoid `NgZone` and manual change detection calls.

**Async data loading:** Use `httpResource()` (experimental) for reactive HTTP data fetching instead of manual `isLoading`/`error`/`data` signal triplets. It wraps `HttpClient` and exposes status and response as signals. For non-HTTP async data, use `resource()`. Requires `provideHttpClient()` in `app.config.ts` providers.

**Firestore real-time listeners:** Use `DestroyRef` to clean up `onSnapshot` subscriptions. This works in both components and services that aren't root singletons:

```typescript
private destroyRef = inject(DestroyRef);

listen(): void {
  const unsub = onSnapshot(query, (snapshot) => { /* update signals */ });
  this.destroyRef.onDestroy(() => unsub());
}
```

Root singleton services (`providedIn: 'root'`) live for the app lifetime — no cleanup needed.

## CSS Conventions

Component-scoped modern CSS via Angular's default `ViewEncapsulation.Emulated`. Small components use inline `styles`; large components use external `.css` files.

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

**Security rules are locked down from day one** — even for prototypes. New projects start with `assets/firestore.rules` (deny all). As features are built, open access per-collection using the auth-owns-data pattern. Use `firebase_get_security_rules` to verify existing rules before deploying. Deploy with `firebase deploy --only firestore:rules`.

**Auth-owns-data pattern** (default for most features):

```
match /users/{userId} {
  allow read, write: if request.auth != null && request.auth.uid == userId;
}
match /users/{userId}/{subcollection=**} {
  allow read, write: if request.auth != null && request.auth.uid == userId;
}
```

**Public-read / authenticated-write** (for shared content):

```
match /posts/{postId} {
  allow read: if true;
  allow write: if request.auth != null;
}
```

## Cloud Functions

Use Cloud Functions for:

- **API endpoints** — HTTP callable functions for client-server communication
- **Firestore triggers** — react to document creates, updates, deletes
- **Auth triggers** — react to user creation, deletion
- **Scheduled tasks** — cron jobs (cleanup, aggregation, notifications)

Keep functions small and focused — one function per concern. Deploy with `firebase deploy --only functions`. Debug with `mcp__plugin_firebase_firebase__functions_get_logs`.

## Testing

**Vitest** is the default test runner (`ng test`). Tests live next to the code they test (e.g., `dashboard.spec.ts` alongside `dashboard.ts`).

- Use `TestBed` for component tests with `provideHttpClient()` and `provideHttpClientTesting()`.
- Test `httpResource` via `HttpTestingController` — flush requests and assert on signal values.
- Test signals directly: update with `.set()` / `.update()`, assert with `()`.
- Use `fixture.componentRef.setInput()` for signal inputs.

```typescript
TestBed.configureTestingModule({
  providers: [provideHttpClient(), provideHttpClientTesting()],
});
const mockBackend = TestBed.inject(HttpTestingController);
// ... flush requests, assert signal values
```

## Linting & Formatting

**ESLint** is added via `ng add angular-eslint` (flat config, `eslint.config.js`). The scaffold script handles this. Run with `ng lint`.

**Prettier** is added manually for formatting. Install with `npm install prettier --save-dev` and add a `.prettierrc` config. Use `eslint-config-prettier` to avoid rule conflicts.

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

For deployment, hosting, and infrastructure details, read `references/DEPLOYMENT.md`.
