// @ts-check
const fs = require("fs");
const path = require("path");
const args = process.argv.slice(2);
const rootDir = path.join(__dirname, "../..");
fs.copyFileSync(path.join(rootDir, "target/plugin.wasm"), path.join(__dirname, "plugin.wasm"));
// the licenses have to be distributed along with the plugin, which includes
// the third-party code in the third_party directory
fs.writeFileSync(path.join(__dirname, "LICENSE"), getLicenseText());

if (args.length > 0) {
  // update the version based on the first argument
  const packageJsonPath = path.join(__dirname, "package.json");
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf8"));
  if (args[0] === "sync-version") {
    packageJson.version = fs.readFileSync(path.join(rootDir, "VERSION"), "utf8").trim();
  } else {
    packageJson.version = args[0];
  }
  fs.writeFileSync(packageJsonPath, JSON.stringify(packageJson, undefined, 2) + "\n");
}

function getLicenseText() {
  const thirdPartyDir = path.join(rootDir, "third_party");
  const texts = [fs.readFileSync(path.join(rootDir, "LICENSE"), "utf8").trim()];
  for (const dirName of fs.readdirSync(thirdPartyDir).sort()) {
    const dirPath = path.join(thirdPartyDir, dirName);
    if (!fs.statSync(dirPath).isDirectory()) {
      continue;
    }
    for (const fileName of fs.readdirSync(dirPath).sort()) {
      const text = fs.readFileSync(path.join(dirPath, fileName), "utf8").trim();
      texts.push(`${dirName} (${fileName}):\n\n${text}`);
    }
  }
  return texts.join("\n\n---\n\n") + "\n";
}
