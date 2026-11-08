// @vitest-environment node

import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterAll, describe, expect, it } from "vitest";

const require = createRequire(import.meta.url);
const nextRequire = createRequire(require.resolve("@next/eslint-plugin-next"));
const globRequire = createRequire(nextRequire.resolve("fast-glob"));
const micromatchRequire = createRequire(globRequire.resolve("micromatch"));
const bracesPath = micromatchRequire.resolve("braces");
const estreeRequire = createRequire(require.resolve("@typescript-eslint/typescript-estree"));
const minimatchPath = estreeRequire.resolve("minimatch");

type Ast = { type: string; nodes: Ast[] };
type Walker = "compile" | "expand" | "stringify";
const braces = micromatchRequire("braces") as Record<Walker, (ast: Ast | string) => unknown> & {
  parse: (pattern: string) => Ast;
};
const depthError = /AST nesting depth exceeds the maximum of 100/;

function nestedAst(depth: number): Ast {
  let ast: Ast = { type: "root", nodes: [] };
  for (let i = 0; i < depth; i++) {
    ast = { type: "root", nodes: [ast] };
  }
  return ast;
}

describe("patched braces on the Next lint dependency path", () => {
  for (const operation of ["parse", "compile", "expand", "stringify", "default"]) {
    it.each(["braces", "parentheses"])(
      `rejects deeply nested %s through ${operation} without exhausting the stack`,
      (container) => {
        const source = `
          const braces = require(${JSON.stringify(bracesPath)});
          const pattern = ${container === "braces" ? "'{'" : "'('"}.repeat(4000)
            + 'x' + ${container === "braces" ? "'}'" : "')'"}.repeat(4000);
          try {
            ${operation === "default" ? "braces(pattern)" : `braces.${operation}(pattern)`};
            console.log(JSON.stringify({ name: 'accepted', message: '' }));
          } catch (error) {
            console.log(JSON.stringify({ name: error.name, message: error.message }));
          }
        `;
        const result = JSON.parse(execFileSync(process.execPath, ["--stack_size=512", "-e", source], {
          encoding: "utf8",
          timeout: 3000
        }));
        expect(result.name).toBe("SyntaxError");
        expect(result.message).toMatch(depthError);
      }
    );
  }

  it.each<Walker>(["compile", "expand", "stringify"])(
    "bounds caller-supplied ASTs through %s, including cycles",
    (operation) => {
      expect(() => braces[operation](nestedAst(4000))).toThrow(depthError);
      const cycle: Ast = { type: "root", nodes: [] };
      cycle.nodes.push(cycle);
      expect(() => braces[operation](cycle)).toThrow(depthError);
      expect(() => braces[operation](nestedAst(100))).not.toThrow();
      expect(() => braces[operation](nestedAst(101))).toThrow(depthError);
    }
  );

  it("preserves ordinary alternatives, ranges, escapes and parsed ASTs", () => {
    expect(braces.compile("app/{reading,writing}/**/*.{js,jsx}")).toBe(
      "app/(reading|writing)/**/*.(js|jsx)"
    );
    expect(braces.expand("page-{1..3}.js")).toEqual(["page-1.js", "page-2.js", "page-3.js"]);
    expect(braces.expand("a\\{b,c\\}")).toEqual(["a{b,c}"]);
    expect(braces.compile(braces.parse("a/{b,c}/d"))).toBe("a/(b|c)/d");
    expect(braces.stringify("unpaired'quote")).toBe("unpaired'quote");
    expect(braces.expand("{1..3,4}")).toEqual(["1..3", "4"]);
    const nested = "{".repeat(99) + "x" + "}".repeat(99);
    expect(braces.stringify(nested)).toBe(nested);
    expect(braces.expand(nested)).toEqual([nested]);
  });
});

describe("Next lint root directory resolution", () => {
  const fixture = mkdtempSync(path.join(tmpdir(), "copilot-lint-roots-"));
  mkdirSync(path.join(fixture, "apps", "web", "app"), { recursive: true });
  mkdirSync(path.join(fixture, "apps", "docs", "app"), { recursive: true });
  afterAll(() => rmSync(fixture, { recursive: true, force: true }));
  const { getRootDirs } = nextRequire("./utils/get-root-dirs.js") as {
    getRootDirs: (context: { cwd: string; settings: { next?: { rootDir: string | string[] } } }) => string[];
  };

  it("keeps the default root and expands configured absolute glob roots", () => {
    expect(getRootDirs({ cwd: fixture, settings: {} })).toEqual([fixture]);
    const web = path.join(fixture, "apps", "web");
    const docs = path.join(fixture, "apps", "docs");
    expect(getRootDirs({ cwd: fixture, settings: { next: { rootDir: web } } })).toEqual([web]);
    expect(getRootDirs({
      cwd: fixture,
      settings: { next: { rootDir: path.join(fixture, "apps", "{web,docs}") } }
    }).sort()).toEqual([web, docs].sort());
  });
});

describe("patched minimatch on the TypeScript parser dependency path", () => {
  it("bounds globstar parent-pattern amplification", () => {
    // GHSA-g3hv-75wg-m249: optimizationLevel 2 used to grow globParts
    // without a bound. A subprocess also catches regressions that hang.
    const source = `
      const { Minimatch } = require(${JSON.stringify(minimatchPath)});
      try {
        new Minimatch('**/../a/b/'.repeat(30) + 'target', { optimizationLevel: 2 });
        console.log(JSON.stringify({ name: 'accepted', message: '' }));
      } catch (error) {
        console.log(JSON.stringify({ name: error.name, message: error.message }));
      }
    `;
    const result = JSON.parse(execFileSync(process.execPath, ["-e", source], {
      encoding: "utf8",
      timeout: 3000
    }));
    expect(result.name).toBe("RangeError");
    expect(result.message).toContain("too many patterns");
  });

  it("preserves the parser's default-project glob matching", () => {
    const fixture = mkdtempSync(path.join(tmpdir(), "copilot-ts-project-"));
    const source = "const answer: number = 42;";
    writeFileSync(path.join(fixture, "tsconfig.json"), JSON.stringify({
      compilerOptions: { strict: true },
      include: ["src/**/*.ts"]
    }));
    writeFileSync(path.join(fixture, "included.ts"), source);
    const parser = require("@typescript-eslint/parser") as {
      parseForESLint: (source: string, options: object) => {
        ast: { type: string };
        services: { program?: unknown };
      };
    };
    try {
      const result = parser.parseForESLint(source, {
        filePath: path.join(fixture, "included.ts"),
        tsconfigRootDir: fixture,
        projectService: { allowDefaultProject: ["*.ts"] }
      });
      expect(result.ast.type).toBe("Program");
      expect(result.services.program).toBeDefined();
    } finally {
      rmSync(fixture, { recursive: true, force: true });
    }
  });
});
