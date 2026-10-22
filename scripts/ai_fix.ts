/**
 * Fixes this plugin after a buildtools (github.com/bazel-contrib/buildtools)
 * update that fails the checks, using OpenAI Codex, in two stages that each run
 * Codex in a session of its own:
 *
 *   1. a Codex agentic session edits the source and runs the checks until they
 *      pass, then
 *   2. a SEPARATE reviewer session reviews the result with Codex, so it can
 *      investigate (read the upstream buildtools source, grep the repo). It is
 *      instructed to review only and not modify anything. If it finds blocking
 *      issues they are fed back to the stage-1 Codex for another pass, bounded
 *      by `REVIEW_MAX_ROUNDS`. If it still isn't approved, this throws so the
 *      workflow fails and nothing is published.
 *
 * Unlike the plugins with formatter options, a bump that passes the checks
 * doesn't need a review, since buildifier has no style options to keep in sync.
 *
 * Both stages use the default model of the Codex CLI, which is installed at
 * its latest version when it is missing, so they move to a new model when
 * Codex does. Set `CODEX_MODEL` or `REVIEW_MODEL` to pin the model of a stage.
 *
 * Codex must NOT commit or push -- `update.ts` captures the working tree
 * changes into the existing buildtools bump commit afterwards.
 */
import { $ } from "automation";

export interface AiFixOptions {
  /** true for a patch bump, false for a minor/major bump. */
  isPatchBump: boolean;
  /** buildtools version currently in go.mod (before the bump). */
  fromVersion: string;
  /** buildtools version being upgraded to. */
  toVersion: string;
  /** Combined output of the failing checks. */
  checkOutput: string;
}

/** Max number of times the reviewer may send changes back to Codex. */
const REVIEW_MAX_ROUNDS = 2;

export async function aiFixBuildtoolsUpdate(options: AiFixOptions): Promise<void> {
  const apiKey = requireApiKey();
  await ensureCodexInstalled();
  await codexLogin(apiKey);

  // stage 1: let Codex fix the update.
  await runCodex(buildFixPrompt(options));

  // stage 2: independent second-model review, with a bounded refix loop.
  for (let round = 1;; round++) {
    const review = await reviewChanges(options);
    logReview(review);
    if (review.approved) {
      return;
    }
    if (round > REVIEW_MAX_ROUNDS) {
      const blocking = review.issues.filter((i) => i.severity === "blocking");
      throw new Error(
        `AI reviewer did not approve after ${REVIEW_MAX_ROUNDS} refix round(s). Blocking issues:\n`
          + blocking.map((i) => `  - ${i.description}`).join("\n"),
      );
    }
    $.logStep(`Reviewer requested changes — Codex refix round ${round}...`);
    await runCodex(buildRefixPrompt(review));
  }
}

// stage 1: Codex ---------------------------------------------------------------

async function runCodex(prompt: string): Promise<void> {
  const args = ["exec", "--dangerously-bypass-approvals-and-sandbox", "--skip-git-repo-check"];
  const model = Deno.env.get("CODEX_MODEL");
  if (model) {
    args.push("--model", model);
  }
  args.push(prompt);

  $.logStep("Running Codex...");
  await $`codex ${args}`;
}

