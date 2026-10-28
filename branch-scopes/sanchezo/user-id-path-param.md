# sanchezo/user-id-path-param

**Date:** 2026-10-28 · **Repos:** app (backend) · **Hours:** not measured separately; done
inside the 2026-10-28 mobile bug-fix sitting (Sentry rollout + preview fixes), which is
recorded with the mobile work.

## Encoded user ids in paths (backend)

The mobile app encodes the Privy DID into URL paths, so `did:privy:…` arrives as
`did%3Aprivy%3A…`. chi matches against the raw path when one is present, so
`chi.URLParam(r, "userId")` returned the still-encoded id and every lookup by it
failed. Found through Sentry (SFLUV-MOBILE-2): `GET /users/:id/public` and
`/users/:id/avatar` answered 404 for real users, so chat headers had no name or photo.

New `userIdParam(r)` (`handlers/path_params.go`) decodes the parameter. Used by all
five `{userId}` routes: public profile, avatar, unblock, remove group member, set
group member role.

## Why "could not be found" (backend)

`POST /conversations` deliberately gives one answer for a deactivated person, a block
and someone outside the chat beta. It now logs which one, with the request id, so a
report from Sentry can be explained from the Railway logs.

## Volume

4 files changed, 1 added. No migrations, no new routes.
