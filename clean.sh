#!/usr/bin/env bash
set -euo pipefail

# Usage: ./clean-heavy-history.sh [threshold-bytes]
# Default threshold: 1,000,000 bytes (~1 MB)
THRESHOLD_BYTES="${1:-1000000}"

echo ">>> Threshold for 'heavy' objects: ${THRESHOLD_BYTES} bytes"

# 1. Ensure we're inside a git repo
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "ERROR: This is not a git repository." >&2
  exit 1
fi

# 2. Ensure working tree is clean
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "ERROR: You have uncommitted changes. Commit or stash them first." >&2
  exit 1
fi

# 3. Ensure git filter-repo is available
if ! git filter-repo -h >/dev/null 2>&1; then
  cat >&2 <<EOF
ERROR: 'git filter-repo' is not installed or not on PATH.

Install it (one of these):

  # macOS / Linux (python):
  python3 -m pip install --user git-filter-repo

  # Or follow official docs:
  https://github.com/newren/git-filter-repo

Then rerun this script.
EOF
  exit 1
fi

echo
echo ">>> Current repo size (just .git directory):"
du -sh .git || true

echo
echo ">>> Scanning for heavy blobs (this may take a moment)..."

# 4. Find blobs above threshold and store a detailed list
HEAVY_BLOBS_FILE=".git/heavy-blobs.txt"
git rev-list --objects --all \
  | git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' \
  | awk -v threshold="$THRESHOLD_BYTES" \
      '$1=="blob" && $3 >= threshold {print $2" "$3" "$4}' \
  | sort -k2nr > "$HEAVY_BLOBS_FILE"

if ! [ -s "$HEAVY_BLOBS_FILE" ]; then
  echo ">>> No blobs larger than ${THRESHOLD_BYTES} bytes found. Nothing to do."
  rm -f "$HEAVY_BLOBS_FILE"
  exit 0
fi

echo
echo ">>> Heavy blobs found (SHA, size, path):"
printf "%-40s %12s %s\n" "BLOB_SHA" "SIZE" "PATH"
echo "------------------------------------------------------------------------"
awk '{printf "%-40s %12d %s\n", $1, $2, $3}' "$HEAVY_BLOBS_FILE"

# 5. Show which commits introduced each heavy blob (for your reference)
echo
echo ">>> Example commits introducing heavy blobs (for inspection):"
while read -r sha size path; do
  echo
  echo "  Blob: $sha  Size: $size  Path: $path"
  echo "  First commit touching this object:"
  # --find-object finds commits containing that blob
  git log --all --find-object="$sha" --pretty=format:"    %h %ad %an %s" --date=short | head -n 1 || true
done < "$HEAVY_BLOBS_FILE"

# 6. Build a unique list of paths to remove from history
HEAVY_PATHS_FILE=".git/heavy-paths.txt"
cut -d' ' -f3 "$HEAVY_BLOBS_FILE" | sort -u > "$HEAVY_PATHS_FILE"

echo
echo ">>> Paths that will be removed from ALL history:"
while read -r p; do
  echo "  - $p"
done < "$HEAVY_PATHS_FILE"

echo
read -r -p ">>> Proceed to rewrite history and remove these paths from ALL commits? [y/N] " answer
case "$answer" in
  [Yy]* ) ;;
  * )
    echo "Aborting. No changes made."
    exit 0
    ;;
esac

# 7. Create a mirror backup of the repo before rewriting history
TOPLEVEL="$(git rev-parse --show-toplevel)"
REPO_NAME="$(basename "$TOPLEVEL")"
BACKUP_DIR="../${REPO_NAME}-backup-$(date +%Y%m%d%H%M%S).git"

echo
echo ">>> Creating mirror backup at: $BACKUP_DIR"
git clone --mirror "$TOPLEVEL" "$BACKUP_DIR"

# 8. Run git filter-repo to strip the heavy paths
echo
echo ">>> Rewriting history to drop these paths..."
git filter-repo --force --invert-paths --paths-from-file "$HEAVY_PATHS_FILE"

# 9. Clean up reflogs and repack to actually shrink size
echo
echo ">>> Running aggressive GC..."
git reflog expire --expire=now --all || true
git gc --prune=now --aggressive || true

echo
echo ">>> New repo size (after cleanup):"
du -sh .git || true

echo
echo ">>> Done."
cat <<EOF

A mirror backup of your ORIGINAL repo is here:

  $BACKUP_DIR

Review the new history and make sure everything looks OK.
If you're happy, force-push the rewritten history:

  git push origin --force --all
  git push origin --force --tags

(Remember: this rewrites history; any other clones will need to reclone or reset.)

EOF
