package jsonparser

import (
	"errors"
	"slices"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/ytakahashi/pino/internal/domain"
)

// parsedComment keeps the one fact needed to assign comments in a gap: whether
// it began before that gap's first newline. The assignment belongs here, next
// to the byte-level reading of Extra, so callers cannot accidentally infer it
// from a Trivia slot after that information has been lost.
type parsedComment struct {
	comment            domain.Comment
	beforeFirstNewline bool
}

// commentScanner carries line state across the separate Extra values that
// hujson places on either side of punctuation. They form one logical gap for
// comment ownership even though punctuation splits them in the syntax tree.
type commentScanner struct {
	tokenOnLine bool
	seenNewline bool
}

// comments reads the comments in extra. UTF-8 errors are normally reported by
// validateCommentUTF8 after hujson rejects the input; NewComment remains here
// as a defensive check in case Extra's guarantees change.
func (s *commentScanner) comments(extra hujson.Extra, offset int, src []byte) ([]parsedComment, error) {
	parsed := make([]parsedComment, 0)

	for i := 0; i < len(extra); {
		switch extra[i] {
		case ' ', '\t':
			i++

		case '\r', '\n':
			// Both CR and LF end a line. Seeing both for CRLF is harmless.
			s.tokenOnLine = false
			s.seenNewline = true
			i++

		case '/':
			start := i
			if i+1 >= len(extra) || (extra[i+1] != '/' && extra[i+1] != '*') {
				return nil, errorAt(src, offset+start, "invalid JSON trivia", nil)
			}
			block := extra[i+1] == '*'

			var text []byte
			if block {
				end := start + 2
				for end+1 < len(extra) && (extra[end] != '*' || extra[end+1] != '/') {
					end++
				}

				// hujson has already checked the syntax. Keeping this guard makes
				// a malformed Extra fail at its source rather than slice past it.
				if end+1 >= len(extra) {
					return nil, errorAt(src, offset+start, "unterminated block comment", nil)
				}

				text = extra[start+2 : end]
				i = end + 2
			} else {
				end := start + 2
				for end < len(extra) && extra[end] != '\r' && extra[end] != '\n' {
					end++
				}
				if end == len(extra) {
					return nil, errorAt(src, offset+start, "unterminated line comment", nil)
				}

				text = extra[start+2 : end]
				i = end
			}

			comment, err := domain.NewComment(string(text), block, !s.tokenOnLine)
			if err != nil {
				return nil, commentError(src, offset+start+2, err)
			}

			parsed = append(parsed, parsedComment{
				comment:            comment,
				beforeFirstNewline: !s.seenNewline,
			})

			for _, b := range extra[start:i] {
				if b == '\r' || b == '\n' {
					s.tokenOnLine = false
					s.seenNewline = true
				} else {
					s.tokenOnLine = true
				}
			}

		default:
			// Extra is produced by hujson and contains only whitespace and
			// comments. If that changes, refusing is safer than dropping bytes.
			return nil, errorAt(src, offset+i, "invalid JSON trivia", nil)
		}
	}

	return parsed, nil
}

// runs reads source-adjacent Extra values without resetting line state between
// them. Punctuation may split one logical gap into several runs in hujson's
// tree, but pino normalizes that punctuation when it writes the document.
func (s *commentScanner) runs(src []byte, runs ...extraRun) ([]parsedComment, error) {
	var parsed []parsedComment
	for _, run := range runs {
		comments, err := s.comments(run.extra, run.offset, src)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, comments...)
	}

	return parsed, nil
}

func commentError(src []byte, offset int, err error) error {
	var invalidUTF8 *domain.InvalidUTF8Error
	if errors.As(err, &invalidUTF8) {
		return errorAt(src, offset+invalidUTF8.Index, "invalid UTF-8 in comment", err)
	}

	return errorAt(src, offset, err.Error(), err)
}

