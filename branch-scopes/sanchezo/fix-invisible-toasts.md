# Branch scope — `sanchezo/fix-invisible-toasts`

Oct 19 2026 · app (frontend) · **~0.3h active**

Hours are wall-clock time from this session's timestamps (about 9:05 to 9:25 AM PDT), not measured with the
`time-accounting` script.

## What this branch is

Every `toast()` in the web app displayed nothing. `useToast()` in `frontend/hooks/use-toast.ts` returned a
mock `toast` that only called `console.log`, and no toast renderer was mounted anywhere. So ~167 success and
error messages across 34 files never reached users. The Website tools had worked around it with their own
`NoticeProvider` (`components/admin/website/notice.tsx`), which is left as it is.

The fix routes `toast()` through `sonner`, which was already a dependency. A themed `<Toaster />`
(`components/ui/toaster.tsx`) is mounted once in `app/layout.tsx`, inside the theme provider so it follows
light and dark. The call shape `toast({ title, description, variant })` is unchanged, so no call sites move;
`variant: "destructive"` shows as an error toast. `toast` and `dismiss` are now module-level and stable across
renders.

| Item | Hours | Repo |
|---|---|---|
| Find the mock, wire `toast()` to sonner, add and mount a themed Toaster, type-check | 0.3 | app |

Volume: 3 files (1 new), +~40 / −183 lines. 0 migrations, 0 routes.

Watch after deploy: messages that were silently swallowed now show. An error toast that fires on every page
load would point at a real, previously hidden failure.
