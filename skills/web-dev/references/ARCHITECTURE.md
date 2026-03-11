# Angular 21 Architecture: Vertical Slice Pattern

## Overview
This project follows a "Vertical Slice" architecture, optimized for **Signals**, **Zoneless** change detection, and **Standalone** components. It prioritizes scalability, lazy loading, and framework-native reactivity.

## High-Level Folder Structure

```text
src/app/
├── core/                 # Global singletons (Auth, Interceptors, Config)
├── features/             # Business features (Dashboard, Profile)
├── shared/               # Reusable UI kit, Pipes, and Directives
├── layout/               # App frame (Navbar, Footer, Main Layout)
├── app.config.ts         # Global providers (Zoneless, Router, Firebase)
├── app.routes.ts         # Root routing
└── app.component.ts      # Root standalone component
```

## 1. Core Layer (`/core`)
- **Responsibility:** Infrastructure, global state, and system-wide logic.
- **Contents:** `AuthService`, `ConfigService`, Interceptors, Guards, and global injection tokens.
- **Pattern:** Use `providedIn: 'root'` for all services and `inject()` for dependency injection.

## 2. Feature Layer (`/features`)
- **Responsibility:** Domain-specific business logic and smart components.
- **Contents:** Lazy-loaded directories (e.g., `features/dashboard/`).
- **Pattern:** Features are "Smart" components that inject services from `core/`.
- **Navigation:** Lazy-loaded at the route level. Use `loadComponent` for single-component routes, `loadChildren` for feature route groups with child routes.

## 3. Shared Layer (`/shared`)
- **Responsibility:** Reusable, "Dumb" presentational building blocks.
- **Contents:** UI components (Button, Card, Input), custom pipes, and directives.
- **Rule:** No service injections. Use `input()` and `output()` exclusively for communication.

## 4. Layout Layer (`/layout`)
- **Responsibility:** The visual frame/skeleton of the application.
- **Contents:** `NavbarComponent`, `FooterComponent`, `SidebarComponent`, and `MainLayoutComponent`.

---

## State Management: Signals-First
- **Local State:** Component-scoped `signal()`.
- **Global State:** Service-scoped Signals (e.g., `authService.user()`).
- **Derived State:** `computed()` for all synchronous transformations.
- **Side Effects:** `effect()` is used sparingly (e.g., localStorage synchronization).
- **Async Data:** Use `httpResource()` (experimental) for reactive HTTP fetching — it exposes status and response as signals, replacing manual `isLoading`/`error`/`data` triplets. Use `resource()` for non-HTTP async data.

## Performance: Zoneless
- **Detection:** All components use `ChangeDetectionStrategy.OnPush`.
- **Constraint:** Avoid `zone.js` and `NgZone`. Rely on Signal updates to trigger the UI.

## Modern Syntax
- **Control Flow:** Use `@if`, `@for`, `@switch` (Native Control Flow).
- **Templates:** Use **spread syntax** for object/array passing in templates.
- **Forms:** Prefer the **Signal Forms API** (Angular 21) for reactive state.
- **DI:** Use `inject()` instead of constructor injection.
