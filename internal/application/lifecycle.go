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
type conflictFlow struct{ status ChangeStatus }

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
		a.save(true)

	case 'c':
		a.flow = nil
	}

	return nil
}

// errorFlow is something that could not be done, waiting to be acknowledged.
//
// It is a flow rather than a field beside one because that is what it is: the
// session is holding a message and taking one key, which is a mode as much as
// a confirmation is. Being one of the flows also means it cannot sit behind
// an edit prompt, unseen, to be answered later.
type errorFlow struct{ message string }

func (*errorFlow) mode() Mode { return ModeConfirm }

func (f *errorFlow) prompt(*App) PromptInfo {
	return PromptInfo{
		Kind:    PromptChoice,
		Title:   f.message,
		Choices: []Choice{{Key: 'o', Label: "OK"}},
	}
}

func (f *errorFlow) choose(a *App, key rune) []Effect {
	if key == 'o' {
		a.flow = nil
	}

	return nil
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
		a.fail(err)

		return
	}

	view := a.view.ViewMode

	a.install(read, src.Path)

	a.view = NewViewState()
	a.view.ViewMode = view

	a.settle(a.render())
}
