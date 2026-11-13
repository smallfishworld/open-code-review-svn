// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/alibaba/open-code-review/internal/agent"
	"github.com/alibaba/open-code-review/internal/diff"
	"github.com/alibaba/open-code-review/internal/gitcmd"
	"github.com/alibaba/open-code-review/internal/tool"
)

type reviewCodeTools struct {
	reader     *tool.FileReader
	access     *tool.CodeAccess
	resolution *diff.InputResolution
}

func prepareReviewCodeTools(ctx context.Context, repoDir string, opts reviewOptions, runner *gitcmd.Runner, resolution *diff.InputResolution, svnMode ...bool) (*reviewCodeTools, error) {
	if len(svnMode) > 0 && svnMode[0] {
		reader := &tool.FileReader{RepoDir: repoDir, Mode: tool.ParseReviewMode(opts.from, opts.to, opts.commit), Ref: opts.to, Runner: runner, SVN: true}
		if opts.commit != "" {
			reader.Ref = opts.commit
		}
		setup := &reviewCodeTools{reader: reader, resolution: resolution}
		if reader.Mode == tool.ModeWorkspace {
			return setup, nil
		}
		// A Subversion comparison is linear: the old endpoint of a single
		// revision is revision-1, while a range uses the explicit from revision.
		oldRef := opts.from
		if reader.Mode == tool.ModeCommit {
			rev, err := strconv.ParseUint(opts.commit, 10, 63)
			if err != nil || rev == 0 {
				return nil, fmt.Errorf("invalid SVN revision %q", opts.commit)
			}
			oldRef = strconv.FormatUint(rev-1, 10)
		}
		old := *reader
		old.Ref = oldRef
		old.Mode = tool.ModeCommit
		if old.Ref == "0" {
			old.Mode = tool.ModeEmpty
		}
		access, err := tool.NewComparisonCodeAccess(*reader, old)
		if err != nil {
			return nil, err
		}
		setup.access = access
		return setup, nil
	}
	mode := tool.ParseReviewMode(opts.from, opts.to, opts.commit)
	if resolution == nil && mode != tool.ModeWorkspace {
		var err error
		resolution, err = agent.ResolveComparisonInput(ctx, agent.Args{
			RepoDir: repoDir, From: opts.from, To: opts.to, Commit: opts.commit, GitRunner: runner,
		})
		if err != nil {
			return nil, err
		}
	}
	reader := &tool.FileReader{
		RepoDir: repoDir, Mode: mode, Ref: fileReadRef(mode, opts, resolution), Runner: runner,
	}
	setup := &reviewCodeTools{reader: reader, resolution: resolution}
	if mode != tool.ModeWorkspace {
		if resolution == nil || resolution.ResolvedHead == "" || (resolution.ResolvedBase == "" && !resolution.BaseIsEmpty) {
			return nil, fmt.Errorf("comparison code access requires resolved comparison endpoints")
		}
		previous := *reader
		previous.Mode = tool.ModeCommit
		previous.Ref = resolution.ResolvedBase
		if resolution.BaseIsEmpty {
			previous.Mode = tool.ModeEmpty
		}
		var err error
		setup.access, err = tool.NewComparisonCodeAccess(*reader, previous)
		if err != nil {
			return nil, err
		}
	}
	return setup, nil
}
