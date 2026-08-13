package presentation

import (
	"testing"

	"github.com/ytakahashi/pino/internal/application"
)

func TestLayoutFor(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		width, height int
		view          application.ViewMode

		// prompt is how many rows the band asking a question wants. Nothing is
		// being asked in most of these, which is the zero.
		prompt int

		want layout
	}{
		// The JSON view has the screen to itself but for the status bar.
		"the JSON view on a wide terminal": {
			width: 120, height: 40, view: application.ViewJSON,
			want: layout{BodyWidth: 120, BodyHeight: 39, Inspector: placeNone},
		},

		"the JSON view on a narrow terminal": {
			width: 60, height: 10, view: application.ViewJSON,
			want: layout{BodyWidth: 60, BodyHeight: 9, Inspector: placeNone},
		},

		// The tree view puts the inspector beside it once there is room, and
		// the rule between them costs a column of its own.
		"the tree view beside an inspector": {
			width: 120, height: 40, view: application.ViewTree,
			want: layout{
				BodyWidth: 87, BodyHeight: 39,
				Inspector: placeSide, InspectorWidth: 32,
			},
		},

		// Just wide enough to put it to the side: the tree keeps 67 columns.
		"the tree view at the wide boundary": {
			width: 100, height: 40, view: application.ViewTree,
			want: layout{
				BodyWidth: 67, BodyHeight: 39,
				Inspector: placeSide, InspectorWidth: 32,
			},
		},

		// One column short of it, and the inspector goes underneath instead.
		"the tree view one column below the wide boundary": {
			width: 99, height: 40, view: application.ViewTree,
			want: layout{
				BodyWidth: 99, BodyHeight: 34,
				Inspector: placeBelow, InspectorHeight: 5,
			},
		},

		"the tree view with the inspector below": {
			width: 60, height: 20, view: application.ViewTree,
			want: layout{
				BodyWidth: 60, BodyHeight: 14,
				Inspector: placeBelow, InspectorHeight: 5,
			},
		},

		// The smallest terminal pino draws in still leaves rows for the
		// document once the bar and the stacked inspector have taken theirs.
		"the tree view at the smallest size": {
			width: 60, height: 10, view: application.ViewTree,
			want: layout{
				BodyWidth: 60, BodyHeight: 4,
				Inspector: placeBelow, InspectorHeight: 5,
			},
		},

		// Below either minimum the size is reported as too small, and the
		// division is worked out all the same: what to do about it belongs to
		// whoever draws.
		"one column too narrow": {
			width: 59, height: 40, view: application.ViewJSON,
			want: layout{TooSmall: true, BodyWidth: 59, BodyHeight: 39, Inspector: placeNone},
		},

		"one row too short": {
			width: 120, height: 9, view: application.ViewJSON,
			want: layout{TooSmall: true, BodyWidth: 120, BodyHeight: 8, Inspector: placeNone},
		},

		// Wide enough and still too short, which is the case a check on the
		// width alone would let through.
		"wide but too short": {
			width: 120, height: 9, view: application.ViewTree,
			want: layout{
				TooSmall: true, BodyWidth: 87, BodyHeight: 8,
				Inspector: placeSide, InspectorWidth: 32,
			},
		},

		// Nothing is ever negative, and the inspector never claims rows the
		// terminal does not have: one row is the status bar's, leaving none.
		"a terminal with one row": {
			width: 40, height: 1, view: application.ViewTree,
			want: layout{
				TooSmall: true, BodyWidth: 40, BodyHeight: 0,
				Inspector: placeBelow, InspectorHeight: 0,
			},
		},

		// Four rows: the bar takes one and the inspector the other three,
		// which is all there is to give it.
		"a terminal too short for the whole pane": {
			width: 40, height: 4, view: application.ViewTree,
			want: layout{
				TooSmall: true, BodyWidth: 40, BodyHeight: 0,
				Inspector: placeBelow, InspectorHeight: 3,
			},
		},

		"a terminal of no size at all": {
			width: 0, height: 0, view: application.ViewJSON,
			want: layout{TooSmall: true, BodyWidth: 0, BodyHeight: 0, Inspector: placeNone},
		},

		// A question takes its rows from the document and from nothing else.
		"the JSON view with a prompt": {
			width: 120, height: 40, view: application.ViewJSON, prompt: 3,
			want: layout{BodyWidth: 120, BodyHeight: 36, Inspector: placeNone, PromptHeight: 3},
		},

		// The band comes out of the screen before the inspector: the answer is
		// being typed into it, while the pane describes a selection the answer
		// is about to change.
		"the tree view with a prompt and an inspector below": {
			width: 60, height: 20, view: application.ViewTree, prompt: 4,
			want: layout{
				BodyWidth: 60, BodyHeight: 10,
				Inspector: placeBelow, InspectorHeight: 5, PromptHeight: 4,
			},
		},

		"a prompt taller than the screen leaves the document nothing": {
			width: 60, height: 4, view: application.ViewJSON, prompt: 9,
			want: layout{
				TooSmall: true, BodyWidth: 60, BodyHeight: 0,
				Inspector: placeNone, PromptHeight: 3,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := layoutFor(tc.width, tc.height, tc.view, tc.prompt); got != tc.want {
				t.Errorf("layoutFor(%d, %d, %v, %d) = %+v, want %+v",
					tc.width, tc.height, tc.view, tc.prompt, got, tc.want)
			}
		})
	}
}

