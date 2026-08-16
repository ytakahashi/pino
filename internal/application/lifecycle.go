package application

// The flows here are about the document's life rather than its contents: what
// to do when the file has changed underneath the session, and what to do when
// something pino was asked to do could not be done. Neither gathers an answer
// the way an edit does — each asks one question, takes one key, and is gone.

// conflictFlow is the question asked when the file is no longer what it was
// when it was read.
//
// It holds what was found rather than what to do about it. The bytes to write
// are not kept: they are made again from the current root if the reader
// chooses to overwrite, so there is never a version of the document waiting
// to be written that is not the one on screen.
type conflictFlow struct {
	status ChangeStatus

	// quitAfter says the save that met this was on the way out, so answering
	// it with Overwrite still ends in leaving. Reload does not: choosing to
	// look at the other document is choosing to stay and look at it.
	quitAfter bool
}

func (*conflictFlow) mode() Mode { return ModeConfirm }

func (f *conflictFlow) prompt(*App) PromptInfo {
	title := "The file has changed outside pino."
	if f.status == ChangeDeleted {
		title = "The file was deleted outside pino."
	}

	return PromptInfo{
		Kind:  PromptChoice,
		Title: title,
		Choices: []Choice{
			{Key: 'r', Label: "Reload"},
			{Key: 'o', Label: "Overwrite"},
			{Key: 'c', Label: "Cancel"},
		},
	}
}

// choose takes the answer. Cancel leaves the document exactly as it is, which
// is the answer for a reader who wants to look at what happened before
// deciding.
func (f *conflictFlow) choose(a *App, key rune) []Effect {
	switch key {
	case 'r':
		a.reload()

	case 'o':
		return a.save(f.quitAfter, true)

	case 'c':
		a.flow = nil
	}

	return nil
}

// quitFlow is the question asked when leaving would throw work away.
//
// It holds nothing. What is being asked about is the document, which the
// session has, and the three answers are the same whatever is in it.
type quitFlow struct{}

func (*quitFlow) mode() Mode { return ModeConfirm }

func (*quitFlow) prompt(*App) PromptInfo {
	return PromptInfo{
		Kind:  PromptChoice,
		Title: "You have unsaved changes.",
		Choices: []Choice{
			{Key: 's', Label: "Save and quit"},
			{Key: 'd', Label: "Discard changes"},
			{Key: 'c', Label: "Cancel"},
		},
	}
}

// choose takes the answer.
//
// Saving does not lead to leaving on its own: what comes back from the save
// is what says whether the document reached the file, and only that ends the
// session. Discarding is the one answer that leaves with work unsaved, and it
// is a key the reader had to read and press.
func (f *quitFlow) choose(a *App, key rune) []Effect {
	switch key {
	case 's':
		// The question has been answered, so it goes now rather than when the
		// save gets somewhere: whatever the save has to say, it says in a
		// prompt of its own.
		a.flow = nil

		return a.save(true, false)

	case 'd':
		return []Effect{EffectQuit{}}

	case 'c':
		a.flow = nil
	}

	return nil
}

// quit is what leaving means with this document open.
//
// A document with nothing unsaved in it leaves at once, which includes one
// whose file has still to be created and which nobody has typed into: there
// is nothing to write, and writing it would create a file the reader never
// asked for.
//
// Anything else asks. Both keys that leave pino come through here — the key
// table binds Ctrl+C in every mode, so making only q ask would leave the
// other as a way out that takes the document with it.
func (a *App) quit() []Effect {
	if a.doc == nil || !a.doc.IsDirty() {
		return []Effect{EffectQuit{}}
	}

	// Whatever else was in progress goes. The question on screen is now the
	// one that was asked last, and an edit half answered underneath it would
	// be answered against a document the reader may be about to discard.
	a.flow = &quitFlow{}

	return nil
}

// noticeFlow is a runtime result waiting to be acknowledged.
//
// It is a flow rather than a field beside one because that is what it is: the
// session is holding a message and taking one key, which is a mode as much as
// a confirmation is. Being one of the flows also means it cannot sit behind
// an edit prompt, unseen, to be answered later.
type noticeFlow struct{ notice NoticeInfo }

func (*noticeFlow) mode() Mode { return ModeConfirm }

// info returns a copy so callers describing the session cannot alter the
// notice the flow is waiting to have acknowledged.
func (f *noticeFlow) info() *NoticeInfo {
	info := f.notice

	return &info
}

func (f *noticeFlow) prompt(*App) PromptInfo {
	return PromptInfo{
		Kind:    PromptChoice,
		Title:   f.notice.Summary,
		Choices: []Choice{{Key: 'o', Label: "OK"}},
		Notice:  f.info(),
	}
}

func (f *noticeFlow) choose(a *App, key rune) []Effect {
	if key == 'o' {
		a.flow = nil
	}

	return nil
}

// helpFlow is the list of what the keys do, on screen in place of the
// document.
//
// It is a flow, and therefore a mode, rather than something the drawing side
// keeps to itself. A screen that replaced the document while the session went
// on answering "normal" would be a state where what is drawn and what a key
// press means disagree, which is the thing one field for whatever is in
// progress exists to prevent.
//
// It holds nothing. What is listed is every key there is, which does not
// depend on the document, and the words are the terminal's own: a category
// heading laid out in 60 columns is not something this layer can check or
// should carry.
type helpFlow struct{}

func (*helpFlow) mode() Mode { return ModeHelp }

// prompt is nothing. Help asks no question, and saying so is what sends a key
// press through the key table rather than into a list of choices: this screen
// is read, and the keys pressed on it are ordinary keys with a mode of their
// own.
func (*helpFlow) prompt(*App) PromptInfo { return PromptInfo{} }

// choose takes no key, there being no choices drawn to press. Closing arrives
// as an Action of its own.
func (*helpFlow) choose(*App, rune) []Effect { return nil }

// showHelp puts the list of keys on screen.
//
// Only a session in the middle of nothing gets it. An edit or a question is
// gathering an answer, and replacing it with a screen that cannot take one
// would abandon the answer without the reader having withdrawn it — the same
// reason a flow is one field rather than a stack.
func (a *App) showHelp() {
	if a.flow != nil {
		return
	}

	a.flow = &helpFlow{}
}

// closeHelp gives the document back.
//
// It closes help and nothing else. The keys that ask for it mean other things
// in other modes, so a request arriving while something else is in progress is
// one that was meant for that other thing.
func (a *App) closeHelp() {
	if _, ok := a.flow.(*helpFlow); ok {
		a.flow = nil
	}
}

// reload reads the file again and shows what it now holds.
//
// Everything about the document goes: the tree, the versions of it, and where
// the reader was standing in it. Positions are not carried across, because
// there is no correspondence to carry them by — an element added to an array
// outside pino leaves /items/2 naming a different value, and pino's own edits
// only survive because each one says which paths it moved. What is kept is
// the session's own: which view is drawing, and how tall the terminal is.
//
// A file that cannot be read leaves everything alone. The document is read
// whole before any of it is installed, so a failure here is a message and
// nothing else — including a file that has been deleted, which is reported
// rather than turned into the empty document a missing path would open as.
func (a *App) reload() {
	src, ok := a.source.(FileSource)
	if !ok {
		return
	}

	read, err := a.read(src.Path, false)
	if err != nil {
		a.notice("Could not reload "+src.Name()+".", NoticeError, err)

		return
	}

	view := a.view.ViewMode

	a.install(read, src.Path)

	a.view = NewViewState()
	a.view.ViewMode = view

	a.settle(a.render())
}
