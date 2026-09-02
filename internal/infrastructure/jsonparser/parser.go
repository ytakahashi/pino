// Package jsonparser reads the bytes of a JSON document into pino's tree.
//
// It is the only place that knows which library does the parsing, and it hands
// back nothing of it: the caller receives domain nodes and, on failure, a
// *SyntaxError naming a position. Nothing above depends on the library, so
// replacing it is confined to this package.
package jsonparser

import (
	"errors"
	"unicode/utf8"

	"github.com/tailscale/hujson"

	"github.com/ytakahashi/pino/internal/domain"
)

// Parser turns JSON text into a tree.
//
// The library underneath parses a superset of JSON that admits comments and
// trailing commas, and keeps a byte offset for every value. Reading the
// superset and then refusing what the dialect does not allow is what lets pino
// report a comment as a comment, in the place it was written, instead of as a
// character the parser did not expect. It is also what accepting JSONC later
// will switch on rather than replace.
type Parser struct{}

func New() *Parser { return &Parser{} }

// Parse turns src into a tree, accepting the extensions d allows.
//
// Every failure is a *SyntaxError carrying the position it happened at.
func (p *Parser) Parse(src []byte, d domain.Dialect) (domain.Node, error) {
	root, err := hujson.Parse(src)
	if err != nil {
		if d.AllowComments && isInvalidCommentUTF8Error(err) {
			if commentErr := validateCommentUTF8(src); commentErr != nil {
				return nil, commentErr
			}
		}

		return nil, parseHujsonError(err)
	}

	// The dialect is enforced before the tree is built, so that a comment is
	// reported where it is written rather than against whatever value the
	// conversion happens to be looking at when it reaches it.
	if err := checkDialect(&root, d, src); err != nil {
		return nil, err
	}

	node, err := convert(&root, src)
	if err != nil {
		return nil, err
	}

	before, err := commentList(root.BeforeExtra, beforeExtraOffset(&root), false, src)
	if err != nil {
		return nil, err
	}

	after, err := commentList(root.AfterExtra, root.EndOffset, true, src)
	if err != nil {
		return nil, err
	}

	return withSurroundingTrivia(node, before, after), nil
}

// convert builds the node for v.
func convert(v *hujson.Value, src []byte) (domain.Node, error) {
	switch tv := v.Value.(type) {
	case *hujson.Object:
		return convertObject(v, tv, src)
	case *hujson.Array:
		return convertArray(v, tv, src)
	case hujson.Literal:
		return convertLiteral(v, tv, src)
	}

	// The library's value type is a closed set of the three above, and a
	// successful parse never leaves it empty. Reaching here would mean the
	// library gained a case pino does not represent.
	return nil, errorAt(src, v.StartOffset, "unsupported JSON value", nil)
}

