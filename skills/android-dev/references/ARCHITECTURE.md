# Android Dojo - Simple Multi-Platform Architecture

## Overview

Multi-platform Android app architecture with **1 shared module** and **multiple feature/platform modules**.

**Shared module:**

- `:core` - Business logic, data, shared UI

**Feature modules:**

- `:features:auth` - Authentication feature
- `:features:dashboard` - Dashboard feature
- `:features:profile` - Profile feature
- (add more as needed)

**Platform modules:**

- `:app` - Mobile Android
- `:wearos` - WearOS
- `:tv` - Android TV (optional)
- `:auto` - Android Auto (optional)

## What is a Feature?

A **feature** is a complete user journey or business capability, not just a single screen. This distinction is crucial for creating a clean and maintainable structure.

### Good Features (Business Capabilities)

- **auth**: The entire authentication flow (login, register, forgot password).
- **profile**: User profile management (view, edit, settings).
- **cart**: The complete shopping cart and checkout process.

### Poor Features (Just Screens)

- **login-screen**: Too granular. This should be part of the `auth` feature.
- **settings-screen**: Should be part of the `profile` feature.

By creating feature modules for business capabilities, you create cohesive and self-contained units that are easier to manage, test, and benefit from incremental build caching.

## Module Structure

```
android-dojo/
├── app/                      # Mobile Android app module
│   └── src/main/kotlin/com/yourpackage/
│       ├── MainActivity.kt
│       ├── di/
│       │   └── AppModule.kt          # Loads all DI modules (core + features)
│       └── navigation/               # Navigation 3 setup
│           ├── AppNavigation.kt      # NavDisplay, entryProvider, sceneStrategy
│           ├── NavigationRoutes.kt   # Defines all serializable NavKey objects
│           └── scenes/
│               └── DashboardScene.kt
│
├── wearos/                   # WearOS app module
│   └── src/main/kotlin/com/yourpackage/wear/
│       ├── MainActivity.kt
│       ├── di/
│       │   └── WearAppModule.kt      # Loads DI modules for wear platform
│       └── navigation/               # WearOS-specific navigation
│           ├── WearNavigation.kt     # Contains SwipeDismissableNavHost
│           └── WearRoutes.kt         # Sealed class routes for type safety
│
├── core/                     # Unified core module
│   └── src/main/kotlin/com/yourpackage/core/
│       ├── common/           # Pure Kotlin utilities, Result class, extensions
│       ├── data/             # ALL repositories, DAOs, network APIs, Room/Retrofit setup
│       │   ├── local/        # Room database, DAOs, entities
│       │   ├── remote/       # Retrofit APIs, DTOs, network layer
│       │   └── repository/   # Repository implementations
│       ├── domain/           # ALL domain models and use cases
│       │   ├── model/        # Business models (User, etc.)
│       │   ├── repository/   # Repository interfaces
│       │   └── usecase/      # Use cases
│       ├── ui/               # Shared design system
│       │   ├── theme/        # App theme, colors, typography
│       │   ├── component/    # Reusable UI components
│       │   └── util/         # UI utilities
│       └── di/               # Core infrastructure DI (repositories, network, database)
│
└── features/
    ├── auth/                 # Auth feature module (presentation only)
    │   ├── build.gradle.kts
    │   └── src/main/kotlin/com/yourpackage/features/auth/
    │       ├── di/
    │       │   └── AuthModule.kt     # DI for the ViewModel
    │       ├── AuthViewModel.kt      # SHARED ViewModel for mobile & wear
    │       ├── LoginScreen.kt        # Mobile or shared UI screen
    │       └── wear/                 # Sub-package for WearOS-specific UI
    │           └── WearLoginScreen.kt
    │
    ├── dashboard/            # Dashboard feature module
    │   ├── build.gradle.kts
    │   └── src/main/kotlin/com/yourpackage/features/dashboard/
    │       ├── di/
    │       │   └── DashboardModule.kt
    │       ├── DashboardViewModel.kt
    │       └── DashboardScreen.kt
    │
    └── profile/              # Additional feature modules...
        ├── build.gradle.kts
        └── src/main/kotlin/com/yourpackage/features/profile/
            └── ...
```

