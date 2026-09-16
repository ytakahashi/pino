package application

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ytakahashi/pino/internal/domain"
)

var (
	// errEncodingUnreadable reports a document that could not be parsed back
	// after being written out.
	errEncodingUnreadable = errors.New("the document could not be read back after writing it out")

	// errEncodingChanged reports a document that parsed back as something
	// else.
	errEncodingChanged = errors.New("writing the document out would change it")

	// errStoreStatus reports a check answering with a state the port does not
	// define.
	errStoreStatus = errors.New("the file store did not say what became of the file")

	// errStoreContract reports a file store answering in a way the port does
	// not allow.
	errStoreContract = errors.New("the file store did not say whether the document was written")
)

// save writes the document to its destination, asking for one when needed.
//
// The order is: write the bytes out and read them back, then ask what became
// of the file, then replace it. Encoding is checked first because a defect
// there is pino's own, and finding it should cost neither a look at the file
// system nor a temporary file. The check on the file comes last, so that as
// little as possible happens between it and the rename.
//
// overwrite skips that check, once. It is what the reader chooses when told
// the file changed underneath them, and it is not remembered: the next save
// asks again.
//
// quitAfter says the save is on the way out. Leaving is then the last step of
// a save that went through, and every way of not going through — a document
// that could not be encoded, a file that changed, a write that failed —
// leaves pino running with the document still in it.
func (a *App) save(quitAfter, overwrite bool) []Effect {
	target, state := a.targetForSave()

	switch state {
	case saveNeedsDestination:
		a.flow = &saveAsFlow{quitAfter: quitAfter}

		return []Effect{EffectBeginInput{}}

	case saveSkipped:
		return nil

	case saveReady:
		return a.saveTo(target, quitAfter, overwrite)
	}

	return nil
}

// saveTo writes to a destination whose safety policy has already been chosen.
//
// Separating the target from Source matters for standard input: it must be
// written even while clean, but an occupied destination cannot be reloaded as
// the document it came from. A new FileSource shares the first property and
// not the second.
func (a *App) saveTo(target saveTarget, quitAfter, overwrite bool) []Effect {
	encoded, err := a.validateEncoding()
	if err != nil {
		a.noticeSaveEncoding(target, err)

		return nil
	}

	if !overwrite {
		status, err := a.deps.Files.HasChangedSince(target.path, target.expected)
		if err != nil {
			a.noticeSaveCheck(target, err)

			return nil
		}

		switch status {
		case ChangeNone:
			// The file is what it was when it was read, so writing it back
			// replaces this session's own work and nobody else's.

		case ChangeModified:
			// The question is carried on: answering it with Overwrite is
			// still a save on the way out, and leaving is what it ends with.
			if target.reloadable {
				a.flow = &conflictFlow{status: status, quitAfter: quitAfter}
			} else {
				a.flow = &overwriteFlow{target: target, quitAfter: quitAfter}
			}

			return nil

		case ChangeDeleted:
			if target.reloadable {
				a.flow = &conflictFlow{status: status, quitAfter: quitAfter}

				return nil
			}

			// A nil Meta describes a path expected to be free. The store does
			// not define a deletion from that state, so do not turn it into an
			// overwrite that bypasses a result nobody understood.
			a.noticeSaveSafely(target, errStoreStatus)

			return nil

		default:
			// A state the port does not define. Putting it to the reader as a
			// change would offer them an Overwrite that skips the very check
			// whose answer could not be read — which is how a file nobody
			// looked at gets written over.
			a.noticeSaveSafely(target, errStoreStatus)

			return nil
		}
	}

	out, err := a.deps.Files.Write(target.path, encoded)

	return a.applyWrite(target, out, err, quitAfter)
}

// saveTarget is where a save writes and what safety rule applies there.
//
// writeClean and reloadable are deliberately separate. Both a new file and a
// stdin document need writing while untouched, but only the former names a
// file that can replace the document when it appears before the first save.
type saveTarget struct {
	path       string
	expected   Meta
	writeClean bool
	reloadable bool
}

// displayName is the part of the destination that fits in a prompt or notice.
func (t saveTarget) displayName() string { return filepath.Base(t.path) }

// saveTargetState is what resolving the current document for a save found.
type saveTargetState uint8

const (
	saveSkipped saveTargetState = iota
	saveReady
	saveNeedsDestination
)

// targetForSave resolves where the current document would be written.
//
// A document that is not dirty and whose file exists is not saved. There is
// nothing to put there — the file already holds this very tree — and writing
// anyway would lay the document out again, turning a file somebody else
// formatted into a diff nobody asked for. A new document is written whether
// or not it was edited, since the file it would create is not there yet. A
// stdin document without a destination is reported separately so its caller
// can ask for one rather than treating it as a save that has nothing to do.
func (a *App) targetForSave() (saveTarget, saveTargetState) {
	if a.doc == nil {
		return saveTarget{}, saveSkipped
	}

	var target saveTarget

	switch src := a.source.(type) {
	case FileSource:
		target = saveTarget{
			path:       src.Path,
			expected:   a.meta,
			writeClean: src.New,
			reloadable: true,
		}

	case StdinSource:
		if src.Out == "" {
			return saveTarget{}, saveNeedsDestination
		}

		target = stdinSaveTarget(src.Out)

	default:
		return saveTarget{}, saveSkipped
	}

	if !a.doc.IsDirty() && !target.writeClean {
		return saveTarget{}, saveSkipped
	}

	return target, saveReady
}

