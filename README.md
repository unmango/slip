# slip

[![Hercules CI](https://hercules-ci.com/api/v1/site/github/account/unmango/project/slip/badge)](https://hercules-ci.com/github/unmango/slip)

`slip` captures a thought into a zettel with as little ceremony as possible, then writes it as markdown a machine can read.

A zettelkasten is a slip box, so the tool is a slip.

Capture is the whole of v1.
Reading, linking, and searching are handled by [zk](https://github.com/zk-org/zk), which `slip` passes through to.

## Install

```sh
nix run github:unmango/slip
```

The packaged binary carries `zk` on its `PATH`.
For a build with no `zk` in its closure:

```sh
nix build .#slip-standalone
```

## Capture

```sh
slip                                  # opens an editor
slip capture a thought worth keeping  # captures the arguments
pbpaste | slip                        # captures stdin
slip < notes.md                       # same
```

`slip` prints only the path it wrote, so it composes:

```sh
$EDITOR "$(slip)"
```

In the editor:

| Key | Action |
| ------------------ | ---------------- |
| `ctrl+s`, `ctrl+d` | save and exit |
| `ctrl+c`, `esc` | discard and exit |
| `enter` | newline |

Enter inserts a newline.
Saving is a deliberate chord, because a capture tool that saves halfway through a thought is worse than no capture tool.

## Where notes go

The first of these that is set wins:

1. `--dir`
1. `$ZK_DIR`
1. `$ZK_NOTEBOOK_DIR`, the same override zk itself honours
1. the zk notebook found by walking up from the working directory
1. `$XDG_DATA_HOME/zettelkasten`, else `~/.local/share/zettelkasten`

Point `ZK_DIR` at a git repository or a synced folder.
The XDG path is a defensible default, not where a corpus you care about should live.

## What a note looks like

```markdown
---
zettel_id: "202609081412"
title: The thing I was thinking about
note_type: NOTE_TYPE_FLEETING
format: CONTENT_FORMAT_MARKDOWN
create_time: 2026-09-08T14:12:33-05:00
---

The thing I was thinking about

and the rest of what I typed, verbatim.
```

The body is stored exactly as typed.
The title is derived from the first line rather than cut out of it.

`zettel_id` is a local-time timestamp to the minute, and it is also the filename.
Two thoughts captured in the same minute get `a`, `b`, and so on appended, in both places at once.

Frontmatter keys are the field names from [`unmango/apis`](https://github.com/unmango/apis), under `proto/unmango/zettelkasten/`, which is the schema of record.
Enum values are the full proto names, so the file is close to protojson and the conversion is mechanical.
Output-only fields are deliberately absent: `uid` is server-assigned, and the link and word counts are computed from the corpus.

## Working with zk

`slip` runs any command it does not implement as `zk`, so one binary covers both:

```sh
slip list      # zk list
slip edit      # zk edit
slip lsp       # zk lsp
```

Notes `slip` writes are already valid zk notes.
zk keeps frontmatter keys it does not recognise verbatim, so `zettel_id`, `note_type`, and `format` survive in its `metadata` map, and it reindexes on every invocation, so a note appears without running `zk index`.

One setting makes zk read our creation timestamp rather than the file's mtime:

```sh
slip init
```

That appends the following to `.zk/config.toml`, and reports what to add by hand if the table already exists:

```toml
[format.markdown.frontmatter]
creation-date-key = "create_time"
```

With that, `zk list --format jsonl` gives back titles, tags parsed from the body, links, word counts, and our own metadata intact.

## Licensing

`slip` is MIT.
zk is GPL-3.0, and is only ever invoked as a separate process, never linked or copied.
