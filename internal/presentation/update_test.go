package presentation

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ytakahashi/pino/internal/application"
)

// What Update makes of a message: the keys it answers, the ones it holds, the
// ones it lets past, and the size it reports back to the session.

func TestUpdateStoresWindowSize(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	if m.width != 80 || m.height != 24 {
		t.Errorf("size = %dx%d, want 80x24", m.width, m.height)
	}
}

func TestUpdateQuitsOnBoundKey(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("Update(q) returned no command, want a quit")
	}

	if msg := cmd(); !isQuit(msg) {
		t.Errorf("Update(q) command produced %T, want tea.QuitMsg", msg)
	}
}

// A key nothing is bound to leaves the session as it was. Resolving it to an
// Action the application then ignores would be the same outcome, but this way
// the application is never asked about a key press that means nothing.
func TestUpdateIgnoresUnboundKey(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Errorf("Update(x) = %v, want no command", cmd)
	}

	assertSameSession(t, next, m)
}

// Mouse reporting is presentation state: changing it redraws the terminal's
// input policy without asking the application to change the session.
func TestUpdateTogglesMouseReportingWithoutChangingTheSession(t *testing.T) {
	app := openApp(t, nestedDocument(t))
	m := sized(t, app, minWidth, minHeight)

	beforeStatus := app.Status()
	beforeFrame := app.Frame()
	beforePrompt := app.Prompt()

	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode = %v, want %v", got, tea.MouseModeCellMotion)
	}

	next, cmd := m.Update(key('m'))
	if cmd != nil {
		t.Errorf("Update(m) = %v, want no command", cmd)
	}

	toggled, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := toggled.View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("MouseMode after m = %v, want %v", got, tea.MouseModeNone)
	}

	if got := app.Status(); !reflect.DeepEqual(got, beforeStatus) {
		t.Errorf("Status() after m = %#v, want %#v", got, beforeStatus)
	}

	if got := app.Frame(); !reflect.DeepEqual(got, beforeFrame) {
		t.Errorf("Frame() after m = %#v, want %#v", got, beforeFrame)
	}

	if got := app.Prompt(); !reflect.DeepEqual(got, beforePrompt) {
		t.Errorf("Prompt() after m = %#v, want %#v", got, beforePrompt)
	}

	restored := press(t, toggled, key('m'))
	if got := restored.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("MouseMode after m twice = %v, want %v", got, tea.MouseModeCellMotion)
	}
}

// A prefix key produces nothing on its own and is remembered until the key
// that completes it arrives.
func TestUpdateRemembersAPrefix(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if cmd != nil {
		t.Errorf("Update(g) = %v, want no command", cmd)
	}

	after, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if after.pending != PendingG {
		t.Errorf("pending = %v after g, want %v", after.pending, PendingG)
	}

	// The second g completes it and is acted on.
	next, _ = after.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})

	done, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if done.pending != PendingNone {
		t.Errorf("pending = %v after gg, want nothing waiting", done.pending)
	}
}

// A mistyped sequence is cancelled as a sequence; its second key is not
// retried as a terminal request that changes input policy unexpectedly.
func TestUpdateDoesNotToggleMouseReportingDuringAPrefix(t *testing.T) {
	for _, prefix := range []tea.KeyPressMsg{key('g'), key('z')} {
		t.Run(prefix.String(), func(t *testing.T) {
			m := sized(t, openTestApp(t), 80, 24)
			after := press(t, m, prefix, key('m'))

			if got := after.View().MouseMode; got != tea.MouseModeCellMotion {
				t.Errorf("MouseMode after %sm = %v, want %v", prefix.String(), got, tea.MouseModeCellMotion)
			}

			if after.pending != PendingNone {
				t.Errorf("pending after %sm = %v, want nothing", prefix.String(), after.pending)
			}
		})
	}
}

// Help accepts only the keys that close it. A terminal key shown on that
// screen takes effect only after the reader returns to the document.
func TestUpdateDoesNotToggleMouseReportingInHelp(t *testing.T) {
	m := press(t, sized(t, openTestApp(t), 80, 24), key('?'))

	if m.app.Mode() != application.ModeHelp {
		t.Fatal("? did not open help")
	}

	after := press(t, m, key('m'))

	if got := after.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("MouseMode after m in help = %v, want %v", got, tea.MouseModeCellMotion)
	}

	if after.app.Mode() != application.ModeHelp {
		t.Error("m closed help")
	}
}

func TestUpdateIgnoresUnknownMessage(t *testing.T) {
	m := sized(t, openTestApp(t), 80, 24)

	next, cmd := m.Update(tea.KeyReleaseMsg{Code: 'q', Text: "q"})
	if cmd != nil {
		t.Errorf("Update() = %v, want no command", cmd)
	}

	assertSameSession(t, next, m)
}

