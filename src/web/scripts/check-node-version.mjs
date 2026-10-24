// Minimum patched releases for CVE-2026-111497 (Node2Shell) and the other
// October 2026 Node.js security fixes. Keep package.json engines in sync.
const minimumVersions = new Map([
  [22, [23, 4]],
  [24, [22, 1]],
  [26, [12, 1]]
]);
const [major, minor, patch] = process.versions.node.split(".").map(Number);
const minimum = minimumVersions.get(major);

if (!/^\d+\.\d+\.\d+$/.test(process.versions.node) || !minimum ||
    minor < minimum[0] || (minor === minimum[0] && patch < minimum[1])) {
  console.error(
    `Node.js ${process.versions.node} is not supported. ` +
    "Use Node.js 26.12.1 or newer in the 26.x line, " +
    "24.22.1 or newer in 24.x, or 22.23.4 or newer in 22.x " +
    "to include the Node2Shell security fix."
  );
  process.exitCode = 1;
}
