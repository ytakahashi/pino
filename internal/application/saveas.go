package application

const errFileNameRequired = "a file name is required"

// saveAsFlow asks where a document with no destination should be written.
//
// The answer is carried only by the save attempt. It is not copied into
// StdinSource: a path that could not be written has not become the document's
// destination, and the next save should let the reader choose again.
type saveAsFlow struct {
	quitAfter bool
	err       string
}

func (*saveAsFlow) mode() Mode { return ModeConfirm }

func (f *saveAsFlow) prompt(*App) PromptInfo {
	return PromptInfo{Kind: PromptText, Title: "Save as", Error: f.err}
}

func (*saveAsFlow) choose(*App, rune) []Effect { return nil }

func (f *saveAsFlow) validate(_ *App, text string) {
	if text == "" {
		f.err = errFileNameRequired

		return
	}

	f.err = ""
}

func (f *saveAsFlow) submit(a *App, text string) []Effect {
	if text == "" {
		f.err = errFileNameRequired

		return nil
	}

	return a.saveTo(stdinSaveTarget(text), f.quitAfter, false)
}