// stdinSaveTarget describes a path that has never held this document.
func stdinSaveTarget(path string) saveTarget {
	return saveTarget{
		path:       path,
		writeClean: true,
	}
}

// validateEncoding is the document as bytes, once those bytes have been shown
// to say what the document says.
//
// The bytes are parsed again and the two trees compared. What that catches is
// a defect in the encoder — a lost member, a number spelled differently, an
// escape written wrongly — while the file on disk is still untouched. It is
// not a check on the file system, which is the store's own business and
// happens later against the bytes handed to it.
//
// The comparison is of trees rather than of bytes, because the two are not
// the same question: a document read from a file that spelled a character
// another admissible way comes back spelled pino's way, and is the same
// document.
func (a *App) validateEncoding() ([]byte, error) {
	encoded := domain.Encode(a.doc.Root(), a.format)

	parsed, err := a.deps.Parser.Parse(encoded, domain.JSONC)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errEncodingUnreadable, err)
	}

	if !domain.Equal(a.doc.Root(), parsed) {
		return nil, errEncodingChanged
	}

	return encoded, nil
}

// applyWrite moves the session to where a write left it.
//
// The rename inside the store is what divides the outcomes, and this is the
// only place that reads which side of it a write ended on. Saving and saving
// on the way out both come through here, so that they cannot come to disagree
// about what a half-finished write meant.
func (a *App) applyWrite(target saveTarget, out WriteOutcome, err error, quitAfter bool) []Effect {
	switch {
	// Nothing was replaced. The document is exactly as dirty as it was, the
	// file is exactly as it was, and the Meta still describes it.
	case !out.Committed && err != nil:
		a.noticeSave(target, err)

	case out.Committed && out.Meta != nil:
		a.doc.MarkSaved()
		a.meta = out.Meta
		a.source = FileSource{Path: target.path}
		a.flow = nil

		// The document is saved and something after the rename still failed —
		// the directory was not told, so the name may not survive a crash.
		// The document is not dirty for it: what is at the path is the new
		// text, and offering to write it again would have pino find its own
		// bytes there and report them as somebody else's change.
		//
		// pino stays open even when the save was on the way out. Leaving now
		// would take the only report of it off the screen with it, and there
		// is nothing left to lose by staying: the document is saved, so the
		// next attempt to leave goes straight out.
		if err != nil {
			a.noticeDurability(target, err)

			return nil
		}

		if quitAfter {
			return []Effect{EffectQuit{}}
		}

	default:
		// A commit without a Meta, or a failure that reports neither, is a
		// combination the port does not allow. Guessing which half to believe
		// is how a document comes to be marked saved when it was not.
		a.noticeSaveSafely(target, errStoreContract)
	}

	return nil
}

// notice puts an operation result on screen until the reader acknowledges it.
// Errors are not returned to the terminal: stopping because a file could not
// be written is how the edits in it are lost.
func (a *App) notice(summary string, severity NoticeSeverity, err error) {
	a.flow = &noticeFlow{notice: NoticeInfo{
		Summary:  summary,
		Detail:   err.Error(),
		Severity: severity,
	}}
}

func (a *App) noticeSaveEncoding(target saveTarget, err error) {
	a.notice("Could not safely encode "+target.displayName()+".", NoticeError, err)
}

func (a *App) noticeSaveCheck(target saveTarget, err error) {
	a.notice("Could not check "+target.displayName()+" for outside changes.", NoticeError, err)
}

func (a *App) noticeSave(target saveTarget, err error) {
	a.notice("Could not save "+target.displayName()+".", NoticeError, err)
}

func (a *App) noticeSaveSafely(target saveTarget, err error) {
	a.notice("Could not save "+target.displayName()+" safely.", NoticeError, err)
}

func (a *App) noticeDurability(target saveTarget, err error) {
	a.notice("Saved "+target.displayName()+", but durability could not be confirmed.", NoticeWarning, err)
}

// overwriteFlow is where saveTo stops when the destination is already taken
// and the document did not come from it. Reload is not offered for that same
// reason: there is nothing at that path this document could return to.
type overwriteFlow struct {
	target    saveTarget
	quitAfter bool
}

func (*overwriteFlow) mode() Mode { return ModeConfirm }

func (f *overwriteFlow) prompt(*App) PromptInfo {
	return PromptInfo{
		Kind:  PromptChoice,
		Title: f.target.displayName() + " already exists.",
		Choices: []Choice{
			{Key: 'o', Label: "Overwrite"},
			{Key: 'c', Label: "Cancel"},
		},
	}
}

func (f *overwriteFlow) choose(a *App, key rune) []Effect {
	switch key {
	case 'o':
		return a.saveTo(f.target, f.quitAfter, true)

	case 'c':
		a.flow = nil
	}

	return nil
}