// Resizing tells the application how much room the document has, which is
// what the window it asks for is worked out against.
func TestUpdateReportsTheBodyHeight(t *testing.T) {
	// Tall enough to reach well past what the shortest terminal could show,
	// without the window having had to move.
	m := sized(t, openApp(t, longDocument(t)), minWidth, 24)

	for range 15 {
		m = press(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	}

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the document still drawn from the top", got)
	}

	// Shrunk to less than the cursor's position, the window has to follow it.
	next, _ := m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})

	small, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if row := selectedRow(t, small); row < 0 || row >= small.layout().BodyHeight {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, small.layout().BodyHeight)
	}
}

// The wheel moves the window, and the selection stays on the screen.
func TestUpdateScrollsOnTheWheel(t *testing.T) {
	m := sized(t, openApp(t, longDocument(t)), minWidth, minHeight)

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Fatalf("row 0 = %q, want the top of the document", got)
	}

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	down, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := rows(t, down)[0]; strings.TrimRight(got, " ") == "{" {
		t.Error("the wheel did not move the window")
	}

	if row := selectedRow(t, down); row < 0 || row >= down.layout().BodyHeight {
		t.Errorf("the selected row is %d, outside the %d rows drawn", row, down.layout().BodyHeight)
	}

	// And back up again.
	next, _ = down.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})

	up, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := rows(t, up)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the top of the document again", got)
	}
}

// A wheel event may already be in flight when the frame disabling reporting
// reaches the terminal. Once reporting is off, that event must not move the
// document pino is no longer asking the terminal to report on.
func TestUpdateIgnoresTheWheelWhileMouseReportingIsOff(t *testing.T) {
	m := press(t, sized(t, openApp(t, longDocument(t)), minWidth, minHeight), key('m'))
	before := m.app.Frame()

	next, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("Update(wheel) = %v, want no command", cmd)
	}

	after, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := after.app.Frame(); !reflect.DeepEqual(got, before) {
		t.Errorf("Frame() after wheel with reporting off = %#v, want %#v", got, before)
	}

	if got := after.View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("MouseMode after wheel = %v, want %v", got, tea.MouseModeNone)
	}

	enabled := press(t, after, key('m'))
	next, _ = enabled.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	scrolled, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if got := scrolled.app.Frame().Scroll; got == before.Scroll {
		t.Error("the wheel did not move the window after reporting was restored")
	}
}

// Sideways there is nothing to scroll: a row too wide for the screen is cut,
// not moved.
func TestUpdateIgnoresTheHorizontalWheel(t *testing.T) {
	m := sized(t, openApp(t, longDocument(t)), minWidth, minHeight)

	next, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelRight})
	if cmd != nil {
		t.Errorf("Update() = %v, want no command", cmd)
	}

	assertSameSession(t, next, m)

	if got := rows(t, m)[0]; strings.TrimRight(got, " ") != "{" {
		t.Errorf("row 0 = %q, want the window left where it was", got)
	}
}

// The session is told how much room the document has whenever that changes,
// and a view that brings an inspector with it changes it without the terminal
// having been touched.
func TestUpdateReportsTheHeightOnAViewSwitch(t *testing.T) {
	// Narrow enough for the inspector to go underneath, which is where it
	// costs the document rows.
	const width, height = 80, 20

	m := sized(t, openApp(t, tallDocument(t)), width, height)

	if got, want := m.reported, height-statusBarRows; got != want {
		t.Fatalf("the session was told %d rows, want %d", got, want)
	}

	m = press(t, m, tabKey)

	l := m.layout()
	if l.Inspector != placeBelow {
		t.Fatalf("the inspector is placed %v on a %d column terminal, want below", l.Inspector, width)
	}

	if got, want := m.reported, height-statusBarRows-l.InspectorHeight; got != want {
		t.Errorf("the session was told %d rows after Tab, want %d", got, want)
	}

	// The document is drawn in the rows it was told about, and the cursor is
	// among them.
	if row := selectedRow(t, m); row < 0 || row >= m.reported {
		t.Errorf("the selected row is %d, outside the %d rows the document has", row, m.reported)
	}

	// And back, which restores what the JSON view had.
	m = press(t, m, tabKey)

	if got, want := m.reported, height-statusBarRows; got != want {
		t.Errorf("the session was told %d rows after switching back, want %d", got, want)
	}
}

// Nothing is reported when the number has not moved. Saying it again would
// settle the session a second time for no reason on every key press.
func TestUpdateDoesNotRepeatTheHeight(t *testing.T) {
	// Wide enough that the inspector stands beside the tree, where it costs
	// columns rather than rows.
	m := sized(t, openApp(t, tallDocument(t)), 120, 20)

	before := m.reported

	next, cmd := m.Update(tabKey)
	if cmd != nil {
		t.Errorf("Update(tab) = %v, want no command; the height did not change", cmd)
	}

	after, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}

	if after.reported != before {
		t.Errorf("the session was told %d rows, want the %d it already had", after.reported, before)
	}

	if after.layout().Inspector != placeSide {
		t.Errorf("the inspector is placed %v, want beside", after.layout().Inspector)
	}
}
