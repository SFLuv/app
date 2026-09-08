# Estimating time for branch scopes

> **This is the project-local copy.** The method is maintained as the
> `time-accounting` skill at
> <https://github.com/pjol/SKILLS/tree/main/time-accounting> (checked out at
> `~/SKILLS/time-accounting/`), which carries a runnable
> `scripts/measure_sittings.py`. Prefer the skill; keep this file pointing at it
> rather than letting the two drift.
>
> Measured figures are recorded in `branch-scopes/<branch-name>.md` and nowhere
> else — the skill supplies the number, that folder is the record.

Branch scopes carry hours. Those hours have been wrong — inflated roughly
**4.7x** across `pjol/merchant-onboarding-revamp` before this document existed —
because they were guessed from the size of the diff rather than measured from
the clock. This is how to measure them instead.

## The rule

**Hours are wall-clock time that a session was actually running. They are not an
estimate of how long the work would have taken to do by hand.**

That is the whole of the error being corrected here. A stepper of ~1,000 lines
*looks* like a day of work, and saying so is a plausible-sounding sentence that
happens to be false. If an agent wrote it in forty minutes, the honest number is
forty minutes. The scope document records what happened, not what a
counterfactual human would have needed.

Nothing else in the branch-scope convention changes: itemised hours still sum to
the round's total, and rounding every item up to a common floor still inflates
it.

## The measurement

The session transcript is the record of the doing. It holds a timestamp on every
message, so working time is recoverable exactly.

```bash
# Session transcripts live here, one JSONL per session:
ls ~/.claude/projects/<sanitized-cwd>/*.jsonl
```

Cluster the timestamps into **sittings**, splitting on any gap longer than **30
minutes**, and sum the span of each sitting. A gap under 30 minutes is somebody
reading a diff, thinking, or fetching coffee — that is working time. A gap over
it is somebody who left.

```sh
# From a local checkout (tested; --gap/--all/--project flags):
python3 ~/SKILLS/time-accounting/scripts/measure_sittings.py --project .

# Or straight from the public repo, no checkout or credentials needed:
curl -fsSL https://raw.githubusercontent.com/pjol/SKILLS/main/time-accounting/scripts/measure_sittings.py -o /tmp/ms.py
python3 /tmp/ms.py --project .
```

Corroborate against file mtimes before writing the number down — they should
fall inside the sittings:

```bash
git status --porcelain | awk '{print $NF}' \
  | while read f; do [ -f "$f" ] && stat -f "%Sm %N" -t "%Y-%m-%d %H:%M" "$f"; done | sort
```

## Splitting a sitting across features

The sitting total is the truth; the per-feature split is an apportionment of it.
Apportion by **when files were touched**, not by how large they are — file
mtimes bracket each feature within the sitting, and the brackets must sum to the
measured total. If two features interleave, say so and split the overlap evenly
rather than inventing a boundary.

## What each source is good for

| Source | Measures | Use it for |
|---|---|---|
| Transcript timestamps | When work actually happened | **The total. This is the number.** |
| File mtimes | When each file was last written | Splitting a sitting across features; corroborating the total |
| Commit timestamps | When work *landed* | Nothing, on its own — see below |
| Diff size | How much changed | The volume line only. Never hours. |

**Commit timestamps are not working time.** A branch committed days after it was
written clusters into the length of the commit session, which is neither the
doing nor an estimate of it. The `feat/merchant-multi-location` scope has a whole
paragraph on discovering this the hard way. Use commits only to establish which
round work belongs to.

## Live measurement

A global `UserPromptSubmit` hook stamps every message with its local time, so
elapsed time is visible in the conversation rather than reconstructed afterwards:

```json
{ "hooks": { "UserPromptSubmit": [ { "hooks": [ {
  "type": "command",
  "command": "printf '{\"hookSpecificOutput\":{\"hookEventName\":\"UserPromptSubmit\",\"additionalContext\":\"Message sent at %s.\"}}' \"$(date '+%Y-%m-%d %H:%M:%S %Z')\"",
  "timeout": 5
} ] } ] } }
```

It lives in `~/.claude/settings.json` and applies to every project. With it,
noting the first and last stamp of a sitting is enough — no transcript parsing.

## Writing the number down

State the method in the round, in one line, and name its weakness if it has one.
These are honest:

- *"1.6h, measured from transcript timestamps (Aug 28 12:18–13:53), corroborated
  by file mtimes."*
- *"0.9h measured; the split across the five features is apportioned by file
  mtime and is approximate below 0.1h."*

This is not:

- *"Derived from volume and the shape of the work."* That is a guess wearing the
  vocabulary of a measurement. If the clock was not consulted, do not report
  hours at all — report the volume and say the time was not measured.

Round to 0.1h. When a measured total and the itemised split disagree, the
measured total wins and the split gets adjusted.
