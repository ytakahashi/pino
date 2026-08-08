package jsonparser

import (
	"bytes"

	"github.com/tailscale/hujson"

	"github.com/ytakahashi/pino/internal/domain"
)

// checkDialect refuses the extensions d does not allow, reporting the first
// one in the document.
//
// The library offers one question, "is this standard JSON", which answers for
// comments and trailing commas together and says nothing about where either
// was found. Dialect names the two separately and pino reports positions, so
// the tree is walked here. That question still decides whether the walk is
// needed at all, which for an ordinary file it is not.
func checkDialect(v *hujson.Value, d domain.Dialect, src []byte) error {
	if d.AllowComments && d.AllowTrailingComma {
		return nil
	}

	if v.IsStandard() {
		return nil
	}

	return checkValue(v, d, src)
}

// checkValue walks v in document order, so that the position reported is the
// first one a reader would come to.
func checkValue(v *hujson.Value, d domain.Dialect, src []byte) error {
	// The extra before a value ends where the value starts, which is what
	// gives it an offset: the library exposes no position of its own for it.
	if err := checkExtra(v.BeforeExtra, v.StartOffset-len(v.BeforeExtra), d, src); err != nil {
		return err
	}

	switch tv := v.Value.(type) {
	case *hujson.Object:
		if err := checkObject(v, tv, d, src); err != nil {
			return err
		}

	case *hujson.Array:
		if err := checkArray(v, tv, d, src); err != nil {
			return err
		}
	}

	return checkExtra(v.AfterExtra, v.EndOffset, d, src)
}

func checkObject(v *hujson.Value, obj *hujson.Object, d domain.Dialect, src []byte) error {
	for i := range obj.Members {
		if err := checkValue(&obj.Members[i].Name, d, src); err != nil {
			return err
		}

		if err := checkValue(&obj.Members[i].Value, d, src); err != nil {
			return err
		}
	}

	var last *hujson.Value
	if n := len(obj.Members); n > 0 {
		last = &obj.Members[n-1].Value
	}

	return checkClose(v, last, obj.AfterExtra, d, src)
}

func checkArray(v *hujson.Value, arr *hujson.Array, d domain.Dialect, src []byte) error {
	for i := range arr.Elements {
		if err := checkValue(&arr.Elements[i], d, src); err != nil {
			return err
		}
	}

	var last *hujson.Value
	if n := len(arr.Elements); n > 0 {
		last = &arr.Elements[n-1]
	}

	return checkClose(v, last, arr.AfterExtra, d, src)
}

// checkClose looks at what sits between the last element and the closing brace
// or bracket: the trailing comma, if there is one, and the extra the library
// hangs off the composite rather than off any value.
//
// last is nil when the composite is empty.
func checkClose(v, last *hujson.Value, after hujson.Extra, d domain.Dialect, src []byte) error {
	// A trailing comma is what the library leaves an element's own extra
	// non-nil for; without one it moves that extra onto the composite. The
	// comma itself follows immediately after the extra.
	trailingComma := last != nil && last.AfterExtra != nil

	if trailingComma && !d.AllowTrailingComma {
		return errorAt(src, last.EndOffset+len(last.AfterExtra), "trailing commas are not supported yet", nil)
	}

	// The composite's own extra begins after whatever punctuation precedes
	// it: the trailing comma when there is one, the last element when there
	// is not, and the opening brace or bracket when there is no element.
	offset := v.StartOffset + 1

	if last != nil {
		offset = last.EndOffset + len(last.AfterExtra)
		if trailingComma {
			offset++
		}
	}

	return checkExtra(after, offset, d, src)
}

// checkExtra refuses a comment within the run of whitespace starting at base.
func checkExtra(extra hujson.Extra, base int, d domain.Dialect, src []byte) error {
	if d.AllowComments || len(extra) == 0 {
		return nil
	}

	if i := commentIndex(extra); i >= 0 {
		return errorAt(src, base+i, "comments are not supported yet", nil)
	}

	return nil
}

// commentIndex returns the index in extra of the first comment, or -1.
//
// Extra holds whitespace and comments and nothing else, already checked by the
// library, so the first slash can only open one.
func commentIndex(extra hujson.Extra) int {
	return bytes.IndexByte(extra, '/')
}