// hujson exposes no typed sentinel for this error, so match only the terminal
// reason whose imprecise position validateCommentUTF8 is meant to replace.
func isInvalidCommentUTF8Error(err error) bool {
	return strings.HasSuffix(err.Error(), ": invalid UTF-8 in comment")
}

// validateCommentUTF8 recovers the precise location and typed cause after
// hujson rejects invalid comment text at the comment's opening delimiter.
func validateCommentUTF8(src []byte) error {
	inString := false
	escaped := false

	for i := 0; i < len(src); i++ {
		if inString {
			if escaped {
				escaped = false
				continue
			}

			switch src[i] {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}

			continue
		}

		if src[i] == '"' {
			inString = true
			continue
		}
		if src[i] != '/' || i+1 == len(src) {
			continue
		}

		block := src[i+1] == '*'
		line := src[i+1] == '/'
		if !block && !line {
			continue
		}

		start := i + 2
		end := start
		if block {
			for end < len(src) && (end+1 == len(src) || src[end] != '*' || src[end+1] != '/') {
				end++
			}
		} else {
			for end < len(src) && src[end] != '\r' && src[end] != '\n' {
				end++
			}
		}

		if invalidUTF8Index(src[start:end]) >= 0 {
			_, err := domain.NewComment(string(src[start:end]), block, false)
			return commentError(src, start, err)
		}

		if block && end < len(src) {
			i = end + 1
		} else {
			i = max(end-1, i)
		}
	}

	return nil
}

func commentList(extra hujson.Extra, offset int, tokenOnLine bool, src []byte) ([]domain.Comment, error) {
	scanner := commentScanner{tokenOnLine: tokenOnLine}
	parsed, err := scanner.runs(src, extraRun{extra, offset})
	if err != nil {
		return nil, err
	}

	return commentValues(parsed), nil
}

func commentValues(parsed []parsedComment) []domain.Comment {
	result := make([]domain.Comment, 0, len(parsed))
	for _, comment := range parsed {
		result = append(result, comment.comment)
	}

	return result
}

type extraRun struct {
	extra  hujson.Extra
	offset int
}

// splitGap assigns comments before a logical gap's first newline to the
// preceding item, and the rest to the following item. hujson splits that gap
// around its comma, but pino writes the comma on the preceding item's line.
// Carrying line state through it keeps OwnLine relative to the layout pino can
// reproduce rather than to a punctuation-only line that disappears on save.
func splitGap(left, right extraRun, src []byte) (after, before []domain.Comment, err error) {
	scanner := commentScanner{tokenOnLine: true}
	parsed, err := scanner.runs(src, left, right)
	if err != nil {
		return nil, nil, err
	}

	for _, comment := range parsed {
		if comment.beforeFirstNewline {
			after = append(after, comment.comment)
		} else {
			before = append(before, comment.comment)
		}
	}

	return after, before, nil
}

func withSurroundingTrivia(n domain.Node, before, after []domain.Comment) domain.Node {
	return domain.WithTrivia(n, domain.NewTrivia(before, after, slices.Collect(n.Trivia().Inside())))
}

func withInsideTrivia(n domain.Node, inside []domain.Comment) domain.Node {
	return domain.WithTrivia(n, domain.NewTrivia(
		slices.Collect(n.Trivia().Before()),
		slices.Collect(n.Trivia().After()),
		inside,
	))
}

func appendMemberAfter(member domain.Member, comments []domain.Comment) domain.Member {
	member.Trivia = domain.NewTrivia(
		slices.Collect(member.Trivia.Before()),
		append(slices.Collect(member.Trivia.After()), comments...),
		slices.Collect(member.Trivia.Inside()),
	)

	return member
}

func appendNodeAfter(n domain.Node, comments []domain.Comment) domain.Node {
	return withSurroundingTrivia(
		n,
		slices.Collect(n.Trivia().Before()),
		append(slices.Collect(n.Trivia().After()), comments...),
	)
}
