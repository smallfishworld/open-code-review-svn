# SVN Fork Changelog

## v1.13.8-svn.1

### Upstream synchronization
- Merge stable upstream v1.13.8 (`4236c871fe8130aa840a2a1c0922f469e87e0416`) with the SVN fork's existing Git history, rather than copying newer unreleased main changes.
- Include the review runtime refactor (#1887), CLI changes (#1894), comparison-side code reading (#1903), and Go 1.26.11 CI upgrade (#1904).

### SVN migration and reliability
- Keep SVN workspace, revision, revision-range review, native SVN diff, preview, code search, file finding, historical file reading, and fork packaging.
- Adapt `file_read`, `code_search`, and `file_find` to use the correct old and new SVN revisions. Real `svnadmin` fixtures cover both sides with spaces and UTF-8 paths.
- Preserve successful SVN content caching while avoiding mutex copies in the new comparison-side access layer.
- Upgrade both the root and ACP Go module minimums to 1.26.11, and retain Linux, Windows and macOS build workflows.

### Compatibility and outstanding issues
- **Breaking upstream change:** the `--tools` CLI override was removed. Specify tool availability in the template configuration. `--max-tools` limits tool-call budgets rather than selecting specific tools.
- Upstream PRs #1812 (failed retry history) and #1803 (partially dropped comments) remain unmerged as of the sync baseline. Neither is claimed fixed by this release.
- Git-style historical `--resume` is not yet supported for SVN reviews. No true external LLM gateway has been validated as part of the local test suite.

### Verification
- Main Go build/test/vet, ACP tests and vet, real SVN repository integration, old/new side regression, GitHub Actions Node contract tests, and six Go cross-compilation targets were exercised locally.
- Release artifacts, registry publications, and the final GitHub CI status must be checked independently before claiming publication.

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