## Auto-Registering Feature Modules

To avoid manually adding each feature to `settings.gradle.kts`, use a wildcard include:

```kotlin
// settings.gradle.kts
file("features").listFiles()?.filter { it.isDirectory }?.forEach {
    include(":features:${it.name}")
}
```

This automatically picks up any subdirectory under `features/` as a module.

## Platform-Specific Navigation

A key strength of this architecture is how it isolates platform-specific implementations. Navigation is a perfect example of this.

**`:app` Module**: Uses the Navigation 3 library (`androidx.navigation3`) to handle adaptive layouts with scenes, a savable back stack with keys, and a central `NavDisplay`.

**`:wearos` Module**: Uses the specialized Wear Compose Navigation library (`androidx.wear.compose:compose-navigation`), which provides components tailored for watches, like the `SwipeDismissableNavHost`.

The feature modules simply provide the `@Composable` screens. The `:app` and `:wearos` modules are independently responsible for calling those screens using the correct navigation library for their platform.

## Why This Works Well

**Incremental Build Caching**: Each feature module is cached independently. Editing one feature doesn't recompile the others.

**Multi-platform Ready**: All platform modules (`:app`, `:wearos`, `:tv`, `:auto`, etc.) can share feature code from day one.

**Clean Dependencies**: Feature modules cannot depend on each other, only on `:core`. This prevents your project from becoming a "ball of mud."

**Easy Scaffolding**: New features just need a directory with a `build.gradle.kts`. The wildcard include handles registration automatically.

## The Dependency Flow

The fundamental principle is strictly enforced. The dependency direction is always:

```
app/wearos → features:* → core
```

## Example Module Dependencies

```kotlin
// In app/build.gradle.kts and wearos/build.gradle.kts
dependencies {
    implementation(project(":core"))
    implementation(project(":features:auth"))
    implementation(project(":features:dashboard"))
    implementation(project(":features:profile"))
}

// In features/auth/build.gradle.kts (and other feature modules)
dependencies {
    // Feature modules depend ONLY on the core module
    implementation(project(":core"))

    // NO dependency on other feature modules allowed!
    // implementation(project(":features:profile")) // This breaks isolation

    // NO dependency on app/wearos modules allowed!
    // implementation(project(":app")) // This would cause circular dependency
}

// In core/build.gradle.kts
dependencies {
    // Core has no dependency on other project modules
    // Only external libraries (Retrofit, Room, etc.)
}
```

## Tech Stack

- **Dependency Injection**: Koin
- **Networking**: Retrofit
- **Database**: Room
- **Serialization**: Kotlinx Serialization
- **Image Loading**: Coil
- **Navigation**:
  - Mobile: Navigation 3 (`androidx.navigation3`)
  - WearOS: Wear Compose Navigation (`androidx.wear.compose:compose-navigation`)
- **State Management**: StateFlow + MVVM
- **Formatting**: Spotless
- **Testing**: MockK
- **Build**: Gradle KTS + version catalogs (`libs.versions.toml`)
- **Platforms**: Mobile Android + WearOS (+ TV, Auto optional)

## Benefits

- **Faster Incremental Builds**: Gradle caches unchanged feature modules independently
- **Multi-platform Code Sharing**: Features work across all platforms from day one
- **Solo Development Friendly**: Wildcard includes reduce boilerplate for new features
- **Platform Flexibility**: Each platform uses optimal navigation solution
- **Feature Isolation**: Features can't depend on each other, keeping architecture clean

## Getting Started

1. **Create core module**: Build `:core` with all shared logic
2. **Create first feature**: Add `features/auth/` directory with `build.gradle.kts`
3. **Add wildcard include**: Set up auto-registration in `settings.gradle.kts`
4. **Platform setup**: Implement platform modules (`:app`, `:wearos`) with appropriate navigation
5. **Iterate**: Add more feature modules as needed, each containing only presentation layer
