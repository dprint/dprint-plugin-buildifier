// Package thirdparty embeds the licenses of the third-party code the plugin
// includes, so the plugin can distribute them in its license text.
package thirdparty

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// the directory of each license is named after the code it's for
//
//go:embed */LICENSE*
var files embed.FS

// Notices gets the license texts of the third-party code, each
// headed by the name of its directory.
func Notices() string {
	// glob results are sorted, so the text is always in the same order
	paths, _ := fs.Glob(files, "*/*")
	var text strings.Builder
	for i, filePath := range paths {
		data, _ := files.ReadFile(filePath)
		if i != 0 {
			text.WriteString("\n\n---\n\n")
		}
		text.WriteString(path.Dir(filePath) + " (" + path.Base(filePath) + "):\n\n")
		text.WriteString(strings.TrimSpace(string(data)))
	}
	return text.String()
}
