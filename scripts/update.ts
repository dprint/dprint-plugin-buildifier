/**
 * This script checks for any buildtools (github.com/bazel-contrib/buildtools,
 * which buildifier is part of) updates and then automatically publishes a new
 * version of the plugin if so.
 *
 * A new major version of buildtools has a new module path (ex. `/v11`), so this
 * fails the workflow when one is out, since the import paths have to be moved
 * over by hand.
 */
import { $, semver } from "automation";
import { aiFixBuildtoolsUpdate } from "./ai_fix.ts";
import { publishRelease } from "./publish_release.ts";

const moduleMajor = 10;
const modulePath = `github.com/bazel-contrib/buildtools/v${moduleMajor}`;
const rootDirPath = $.path(import.meta.dirname!).parentOrThrow();
const goModPath = rootDirPath.join("go.mod");

await updateBuildtools();

$.logStep("Checking for a new major version of buildtools...");
const nextMajorVersion = await tryGetLatestVersion(`github.com/bazel-contrib/buildtools/v${moduleMajor + 1}`);
if (nextMajorVersion != null) {
  throw new Error(
    `buildtools ${nextMajorVersion} is out, which has the new module path github.com/bazel-contrib/buildtools/v${
      moduleMajor + 1
    }. Move the imports, go.mod and scripts/update.ts over to it by hand.`,
  );
}

async function updateBuildtools() {
  const currentVersion = getGoModVersion();
  $.logStep("Getting latest version...");
  const latestVersion = await tryGetLatestVersion(modulePath);
  if (latestVersion == null) {
    throw new Error(`Could not find the latest version of ${modulePath} on the Go module proxy.`);
  }
  if (semver.compare(semver.parse(currentVersion), semver.parse(latestVersion)) >= 0) {
    $.log("No new buildtools update found.");
    return;
  }

  $.log(`Found new version: ${currentVersion} -> ${latestVersion}`);
  const isPatchBump = semver.parse(currentVersion).minor === semver.parse(latestVersion).minor;
  $.logStep("Updating go.mod...");
  // this also raises the go directive when the new version needs a newer Go
  await $`go get ${modulePath}@v${latestVersion}`.cwd(rootDirPath);
  await $`go mod tidy`.cwd(rootDirPath);

  // Verify the update. The plugin has no style options to keep in sync with
  // buildifier, and the tests compare the output with the golden files of the
  // new buildtools version, so a bump that passes the checks publishes as is,
  // and one that fails them gets an AI fix attempt.
  $.logStep("Running checks (vet + test + wasm build + wasm test)...");
  const checks = await runChecks();

  if (!checks.passed) {
    $.logStep(`${isPatchBump ? "Patch" : "Minor"} update failed the checks — running AI fix...`);
    await aiFixBuildtoolsUpdate({
      isPatchBump,
      fromVersion: currentVersion,
      toVersion: latestVersion,
      // hand the failing output to the AI so it can go straight to fixing
      // instead of re-running the checks just to rediscover the errors.
      checkOutput: checks.output,
    });

    // the AI must leave the project in a passing state, otherwise fail the
    // workflow (nothing gets published and the maintainer is notified).
    $.logStep("Re-running checks after AI changes...");
    await assertChecks();
  }

  if (Deno.args.includes("--skip-publish")) {
    return;
  }

  $.logStep("Committing buildtools version bump commit...");
  await $`git add .`;
  const message = `${isPatchBump ? "fix" : "feat"}: update to buildifier ${latestVersion}`;
  await $`git commit -m ${message}`;

  await publishRelease(isPatchBump ? "patch" : "minor");
}

// the checks that must pass before publishing. The wasm build and its test are
// included because that is what actually ships, and TinyGo can fail on (or
// miscompile) code that the Go compiler accepts. `inheritPiped` +
// `captureCombined` streams the output to the CI log live while also capturing
// it so a failure can be handed to the AI.
async function runChecks(): Promise<{ passed: boolean; output: string }> {
  const results = [];
  for (const command of checkCommands()) {
    results.push(await capture(command()));
  }
  const failures = results.filter((r) => r.code !== 0);
  return {
    passed: failures.length === 0,
    output: failures.map((r) => r.combined).join("\n\n"),
  };
}

function capture(command: ReturnType<typeof $>) {
  return command
    .stdout("inheritPiped")
    .stderr("inheritPiped")
    .captureCombined()
    .noThrow();
}

// same checks as `runChecks`, but throws on the first failure so the workflow
// aborts before anything is committed, tagged, or published.
async function assertChecks(): Promise<void> {
  for (const command of checkCommands()) {
    await command();
  }
}

// a function (not a top-level const) so it is hoisted -- `runChecks` is called
// before a const declared down here would be initialized.
function checkCommands() {
  return [
    () => $`go vet ./...`.cwd(rootDirPath),
    () => $`go test ./...`.cwd(rootDirPath),
    () => $`deno run -A ./scripts/build.ts`.cwd(rootDirPath),
    () => $`deno run -A ./scripts/test_wasm.ts`.cwd(rootDirPath),
  ];
}

function getGoModVersion() {
  const match = goModPath.readTextSync().match(/^\s*github\.com\/bazel-contrib\/buildtools\/v\d+ v(\S+)$/m);
  const version = match?.[1];
  if (version == null) {
    throw new Error(`Could not find ${modulePath} version in go.mod.`);
  }
  $.logLight(`Found ${modulePath} version in go.mod:`, version);
  return version;
}

async function tryGetLatestVersion(path: string) {
  const response = await $.request(`https://proxy.golang.org/${path}/@latest`).noThrow();
  if (response.status === 404 || response.status === 410) {
    return undefined;
  }
  if (!response.ok) {
    throw new Error(`Failed to get the latest version of ${path}: ${response.status}`);
  }
  const version = (await response.json() as { Version?: string }).Version?.replace(/^v/, "");
  $.logLight(`Latest ${path} version:`, version);
  return version;
}
