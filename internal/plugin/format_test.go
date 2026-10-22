package plugin

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFormatUpstreamGoldens formats the inputs of buildtools' own printer tests
// (`build/testdata/*.in` in the buildtools version in go.mod) and compares them
// with its golden files, through a file name of each kind of file the golden
// is for, like `BUILD` for a `.build.golden`.
func TestFormatUpstreamGoldens(t *testing.T) {
	cases := upstreamGoldenCases(t)
	if len(cases) < 50 {
		t.Fatalf("only found %d golden cases", len(cases))
	}
	for _, c := range cases {
		t.Run(filepath.Base(c.goldenPath)+"/"+c.fileName, func(t *testing.T) {
			input := readFile(t, c.inputPath)
			expected := readFile(t, c.goldenPath)
			output, err := FormatText("/repo/pkg/"+c.fileName, input, defaultConfig())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if output == nil {
				output = input
			}
			if !bytes.Equal(output, expected) {
				t.Fatalf("got:\n%s\nwant:\n%s", output, expected)
			}

			// formatting again shouldn't change anything
			again, err := FormatText("/repo/pkg/"+c.fileName, output, defaultConfig())
			if err != nil {
				t.Fatalf("unexpected error when formatting twice: %v", err)
			}
			if again != nil {
				t.Fatalf("formatting twice changed the text:\n%s", again)
			}
		})
	}
}

func TestFormatFileKinds(t *testing.T) {
	// BUILD files get buildifier's BUILD rewrites, like sorting the `deps`
	// and putting `name` first, while other Starlark files are left alone
	input := "cc_library(deps = [\":b\", \":a\"], name = \"x\")\n"
	build := "cc_library(\n    name = \"x\",\n    deps = [\n        \":a\",\n        \":b\",\n    ],\n)\n"
	other := input
	tests := map[string]string{
		"/repo/BUILD":                build,
		"/repo/BUILD.bazel":          build,
		"/repo/WORKSPACE":            build,
		"/repo/WORKSPACE.bazel":      build,
		"/repo/WORKSPACE.bzlmod":     build,
		"/repo/MODULE.bazel":         build,
		"/repo/deps.MODULE.bazel":    build,
		"/repo/REPO.bazel":           build,
		"/repo/VENDOR.bazel":         build,
		"/repo/defs.bzl":             other,
		"/repo/config.star":          other,
		"/repo/config.sky":           other,
		"/repo/third_party/x.BUILD":  build,
		"/repo/Tiltfile":             other,
		"C:/repo/BUILD":              build,
		"/repo/pkg/subpkg/BUILD.oss": build,
	}
	for path, expected := range tests {
		output, err := FormatText(path, []byte(input), defaultConfig())
		if err != nil {
			t.Errorf("%s: unexpected error: %v", path, err)
			continue
		}
		if output == nil {
			output = []byte(input)
		}
		if string(output) != expected {
			t.Errorf("%s: got:\n%s\nwant:\n%s", path, output, expected)
		}
	}
}

func TestFormatSkipsMiscasedFileNames(t *testing.T) {
	// dprint matches file names case insensitively, but a `build` script
	// isn't a BUILD file
	for _, path := range []string{"/repo/build", "/repo/Build", "/repo/workspace", "/repo/workspace.BZLMOD"} {
		output, err := FormatText(path, []byte("#!/bin/sh\nset -e\n"), defaultConfig())
		if err != nil || output != nil {
			t.Errorf("%s: got %q, %v", path, output, err)
		}
	}
}

func TestFormatUnchanged(t *testing.T) {
	output, err := FormatText("/repo/BUILD", []byte("cc_library(name = \"x\")\n"), defaultConfig())
	if err != nil || output != nil {
		t.Fatalf("got %q, %v", output, err)
	}
}

func TestFormatErrors(t *testing.T) {
	// the error has no file path, since dprint already says which file failed
	_, err := FormatText("/repo/BUILD", []byte("cc_library(\n  name = ,\n)\n"), defaultConfig())
	expected := "2:11: syntax error near ,"
	if err == nil || err.Error() != expected {
		t.Fatalf("got error:\n%v\nwant:\n%s", err, expected)
	}
}

func TestFormatNewLines(t *testing.T) {
	tests := []struct {
		name        string
		newLineKind string
		input       string
		expected    string
	}{
		// buildifier on its own would add a blank line after each crlf line
		{"crlf input to lf", "lf", "a = 1\r\nb  =  2\r\n", "a = 1\nb = 2\n"},
		{"crlf", "crlf", "a = 1\nb  =  2\n", "a = 1\r\nb = 2\r\n"},
		{"auto keeps crlf", "auto", "a = 1\r\nb  =  2\r\n", "a = 1\r\nb = 2\r\n"},
		{"auto keeps lf", "auto", "a = 1\nb  =  2\n", "a = 1\nb = 2\n"},
		// the newlines in strings are changed too
		{"multiline string", "lf", "x = \"\"\"a\r\nb\"\"\"\r\n", "x = \"\"\"a\nb\"\"\"\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Configuration{NewLineKind: test.newLineKind}
			output, err := FormatText("/repo/defs.bzl", []byte(test.input), config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(output) != test.expected {
				t.Fatalf("got %q, want %q", output, test.expected)
			}
			again, err := FormatText("/repo/defs.bzl", output, config)
			if err != nil || again != nil {
				t.Fatalf("formatting twice changed the text: %q, %v", again, err)
			}
		})
	}

	// a crlf file that's already formatted is unchanged
	output, err := FormatText("/repo/BUILD", []byte("cc_library(name = \"x\")\r\n"), Configuration{NewLineKind: "crlf"})
	if err != nil || output != nil {
		t.Fatalf("got %q, %v", output, err)
	}
}

func defaultConfig() Configuration {
	config, _ := resolveConfig(nil, nil, nil)
	return config
}

type goldenCase struct {
	inputPath  string
	goldenPath string
	fileName   string
}

// upstreamGoldenCases gets the golden cases in the testdata directory of the
// buildtools `build` package, the same way its `TestPrintRewrite` does.
func upstreamGoldenCases(t *testing.T) []goldenCase {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/bazel-contrib/buildtools/v10")
	moduleDir, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find the buildtools module: %v", err)
	}
	testdataDir := filepath.Join(strings.TrimSpace(string(moduleDir)), "build", "testdata")
	inputPaths, err := filepath.Glob(filepath.Join(testdataDir, "*.in"))
	if err != nil {
		t.Fatal(err)
	}

	// a `.golden` is the output for every kind of file
	fileNamesByGoldenSuffix := []struct {
		suffix    string
		fileNames []string
	}{
		{".golden", []string{"BUILD", "rules.bzl", "file.star"}},
		{".build.golden", []string{"BUILD"}},
		{".bzl.golden", []string{"rules.bzl"}},
		{".default.golden", []string{"file.star"}},
	}
	var cases []goldenCase
	for _, inputPath := range inputPaths {
		prefix := strings.TrimSuffix(inputPath, ".in")
		// tests the `ShortenAbsoluteLabelsToRelative` table, which is off by default
		if filepath.Base(prefix) == "050" {
			continue
		}
		for _, entry := range fileNamesByGoldenSuffix {
			goldenPath := prefix + entry.suffix
			if _, err := os.Stat(goldenPath); err != nil {
				continue
			}
			for _, fileName := range entry.fileNames {
				cases = append(cases, goldenCase{inputPath, goldenPath, fileName})
			}
		}
	}
	return cases
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
