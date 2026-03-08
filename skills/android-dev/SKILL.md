---
name: android-dev
description: >
  Android multi-platform development skill (mobile, Wear OS, TV, Auto) with architecture conventions,
  tech stack rules, and scaffolding scripts. Use whenever working on an Android project: creating features,
  writing ViewModels/Composables, configuring Gradle, wiring Koin/Navigation/Room/Retrofit, or writing
  tests. Also trigger on build errors (unresolved references, dependency conflicts, Gradle sync failures,
  circular dependencies) and any question about where code belongs in a multi-module project. When in
  doubt, use it — better to check conventions than produce inconsistent code.
---

You are working on a multi-platform Android project following this architecture and conventions. Read `references/ARCHITECTURE.md` for the full module structure and dependency rules before making architectural decisions.

## Core Principles

The architecture supports multiple Android platforms (mobile, Wear OS, TV, Auto) from a single codebase. Everything flows in one direction:

```
app           → features:*:app  → core
wear          → features:*:wear → core
widget        → core
complications → core
```

Features are **business capabilities** (auth, profile, cart), not individual screens — grouping by user journey keeps modules cohesive and avoids a proliferation of tiny modules that each cache poorly. A feature module contains only presentation logic — business logic lives in `:core` so it can be reused across platforms without duplication. Features never depend on each other, which Gradle enforces at compile time, preventing the codebase from devolving into a tangled dependency graph as it grows.

## Do Not

- **Add dependencies between feature modules** — features depend only on `:core`. If two features need the same type, move it to `:core`.
- **Add third-party libraries without asking** — the current stack covers most needs. Explain what's missing before adding anything.
- **Put business logic in feature modules** — repositories, use cases, and domain models belong in `:core`. Features are presentation only.
- **Create feature modules for single screens** — a feature is a complete user journey (e.g., `:features:auth` covers login, register, and forgot password).
- **Create flat feature modules** — always use platform submodules (`app/`, `wear/`), even for phone-only features.
- **Put widget or complication code inside `:app` or `:wear`** — they're standalone entry points and get their own root-level modules.
- **Use LiveData** — the entire codebase uses StateFlow + coroutines.
- **Skip writing tests** — follow red-green TDD. Write the failing test first.
- **Skip `@Preview`** — every `@Composable` needs one.

## Tech Stack

| Concern             | Choice                                               |
| ------------------- | ---------------------------------------------------- |
| UI                  | Jetpack Compose + Material 3                         |
| DI                  | Koin                                                 |
| Networking          | Retrofit + OkHttp                                    |
| Database            | Room                                                 |
| Serialization       | Kotlinx Serialization                                |
| Image loading       | Coil                                                 |
| Navigation (mobile) | Navigation 3 (`androidx.navigation3`)                |
| Navigation (wear)   | Wear Compose Navigation                              |
| State management    | StateFlow + MVVM                                     |
| Formatting          | Spotless                                             |
| Testing             | MockK                                                |
| Build               | Gradle KTS + version catalogs (`libs.versions.toml`) |

Do not add third-party dependencies without asking first — every dependency is a long-term maintenance commitment, and the chosen stack already covers most needs.

## Module Structure

- **`:core`** — business logic, data layer (Room, Retrofit, repositories), domain models, use cases, shared UI (theme, components), and DI for core infrastructure. All UI strings live here for cross-platform reuse.
- **`:features:<name>:app`** — phone presentation: ViewModel, Composable screens (Material 3), feature-scoped DI module.
- **`:features:<name>:wear`** — wear presentation: ViewModel, Composable screens (Wear Material 3), feature-scoped DI module.
- **`:app`**, **`:wear`**, **`:tv`**, **`:auto`** — platform shells that wire navigation and load DI modules. Each platform uses its own navigation library.
- **`:widget`** — home screen widget (Glance or `AppWidgetProvider`). Depends only on `:core` because widgets are standalone OS entry points that need data but not app navigation or feature screens.
- **`:complications`** — Wear OS complication data providers. Depends only on `:core` for the same reason — the watch face calls them directly, outside of the app's UI.

Every feature always uses platform submodules (`app/`, `wear/`, etc.) — even if it only targets one platform today. This removes the "is this feature flat or nested?" guessing game and means adding a Wear or TV variant later is just adding a sibling submodule, not restructuring existing code.

Widgets and complications are **not** features — they're standalone entry points the OS launches independently. They sit at the same level as `:app` and `:wear`, depending directly on `:core` without going through feature modules.

New feature modules are auto-registered via wildcard include in `settings.gradle.kts`:

```kotlin
file("features").listFiles()?.filter { it.isDirectory }?.forEach {
    include(":features:${it.name}")
}
```

## ViewModel Pattern

Each platform submodule has its own ViewModel. ViewModels use StateFlow and live inside their platform submodule (e.g., `features/dashboard/app/` has `DashboardViewModel`, `features/dashboard/wear/` has `WearDashboardViewModel`).

```kotlin
class DashboardViewModel(
    private val getDashboardUseCase: GetDashboardUseCase
) : ViewModel() {

    private val _uiState = MutableStateFlow(DashboardUiState())
    val uiState: StateFlow<DashboardUiState> = _uiState.asStateFlow()

    fun onRefresh() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true) }
            getDashboardUseCase()
                .onSuccess { data -> _uiState.update { it.copy(data = data, isLoading = false) } }
                .onFailure { error -> _uiState.update { it.copy(error = error.message, isLoading = false) } }
        }
    }
}
```

## Composable Conventions

