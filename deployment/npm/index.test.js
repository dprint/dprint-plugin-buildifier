// @ts-check
const assert = require("assert");
const createFromBuffer = require("@dprint/formatter").createFromBuffer;
const getBuffer = require("./index").getBuffer;

const formatter = createFromBuffer(getBuffer());

assert.deepStrictEqual(formatter.getFileMatchingInfo().fileExtensions, ["bzl", "bazel", "star", "sky"]);
assert.deepStrictEqual(formatter.getFileMatchingInfo().fileNames, ["BUILD", "WORKSPACE", "WORKSPACE.bzlmod"]);

// BUILD files get buildifier's BUILD rewrites
assert.strictEqual(
  formatter.formatText({
    filePath: "pkg/BUILD.bazel",
    fileText: "cc_library(deps = [\":b\", \":a\"], name = \"x\")\n",
  }),
  "cc_library(\n    name = \"x\",\n    deps = [\n        \":a\",\n        \":b\",\n    ],\n)\n",
);

// other Starlark files only get the rewrites that apply to them
assert.strictEqual(
  formatter.formatText({
    filePath: "defs.bzl",
    fileText: "load(\":b.bzl\", \"b\")\nload(\":a.bzl\", \"a\")\ndef f( x ):\n  return x\n",
  }),
  "load(\":a.bzl\", \"a\")\nload(\":b.bzl\", \"b\")\n\ndef f(x):\n    return x\n",
);

// uses the global newLineKind
formatter.setConfig({ newLineKind: "crlf" }, {});
assert.strictEqual(
  formatter.formatText({ filePath: "BUILD", fileText: "x=1\n" }),
  "x = 1\r\n",
);
formatter.setConfig({}, {});

assert.throws(
  () => formatter.formatText({ filePath: "BUILD", fileText: "cc_library(\n  name = ,\n)\n" }),
  /2:11: syntax error near ,/,
);
