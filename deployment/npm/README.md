# @dprint/buildifier

npm distribution of [dprint-plugin-buildifier](https://github.com/dprint/dprint-plugin-buildifier), which formats Bazel and Starlark files like [buildifier](https://github.com/bazel-contrib/buildtools/tree/main/buildifier) does.

Use this with [@dprint/formatter](https://github.com/dprint/js-formatter) or just use @dprint/formatter and download the [dprint-plugin-buildifier Wasm file](https://github.com/dprint/dprint-plugin-buildifier/releases).

## Example

```ts
import { getBuffer } from "@dprint/buildifier";
import { createFromBuffer } from "@dprint/formatter";

const formatter = createFromBuffer(getBuffer());

console.log(formatter.formatText({
  filePath: "BUILD.bazel",
  fileText: "cc_library(deps = [\":b\", \":a\"], name = \"x\")\n",
}));
```
