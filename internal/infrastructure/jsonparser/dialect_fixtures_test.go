package jsonparser

import (
	"github.com/ytakahashi/pino/internal/domain"
)

// jwcc accepts everything the underlying parser reads. pino does not offer it
// yet; it is here to show that the dialect is what refuses the extensions,
// rather than the parser being unable to read them.
var jwcc = domain.Dialect{AllowComments: true, AllowTrailingComma: true}
