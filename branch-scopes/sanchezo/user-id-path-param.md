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

## Messaging someone without chat (backend)

`POST /conversations` gave the same "could not be found" for a deactivated person, a
block and someone outside the chat beta, so testers took the beta gate for a broken
feature. Someone without chat now gets 422 `chat_not_enabled`: "You can't message this
person: they don't have chat enabled yet. An SFLuv admin can turn it on for them in the
admin panel." (Not 403: the app reads any 403 as an auth failure.) Deactivated and
blocked still read "could not be found". Every refusal is logged with the request id.

## Volume

4 files changed, 1 added. No migrations, no new routes.
