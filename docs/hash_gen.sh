# Example hashing 2 entries per wordlist in dir
# bash ./hash-gen.sh -d <wordlist_dir> -n 2 -o <out_path>
set -euo pipefail
IFS=$'\n\t'

usage() {
  cat <<EOF
Usage: $(basename "$0") [-d dir] [-p pattern] [-n per_file] [-o outfile] [-a] [-v]
  -d DIR       directory containing wordlists (default: .)
  -p PATTERN   filename glob pattern (default: '*')
  -n NUMBER    number of hashes to generate per file (default: 100)
  -o OUTFILE   output file (default: hashes.txt)
  -a           append to OUTFILE instead of overwrite
  -v           verbose
  -h           show this help
EOF
  exit 1
}

# defaults
DIR="."
PATTERN="*"
NUM=100
OUTFILE="hashes.txt"
APPEND=false
VERBOSE=false

while getopts ":d:p:n:o:avh" opt; do
  case "$opt" in
    d) DIR="$OPTARG" ;;
    p) PATTERN="$OPTARG" ;;
    n) NUM="$OPTARG" ;;
    o) OUTFILE="$OPTARG" ;;
    a) APPEND=true ;;
    v) VERBOSE=true ;;
    h) usage ;;
    *) usage ;;
  esac
done

# sanity
if ! [[ -d "$DIR" ]]; then
  echo "Error: directory '$DIR' not found." >&2
  exit 2
fi

if ! [[ "$NUM" =~ ^[0-9]+$ ]] || [[ "$NUM" -le 0 ]]; then
  echo "Error: -n must be a positive integer." >&2
  exit 2
fi

# prepare output
if [ "$APPEND" = false ]; then
  : > "$OUTFILE"   # truncate
fi

shopt -s nullglob
files=( "$DIR"/$PATTERN )
shopt -u nullglob

if [ ${#files[@]} -eq 0 ]; then
  echo "No files matched pattern '$PATTERN' in directory '$DIR'." >&2
  exit 0
fi

# Helper: produce a 32-bit random number from /dev/urandom
rand32() {
  od -An -N4 -tu4 /dev/urandom 2>/dev/null | tr -d ' '
}

for file in "${files[@]}"; do
  # skip non-regular or empty
  if ! [ -f "$file" ]; then
    $VERBOSE && echo "Skipping (not regular): $file"
    continue
  fi

  line_count=$(wc -l < "$file" | tr -d '[:space:]')
  if [ -z "$line_count" ] || [ "$line_count" -eq 0 ]; then
    $VERBOSE && echo "Skipping (empty): $file"
    continue
  fi

  $VERBOSE && echo "Processing '$file' ($line_count lines): generating $NUM hashes..."

  for ((i=1;i<=NUM;i++)); do
    r=$(rand32)
    lineno=$(( (r % line_count) + 1 ))

    # your requested awk trick, then hash
    awk -v n="$lineno" 'NR==n{print; exit}' "$file" \
      | tr -d '\n' \
      | sha512sum \
      | sed 's/  -$//' >> "$OUTFILE"

    # ensure newline between entries
    echo >> "$OUTFILE"

    $VERBOSE && printf "\r  %d/%d" "$i" "$NUM" >&2
  done

  $VERBOSE && printf "\n"
done

$VERBOSE && echo "Done. Hashes saved to: $OUTFILE"
