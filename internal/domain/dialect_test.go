package domain

import "testing"

func TestJSONCAcceptsBothJWCCExtensions(t *testing.T) {
	t.Parallel()

	if !JSONC.AllowComments || !JSONC.AllowTrailingComma {
		t.Errorf("JSONC = %+v, want comments and trailing commas allowed", JSONC)
	}

	if StrictJSON.AllowComments || StrictJSON.AllowTrailingComma {
		t.Errorf("StrictJSON = %+v, want both extensions refused", StrictJSON)
	}
}
