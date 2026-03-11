# Firebase Auth

## Setup

1. Enable Google sign-in in Firebase console (authorized domain: `projectname.charlies.bot`)
2. Contact email: sudo@charlies.bot

## Angular integration

Use the Firebase JS SDK directly — no `@angular/fire` needed.

Initialize Firebase once (e.g., in a service or `main.ts`):

```typescript
import { initializeApp } from "firebase/app";
import { getAuth } from "firebase/auth";

const app = initializeApp({
  apiKey: "...",
  authDomain: "...",
  projectId: "...",
  // ...
});
```

## Auth service pattern

In `core/auth/auth.ts`:

```typescript
import { Injectable, signal, computed } from "@angular/core";
import {
  getAuth,
  onAuthStateChanged,
  signInWithPopup,
  signOut,
  GoogleAuthProvider,
  User,
} from "firebase/auth";

@Injectable({ providedIn: "root" })
export class Auth {
  private firebaseAuth = getAuth();
  user = signal<User | null>(null);
  isAuthenticated = computed(() => this.user() !== null);

  constructor() {
    // Root singleton — lives for app lifetime, no cleanup needed
    onAuthStateChanged(this.firebaseAuth, (user) => this.user.set(user));
  }

  async signInWithGoogle(): Promise<void> {
    await signInWithPopup(this.firebaseAuth, new GoogleAuthProvider());
  }

  async signOut(): Promise<void> {
    await signOut(this.firebaseAuth);
  }
}
```

## Route guard pattern

In `core/auth/auth-guard.ts`:

```typescript
export const authGuard: CanActivateFn = () => {
  const auth = inject(Auth);
  const router = inject(Router);
  return auth.isAuthenticated() || router.createUrlTree(["/login"]);
};
```
