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
		want          layout
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := layoutFor(tc.width, tc.height, tc.view); got != tc.want {
				t.Errorf("layoutFor(%d, %d, %v) = %+v, want %+v",
					tc.width, tc.height, tc.view, got, tc.want)
			}
		})
	}
}

// Whatever the size, the parts of the screen have to fit on it: a division
// that overlapped would draw the inspector over the document.
func TestLayoutForFitsTheTerminal(t *testing.T) {
	t.Parallel()

	views := []application.ViewMode{application.ViewJSON, application.ViewTree}

	for _, view := range views {
		for width := range 130 {
			for height := range 45 {
				l := layoutFor(width, height, view)

				if used := l.BodyWidth + l.InspectorWidth; used > width {
					t.Fatalf("layoutFor(%d, %d, %v) uses %d columns of %d",
						width, height, view, used, width)
				}

				if used := l.BodyHeight + l.InspectorHeight + statusBarRows; used > max(height, 1) {
					t.Fatalf("layoutFor(%d, %d, %v) uses %d rows of %d",
						width, height, view, used, height)
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
