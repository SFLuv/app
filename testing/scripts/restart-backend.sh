#!/usr/bin/env bash
# Restarts the dev backend from current source, with the env dev-up gave it.
#
# Needed because dev-up starts the backend once and never rebuilds it, so a
# stack left running overnight serves yesterday's code — routes 404 and fields
# go missing while the source in front of you clearly has them. preflight.sh
# detects that; this fixes it.
#
# Why python parses the env file rather than `source`:
#
#   PRIVY_VKEY is a PEM public key spanning several literal newlines. `set -a;
#   source` truncates it at the first one, the backend then cannot verify a
#   Privy token, and EVERY authenticated request returns a bare 403 with
#   nothing logged. dev-up avoids this by passing env as an array. Sourcing
#   that file looks right and quietly breaks authentication.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

ENV_FILE="$SFLUV_ROOT/tmp/backend.dev.env"
LOG="$SFLUV_ROOT/tmp/logs/backend-test.log"
[[ -f "$ENV_FILE" ]] || die "no $ENV_FILE — has dev-up ever run?"

step "Restarting the backend"

port="${API##*:}"
for pid in $(lsof -ti:"$port" -sTCP:LISTEN 2>/dev/null); do
  parent="$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ')"
  # `go run` spawns the real binary; killing only the child leaves the parent
  # holding the port on its next build.
  [[ -n "$parent" ]] && kill "$parent" 2>/dev/null
  kill "$pid" 2>/dev/null
done

for _ in $(seq 1 15); do
  lsof -ti:"$port" -sTCP:LISTEN >/dev/null 2>&1 || break
  sleep 1
done
lsof -ti:"$port" -sTCP:LISTEN >/dev/null 2>&1 && die "port $port is still held"
pass "old backend stopped"

python3 - "$ENV_FILE" "$SFLUV_ROOT/backend" "$LOG" <<'PY'
import os, re, subprocess, sys

env_file, workdir, log = sys.argv[1], sys.argv[2], sys.argv[3]
raw = open(env_file).read()

# Values run to the next KEY= at the start of a line, so multi-line PEMs survive.
env = dict(os.environ)
for m in re.finditer(r'^([A-Z_][A-Z0-9_]*)=(.*?)(?=^[A-Z_][A-Z0-9_]*=|\Z)', raw, re.M | re.S):
    env[m.group(1)] = m.group(2).rstrip('\n')

with open(log, 'w') as out:
    subprocess.Popen(['go', 'run', './cmd/server'], cwd=workdir, env=env,
                     stdout=out, stderr=subprocess.STDOUT, start_new_session=True)
print(f"  launched with {len(env)} env vars; PRIVY_VKEY is "
      f"{len(env.get('PRIVY_VKEY',''))} chars over "
      f"{env.get('PRIVY_VKEY','').count(chr(10))+1} lines")
PY

info "compiling and starting — logs at $LOG"
for i in $(seq 1 40); do
  if [[ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$API/config" 2>/dev/null)" == "200" ]]; then
    pass "backend up after ~${i}s"
    break
  fi
  sleep 1
done

[[ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$API/config")" == "200" ]] \
  || { fail "backend did not come up — see $LOG"; tail -20 "$LOG"; }

summary "Restart backend"
