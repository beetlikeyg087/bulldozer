// Copyright 2019 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bulldozer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog"

	"github.com/palantir/bulldozer/pull"
)

type Signals struct {
	Label             SubSignal `yaml:"label"`
	CommentSubstrings []string  `yaml:"comment_substrings"`
	Comments          []string  `yaml:"comments"`
	PRBodySubstrings  []string  `yaml:"pr_body_substrings"`
	Branches          []string  `yaml:"branches"`
	BranchPatterns    []string  `yaml:"branch_patterns"`
	PRCreator         []string  `yaml:"creators"`
	Match             match     `yaml:"match"	default:"one"`
}

type SubSignal struct {
	Match  match    `yaml:"match"	default:"one"`
	Values []string `yaml:"values"`
}

type match string

const (
	MATCH_ONE        match  = "one"
	MATCH_ALL        match  = "all"
	SIGNAL_NOT_FOUND string = "Signal not found"
	SIGNAL_NOT_MATCH string = "Signal not match"
)

func (s *Signals) Enabled() bool {
	size := 0
	size += len(s.Label.Values)
	size += len(s.CommentSubstrings)
	size += len(s.Comments)
	size += len(s.PRBodySubstrings)
	size += len(s.Branches)
	size += len(s.BranchPatterns)
	size += len(s.PRCreator)
	return size > 0
}

// Matches returns true if the pull request meets one or more signals. It also
// returns a description of the signal that was met. The tag argument appears
// in this description and indicates the behavior (trigger, ignore) this
// set of signals is associated with.
func (s *Signals) Matches(ctx context.Context, pullCtx pull.Context, tag string) (bool, string, error) {
	logger := zerolog.Ctx(ctx)

	if s.Match == MATCH_ALL {
		return s.matchesForAll(ctx, pullCtx, tag, logger)
	}

	return s.matchesForOne(ctx, pullCtx, tag, logger)
}

