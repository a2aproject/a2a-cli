#!/usr/bin/env bash
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
#
# Builds the documentation site into site/.out/site.
#
# Usage:
#   pip install -r site/requirements.txt
#   site/build.sh            # build
#   site/build.sh serve      # build the content, then serve it with live reload

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
python3 prepare.py .build/docs
if [[ "${1:-}" == "serve" ]]; then
  exec mkdocs serve -f mkdocs.yml
fi
mkdocs build --strict -f mkdocs.yml
