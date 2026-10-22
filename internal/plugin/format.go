package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/bazel-contrib/buildtools/v10/build"
)

// FileExtensions are the extensions of the Starlark files buildifier formats
// when walking a directory. `bazel` covers `BUILD.bazel`, `MODULE.bazel`,
// `WORKSPACE.bazel`, `REPO.bazel` and `VENDOR.bazel`. buildifier's `.BUILD`
// isn't included, since dprint matches extensions case insensitively and
// `build` would also claim `meson.build` files.
var FileExtensions = []string{"bzl", "bazel", "star", "sky"}

// FileNames are the extensionless file names buildifier formats when walking a
// directory.
var FileNames = []string{"BUILD", "WORKSPACE", "WORKSPACE.bzlmod"}

// FormatText formats the Starlark file the same way `buildifier` does, which
// picks the kind of file (BUILD, WORKSPACE, MODULE.bazel, .bzl, ...) from its
// name. It returns nil when the text is unchanged.
func FormatText(filePath string, src []byte, config Configuration) ([]byte, error) {
	if !isBazelFileName(path.Base(filePath)) {
		return nil, nil
	}
	// buildifier's lexer reads `\r\n` as two newlines, which would add a blank
	// line after every line of a file with CRLF line endings
	text := bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	file, err := build.Parse(filePath, text)
	if err != nil {
		return nil, parseError(err)
	}
	output := build.Format(file)
	if newLine := resolveNewLine(config.NewLineKind, src); newLine != "\n" {
		output = bytes.ReplaceAll(output, []byte("\n"), []byte(newLine))
	}
	if bytes.Equal(output, src) {
		return nil, nil
	}
	return output, nil
}

// resolveNewLine gets the newline to output for the `newLineKind`, the same
// way dprint-core does, which for `auto` is the last newline in the file.
func resolveNewLine(newLineKind string, src []byte) string {
	switch newLineKind {
	case "crlf":
		return "\r\n"
	case "auto":
		if index := bytes.LastIndexByte(src, '\n'); index > 0 && src[index-1] == '\r' {
			return "\r\n"
		}
	}
	return "\n"
}

// isBazelFileName gets whether the file isn't one that dprint only matched
// because it matches file names case insensitively. `BUILD` would otherwise
// also claim `build` scripts, but Bazel only reads `BUILD`.
func isBazelFileName(name string) bool {
	for _, fileName := range FileNames {
		if name != fileName && strings.EqualFold(name, fileName) {
			return false
		}
	}
	return true
}

// parseError drops the file path from buildifier's `path:line:col: message`
// error, since dprint already says which file failed.
func parseError(err error) error {
	var parseErr build.ParseError
	if errors.As(err, &parseErr) {
		return fmt.Errorf("%d:%d: %s", parseErr.Pos.Line, parseErr.Pos.LineRune, parseErr.Message)
	}
	return err
}
