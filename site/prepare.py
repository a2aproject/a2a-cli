#!/usr/bin/env python3
# Copyright 2026 The A2A Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Assembles the site's content from docs/, lessons/ and examples/.

The markdown in the repository links between files the way GitHub resolves
them. This copies docs/ to <out>/, and lessons/ and examples/ to
<out>/lessons/ and <out>/examples/, renames README.md to index.md, and
rewrites every relative link: links to pages and images in those directories
point at the copies, and links to other files point at GitHub. Each page gets
a meta description from its first paragraph, and the pages in HIDE_NAV and
HIDE_TOC hide the navigation and their sections, so the repository's markdown
needs no front matter, which GitHub renders as a table.

Usage: site/prepare.py <out>
"""

import fnmatch
import json
import os
import re
import shutil
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GITHUB = "https://github.com/a2aproject/a2a-cli"
SOURCES = {"docs": "", "lessons": "lessons", "examples": "examples"}
# Files the site serves; links to any other file go to GitHub.
SERVED = (".md", ".png", ".svg", ".jpg", ".gif", ".json", ".txt")

LINK = re.compile(r"(!?\[[^\]]*\]\()([^)\s]+)(\))")
IMG_SRC = re.compile(r'(<img\s[^>]*src=")([^"]+)(")')
FENCE = re.compile(r"^\s*(```|~~~)")
# Lines that don't start a prose paragraph.
NOT_PROSE = re.compile(r"^\s*([#>|!<*+-]|\d+\.|```|~~~)")
DESCRIPTION_MAX = 160
# Pages whose sections stay out of the navigation.
HIDE_TOC = ("index.md", "examples/index.md", "lessons/index.md")
# Pages shown without the navigation sidebar.
HIDE_NAV = ("index.md",)


def site_path(repo_path):
    """Maps a repository path to its path in the site, or None."""
    for src, dst in SOURCES.items():
        if repo_path == src or repo_path.startswith(src + "/"):
            rel = os.path.relpath(repo_path, src)
            if os.path.isdir(os.path.join(REPO, repo_path)):
                if not os.path.exists(os.path.join(REPO, repo_path, "README.md")):
                    return None
                rel = os.path.join(rel, "index.md")
            elif os.path.basename(rel) == "README.md":
                rel = os.path.join(os.path.dirname(rel), "index.md")
            return os.path.normpath(os.path.join(dst, rel))
    return None


def rewrite_target(target, src_repo_file, dst_site_file):
    if re.match(r"^[a-z]+:", target) or target.startswith("#"):
        return target
    path, _, anchor = target.partition("#")
    if path.startswith("/"):
        sys.exit(f"error: {src_repo_file}: root-relative link not allowed: {target}")
    repo_path = os.path.normpath(os.path.join(os.path.dirname(src_repo_file), path))
    if repo_path.startswith(".."):
        sys.exit(f"error: {src_repo_file}: relative link escapes repository: {target}")
    if not os.path.exists(os.path.join(REPO, repo_path)):
        sys.exit(f"error: {src_repo_file}: target not found: {target} (resolved to {repo_path})")
    mapped = site_path(repo_path)
    if mapped is None or not mapped.endswith(SERVED):
        kind = "tree" if os.path.isdir(os.path.join(REPO, repo_path)) else "blob"
        new = f"{GITHUB}/{kind}/main/{repo_path}"
    else:
        new = os.path.relpath(mapped, os.path.dirname(dst_site_file))
    return new + ("#" + anchor if anchor else "")


def rewrite(text, src_repo_file, dst_site_file):
    out, in_code = [], False
    for line in text.split("\n"):
        if FENCE.match(line):
            in_code = not in_code
        if not in_code:
            fix = lambda m: (
                m.group(1)
                + rewrite_target(m.group(2), src_repo_file, dst_site_file)
                + m.group(3)
            )
            line = IMG_SRC.sub(fix, LINK.sub(fix, line))
        out.append(line)
    return "\n".join(out)


def description(text):
    """Returns the first prose paragraph of a page as plain text, shortened
    to a sentence or word boundary within DESCRIPTION_MAX characters."""
    para, in_code, skip = [], False, False
    for line in text.split("\n"):
        if FENCE.match(line):
            in_code = not in_code
            continue
        if in_code:
            continue
        if not line.strip():
            if para and not para[-1].endswith(":"):
                break
            para, skip = [], False
            continue
        if not para and NOT_PROSE.match(line):
            skip = True
        if not skip:
            para.append(line.strip())
    plain = " ".join(para)
    plain = re.sub(r"!?\[([^\]]*)\]\([^)]*\)", r"\1", plain)
    plain = re.sub(r"[`*_]", "", plain)
    if len(plain) <= DESCRIPTION_MAX:
        return plain
    cut = plain[:DESCRIPTION_MAX]
    end = cut.rfind(". ")
    if end > DESCRIPTION_MAX // 2:
        return cut[: end + 1]
    return cut[: cut.rfind(" ")].rstrip(",;:") + "…"


def main(out):
    if os.path.exists(out):
        shutil.rmtree(out)
    for src, dst in SOURCES.items():
        for dirpath, _, files in os.walk(os.path.join(REPO, src)):
            for name in files:
                repo_file = os.path.relpath(os.path.join(dirpath, name), REPO)
                site_file = site_path(repo_file)
                if site_file is None or not name.endswith(SERVED):
                    continue
                target = os.path.join(out, site_file)
                os.makedirs(os.path.dirname(target), exist_ok=True)
                if not name.endswith(".md"):
                    shutil.copy2(os.path.join(REPO, repo_file), target)
                    continue
                with open(os.path.join(REPO, repo_file)) as f:
                    text = f.read()
                with open(target, "w") as f:
                    meta = []
                    desc = description(text)
                    if desc:
                        meta.append(f"description: {json.dumps(desc, ensure_ascii=False)}")
                    hide = []
                    if any(fnmatch.fnmatch(site_file, p) for p in HIDE_NAV):
                        hide.append("navigation")
                    if any(fnmatch.fnmatch(site_file, p) for p in HIDE_TOC):
                        hide.append("toc")
                    if hide:
                        meta.append(f"hide: [{', '.join(hide)}]")
                    if meta:
                        f.write("---\n" + "\n".join(meta) + "\n---\n\n")
                    f.write(rewrite(text, repo_file, site_file))


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: site/prepare.py <out>")
    main(sys.argv[1])
