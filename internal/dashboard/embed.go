package dashboard

import (
	_ "embed"
	"strings"
)

//go:embed assets/index.html
var indexHTML string

//go:embed assets/style.css
var styleCSS string

//go:embed assets/app.js
var appJS string

func embeddedDocument() string {
	doc := strings.Replace(indexHTML, "__STYLE__", styleCSS, 1)
	doc = strings.Replace(doc, "__SCRIPT__", appJS, 1)
	return doc
}
