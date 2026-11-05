# Branch scope — `pgo/next-15.5.28`

Oct 22 2026 · app (frontend) · **time not measured**

The clock was not consulted for this branch, so no hours are recorded. The work was a single short session:
research the advisories, bump the dependency, run a production build.

## What this branch is

A security patch bump of `next` from 15.5.24 to 15.5.28, the Maintenance LTS release from the out-of-band
[Oct 14 2026 security update](https://nextjs.org/blog/nextjs-security-update-october-14-2026). It also picks up
the September fixes that shipped in 15.5.27.

Exposure check against the October advisories. None of them was reachable in our code:

| Advisory | Precondition | Us |
|---|---|---|
| GHSA-673q-mjm6-r5xr (critical, RCE) | 16.x with Cache Components and `use cache` | 15.x, no `use cache` |
| CVE-2026-96889 / GHSA-2464-fhr4-w9xw (critical, RCE) | Image Optimization on glibc Linux with `sharp` < 0.35.5 | `images.unoptimized: true`; lockfile already has `sharp` 0.35.5 |
| GHSA-p3m5-59mp-jv5r (high, DoS) | App Router with at least one Server Action | no `"use server"` anywhere |

We patch anyway so that adding a Server Action or turning image optimization back on doesn't silently expose us.

Next.js 15 reached the end of Maintenance LTS on Oct 21 2026 (two years after its release, per the
[support policy](https://nextjs.org/support-policy)), so this is likely the last routine 15.x patch. Moving
to 16.x is follow-up work for its own branch.

| Item | Hours | Repo |
|---|---|---|
| Bump `next` to 15.5.28 (lockfile changes are only `next`, `@next/env`, `@next/swc-*`); `pnpm build` passes | not measured | app |

Volume: 2 files (`frontend/package.json`, `frontend/pnpm-lock.yaml`), +63/−43. 0 migrations, 0 routes.