function buildFixPrompt(options: AiFixOptions): string {
  const { isPatchBump, fromVersion, toVersion, checkOutput } = options;

  const failureOutput = checkOutput.trim().length > 0
    ? [
      ``,
      `The checks currently fail with the output below. Use it as your starting point instead of re-running the checks just to rediscover the errors:`,
      "```",
      truncateHead(checkOutput.trim()),
      "```",
    ]
    : [];

  return [
    `You are updating "dprint-plugin-buildifier", a dprint plugin written in Go and compiled to Wasm with TinyGo. It formats Bazel and Starlark files exactly like the buildifier CLI does, using the \`build\` package of the \`github.com/bazel-contrib/buildtools/v10\` Go module (\`build.Parse\` and \`build.Format\`).`,
    ``,
    `buildtools was upgraded from ${fromVersion} to ${toVersion} (a ${
      isPatchBump ? "patch" : "minor"
    } bump) and the checks fail (the project no longer builds, \`go vet\` or \`go test\` fails, the TinyGo Wasm build fails, or the Wasm output differs from buildtools' golden files). This is usually because the API of the \`build\` package changed, because buildtools now uses something TinyGo doesn't support or needs a Go version that TinyGo doesn't support yet, or because TinyGo compiles some code differently than the Go compiler.`,
    ...failureOutput,
    ``,
    `Your goal: make the checks pass while keeping the output byte for byte the same as the buildifier CLI's (\`buildifier/buildifier.go\` in buildtools, which parses with \`build.Parse\` and formats with \`build.Format\`, with linting off). Do NOT commit or push; only edit files in the working tree.`,
    ``,
    describeWiring(),
    ``,
    `To see exactly what changed in buildtools, inspect the actual dependency source that Go already downloaded, e.g.:`,
    `  find "$(go env GOMODCACHE)/github.com/bazel-contrib/buildtools" -maxdepth 1 -type d -name 'v10@v*'`,
    `then diff its \`build\` and \`buildifier\` directories against the directory of the previous version if it is there too.`,
    ``,
    `Rules:`,
    `1. The expected outputs of the tests are buildtools' own golden files (\`build/testdata\` in the module), read straight from the module. Do NOT skip or filter out golden cases to make a test pass. If the plugin can't match one, explain why in your final message rather than hiding it.`,
    `2. If TinyGo can't compile something, work around it in this repo (e.g. a different build option in \`scripts/build.ts\`) rather than forking buildtools, and explain the workaround in your final message. Keep \`-panic-unwind=explicit\` (buildifier's parser reports syntax errors with panic and recover) and \`-gc=precise\` (the default GC never frees memory).`,
    `3. If a new module ends up in the Wasm file, add its license to \`third_party\`.`,
    `4. Do NOT edit \`README.md\`.`,
    `5. Preserve the existing code style and run \`gofmt -w .\`. New comments start lowercase unless multiple sentences.`,
    `6. When done, ALL of these must pass (the TinyGo Wasm build is what actually ships, and TinyGo can reject or miscompile code the Go compiler accepts) — iterate until they are all clean:`,
    `     go vet ./...`,
    `     go test ./...`,
    `     deno run -A ./scripts/build.ts`,
    `     deno run -A ./scripts/test_wasm.ts`,
    `7. Do not change the plugin's own version in the VERSION file, do not run git commit, and do not push.`,
  ].join("\n");
}

function buildRefixPrompt(review: ReviewResult): string {
  return [
    `An independent reviewer examined your changes to dprint-plugin-buildifier and found issues that must be fixed. Address every blocking issue below, keeping \`go test ./...\` green. Do not commit or push.`,
    ``,
    `Reviewer summary: ${review.summary}`,
    ``,
    `Issues:`,
    ...review.issues.map((i) => `  - [${i.severity}] ${i.description}`),
    ``,
    describeWiring(),
  ].join("\n");
}

function describeWiring(): string {
  return [
    `How the plugin is wired:`,
    `- \`internal/plugin/format.go\` -> \`FormatText\` normalizes CRLF to LF, parses with \`build.Parse\` (which picks the kind of file from its name), formats with \`build.Format\`, and applies dprint's \`newLineKind\`.`,
    `- \`internal/plugin/config.go\` -> the plugin's configuration, which only has \`newLineKind\`, since buildifier has no style options.`,
    `- \`internal/plugin/plugin.go\` and \`main.go\` -> the dprint Wasm plugin interface.`,
    `- \`internal/plugin/format_test.go\` -> \`TestFormatUpstreamGoldens\` runs buildtools' golden files (\`build/testdata\` in the module) through \`FormatText\`, and \`scripts/test_wasm.ts\` runs them through the built Wasm file.`,
    `- \`scripts/build.ts\` -> the TinyGo build and its options.`,
    `- \`third_party\` -> the licenses of the third-party code the plugin includes, which have to cover every module that ends up in the Wasm file.`,
  ].join("\n");
}

// keep the tail of long check output -- the errors and the "could not compile"
// summary are at the end, which is the most useful part for the AI.
function truncateHead(text: string, max = 20_000): string {
  return text.length > max ? `... (truncated)\n${text.slice(text.length - max)}` : text;
}

// stage 2: independent reviewer ------------------------------------------------

interface ReviewResult {
  approved: boolean;
  summary: string;
  issues: ReviewIssue[];
}

interface ReviewIssue {
  severity: "blocking" | "nit";
  description: string;
}

