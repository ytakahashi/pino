package domain

// Dialect selects which extensions the parser accepts.
//
// The zero value is standard JSON as defined by RFC 8259. The underlying
// parser accepts JWCC, so pino rejects the extensions explicitly rather than
// being unable to read them; supporting JSONC later means flipping these
// flags and carrying the resulting Trivia through, not replacing the parser.
type Dialect struct {
	AllowComments      bool
	AllowTrailingComma bool
}

// StrictJSON accepts RFC 8259 JSON only. It is the zero value of Dialect.
var StrictJSON = Dialect{}
