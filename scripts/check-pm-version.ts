// Verifies that the installed npm prosemirror-model package matches the version the git submodule (prosemirror/prosemirror-model) is pinned to. The fixture
// generator, and any other script that relies on the submodule being a specific version, must call this before doing work, so an npm <-> submodule version
// skew fails loudly instead of silently producing fixtures from a different reference version than the Go port and its tests were written against.

import { existsSync, readFileSync } from "node:fs"
import { createRequire } from "node:module"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const scriptDir = dirname(fileURLToPath(import.meta.url))

function readVersion(packageJsonPath: string): string {
  const json = JSON.parse(readFileSync(packageJsonPath, "utf8")) as { version?: string }
  if (typeof json.version !== "string") {
    throw new Error(`no version field in ${packageJsonPath}`)
  }
  return json.version
}

// Locates the package.json of an installed npm package by resolving its entry point and walking up to the nearest package.json. The package is resolved
// rather than reading a fixed node_modules path so hoisting is handled, and the package.json is read from disk rather than imported because the package's
// exports block a direct "prosemirror-model/package.json" import.
function packageJsonFor(packageName: string): string {
  const require = createRequire(import.meta.url)
  let dir = dirname(require.resolve(packageName))
  for (;;) {
    const candidate = join(dir, "package.json")
    if (existsSync(candidate)) {
      return candidate
    }
    const parent = dirname(dir)
    if (parent === dir) {
      throw new Error(`could not locate package.json for ${packageName}`)
    }
    dir = parent
  }
}

export function assertProseMirrorModelVersionMatch(): void {
  const submoduleVersion = readVersion(join(scriptDir, "..", "prosemirror", "prosemirror-model", "package.json"))
  const npmVersion = readVersion(packageJsonFor("prosemirror-model"))
  if (submoduleVersion !== npmVersion) {
    throw new Error(
      `prosemirror-model version mismatch: the npm package is ${npmVersion} but the git submodule is pinned to ${submoduleVersion}. ` +
        `Bring them into lockstep (update the submodule to the v${npmVersion} tag, or run "npm install prosemirror-model@${submoduleVersion}") and regenerate the fixtures.`,
    )
  }
}