func convertObject(v *hujson.Value, obj *hujson.Object, src []byte) (domain.Node, error) {
	members := make([]domain.Member, 0, len(obj.Members))

	for i := range obj.Members {
		name := &obj.Members[i].Name
		var memberBefore []domain.Comment

		if i == 0 {
			var err error
			memberBefore, err = commentList(name.BeforeExtra, beforeExtraOffset(name), true, src)
			if err != nil {
				return nil, err
			}
		} else {
			previous := &obj.Members[i-1].Value
			after, before, err := splitGap(
				extraRun{previous.AfterExtra, previous.EndOffset},
				extraRun{name.BeforeExtra, beforeExtraOffset(name)},
				src,
			)
			if err != nil {
				return nil, err
			}

			members[i-1] = appendMemberAfter(members[i-1], after)
			memberBefore = before
		}

		lit, ok := name.Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			// The library refuses a non-string member name while parsing, so
			// this guards the type assertion rather than the document.
			return nil, errorAt(src, name.StartOffset, "object name is not a string", nil)
		}

		key, err := decodeString(lit, name.StartOffset, src)
		if err != nil {
			return nil, err
		}

		value, err := convert(&obj.Members[i].Value, src)
		if err != nil {
			return nil, err
		}

		// hujson stores the two sides of ':' separately. pino has no key-after
		// slot, so both runs become trivia before the value in source order. One
		// scanner carries line state across them because pino moves ':' back to
		// the key's line when it writes the document.
		colonScanner := commentScanner{tokenOnLine: true}
		valueComments, err := colonScanner.runs(
			src,
			extraRun{name.AfterExtra, name.EndOffset},
			extraRun{
				obj.Members[i].Value.BeforeExtra,
				beforeExtraOffset(&obj.Members[i].Value),
			},
		)
		if err != nil {
			return nil, err
		}
		valueBefore := commentValues(valueComments)

		// Object punctuation belongs to the member, so its trailing comments
		// live on Member.Trivia. Array elements have no wrapper and keep the
		// corresponding comments on the element node itself.
		value = withSurroundingTrivia(value, valueBefore, nil)

		members = append(members, domain.Member{
			Key:    key,
			Value:  value,
			Trivia: domain.NewTrivia(memberBefore, nil, nil),
		})
	}

	var inside []domain.Comment
	if len(members) == 0 {
		comments, err := commentList(obj.AfterExtra, v.StartOffset+1, true, src)
		if err != nil {
			return nil, err
		}
		inside = comments
	} else {
		last := &obj.Members[len(obj.Members)-1].Value
		after, trailing, splitErr := splitGap(
			extraRun{last.AfterExtra, last.EndOffset},
			extraRun{obj.AfterExtra, closeExtraOffset(v, last)},
			src,
		)
		if splitErr != nil {
			return nil, splitErr
		}
		members[len(members)-1] = appendMemberAfter(members[len(members)-1], after)
		inside = trailing
	}

	node, err := domain.NewObject(members)
	if err != nil {
		return nil, objectError(err, v, obj, src)
	}

	return withInsideTrivia(node, inside), nil
}

// objectError gives a position to what the domain refused.
//
// The domain identifies a duplicate key by the index of the member that
// repeats it, having no notion of where the document put it; the parsed object
// is what turns that index back into an offset. Anything else is reported
// against the object as a whole, which is the closest position known.
func objectError(err error, v *hujson.Value, obj *hujson.Object, src []byte) error {
	offset := v.StartOffset

	var dup *domain.DuplicateKeyError
	if errors.As(err, &dup) && dup.Dup >= 0 && dup.Dup < len(obj.Members) {
		offset = obj.Members[dup.Dup].Name.StartOffset
	}

	return errorAt(src, offset, err.Error(), err)
}

func convertArray(v *hujson.Value, arr *hujson.Array, src []byte) (domain.Node, error) {
	elements := make([]domain.Node, 0, len(arr.Elements))

	for i := range arr.Elements {
		var before []domain.Comment
		if i == 0 {
			var err error
			before, err = commentList(
				arr.Elements[i].BeforeExtra,
				beforeExtraOffset(&arr.Elements[i]),
				true,
				src,
			)
			if err != nil {
				return nil, err
			}
		} else {
			previous := &arr.Elements[i-1]
			after, leading, err := splitGap(
				extraRun{previous.AfterExtra, previous.EndOffset},
				extraRun{arr.Elements[i].BeforeExtra, beforeExtraOffset(&arr.Elements[i])},
				src,
			)
			if err != nil {
				return nil, err
			}

			elements[i-1] = appendNodeAfter(elements[i-1], after)
			before = leading
		}

		element, err := convert(&arr.Elements[i], src)
		if err != nil {
			return nil, err
		}
		element = withSurroundingTrivia(element, before, nil)

		elements = append(elements, element)
	}

	var inside []domain.Comment
	if len(elements) == 0 {
		comments, err := commentList(arr.AfterExtra, v.StartOffset+1, true, src)
		if err != nil {
			return nil, err
		}
		inside = comments
	} else {
		last := &arr.Elements[len(arr.Elements)-1]
		after, trailing, splitErr := splitGap(
			extraRun{last.AfterExtra, last.EndOffset},
			extraRun{arr.AfterExtra, closeExtraOffset(v, last)},
			src,
		)
		if splitErr != nil {
			return nil, splitErr
		}
		elements[len(elements)-1] = appendNodeAfter(elements[len(elements)-1], after)
		inside = trailing
	}

	node := domain.NewArray(elements)

	return withInsideTrivia(node, inside), nil
}

