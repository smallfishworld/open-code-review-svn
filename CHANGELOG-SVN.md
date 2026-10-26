# SVN Fork Changelog

## v1.13.3-svn.1 (released 2026-10-26)

### Upstream sync
- Merge official Alibaba Open Code Review v1.13.3 (`375f64cd6f67a48a135da83cfcf59c52669a09cb`) without replacing SVN-specific Git history.
- Retain upstream Git review, scan, providers, MCP and updated release automation.

### SVN features preserved
- Review Subversion working-copy changes, specific revisions (`--commit`) and ranges (`--from`/`--to`).
- SVN-native diff, historical file reading, `code_search`, `file_find` and file filtering.
- Review preview/JSON output, internal model gateways and per-run LLM concurrency limits.

### Reliability and distribution
- Bound large Git/SVN code-search match lines to avoid oversized LLM tool output; preserve UTF-8.
- Restore six-platform release assets plus verified Linux x86_64 offline tarball.
- Preserve `@smallfishworld/open-code-review-svn` and `svn` npm dist-tag.
- Pin CI Actions and allow fork npm token authentication alongside upstream trusted publishing.

### Verification
- Official upstream three-way merge, Go build and full Go test suite succeeded in the initial integration run.
- All eleven migration checks and merged main CI passed; Go Race coverage 91.1%.
- Published six platform binaries, a verified Linux offline archive, and seven npm packages.
