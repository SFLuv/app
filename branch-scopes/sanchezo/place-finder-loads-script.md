# Branch scope — `sanchezo/place-finder-loads-script`

Oct 23 2026 · app (frontend) · **~0.2h active**

Hours are wall-clock time from this session's timestamps (about 12:59 to 1:12 PM PDT), not measured with the
`time-accounting` script.

## What this branch is

Niebla SF had no opening hours. Its location was linked to the Google listing for the building
("374 Golden Gate Ave", Place ID `ChIJTc85bJqAhYARLs2l0Im_VsE`), not the business ("Niebla SF",
`ChIJNxXrIymBhYARrisJtu-ehWM`), so the nightly hours sync never got hours. The admin fix is "Re-point at a
Google listing" in the merchant editor, but its search box always said "Could not reach Google to search",
with no network request at all.

`PlaceAutocomplete` (`components/merchant/google_place_finder.tsx`) calls `google.maps.importLibrary` and
assumes the host page already loaded the Maps script. Settings, Locations and merchant onboarding do; the admin
page doesn't. So `google` was undefined, the call threw, and the catch-all showed a connection error. The
component now calls `ensureGooglePlacesScript()` itself, which is a no-op when the script is already present,
and logs the caught error to the console instead of swallowing it.

| Item | Hours | Repo |
|---|---|---|
| Trace Niebla's missing hours to the building Place ID; finder loads the Maps script itself; log search errors | 0.2 | app |

Volume: 1 file (+ this scope file). 0 migrations, 0 routes.
