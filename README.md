# tagged-obsidian-to-hugo

`tagged-obsidian-to-hugo` exports tagged Obsidian notes into Hugo page bundles. It follows the same broad note shape as `bhugo`: the first line is the title, the configured hashtag line selects exportable notes, and the generated bundle lives at `<content-dir>/<slug>/index.md`.

The tool runs once per invocation. It scans a vault, exports only notes matching the requested tag prefix, rewrites links between exported notes, downgrades unresolved note links to plain text, and copies referenced local attachments into the bundle.

## Assumptions

- Notes may start with YAML front matter delimited by `---`. The first non-front-matter line is the title, and a leading `#` heading marker is stripped.
- The hashtag line defaults to the last non-empty line.
- Matching is hierarchical. Exporting `#blog/3` includes notes tagged `#blog/3` and `#blog/3/...`.
- Vault note content and metadata are the source of truth for generated Hugo bundles.

## Usage

```bash
tagged-obsidian-to-hugo /path/to/vault --tag '#blog/3' --hugo-dir /path/to/site --content-dir content/blog
```

Flags:

- `--tag` required Obsidian tag prefix including `#`
- `--hugo-dir` Hugo site root, default `.`
- `--content-dir` bundle output directory relative to `--hugo-dir`, default `content/blog`
- `--tag-line` hashtag line index, default `-1`
- `--categories` default `true`
- `--tags` default `false`
- `--time-format` front matter date format, default `2006-01-02T15:04:05-07:00`
- `-v` enable verbose logging

## Dates

If a note front matter contains `created` or `date created`, the exporter uses it for Hugo `date`. If it contains `last modified` or `date modified`, the exporter emits Hugo `lastmod`. When either field is missing, the exporter falls back to a `#y/YYYY/MM/DD` tag on the configured hashtag line and uses `12:00:00Z` for the missing timestamp. If an exportable note still does not have both timestamps after that fallback, the exporter skips it. File modification times are not used. Front matter timestamps may use either RFC3339 offsets like `2026-04-21T08:45:43+03:00` or basic numeric offsets like `2026-04-21T08:45:43+0300`.

## Supported link forms

- Wiki note links: `[[Other Note]]`, `[[Other Note|Alias]]`
- Wiki embeds: `![[image.png]]`
- Markdown links and images to local files

Local links resolve relative to the source note first. If that misses, the exporter falls back to searching the whole vault by note name or attachment basename. Links to exported notes become relative Hugo markdown links such as `../other-note/`. Links to notes that are not exported are rewritten to plain text. Wiki links with headings or block references are downgraded to plain text in this version. If vault-wide fallback matches multiple notes or attachments, the export fails with an explicit error instead of guessing.

## Attachments

Referenced local attachments are copied into the note bundle and rewritten to bundle-local paths. This also applies when the source link misses locally but resolves elsewhere in the vault. If two different source attachments would produce the same destination filename in a single bundle, or if vault-wide fallback matches multiple attachments, the export fails with an explicit error.