// Whatever the size, the parts of the screen have to fit on it: a division
// that overlapped would draw the inspector over the document.
func TestLayoutForFitsTheTerminal(t *testing.T) {
	t.Parallel()

	views := []application.ViewMode{application.ViewJSON, application.ViewTree}

	// No prompt, the tallest band an edit can put up, and one taller than any
	// of them: what is asked for is not what is granted on a short screen.
	prompts := []int{0, 4, 12}

	for _, view := range views {
		for _, prompt := range prompts {
			for width := range 130 {
				for height := range 45 {
					l := layoutFor(width, height, view, prompt)

					if used := l.BodyWidth + l.InspectorWidth; used > width {
						t.Fatalf("layoutFor(%d, %d, %v, %d) uses %d columns of %d",
							width, height, view, prompt, used, width)
					}

					used := l.BodyHeight + l.InspectorHeight + l.PromptHeight + statusBarRows
					if used > max(height, 1) {
						t.Fatalf("layoutFor(%d, %d, %v, %d) uses %d rows of %d",
							width, height, view, prompt, used, height)
					}
				}
			}
		}
	}
}

// The boundaries are the whole of what the division says, and a question being
// asked is not one of them: the same terminal falls on the same side of each
// with a band up as without one.
func TestLayoutForKeepsItsBoundariesWithAPrompt(t *testing.T) {
	t.Parallel()

	views := []application.ViewMode{application.ViewJSON, application.ViewTree}

	for _, view := range views {
		for _, width := range []int{59, 60, 99, 100} {
			for _, height := range []int{9, 10, 40} {
				bare := layoutFor(width, height, view, 0)
				asked := layoutFor(width, height, view, 3)

				if bare.TooSmall != asked.TooSmall {
					t.Errorf("layoutFor(%d, %d, %v) reports too small = %v with a prompt and %v without",
						width, height, view, asked.TooSmall, bare.TooSmall)
				}

				if bare.Inspector != asked.Inspector || bare.BodyWidth != asked.BodyWidth {
					t.Errorf("layoutFor(%d, %d, %v) divides the columns differently with a prompt: %+v, want %+v",
						width, height, view, asked, bare)
				}
			}
		}
	}
}

func TestPlacementString(t *testing.T) {
	t.Parallel()

	tests := map[placement]string{
		placeNone:     "none",
		placeSide:     "side",
		placeBelow:    "below",
		placement(99): "unknown",
	}

	for p, want := range tests {
		if got := p.String(); got != want {
			t.Errorf("placement(%d).String() = %q, want %q", p, got, want)
		}
	}
}