func (s *Signals) matchesForOne(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {

	if match, reason, err := s.doesLabelSignalMatch(ctx, pullCtx, tag, logger); err != nil || match {
		return true, reason, err
	}
	if match, reason, err := s.doesCommentSingalMatch(ctx, pullCtx, tag, logger); err != nil || match {
		return true, reason, err
	}
	if match, reason, err := s.doesCommentSubstringSingalMatch(ctx, pullCtx, tag, logger); err != nil || match {
		return true, reason, err
	}
	if match, reason, err := s.doesPRSubstringSingalMatch(ctx, pullCtx, tag, logger); err != nil || match {
		return true, reason, err
	}
	if match, reason, err := s.doesTargetBranchSingalMatch(ctx, pullCtx, tag, logger); err != nil || match {
		return true, reason, err
	}
	if match, reason, err := s.doesCreatorSingalMatch(ctx, pullCtx, tag, logger); err != nil || match {
		return true, reason, err
	}

	return false, fmt.Sprintf("pull request does not match the %s", tag), nil
}

func (s *Signals) matchesForAll(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {

	if match, reason, err := s.doesLabelSignalMatch(ctx, pullCtx, tag, logger); err != nil || (!match && reason != SIGNAL_NOT_FOUND) {
		return false, reason, err
	}
	if match, reason, err := s.doesCommentSingalMatch(ctx, pullCtx, tag, logger); err != nil || (!match && reason != SIGNAL_NOT_FOUND) {
		return false, reason, err
	}
	if match, reason, err := s.doesCommentSubstringSingalMatch(ctx, pullCtx, tag, logger); err != nil || (!match && reason != SIGNAL_NOT_FOUND) {
		return false, reason, err
	}
	if match, reason, err := s.doesPRSubstringSingalMatch(ctx, pullCtx, tag, logger); err != nil || (!match && reason != SIGNAL_NOT_FOUND) {
		return false, reason, err
	}
	if match, reason, err := s.doesTargetBranchSingalMatch(ctx, pullCtx, tag, logger); err != nil || (!match && reason != SIGNAL_NOT_FOUND) {
		return false, reason, err
	}
	if match, reason, err := s.doesCreatorSingalMatch(ctx, pullCtx, tag, logger); err != nil || (!match && reason != SIGNAL_NOT_FOUND) {
		return false, reason, err
	}

	return true, fmt.Sprintf("pull request matches the %s", tag), nil
}


func (s *Signals) doesLabelSignalMatch(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {
	labels, err := pullCtx.Labels(ctx)
	if err != nil {
		return false, "unable to list pull request labels", err
	}

	if len(s.Label.Values) == 0 {
		logger.Debug().Msgf("Singal [label] is not found. Skipping...")
		return false, SIGNAL_NOT_FOUND, nil
	}

	if s.Label.Match == MATCH_ALL {
		var match bool
		for _, r := range s.Label.Values {
			match = false
			for _, c := range labels {
				if strings.EqualFold(r, c) {
					match = true
					break
				}
			}
			if !match {
				return false, SIGNAL_NOT_MATCH, nil
			}
		}
		return true, "pull request has all labels", nil
	}
	else{
		for _, r := range s.Label.Values {
			for _, c := range labels {
				if strings.EqualFold(r, c) {
					return true, fmt.Sprintf("pull request matches the label %s", c), nil
				}
			}
		}
		return false, "pull request has no labels match", nil
	}
}

func (s *Signals) doesCommentSingalMatch(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {
	body := pullCtx.Body()
	comments, err := pullCtx.Comments(ctx)
	if err != nil {
		return false, "unable to list pull request comments", err
	}

	if len(s.Comments) == 0 {
		logger.Debug().Msgf("Singal [comments] is not found. Skipping...")
		return false, SIGNAL_NOT_FOUND, nil
	}

	if len(comments) == 0 {
		logger.Debug().Msgf("No comments found to match against")
		return false, SIGNAL_NOT_MATCH, nil
	}

	for _, signalComment := range s.Comments {
		if body == signalComment {
			return true, fmt.Sprintf("pull request body is a %s comment: %q", tag, signalComment), nil
		}
		for _, comment := range comments {
			if comment == signalComment {
				return true, fmt.Sprintf("pull request has a %s comment: %q", tag, signalComment), nil
			}
		}
	}
	return false, SIGNAL_NOT_MATCH, nil
}

func (s *Signals) doesCommentSubstringSingalMatch(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {
	body := pullCtx.Body()
	comments, err := pullCtx.Comments(ctx)
	if err != nil {
		return false, "unable to list pull request comments", err
	}

	if len(s.CommentSubstrings) == 0 {
		logger.Debug().Msgf("Singal [comment_substrings] is not found. Skipping...")
		return false, SIGNAL_NOT_FOUND, nil
	}
	for _, signalSubstring := range s.CommentSubstrings {
		if strings.Contains(body, signalSubstring) {
			return true, fmt.Sprintf("pull request body matches a %s substring: %q", tag, signalSubstring), nil
		}
		for _, comment := range comments {
			if strings.Contains(comment, signalSubstring) {
				return true, fmt.Sprintf("pull request comment matches a %s substring: %q", tag, signalSubstring), nil
			}
		}
	}
	return false, SIGNAL_NOT_MATCH, nil
}

func (s *Signals) doesPRSubstringSingalMatch(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {
	body := pullCtx.Body()
	if len(s.PRBodySubstrings) == 0 {
		logger.Debug().Msgf("Singal [pr_body_substrings] is not found. Skipping...")
		return false, SIGNAL_NOT_FOUND, nil
	}
	for _, signalSubstring := range s.PRBodySubstrings {
		if strings.Contains(body, signalSubstring) {
			return true, fmt.Sprintf("pull request body matches a %s substring: %q", tag, signalSubstring), nil
		}
	}
	return false, SIGNAL_NOT_MATCH, nil
}

func (s *Signals) doesTargetBranchSingalMatch(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {
	targetBranch, _ := pullCtx.Branches()
	if len(s.Branches) == 0 && len(s.BranchPatterns) == 0 {
		logger.Debug().Msgf("Singal [branches] or [branch_patterns] is not found. Skipping...")
		return false, SIGNAL_NOT_FOUND, nil
	}
	for _, signalBranch := range s.Branches {
		if targetBranch == signalBranch {
			return true, fmt.Sprintf("pull request target is a %s branch: %q", tag, signalBranch), nil
		}
	}
	for _, signalBranch := range s.BranchPatterns {
		if matched, _ := regexp.MatchString(fmt.Sprintf("^%s$", signalBranch), targetBranch); matched {
			return true, fmt.Sprintf("pull request target branch (%q) matches pattern: %q", targetBranch, signalBranch), nil
		}
	}
	return false, SIGNAL_NOT_MATCH, nil
}

func (s *Signals) doesCreatorSingalMatch(ctx context.Context, pullCtx pull.Context, tag string, logger *zerolog.Logger) (bool, string, error) {
	creator := pullCtx.Creator()
	if len(s.PRCreator) == 0 {
		logger.Debug().Msgf("Singal [creators] is not found. Skipping...")
		return false, SIGNAL_NOT_FOUND, nil
	}
	for _, signalPRCreator := range s.PRCreator {
		if creator == signalPRCreator {
			return true, fmt.Sprintf("pull request matches a creator %s", creator), nil
		}
	}
	return false, SIGNAL_NOT_MATCH, nil
}
