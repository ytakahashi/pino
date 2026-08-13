package application

import (
	"strconv"

	"github.com/ytakahashi/pino/internal/domain"
)

// PromptKind says which shape of answer pino is waiting for.
//
// There are two because there are only two ways to answer: by typing text, or
// by pressing one of the keys offered. A confirmation is the second with two
// choices in it rather than a third kind of its own — drawing it and resolving
// its keys would then be written three times where twice will do.
type PromptKind uint8

const (
	PromptNone PromptKind = iota
	PromptText
	PromptChoice
)

// Choice is one answer a prompt accepts, together with the key that gives it.
//
// The key travels with the label because the two are drawn together: "[s]
// string" is a promise that s does something, and the promise and its keeping
// belong in one place. This is not a second home for the key table. The line
// is that a key drawn on screen belongs to the prompt drawing it, while a key
// a reader has to know already belongs to the keymap.
type Choice struct {
	Key   rune
	Label string
}

// PromptInfo is what pino is waiting to be told.
//
// It is worked out from the edit in progress on every read, the way the status
// bar and the inspector are worked out from the document. Nothing here is
// state, so there is no copy of the flow that could fall out of step with it.
type PromptInfo struct {
	Kind PromptKind

	// Title names what is being asked for: "Edit number", "New key", "type",
	// or, for a confirmation, the whole question.
	Title string

	// Choices are the answers a PromptChoice accepts, and are empty otherwise.
	Choices []Choice

	// Error is why the last answer was refused, and is empty when none was.
	Error string

	// Multiline says the answer may contain newlines, which is true of every
	// JSON string and of nothing else. A key is a string, so a key holding a
	// newline stays editable; a number is not, and a widget that cannot make
	// one is the shortest way of saying so.
	Multiline bool
}

// Prompt is what the session is waiting to be told, and nothing when it is
// waiting for a command instead.
func (a *App) Prompt() PromptInfo {
	if a.flow == nil {
		return PromptInfo{}
	}

	info := PromptInfo{Error: a.flow.err}

	switch a.flow.step {
	case stepText:
		info.Kind = PromptText
		info.Title = a.flow.title()

		// A newline can go into any JSON string and into nothing else, so the
		// kind the answer is read as settles this on its own. A key is a
		// string, which is what keeps a key holding a newline editable.
		info.Multiline = a.flow.kind == domain.KindString

	case stepType:
		info.Kind = PromptChoice
		info.Title = "type"
		info.Choices = typeChoices()

	case stepConfirm:
		info.Kind = PromptChoice
		info.Title = a.confirmTitle()
		info.Choices = confirmChoices()
	}

	return info
}

// confirmTitle is the question asked before a subtree is discarded, and says
// how much of the document is about to go.
//
// The count is of the whole subtree rather than of the immediate children,
// because that is the number a reader needs in order to judge what they are
// agreeing to: an object holding two objects of ten members loses twelve
// nodes, and a confirmation saying "2" would be worth less than none.
func (a *App) confirmTitle() string {
	n, ok := domain.Resolve(a.doc.Root(), a.flow.target)
	if !ok {
		// Not reached: a flow is abandoned as soon as its target goes away.
		return ""
	}

	count := domain.CountDescendants(n)

	nodes := " child nodes under "
	if count == 1 {
		nodes = " child node under "
	}

	return "Discard " + strconv.Itoa(count) + nodes + pointerText(a.flow.target) + "?"
}

// pointerText is a pointer as it reads inside a sentence, where the root is
// "/" rather than the empty string RFC 6901 spells it as.
//
// The status bar and the inspector hand their pointer over and leave this
// choice to whoever draws them. A question is written here, so the spelling is
// settled here too.
func pointerText(p domain.Path) string {
	if p.IsRoot() {
		return "/"
	}

	return p.String()
}

// typeTable is the six types a value can be changed to, in the order they are
// offered, each with the key that asks for it.
//
// The initials are used where they are free: null takes z because n is number,
// and array takes a because o is object. What is drawn and what is accepted
// both come from here, so a key shown on screen cannot turn out to do nothing.
var typeTable = []struct {
	key   rune
	label string
	kind  domain.Kind
}{
	{'s', "string", domain.KindString},
	{'n', "number", domain.KindNumber},
	{'b', "boolean", domain.KindBool},
	{'z', "null", domain.KindNull},
	{'o', "object", domain.KindObject},
	{'a', "array", domain.KindArray},
}

// typeChoices is the table as a prompt offers it.
func typeChoices() []Choice {
	choices := make([]Choice, 0, len(typeTable))
	for _, t := range typeTable {
		choices = append(choices, Choice{Key: t.key, Label: t.label})
	}

	return choices
}

// kindFor is the type a key asks for, and false for a key that asks for none.
func kindFor(key rune) (domain.Kind, bool) {
	for _, t := range typeTable {
		if t.key == key {
			return t.kind, true
		}
	}

	// No kind is the answer when no key matched; the caller is told which of
	// the two it got.
	var none domain.Kind

	return none, false
}

// The two answers a confirmation takes. Esc withdraws the question entirely,
// which is the same thing "no" does and is bound everywhere rather than here.
const (
	confirmYes = 'y'
	confirmNo  = 'n'
)

func confirmChoices() []Choice {
	return []Choice{
		{Key: confirmYes, Label: "Yes"},
		{Key: confirmNo, Label: "No"},
	}
}
