package assert

import _ "embed"

//go:embed header-assert-macros.lisp
var headerAssertMacros string

func HeaderAssertMacros() string { return headerAssertMacros }