func convertLiteral(v *hujson.Value, lit hujson.Literal, src []byte) (domain.Node, error) {
	switch lit.Kind() {
	case '"':
		s, err := decodeString(lit, v.StartOffset, src)
		if err != nil {
			return nil, err
		}

		node, err := domain.NewString(s)
		if err != nil {
			return nil, errorAt(src, v.StartOffset, err.Error(), err)
		}

		return node, nil

	case '0':
		// string(lit) copies. A literal aliases the buffer it was parsed from,
		// and a tree still pointing into that buffer would change underneath
		// the document when the caller reuses it.
		return domain.NewNumber(string(lit)), nil

	case 't':
		return domain.NewBool(true), nil

	case 'f':
		return domain.NewBool(false), nil

	case 'n':
		return domain.NewNull(), nil
	}

	return nil, errorAt(src, v.StartOffset, "invalid literal", nil)
}

// decodeString unescapes a JSON string literal, refusing text that is not
// valid UTF-8.
//
// The check belongs here, against the bytes of the source, because the library
// unescapes through encoding/json, which substitutes U+FFFD for anything it
// cannot decode instead of failing. The tree built from that would look valid
// while no longer holding what the file holds, and the substitution would
// surface only at save time, having rewritten a file the user had opened to
// look at. RFC 8259 requires JSON to be UTF-8, so the document is refused
// while there is still a position to name.
func decodeString(lit hujson.Literal, offset int, src []byte) (string, error) {
	if i := invalidUTF8Index(lit); i >= 0 {
		return "", errorAt(src, offset+i, "invalid UTF-8 in string", nil)
	}

	if i := loneSurrogateIndex(lit); i >= 0 {
		return "", errorAt(src, offset+i, "unpaired surrogate escape in string", nil)
	}

	return lit.String(), nil
}

// invalidUTF8Index returns the index in b of the first byte that is not part
// of a valid UTF-8 sequence, or -1.
func invalidUTF8Index(b []byte) int {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])

		// U+FFFD is a character in its own right and may legitimately appear
		// in a document. Only a decode one byte wide is a failed one.
		if r == utf8.RuneError && size == 1 {
			return i
		}

		i += size
	}

	return -1
}

// escapeLen is the length of a \uXXXX escape.
const escapeLen = len(`\uXXXX`)

// loneSurrogateIndex returns the index in b of the first \uXXXX escape naming
// a surrogate that does not form a pair, or -1.
//
// Such an escape passes the UTF-8 check above, its own bytes being ASCII, but
// encoding/json still turns it into U+FFFD, so it is refused for the same
// reason: pino would write back a document it was only asked to show.
func loneSurrogateIndex(b []byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' {
			continue
		}

		// A backslash escapes the byte after it, so the pair is stepped over
		// together. That is what keeps the u in \\u1234, which is literal
		// text, from being read as the start of an escape.
		if i+1 >= len(b) || b[i+1] != 'u' {
			i++

			continue
		}

		if i+escapeLen > len(b) {
			return i
		}

		switch r := hexRune(b[i+2 : i+escapeLen]); {
		case r < 0:
			return i

		case r < 0xD800 || r > 0xDFFF:
			i += escapeLen - 1

		case r > 0xDBFF:
			// A low surrogate reached on its own. A paired one is stepped
			// over below, with the high half that introduces it.
			return i

		case i+2*escapeLen > len(b) || b[i+escapeLen] != '\\' || b[i+escapeLen+1] != 'u':
			return i

		default:
			if low := hexRune(b[i+escapeLen+2 : i+2*escapeLen]); low < 0xDC00 || low > 0xDFFF {
				return i
			}

			i += 2*escapeLen - 1
		}
	}

	return -1
}

// hexRune reads the four hexadecimal digits of a \u escape, or returns -1 if
// they are not four hexadecimal digits.
func hexRune(b []byte) rune {
	var r rune

	for _, c := range b {
		var d rune

		switch {
		case c >= '0' && c <= '9':
			d = rune(c - '0')
		case c >= 'a' && c <= 'f':
			d = rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = rune(c-'A') + 10
		default:
			return -1
		}

		r = r<<4 | d
	}

	return r
}
