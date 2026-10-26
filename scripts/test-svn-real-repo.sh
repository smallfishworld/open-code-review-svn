#!/usr/bin/env bash
set -euo pipefail
export LANG=C.UTF-8 LC_ALL=C.UTF-8
ocr="$(realpath "${1:-./opencodereview}")"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
svnadmin create "$tmp/repository"
svn checkout "file://$tmp/repository" "$tmp/wc" >/dev/null
wc="$tmp/wc"
mkdir -p "$wc/src/nested"
echo 'int main = 1;' > "$wc/src/main.c"
echo 'int old = 1;' > "$wc/src/deleted.c"
echo 'int spaced = 1;' > "$wc/space name.c"
echo 'int unicode = 1;' > "$wc/中文文件.c"
svn add "$wc/src" "$wc/space name.c" "$wc/中文文件.c" >/dev/null
svn commit "$wc" -m first >/dev/null
echo 'int main = 2;' > "$wc/src/main.c"
echo 'int unicode = 2;' > "$wc/中文文件.c"
svn delete "$wc/src/deleted.c" >/dev/null
echo 'int new_file = 2;' > "$wc/src/nested/new.c"
svn add "$wc/src/nested/new.c" >/dev/null
svn commit "$wc" -m second >/dev/null
echo LOCAL_DIRTY_ONLY > "$wc/src/main.c"
echo 'int scheduled = 3;' > "$wc/scheduled.c"
svn add "$wc/scheduled.c" >/dev/null
echo 'int untracked = 4;' > "$wc/untracked.c"
for mode in commit range workspace subdir; do
  case "$mode" in
    commit) args=(--repo "$wc" --commit 2);;
    range) args=(--repo "$wc" --from 1 --to 2);;
    workspace) args=(--repo "$wc");;
    subdir) args=(--repo "$wc/src/nested" --commit 2);;
  esac
  timeout 40s "$ocr" review "${args[@]}" --preview --format json > "$tmp/$mode.json"
done
python3 - "$tmp" <<'PY'
import json, pathlib, sys
base=pathlib.Path(sys.argv[1])
def get(name):
    obj=json.loads((base/(name+'.json')).read_text())
    assert obj['total_files']==len(obj['files']),name
    return {x['path']:x for x in obj['files']}
commit=get('commit')
for path,status in {'src/main.c':'modified','src/deleted.c':'deleted','src/nested/new.c':'added','中文文件.c':'modified'}.items():
    assert commit[path]['status']==status, (path,commit)
assert set(commit)==set(get('range'))==set(get('subdir'))
assert 'scheduled.c' not in commit
workspace=get('workspace')
assert workspace['src/main.c']['status']=='modified'
assert workspace['scheduled.c']['status']=='added'
assert 'untracked.c' not in workspace
print('PASS: SVN real repo commit/range/workspace/subdir, Chinese paths, adds/deletes, dirty checkout')
PY
for args in '--commit invalid' '--commit 0' '--from 2 --to 1'; do
  if "$ocr" review --repo "$wc" $args --preview --format json >/dev/null 2>&1; then
    echo "invalid SVN arguments accepted: $args" >&2
    exit 1
  fi
done
echo 'SVN real repository end-to-end PASSED'
