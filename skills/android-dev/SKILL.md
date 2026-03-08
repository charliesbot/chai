---
name: android-dev
description: >
  Android development skill tailored to a specific multi-platform architecture and conventions.
  Use this skill whenever working on an Android project — creating features, setting up modules,
  writing ViewModels, configuring Gradle, reviewing architecture decisions, or scaffolding new
  feature/platform modules. Trigger on any Android/Kotlin/Compose/Gradle work including:
  build issues, dependency setup, module creation, navigation, DI configuration, UI components,
  testing, and project structure questions.
---

You are working on a multi-platform Android project following this architecture and conventions. Read `references/ARCHITECTURE.md` for the full module structure and dependency rules before making architectural decisions.

## Core Principles

The architecture supports multiple Android platforms (mobile, Wear OS, TV, Auto) from a single codebase. Everything flows in one direction:

```
app/wearos/tv/auto → features:* → core
```

Features are **business capabilities** (auth, profile, cart), not individual screens. A feature module contains only presentation logic — business logic lives in `:core`. Features never depend on each other.

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

Do not add third-party dependencies without asking first.

## Module Structure

- **`:core`** — business logic, data layer (Room, Retrofit, repositories), domain models, use cases, shared UI (theme, components), and DI for core infrastructure. All UI strings live here for cross-platform reuse.
- **`:features:<name>`** — presentation only: ViewModel, Composable screens, feature-scoped DI module. Wear-specific UI goes in a `wear/` sub-package within the feature.
- **`:app`**, **`:wearos`**, **`:tv`**, **`:auto`** — platform shells that wire navigation and load DI modules. Each platform uses its own navigation library.

New feature modules are auto-registered via wildcard include in `settings.gradle.kts`:

```kotlin
file("features").listFiles()?.filter { it.isDirectory }?.forEach {
    include(":features:${it.name}")
}
```

## ViewModel Pattern

ViewModels use StateFlow and are shared across platforms (mobile and Wear use the same ViewModel). Keep them in the feature module root, not inside a platform sub-package.

```kotlin
class AuthViewModel(
    private val loginUseCase: LoginUseCase
) : ViewModel() {

    private val _uiState = MutableStateFlow(AuthUiState())
    val uiState: StateFlow<AuthUiState> = _uiState.asStateFlow()

    fun onLogin(email: String, password: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true) }
            loginUseCase(email, password)
                .onSuccess { user -> _uiState.update { it.copy(user = user, isLoading = false) } }
                .onFailure { error -> _uiState.update { it.copy(error = error.message, isLoading = false) } }
        }
    }
}
```

## Composable Conventions

- Every `@Composable` function needs a `@Preview`.
- Use Material 3 components and the app's shared theme from `:core:ui:theme`.
- Wear-specific screens go in a `wear/` sub-package inside the feature.
- Platform modules call feature screens — features don't know which platform they're on.

## Strings

Every UI string must be defined in `:core` resources with both English and Spanish translations. This enables cross-platform reuse and consistent localization.

## Testing

Follow red-green TDD: write failing tests first, then implement until they pass. Run tests after every change.

- Use MockK for mocking.
- Prefer module-scoped test commands (`./gradlew :features:auth:test`) over `./gradlew test` when working on a single feature.
- Run `./gradlew spotlessApply` before committing.

## Scaffolding a New Feature

When creating a new feature module:

1. Create the directory: `features/<feature-name>/`
2. Add `build.gradle.kts` depending only on `:core`:

   ```kotlin
   plugins {
       alias(libs.plugins.android.library)
       alias(libs.plugins.kotlin.android)
   }

   android {
       namespace = "com.yourpackage.features.<feature>"
       // ...
   }

   dependencies {
       implementation(project(":core"))
   }
   ```

3. Create the package structure:
   ```
   src/main/kotlin/com/yourpackage/features/<feature>/
   ├── di/
   │   └── <Feature>Module.kt      # Koin module for the ViewModel
   ├── <Feature>ViewModel.kt       # Shared ViewModel
   ├── <Feature>Screen.kt          # Mobile composable + preview
   └── wear/                        # (if needed)
       └── Wear<Feature>Screen.kt  # Wear composable + preview
   ```
4. Register the Koin module in the platform's DI setup.
5. Add navigation routes in the platform module.
6. Add strings in `:core` (English + Spanish).
7. Write tests first, then implement.

## Scaffolding a New Platform Module

When adding a new platform (e.g., `:tv`):

1. Create the directory at root level: `tv/`
2. Set up `build.gradle.kts` as an application module depending on `:core` and relevant `:features:*`
3. Implement platform-appropriate navigation
4. Create a DI module that loads core + feature modules
5. Add to `settings.gradle.kts` with `include(":tv")`

## Common Commands

```bash
./gradlew build                          # Build all modules
./gradlew :app:installDebug              # Install mobile app
./gradlew test                           # Run all tests
./gradlew :features:<name>:test          # Run single feature tests
./gradlew :core:test                     # Run core tests
./gradlew spotlessApply                  # Format code
```

## Reference

For the full module structure, dependency rules, and rationale, read `references/ARCHITECTURE.md`.
