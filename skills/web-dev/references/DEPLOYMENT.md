# Deployment & Infrastructure Guide

## Table of Contents

- [Firebase App Hosting](#firebase-app-hosting)
- [apphosting.yaml](#apphostingyaml)
- [Cloud Functions](#cloud-functions)
- [Cloud DNS Setup](#cloud-dns-setup)
- [Firestore Security Rules](#firestore-security-rules)
- [Firebase Auth](#firebase-auth)
- [Free Tier Reference](#free-tier-reference)
- [Cloud Run (Escape Hatch)](#cloud-run-escape-hatch)

## Firebase App Hosting

All projects use **Firebase App Hosting** — SPA and SSR alike. Git-push deploys with zero infra config.

### Setup

1. Connect your GitHub repo to App Hosting in the Firebase console
2. App Hosting auto-detects Angular and builds accordingly (SSR is enabled by default)

### Deploy

Push to the connected branch. App Hosting builds and deploys automatically. Each push creates a rollout — use rollout URLs for previewing before promoting.

## apphosting.yaml

Use `apphosting.yaml` in the root directory to manage runtime settings and environment variables.

### Example configuration

```yaml
runConfig:
  cpu: 1
  memoryMiB: 1024
  minInstances: 0      # Scale to zero when inactive (free tier friendly)
  maxInstances: 10     # Limit scaling to control costs
  concurrency: 100     # Handle up to 100 requests per instance

env:
  # Static environment variables
  - variable: STORAGE_BUCKET
    value: my-app.appspot.com
    availability: [RUNTIME]

  # Secrets from Cloud Secret Manager
  # Set via: firebase apphosting:secrets:set API_KEY
  - variable: API_KEY
    secret: API_KEY
    availability: [BUILD, RUNTIME]
```

### Custom domain
...
1. Add `projectname.charlies.bot` in Firebase console → App Hosting → Custom domains
2. Use `run_gcloud_command` to add the DNS records in Cloud DNS (see DNS section below)
3. SSL is auto-provisioned — no manual cert setup

### MCP tools

- `firebase_list_apps` — list all Firebase apps in the project.
- `firebase_get_environment` — view current Firebase project and user context.

## Cloud Functions

Use for backend logic alongside App Hosting.

### HTTP callable functions

```typescript
import { onRequest } from 'firebase-functions/v2/https';

export const api = onRequest(async (req, res) => {
  // Handle API request
  res.json({ status: 'ok' });
});
```

### Firestore triggers

```typescript
import { onDocumentCreated } from 'firebase-functions/v2/firestore';

export const onUserCreated = onDocumentCreated('users/{userId}', async (event) => {
  const userData = event.data?.data();
  // React to new user document
});
```

### Auth triggers

```typescript
import { beforeUserCreated } from 'firebase-functions/v2/identity';

export const onNewUser = beforeUserCreated(async (event) => {
  // Custom logic on user creation
});
```

### Scheduled functions

```typescript
import { onSchedule } from 'firebase-functions/v2/scheduler';

export const dailyCleanup = onSchedule('every day 03:00', async () => {
  // Run daily cleanup
});
```

### Deploy and debug

```bash
firebase deploy --only functions        # Deploy all functions
firebase deploy --only functions:api    # Deploy specific function
```

Debug with `mcp__plugin_firebase_firebase__functions_get_logs` — don't dig through the console manually.

### Local development

```bash
firebase emulators:start --only functions,firestore,auth
```

## Cloud DNS Setup

Every project gets `projectname.charlies.bot`. DNS is managed in Cloud DNS via gcloud MCP.

### Add a subdomain record

Use `run_gcloud_command` with:

```bash
gcloud dns record-sets create projectname.charlies.bot. \
  --zone=charlies-bot \
  --type=A \
  --ttl=300 \
  --rrdatas=<firebase-hosting-ip>
```

For App Hosting, you may also need a TXT record for domain verification. Firebase console shows the exact values.

### CNAME alternative (Firebase App Hosting)

Use `run_gcloud_command` with:

```bash
gcloud dns record-sets create projectname.charlies.bot. \
  --zone=charlies-bot \
  --type=CNAME \
  --ttl=300 \
  --rrdatas=<app-hosting-domain>.
```

## Firestore Security Rules

### Locked-down default

Every project starts with this — no open rules, ever:

```
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    match /{document=**} {
      allow read, write: if false;
    }
  }
}
```

### Auth-owns-data pattern

The default for most features — users can only access their own data:

```
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    match /users/{userId} {
      allow read, write: if request.auth != null && request.auth.uid == userId;
    }
    match /users/{userId}/{subcollection=**} {
      allow read, write: if request.auth != null && request.auth.uid == userId;
    }
  }
}
```

### Public-read / authenticated-write pattern

For public content that only authenticated users can modify:

```
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    match /posts/{postId} {
      allow read: if true;
      allow write: if request.auth != null;
    }
  }
}
```

### Validate before deploying

Deploy with:

```bash
firebase deploy --only firestore:rules
```

## Firebase Auth

### Setup

1. Enable Google sign-in in Firebase console (authorized domain: `projectname.charlies.bot`)
2. Contact email: sudo@charlies.bot

### Angular integration

Use the Firebase JS SDK directly — no `@angular/fire` needed.

Initialize Firebase once (e.g., in a service or `main.ts`):

```typescript
import { initializeApp } from 'firebase/app';
import { getAuth } from 'firebase/auth';

const app = initializeApp({
  apiKey: '...',
  authDomain: '...',
  projectId: '...',
  // ...
});
```

### AuthService pattern

In `core/auth.service.ts`:

```typescript
import { Injectable, signal, computed, inject, DestroyRef } from '@angular/core';
import { getAuth, onAuthStateChanged, signInWithPopup, signOut, GoogleAuthProvider, User } from 'firebase/auth';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private destroyRef = inject(DestroyRef);
  private auth = getAuth();
  user = signal<User | null>(null);
  isAuthenticated = computed(() => this.user() !== null);

  constructor() {
    const unsubscribe = onAuthStateChanged(this.auth, (user) => this.user.set(user));
    this.destroyRef.onDestroy(unsubscribe);
  }

  async signInWithGoogle(): Promise<void> {
    await signInWithPopup(this.auth, new GoogleAuthProvider());
  }

  async signOut(): Promise<void> {
    await signOut(this.auth);
  }
}
```

### Route guard pattern

In `core/auth.guard.ts`:

```typescript
export const authGuard: CanActivateFn = () => {
  const authService = inject(AuthService);
  const router = inject(Router);
  return authService.isAuthenticated() || router.createUrlTree(['/login']);
};
```

## Free Tier Reference

| Service | Free tier limit |
| --- | --- |
| App Hosting | Git-push deploys, auto-scaling, included in Firebase plan |
| Firestore | 1 GiB storage, 50K reads/day, 20K writes/day, 20K deletes/day |
| Firebase Auth | 50K MAU (email/password, Google) |
| Cloud Functions | 2M invocations/month, 400K GB-seconds, 200K GHz-seconds |
| Cloud Storage | 5 GB storage, 1 GB/day download |

Design for these limits. If a side project exceeds free tier, it's probably successful enough to pay for.

## Cloud Run (Escape Hatch)

Use Cloud Run only when Cloud Functions genuinely can't handle it — e.g., long-running processes, WebSocket servers, or custom container requirements.

Deploy via gcloud MCP:

```bash
gcloud run deploy <service-name> \
  --source . \
  --region us-central1 \
  --allow-unauthenticated
```

Map to subdomain via Cloud DNS (same process as above). This should be rare — Cloud Functions cover the vast majority of backend needs.
