/**
 * Checks that `target/plugin.wasm` formats like buildifier, by formatting the
 * inputs of buildtools' own printer tests with it and comparing them with the
 * golden files, the same as `TestFormatUpstreamGoldens` does for the Go code.
 * TinyGo can compile code differently than the Go compiler, so this tests what
 * actually ships.
 *
 * Run `deno run -A ./scripts/build.ts` first.
 */
import { createFromBuffer } from "@dprint/formatter";
import { $ } from "automation";

const rootDirPath = $.path(import.meta.dirname!).parentOrThrow();
const formatter = createFromBuffer(rootDirPath.join("target/plugin.wasm").readBytesSync());

const moduleDir = (await $`go list -m -f ${"{{.Dir}}"} github.com/bazel-contrib/buildtools/v10`.cwd(rootDirPath).text())
  .trim();
const testdataDir = $.path(moduleDir).join("build/testdata");

// a `.golden` is the output for every kind of file
const fileNamesByGoldenSuffix: [string, string[]][] = [
  [".golden", ["BUILD", "rules.bzl", "file.star"]],
  [".build.golden", ["BUILD"]],
  [".bzl.golden", ["rules.bzl"]],
  [".default.golden", ["file.star"]],
];

let passed = 0;
const failures: string[] = [];
for (const entry of testdataDir.readDirSync()) {
  if (!entry.name.endsWith(".in")) {
    continue;
  }
  const prefix = entry.name.slice(0, -".in".length);
  // tests the `ShortenAbsoluteLabelsToRelative` table, which is off by default
  if (prefix === "050") {
    continue;
  }
  const input = entry.path.readTextSync();
  for (const [suffix, fileNames] of fileNamesByGoldenSuffix) {
    const goldenPath = testdataDir.join(prefix + suffix);
    if (!goldenPath.existsSync()) {
      continue;
    }
    const expected = goldenPath.readTextSync();
    for (const fileName of fileNames) {
      const output = formatter.formatText({ filePath: `/repo/pkg/${fileName}`, fileText: input });
      if (output === expected) {
        passed++;
      } else {
        failures.push(`${prefix}${suffix} as ${fileName}`);
      }
    }
  }
}

if (failures.length > 0) {
  throw new Error(`The wasm output differs from buildifier's for:\n${failures.map((f) => `  ${f}`).join("\n")}`);
}
if (passed < 50) {
  throw new Error(`Only found ${passed} golden cases.`);
}
$.log(`The wasm output matches buildifier's for ${passed} golden cases.`);