- Every `@Composable` function needs a `@Preview` — this catches layout issues early without needing to run the full app, which is especially valuable in a multi-platform project where you can't easily test every screen on every device.
- Use Material 3 components and the app's shared theme from `:core:ui:theme`.
- Wear screens use Wear Material 3 components in the `features/<name>/wear/` submodule.
- Platform modules call feature screens — features don't know which platform they're on.
- Feature-scoped components go in a `component/` package inside the platform submodule. If only the dashboard uses a `StatCard`, it lives in `features/dashboard/app/component/`, not in `:core`. Promote to `:core:ui:component` only when multiple features need it.
- The `app/` and `wear/` submodules within a feature do not share UI or ViewModels. They use different Compose toolkits and typically manage different UI state. The only shared code is in `:core` (use cases, repositories, domain models).

## Strings

Every UI string must be defined in `:core` resources with both English and Spanish translations. Centralizing strings in `:core` means all platforms (phone, wear, TV) share the same copy, so translations stay in sync and you never end up with a string defined in one platform but missing in another.

## Testing

Follow red-green TDD: write failing tests first, then implement until they pass. Run tests after every change. Writing the test first forces you to think about the API before the implementation, and catching regressions immediately is far cheaper than debugging them later.

- Use MockK for mocking.
- Prefer module-scoped test commands (`./gradlew :features:dashboard:app:test`) over `./gradlew test` when working on a single feature — this leverages the modular architecture for faster feedback loops instead of recompiling and testing everything.
- Run `./gradlew spotlessApply` before committing to keep formatting consistent across the codebase without manual effort.

## Scaffolding a New Feature

Use the bundled script to generate all the boilerplate for a new feature module:

```bash
# Phone only
./scripts/scaffold-feature.sh <feature-name> <base-package>

# Phone + Wear
./scripts/scaffold-feature.sh <feature-name> <base-package> --wear
```

This creates the full directory structure with `build.gradle.kts`, ViewModel (StateFlow), Screen (Composable + Preview), and Koin DI module for each platform submodule.

After running the script:

1. Register the Koin module in the platform's DI setup.
2. Add navigation routes in the platform module.
3. Add strings in `:core` (English + Spanish).
4. Write tests first, then implement.

## Scaffolding a Use Case

Use the bundled script to generate a use case in `:core`:

```bash
# Suspend function returning Result<T>
./scripts/scaffold-usecase.sh <UseCaseName> <base-package> <RepositoryName>

# Flow-based (reactive, non-suspend)
./scripts/scaffold-usecase.sh <UseCaseName> <base-package> <RepositoryName> --flow
```

Examples:

```bash
./scripts/scaffold-usecase.sh GetArticles com.myapp FeedRepository
./scripts/scaffold-usecase.sh ObserveAuthState com.myapp AuthRepository --flow
```

After running the script:

1. Replace the `TODO` placeholders with the actual return type and repository call.
2. Register in the Koin core DI module: `factory { GetArticlesUseCase(get()) }`
3. Write a test for the use case.

## Scaffolding a New Platform Module

When adding a new platform (e.g., `:tv`):

1. Create the directory at root level: `tv/`
2. Set up `build.gradle.kts` as an application module depending on `:core` and relevant `:features:*:<platform>`
3. Implement platform-appropriate navigation
4. Create a DI module that loads core + feature modules
5. Add to `settings.gradle.kts` with `include(":tv")`

## Common Commands

```bash
./gradlew build                          # Build all modules
./gradlew :app:installDebug              # Install mobile app
./gradlew test                           # Run all tests
./gradlew :features:<name>:app:test       # Run single feature tests
./gradlew :core:test                     # Run core tests
./gradlew spotlessApply                  # Format code
```

## Common Scenarios

**"I need to share a data class between two features"**
Move it to `:core:domain:model`. Features only depend on `:core`, so any shared type must live there. Do not add a dependency between features — Gradle will reject it, and even if it didn't, it would break the isolation that keeps builds fast.

**"Where should I put this new screen?"**
First decide which feature (business capability) it belongs to. A "forgot password" screen belongs in `:features:auth`, not a new `:features:forgot-password` module. Then place it in the appropriate platform submodule (`app/` or `wear/`).

**"I want to add a Wear version of an existing feature"**
Create a `wear/` submodule alongside the existing `app/` submodule under that feature. The Wear submodule gets its own ViewModel, DI module, and Composable screens using Wear Material 3. Wire the navigation in the `:wear` platform module. The business logic in `:core` is already shared — no changes needed there.

**"Should I use LiveData or StateFlow?"**
StateFlow. The entire codebase uses StateFlow + coroutines for reactive state. LiveData is not part of this stack.

**"Can I add library X?"**
Ask first. The current stack (Koin, Retrofit, Room, Coil, Kotlinx Serialization, MockK) covers most needs. If you think something is missing, explain what problem it solves and why the existing stack can't handle it.

**"I need to add a home screen widget"**
Create a `:widget` module at root level. It depends only on `:core` — widgets are standalone OS entry points that need data (use cases, repositories) but not app navigation or feature screens. Use Glance for Compose-based widgets or `AppWidgetProvider` for traditional ones. Do not put widget code inside `:app`.

**"I need to add a Wear OS complication"**
Create a `:complications` module at root level. It depends only on `:core` — complications are data providers the watch face calls directly, outside of the app's UI. They use `SuspendingComplicationDataSourceService` to serve data. Do not put complication code inside `:wear`.

**"I'm getting unresolved reference errors across modules"**
Check the dependency flow: `app/wear → features:*:app/wear → core`. If a feature can't see something, it probably lives in another feature (not allowed) or hasn't been added to `:core` yet. If a platform module can't see a feature, check that `build.gradle.kts` includes the right `:features:<name>:<platform>` dependency.

## Reference

For the full module structure, dependency rules, and rationale, read `references/ARCHITECTURE.md`.
