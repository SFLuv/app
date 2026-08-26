package handlers

import "net/http"

// ServeW9CompletePage is where the vendor lands somebody after they submit.
//
// Served by us rather than left to the vendor's own confirmation, and served
// from the backend rather than the web app, because it has to exist wherever
// the backend does: it is quoted to the vendor as ReturnUrl at the moment a
// form request is created, and a 404 there is the last thing somebody sees
// after handing over a tax identification number.
//
// It says almost nothing on purpose. The app is already closing this browser
// on its own, as soon as the backend confirms the filing cleared, so anything
// more than a held breath would be read for a second and then vanish. It
// exists to not be a dead end.
//
// Deliberately unauthenticated and free of identifiers. The vendor redirects
// here in whatever browser the person had open, which may be nobody's session,
// and the URL can end up in history or a referrer.
func (a *AppService) ServeW9CompletePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>W-9 received</title>
</head>
<body style="font-family:system-ui,-apple-system,sans-serif;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#fbf4f0;color:#20303c">
  <main style="text-align:center;padding:2rem;max-width:22rem">
    <div style="font-size:2.5rem;line-height:1">&#10003;</div>
    <h1 style="font-size:1.25rem;margin:.75rem 0 .5rem">Your W-9 has been received</h1>
    <p style="margin:0;color:#5b6b78;line-height:1.5">
      You can close this page and return to SFLuv. Any rewards being held will
      be sent to you shortly.
    </p>
  </main>
</body>
</html>`))
}
