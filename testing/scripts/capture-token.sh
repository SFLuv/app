#!/usr/bin/env bash
# Captures a Privy access token so API scenarios can act as a real user.
#
# Why by hand: captcha is on with no env override, so a script cannot log in.
# And an X-Test-User header was considered and rejected — it would be a
# universal impersonation key in front of a system that moves real value, gated
# on an env var that fails open. A token copied from a browser you logged into
# yourself is strictly weaker, which is the point.
#
# It is an access token, so it lasts about an hour. Re-run before a session.
# Written to artifacts/.token, gitignored, mode 600.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

cat <<'INSTRUCTIONS'

Getting the token — use the Network tab, not Local Storage.

Privy keeps its session split across storage and cookies and none of the keys
are named usefully. The request header is the sure thing, because the app puts
the token on every backend call.

  1. Open  https://localhost:3000  and log in.
       (self-signed cert — click through the browser warning)

  2. DevTools → Network tab.

  3. Reload the page, then click any request going to  localhost:8080
       — /users and /config are good ones.

  4. Scroll to "Request Headers" and find:

         Access-Token: eyJhbGciOi....

       Copy the VALUE only — everything after "Access-Token: ".
       It is a JWT, so it starts with "eyJ".

  5. Paste it below.

Note it is Access-Token, not Authorization. This backend does not use bearer
auth, and a bearer header authenticates nothing.

INSTRUCTIONS

printf 'Paste the token: '
read -r token

# Tolerate a whole header line being pasted, which is the natural thing to do
# after copying from devtools.
token="$(printf '%s' "$token" | sed -E 's/^[[:space:]]*(Access-Token|Authorization)[[:space:]]*:[[:space:]]*//I; s/^[Bb]earer[[:space:]]+//')"
token="$(printf '%s' "$token" | tr -d '[:space:]')"

[[ -n "$token" ]] || die "nothing pasted"
case "$token" in
  eyJ*) ;;
  *) c '0;33' "  warning: that does not look like a JWT (expected it to start with eyJ)" ;;
esac

printf '%s' "$token" > "$TOKEN_FILE"
chmod 600 "$TOKEN_FILE"

body="$(api GET /users)"
if [[ "$(status)" != "200" ]]; then
  rm -f "$TOKEN_FILE"
  die "the backend rejected that token (HTTP $(status)). Not saved.
Most likely it has expired — they last about an hour — or only part of it was copied."
fi

# GET /users nests the person under .user alongside contacts, locations and
# wallets — it is a bootstrap payload, not a user object.
did="$(printf '%s' "$body" | jq -r '.user.id // empty')"

# Fall back to the JWT's own sub claim, which is the did by definition and does
# not depend on any response shape staying put.
if [[ -z "$did" ]]; then
  did="$(printf '%s' "$token" | cut -d. -f2 \
    | tr '_-' '/+' | { read -r p; printf '%s' "$p$(printf '%*s' $(( (4 - ${#p} % 4) % 4 )) '' | tr ' ' '=')"; } \
    | base64 -d 2>/dev/null | jq -r '.sub // empty' 2>/dev/null)"
  [[ -n "$did" ]] && info "read the did from the token's sub claim"
fi

pass "token accepted, acting as ${did:-unknown}"

# The role flags decide which scenarios can run at all, so print them rather
# than letting a scenario fail later with a bare 403.
roles="$(printf '%s' "$body" | jq -r '[.user | to_entries[] | select(.key|startswith("is_")) | select(.value == true) | .key] | join(", ")' 2>/dev/null)"
if [[ -n "$roles" && "$roles" != "null" ]]; then
  info "roles: $roles"
else
  info "roles: none — merchant, workflow and admin scenarios will need prank forwarding"
fi

cat <<EOF

Export this so prank forwarding knows who is doing the pranking:

  export SFLUV_PRANKER_DID="$did"

EOF
