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
// The form opens in the system browser, so this page's job is to hand the
// person back to the app. iOS will not switch apps silently on a timer — the
// closest it allows is the page invoking the app's URL scheme, which Safari
// answers with an "Open in SFLuv?" prompt — so the script waits a breath and
// invokes it, and a visible link does the same for anyone the prompt missed.
// Desktop browsers get neither: the web app has no scheme, so the page just
// says what happened. The scheme URL is one the app deliberately does not
// parse — it matches no link pattern, so it only brings the app forward, and
// the app's own status polling takes it from there.
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
      Any rewards being held will be sent to you shortly.
    </p>
    <p id="return-hint" style="margin:1rem 0 0;color:#5b6b78;line-height:1.5;display:none">
      Sending you back to SFLuv&hellip;
    </p>
    <p style="margin:1.25rem 0 0">
      <a id="return-link" href="sfluv://return-from-w9" style="display:none;background:#16794c;color:#fff;text-decoration:none;padding:.75rem 1.5rem;border-radius:999px;font-weight:600">Back to SFLuv</a>
    </p>
  </main>
  <script>
    (function () {
      // Only phones have the app; a desktop hitting this page after the web
      // flow has nowhere to deep-link to and would get a browser error.
      if (!/iPhone|iPad|Android/i.test(navigator.userAgent)) return;
      document.getElementById("return-hint").style.display = "block";
      document.getElementById("return-link").style.display = "inline-block";
      setTimeout(function () {
        // Safari answers this with an "Open in SFLuv?" prompt — the OS does
        // not allow a silent app switch from a timer. The link above covers
        // anyone who dismisses it.
        window.location.href = "sfluv://return-from-w9";
      }, 1600);
    })();
  </script>
</body>
</html>`))
}
