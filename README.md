# dprint-plugin-buildifier

[![npm version](https://img.shields.io/npm/v/@dprint/buildifier.svg)](https://www.npmjs.com/package/@dprint/buildifier) [![CI](https://github.com/dprint/dprint-plugin-buildifier/workflows/CI/badge.svg)](https://github.com/dprint/dprint-plugin-buildifier/actions?query=workflow%3ACI)

Wrapper for [buildifier](https://github.com/bazel-contrib/buildtools/tree/main/buildifier) for use as a formatting plugin in [dprint](https://github.com/dprint/dprint). It formats Bazel and Starlark files (`BUILD`, `MODULE.bazel`, `WORKSPACE`, `.bzl`, ...) exactly like the buildifier CLI does, since it's buildifier's own Go code compiled to Wasm with [TinyGo](https://tinygo.org).

## Install

[Install](https://dprint.dev/install/) and [setup](https://dprint.dev/setup/) dprint.

Then in your project's directory with a dprint.json file, run:

```shellsession
dprint add buildifier
```

Note: You do not need buildifier installed globally as dprint will run buildifier from the .wasm file in a sandboxed environment.

## Files

The plugin formats the same files buildifier finds when walking a directory:

| Files                                                                                                                                         | Formatted as                                                                                                       |
| --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `BUILD`, `BUILD.bazel`, `WORKSPACE`, `WORKSPACE.bazel`, `WORKSPACE.bzlmod`, `MODULE.bazel`, `*.MODULE.bazel`, `REPO.bazel` and `VENDOR.bazel` | Bazel files, which get buildifier's rewrites for them, like sorting `deps` and putting `name` first in a rule call |
| `.bzl`, `.star` and `.sky` files                                                                                                              | Starlark files, which only get the rewrites that apply to all Starlark, like sorting `load` statements             |

buildifier picks how to format a file from its name, so files matched through dprint's [`associations`](https://dprint.dev/config/#associations) (ex. a `Tiltfile`) are formatted as plain Starlark.

buildifier also formats `.BUILD` files (ex. `third_party/zlib.BUILD`), but the plugin doesn't match them by default, since dprint matches extensions case insensitively and `.build` would also claim files like `meson.build`. They can be added with the plugin's [`associations`](https://dprint.dev/config/#associations).

## Configuration

buildifier has no style options, so there's nothing to configure for the formatting itself. Like other dprint plugins, it uses dprint's `newLineKind`:

```jsonc
{
  "buildifier": {
    "newLineKind": "auto"
  }
}
```

| Option        | Default | Description                                                                                                                                  |
| ------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `newLineKind` | `lf`    | `lf`, `crlf`, or `auto` to keep the newline kind found at the end of the file's last line. Uses the global `newLineKind` when not specified. |

buildifier itself always outputs line feeds, and reads a carriage return line feed as two newlines, so the plugin turns them into line feeds before formatting.

The plugin formats like buildifier with its default settings. It doesn't read `.buildifier.json` files, since it can't read files, so buildifier's custom `tables` aren't supported, and it doesn't lint (`--lint=fix`).

For an overview of the config, see https://dprint.dev/plugins/buildifier/config/

## JS Formatting API

- [JS Formatter](https://github.com/dprint/js-formatter) - Browser/Deno and Node
- [npm package](https://www.npmjs.com/package/@dprint/buildifier)

## Versioning

This repo automatically upgrades to the latest version of buildifier once a day. You can check which version of buildifier is being used by looking at the `github.com/bazel-contrib/buildtools/v10` entry in the go.mod file in this repo:

https://github.com/dprint/dprint-plugin-buildifier/blob/main/go.mod

At the moment, the version of this plugin does not reflect the version of buildifier.

## Development

The plugin is built with [TinyGo](https://tinygo.org), which produces a much smaller module than the Go compiler, and one that has no imports, so it doesn't rely on the dprint CLI providing WASI.

```shellsession
go test ./...
deno run -A ./scripts/build.ts # outputs target/plugin.wasm
deno run -A ./scripts/test_wasm.ts # tests target/plugin.wasm
```

The build uses a local `tinygo` (which needs binaryen's `wasm-opt`) when there is one, and otherwise the TinyGo Docker image.

The tests format the inputs of buildtools' own printer tests and compare them with its golden files, read from the buildtools module in the Go module cache. `go test` does this with the Go code, and `scripts/test_wasm.ts` does it with the built Wasm file.

## Credits

- [buildifier](https://github.com/bazel-contrib/buildtools) by Google, under the Apache License 2.0.
- The plugin interface is adapted from [dprint-plugin-shfmt](https://github.com/dprint/dprint-plugin-shfmt), which adapted it from Jake Bailey's [dprint-plugin-gofumpt](https://github.com/jakebailey/dprint-plugin-gofumpt).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details. See [third_party](third_party) for the licenses of the code it includes.
