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
"""MkDocs hooks: versions the extra CSS URL by its content, so browsers
fetch it again after every change, drops the table of contents of pages
that hide it, and unescapes pipes in code spans of table cells, which
GitHub renders as "|" and Python-Markdown leaves as "\\|"."""

import hashlib
import os
import re

_CSS = "css/extra.css"
_version = ""


def on_config(config):
    global _version
    path = os.path.join(os.path.dirname(config.config_file_path), "overrides", _CSS)
    with open(path, "rb") as f:
        _version = hashlib.sha256(f.read()).hexdigest()[:10]


def on_page_context(context, page, config, nav):
    if "toc" in page.meta.get("hide", []):
        page.toc.items = []
    return context


_CELL = re.compile(r"(<td[^>]*>.*?</td>)", re.S)


def on_page_content(html, page, config, files):
    return _CELL.sub(lambda m: m.group(1).replace("\\|", "|"), html)


def on_post_page(output, page, config):
    return output.replace(f'{_CSS}"', f'{_CSS}?v={_version}"')
