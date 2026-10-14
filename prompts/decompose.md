# Decomposing a problem into a roadmap

You are planning work that smaller models will carry out. This document explains who those models are, what a good plan looks like for them, and how to record it. The quality of the result is mostly decided here, by how you cut the problem and how you write each piece.

## Who does the work

Each issue you write is worked by a small model: Claude Haiku, or a small local model such as Bonsai. An issue passes through three loops, and each loop is a separate run that starts with an empty context:

1. **Refinement** reads the issue and the code it names, then tightens the issue: confirms the files exist, fills in exact signatures, and corrects anything the code contradicts.
2. **Implementation** makes the change.
3. **Testing** writes or extends the tests and runs the checks.

None of these workers has seen this conversation, the roadmap, or any other issue. A worker knows what the issue says and what it reads from the repository, and nothing else. It does careful work on a well-bounded change. It will not reliably infer intent, find its own way around a large codebase, or notice that a neighbouring issue is changing the same file.

## The structure

A **roadmap** is an ordered list of epics that together deliver the goal. An **epic** is a set of issues. An **issue** is one change, small enough for one worker in one run.

Two properties define an epic:

- Every issue in it can be in progress at the same moment, worked by different workers, with none of them waiting on or colliding with another.
- When all of its issues are done, the repository builds and its tests pass, so the next epic can start from a working state.

The roadmap's order carries the sequencing; within an epic there is none. Elapsed time is therefore roughly the number of epics, not the number of issues, so prefer few, wide epics over many narrow ones.

## Finding the parallelism

Two issues can run in parallel when neither needs the other's result and they do not edit the same file. Most work that looks sequential is sequential only because of a shared interface: one piece calls a function that another piece is writing. You can decide that interface now and write it into both issues, and then neither has to wait.

Use these techniques, in this order of preference:

1. **Fix the contract in the plan.** Write the exact signature, type, schema or route into every issue that touches it. Both sides then build against the text of the issue.
2. **Move the shared foundation to an earlier epic.** When code must exist before others can compile against it, such as an interface or a schema migration, make it a small epic of its own.
3. **Give every shared file one owner.** Files where many features register themselves, such as a router, the dependency wiring or a tool list, are where parallel workers collide. Assign all edits to such a file to a single issue, or leave them to one wiring issue in a later epic.
4. **Merge.** Two issues that cannot be separated are one issue.

### Dependencies

An issue may declare that it depends on another issue in the same epic. Treat this as a last resort. A dependency turns parallel work back into a queue, and the worker behind it sits idle. Before adding one, try each technique above. If you still need it, say in one sentence why none of them worked. An issue never needs to declare a dependency on an earlier epic, because the roadmap order already covers that.

## Sizing an issue

The budget you are spending is the worker's reading. Small models get less reliable as their context fills, so an issue is the right size when the worker has to read only a few files and change fewer. As a guide, aim for at most five files to read and three to change. If an issue needs more, split it.

Put in everything the worker needs and nothing else:

- **Include** what you learned from exploring that the worker would otherwise have to rediscover: the naming convention, a helper that already exists, the command that runs the tests, a rule from the repository's own instructions that applies to this change.
- **Leave out** why the project exists, the roadmap, and what other epics do.
- **Paste, never point.** "See the epic", "as discussed" and "like the other issue" refer to things the worker cannot see. Copy the content into the issue.

## Issue format

Give each issue an imperative title that names the thing being changed. Write the description in this form:

```
Epic: <number and name>
Depends on: none
  (or: <issue title>, followed by why it is unavoidable)

## Environment
Where to work and how: the working directory, how to reach the toolchain,
which of the repository's rules apply, and whether other workers are
editing nearby at the same time.

## Goal
One or two sentences: what exists after this issue that did not before, and what it is for.

## Read first
- <path>: what to look for in it

## Change
- <path>: what to add or modify, with exact names, signatures and field names

## Contract
The interfaces this issue provides or must match, verbatim.

## Done when
- <command> succeeds
- <observable behaviour>

## Out of scope
What is nearby that the worker should not touch, especially files another issue owns.
```

"Done when" needs at least one command the worker can run, scoped so that it can pass before the rest of the epic lands. Choose commands that leave nothing behind in the repository: for example `go build -o /dev/null ./cmd/api`, since a plain `go build ./cmd/api` drops a binary in the working directory.

## Before you decompose

Read the code first. Every path and signature in an issue should be one you confirmed by opening the file, because a worker given a wrong path will spend its run looking for it. Read the repository's own instructions as well, such as `CLAUDE.md` and `ARCHITECTURE.md`.

If the goal is ambiguous in a way that would change how you cut it, ask before planning.

## Check the plan

Before you record anything, go through each epic:

- List the files each issue changes. A file that appears under two issues is a collision; resolve it with one of the techniques above.
- List the packages each issue's "Done when" commands compile, including everything those packages import. If that list contains a package another issue in the epic is editing, the check can fail on the other worker's half-written file. Put parallel work in packages that do not import one another, and have a consumer declare the small interface it needs instead of importing the package that will satisfy it.
- List what each issue's "Done when" commands use while they run: ports, databases, fixed temp paths. Two issues in one epic that use the same one will interfere when their checks run at the same moment. Give each its own, for example a different port per issue.
- For each issue, ask whether it could start right now if only the earlier epics were finished.
- Read each issue as though you knew nothing else. Anything you had to fill in from memory is missing from the issue.
- Look at every dependency and confirm its one-sentence reason still holds.
- Look at each pair of adjacent epics and ask whether fixing a contract would let them become one.

## Record the plan

1. Write the roadmap to `docs/roadmaps/<short-name>.md`: the goal, then each epic in order with one line stating what is true when it is done, followed by its issues in full.
2. Create the issues of the **first epic only** in codeplat with `issue_create`, in stage `refinement`. Ask which project key to use if you were not told.
3. Report the roadmap's shape: how many epics, how many issues in each, and every dependency with its reason.

Later epics stay in the roadmap file until the epic before them is done. The tracker has no epics yet, so an issue created early would be picked up early. Holding them back also lets you re-check each epic against the code as it then stands, since earlier work often changes a detail.

## Illustration

This shows the reasoning on one small case. Do not copy its shape; the right cut depends on the code in front of you.

Goal: add comments to issues in this repository, available over REST, the CLI and MCP.

A first cut might be one issue per layer, five in a row: core, memory store, SQLite store, then the three transports. That is five steps of elapsed time. Looking at the code changes the picture:

- Adding methods to the `core.Store` interface breaks the build until both stores implement them. Those changes cannot be separated, so they are one issue (technique 4), and they form the first epic on their own (technique 2).
- The three transports live in separate packages and share no file. Once the service methods exist, they can be written at the same time. The exact method signatures go into each of the three issues (technique 1).

Result: two epics. Epic 1 has one issue: comment types, store port, both store implementations and the service methods. Epic 2 has three parallel issues, one per transport, and no dependencies.