// The reviewer runs Codex too so it can actually investigate (read the
// buildtools source Go downloaded, grep the repo, verify against the real upstream
// API). Its verdict is captured as JSON via --output-last-message.
//
// NOTE: we deliberately do NOT use `--sandbox read-only` here. That makes Codex
// wrap commands in bubblewrap, which cannot set up its network namespace on the
// GitHub Actions runner ("bwrap: loopback: Failed RTM_NEWADDR: Operation not
// permitted"), so every command the reviewer runs fails before executing. We
// use the same no-sandbox mode as the fixer and enforce read-only via the
// prompt instead. (CI is itself a throwaway VM, and the final check gate in
// update.ts re-verifies whatever ends up in the working tree.)
async function reviewChanges(options: AiFixOptions): Promise<ReviewResult> {
  // the reviewer starts without the context of the fixer, which is what makes
  // it a second opinion. It uses the default model of Codex unless one is set.
  const model = Deno.env.get("REVIEW_MODEL");

  // record intent-to-add so any files Codex created also show in `git diff`.
  await $`git add -N .`.quiet();

  const outputFile = await Deno.makeTempFile({ prefix: "codex-review-", suffix: ".json" });
  try {
    $.logStep(`Reviewing changes with Codex (${model ?? "default model"})...`);
    const args = ["exec", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox"];
    if (model) {
      args.push("--model", model);
    }
    args.push("--output-last-message", outputFile, buildReviewPrompt(options));
    await $`codex ${args}`;
    return parseReview(await Deno.readTextFile(outputFile));
  } finally {
    await Deno.remove(outputFile).catch(() => {});
  }
}

function buildReviewPrompt(options: AiFixOptions): string {
  return [
    `You are an independent reviewer for dprint-plugin-buildifier, a dprint plugin written in Go (compiled to Wasm with TinyGo) that formats Bazel and Starlark files exactly like the buildifier CLI, using the \`build\` package of the github.com/bazel-contrib/buildtools/v10 Go module. buildtools was just upgraded from ${options.fromVersion} to ${options.toVersion}, the checks failed, and another AI edited this plugin to fix them. Review the UNCOMMITTED working-tree changes.`,
    `IMPORTANT: this is REVIEW ONLY. Read any files and run read-only commands (git diff, cat, grep, find, etc.) to investigate, but you MUST NOT edit, create, or delete any files, and MUST NOT run git commit or git push. Leave the working tree exactly as you found it.`,
    ``,
    `Investigate as needed:`,
    `- Run \`git --no-pager diff\` (and \`git status\`) to see exactly what changed, including any new files.`,
    `- Verify against the REAL buildtools ${options.toVersion} source Go downloaded:`,
    `    find "$(go env GOMODCACHE)/github.com/bazel-contrib/buildtools" -maxdepth 1 -type d -name 'v10@v*'`,
    ``,
    describeWiring(),
    ``,
    `Verify specifically:`,
    `- No golden case of buildtools was skipped or filtered out in \`internal/plugin/format_test.go\` or \`scripts/test_wasm.ts\` to make the checks pass. That's a blocking issue.`,
    `- \`FormatText\` still formats the same way the buildifier CLI (\`buildifier/buildifier.go\`) does with linting off.`,
    `- \`scripts/build.ts\` still builds with \`-panic-unwind=explicit\` and \`-gc=precise\`.`,
    `- If a new module ends up in the Wasm file, its license was added to \`third_party\`.`,
    `- README.md was NOT modified; flag any README.md change as a blocking issue.`,
    `- No obvious correctness bugs, and code style matches the surrounding code.`,
    ``,
    `When done, your FINAL message must be ONLY a JSON object (no markdown code fences, no extra prose) of exactly this shape:`,
    `{"approved": true|false, "summary": "one sentence", "issues": [{"severity": "blocking"|"nit", "description": "..."}]}`,
    `Set approved=false if there is any blocking issue; nits alone do not block.`,
  ].join("\n");
}

function parseReview(message: string): ReviewResult {
  const review = JSON.parse(extractJsonObject(message)) as ReviewResult;
  if (typeof review.approved !== "boolean" || !Array.isArray(review.issues)) {
    throw new Error(`Codex review returned malformed JSON:\n${message}`);
  }
  return review;
}

// Codex is instructed to emit only JSON, but tolerate an accidental code fence
// or surrounding prose by extracting the outermost { ... } object.
function extractJsonObject(text: string): string {
  const start = text.indexOf("{");
  const end = text.lastIndexOf("}");
  if (start === -1 || end === -1 || end < start) {
    throw new Error(`Could not find a JSON object in Codex review output:\n${text}`);
  }
  return text.slice(start, end + 1);
}

function logReview(review: ReviewResult): void {
  $.log(`Review: ${review.approved ? "approved" : "changes requested"} — ${review.summary}`);
  for (const issue of review.issues) {
    $.logLight(`  [${issue.severity}] ${issue.description}`);
  }
}

// setup ------------------------------------------------------------------------

function requireApiKey(): string {
  const apiKey = Deno.env.get("OPENAI_API_KEY");
  if (!apiKey) {
    throw new Error("OPENAI_API_KEY is not set. It is required to run the AI fixer and reviewer.");
  }
  return apiKey;
}

async function ensureCodexInstalled(): Promise<void> {
  const found = await $`codex --version`.noThrow().quiet();
  if (found.code === 0) {
    return;
  }
  $.logStep("Installing OpenAI Codex CLI...");
  await $`npm install -g @openai/codex`;
}

// `codex exec` does not read OPENAI_API_KEY on its own -- it needs credentials
// stored via `codex login` first, otherwise its requests go out with no auth
// header and the API returns 401. The key is piped over stdin so it never
// appears in the process arguments.
async function codexLogin(apiKey: string): Promise<void> {
  $.logStep("Authenticating Codex with the OpenAI API key...");
  await $`codex login --with-api-key`.stdinText(apiKey);
}
