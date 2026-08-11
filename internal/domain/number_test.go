package domain_test

import (
	"errors"
	"testing"

	"github.com/ytakahashi/pino/internal/domain"
)

func TestParseNumberAcceptsTheJSONGrammar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "zero", text: "0"},
		{name: "negative zero", text: "-0"},
		{name: "an integer", text: "8080"},
		{name: "a negative integer", text: "-8080"},
		{name: "a fraction", text: "1.5"},
		{name: "a negative fraction with trailing zeros", text: "-0.0"},
		{name: "an exponent", text: "1e10"},
		{name: "an upper case exponent with a sign", text: "1E+10"},
		{name: "a fraction with a negative exponent", text: "1.5e-3"},
		{name: "a zero exponent", text: "0e0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n, err := domain.ParseNumber(tt.text)
			if err != nil {
				t.Fatalf("ParseNumber(%q): %v", tt.text, err)
			}

			// The literal is kept byte for byte, which is what lets a number
			// entered as 1.50 be saved as 1.50.
			if got := n.Raw(); got != tt.text {
				t.Errorf("Raw() = %q, want %q", got, tt.text)
			}
		})
	}
}

func TestParseNumberRefusesWhatIsNotOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		reason string
	}{
		{name: "nothing at all", text: "", reason: "a number cannot be empty"},
		{name: "a leading plus", text: "+1", reason: "a leading plus is not allowed"},
		{name: "a leading zero", text: "01", reason: "leading zeros are not allowed"},
		{name: "a negative leading zero", text: "-01", reason: "leading zeros are not allowed"},
		{name: "no integer part", text: ".5", reason: "not a JSON number"},
		{name: "no fraction digits", text: "5.", reason: "a digit must follow the decimal point"},
		{name: "no exponent digits", text: "1e", reason: "a digit must follow the exponent"},
		{name: "a signed exponent with no digits", text: "1e+", reason: "a digit must follow the exponent"},
		{name: "a lone minus sign", text: "-", reason: "a digit must follow the minus sign"},
		{name: "two minus signs", text: "--1", reason: "not a JSON number"},
		{name: "a hexadecimal literal", text: "0x10", reason: "not a JSON number"},
		{name: "infinity", text: "Infinity", reason: "not a JSON number"},
		{name: "not a number", text: "NaN", reason: "not a JSON number"},
		{name: "trailing space", text: "1 ", reason: "not a JSON number"},
		{name: "leading space", text: " 1", reason: "not a JSON number"},
		{name: "a trailing comma", text: "1,", reason: "not a JSON number"},
		{name: "a second decimal point", text: "1.2.3", reason: "not a JSON number"},
		// Full-width digits read as digits to a person and to no JSON parser.
		{name: "full-width digits", text: "１２３", reason: "not a JSON number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n, err := domain.ParseNumber(tt.text)
			if n != nil {
				t.Fatalf("ParseNumber(%q) = %q, want no number", tt.text, n.Raw())
			}

			var invalid *domain.InvalidNumberError
			if !errors.As(err, &invalid) {
				t.Fatalf("ParseNumber(%q) error = %v, want *InvalidNumberError", tt.text, err)
			}

			if invalid.Text != tt.text {
				t.Errorf("Text = %q, want %q", invalid.Text, tt.text)
			}

			if invalid.Reason != tt.reason {
				t.Errorf("Reason = %q, want %q", invalid.Reason, tt.reason)
			}
		})
	}
}

func TestInvalidNumberErrorNamesTheTextAndTheRule(t *testing.T) {
	t.Parallel()

	err := &domain.InvalidNumberError{Text: "80x0", Reason: "not a JSON number"}

	want := `invalid JSON number "80x0": not a JSON number`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
