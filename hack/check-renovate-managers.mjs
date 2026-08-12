#!/usr/bin/env node
// Verifies that every regex custom manager in .github/renovate.json5 actually
// matches the dependency annotations it is expected to manage in this repo.
//
// Custom manager regexes fail silently: if an annotation moves one line away
// from where the regex expects it (or the regex changes), Renovate simply
// stops updating that dependency and nothing in CI complains. This script
// turns that into a CI failure.
//
// Run: node hack/check-renovate-managers.mjs

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const configFile = process.argv[2]
  ? resolve(process.argv[2])
  : join(root, '.github', 'renovate.json5');

// Minimal JSON5 -> JSON: strips comments and trailing commas, string-aware.
const isWhitespace = (c) => c === ' ' || c === '\t' || c === '\n' || c === '\r';

function stripComments(src) {
  let out = '';
  let inString = false;
  let i = 0;
  while (i < src.length) {
    const c = src[i];
    const next = src[i + 1];
    if (inString) {
      out += c;
      if (c === '\\' && next !== undefined) {
        out += next;
        i += 2;
        continue;
      }
      if (c === '"') inString = false;
      i += 1;
      continue;
    }
    if (c === '"') {
      inString = true;
      out += c;
      i += 1;
      continue;
    }
    if (c === '/' && next === '/') {
      while (i < src.length && src[i] !== '\n') i += 1;
      continue;
    }
    if (c === '/' && next === '*') {
      i += 2;
      while (i < src.length && !(src[i] === '*' && src[i + 1] === '/')) i += 1;
      i += 2;
      continue;
    }
    out += c;
    i += 1;
  }
  return out;
}

function stripTrailingCommas(src) {
  let out = '';
  let inString = false;
  let i = 0;
  while (i < src.length) {
    const c = src[i];
    const next = src[i + 1];
    if (inString) {
      out += c;
      if (c === '\\' && next !== undefined) {
        out += next;
        i += 2;
        continue;
      }
      if (c === '"') inString = false;
      i += 1;
      continue;
    }
    if (c === '"') {
      inString = true;
      out += c;
      i += 1;
      continue;
    }
    if (c === ',') {
      let j = i + 1;
      while (j < src.length && isWhitespace(src[j])) j += 1;
      if (src[j] === '}' || src[j] === ']') {
        i += 1;
        continue;
      }
    }
    out += c;
    i += 1;
  }
  return out;
}

function toJson(src) {
  return stripTrailingCommas(stripComments(src));
}

const config = JSON.parse(toJson(readFileSync(configFile, 'utf8')));

function walk(dir, files = []) {
  for (const entry of readdirSync(dir)) {
    if (entry === '.git' || entry === 'node_modules' || entry === 'bin') continue;
    const p = join(dir, entry);
    const st = statSync(p);
    if (st.isDirectory()) walk(p, files);
    else files.push(p);
  }
  return files;
}

const files = walk(root);

const found = [];
for (const cm of config.customManagers ?? []) {
  if (cm.customType !== 'regex') continue;
  const fileRegexes = (cm.fileMatch ?? []).map((p) => new RegExp(p));
  const matchRegexes = (cm.matchStrings ?? []).map((p) => new RegExp(p, 'g'));
  for (const file of files) {
    const rel = relative(root, file);
    if (!fileRegexes.some((re) => re.test(rel))) continue;
    const content = readFileSync(file, 'utf8');
    for (const re of matchRegexes) {
      for (const m of content.matchAll(re)) {
        const g = m.groups ?? {};
        found.push({
          depName: g.depName,
          datasource: g.datasource,
          currentValue: g.currentValue,
          file: rel,
        });
      }
    }
  }
}

// [datasource, depName, minMatches]. currentValue is intentionally not
// asserted: versions move constantly and the guard would go stale on every
// renovate bump. Presence (and, where the same dep is annotated in several
// files, count) is what catches silent regex regressions.
const expected = [
  ['github-releases', 'golangci/golangci-lint', 2], // Makefile + ci.yaml env
  ['github-releases', 'kubernetes-sigs/kind'],
  ['github-releases', 'kubernetes-sigs/controller-tools'],
  ['github-releases', 'ko-build/ko'],
  ['github-releases', 'gotestyourself/gotestsum'],
  ['go', 'github.com/kyverno/chainsaw'],
  ['docker', 'renovate'],
  ['docker', 'docker.io/ollama/ollama'],
];

let ok = true;
if (process.env.DEBUG) {
  console.error('found deps:');
  for (const f of found) {
    console.error(`  ${f.file}: ${f.datasource} ${f.depName} = ${f.currentValue}`);
  }
}

// Guard against cross-line mis-associations: a single annotation in a file
// must resolve to exactly one value. If two different currentValues end up on
// the same (file, depName), the regex is matching across the wrong lines and
// renovate would produce a garbage dependency.
const byFileAndDep = new Map();
for (const f of found) {
  const key = `${f.file} :: ${f.depName}`;
  if (!byFileAndDep.has(key)) byFileAndDep.set(key, new Set());
  byFileAndDep.get(key).add(f.currentValue);
}
for (const [key, values] of byFileAndDep) {
  if (values.size > 1) {
    ok = false;
    console.error(
      `FAIL: ${key} resolves to ${values.size} different values: ${[...values].join(', ')}`,
    );
  }
}

for (const [datasource, depName, min = 1] of expected) {
  const matches = found.filter(
    (f) => f.datasource === datasource && f.depName === depName,
  );
  if (matches.length < min) {
    ok = false;
    console.error(
      `FAIL: expected ${min} match(es) for ${datasource} ${depName}, found ${matches.length}`,
    );
    for (const f of found.filter((f) => f.depName === depName)) {
      console.error(`  found in ${f.file}: datasource=${f.datasource} currentValue=${f.currentValue}`);
    }
  }
}

if (ok) {
  console.log(
    `OK: all ${expected.length} expected dependency annotations matched (${found.length} custom-manager deps found)`,
  );
} else {
  process.exit(1);
}
