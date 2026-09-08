# Scripts folder maintenance

## Why these scripts exist

Testing here is **human-in-the-loop product use**: a person clicks through the
running app, inspects what they see, and reports what is wrong; an agent fixes
it and the person looks again. Given how quickly agents can fix code, that loop
finds more real problems per hour than any automated suite we have tried — the
automated approach was tried and deliberately unwound.

The only thing worth automating is the boring setup *around* that loop. That is
what this folder is: shortcuts through specific actions (boot the stack, put
money in an account, reset a person's W-9 history) so the human gets to the
interesting click faster.

So: do **not** add test harnesses, e2e suites, unit-test scaffolding, or
"run all the checks" runners here or anywhere else in the repo. If a repetitive
manual action is slowing testing down, add a script for that one action.

## Layout rules

- Every subfolder holds exactly **one script and one markdown file**, named
  after the script.
- Subfolders must be named clearly enough that a future agent can find the
  script a user is asking for by skimming `ls scripts/`.
- The markdown file is a **short, accurate, plain-English description** of what
  the script does, with a usage line.

`lib.sh` at this level is the one exception: shared plumbing (stack discovery,
a localhost-only guard, admin API calls, chain helpers) that the scripts
source. It is not runnable on its own.

## Duties when touching a script

- If you run a script and its description no longer matches what it actually
  does, **update the description** so it matches the script.
- If the description is right but the script is broken for structural reasons
  (a moved path, a renamed route, a changed schema), **fix the script** so it
  works and matches its description again. The description must never be
  "this script is broken".
- New scripts follow the same shape: one folder, one script, one honest
  markdown file.
