package consent

import (
	"github.com/stretchr/testify/assert"
)

// assertBranded checks a rendered page carries the inline logo, favicon and
// brand tokens unescaped, i.e. that html/template treated them as trusted.
func assertBranded(a *assert.Assertions, body, title string) {
	a.Contains(body, "<title>"+title+"</title>")
	a.Contains(body, `<svg xmlns="http://www.w3.org/2000/svg"`)
	a.Contains(body, `aria-label="mykeebs"`)
	a.Contains(body, `href="data:image/svg`)
	a.Contains(body, "--accent: #e8a33d;")
}
