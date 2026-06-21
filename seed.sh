#!/bin/bash
set -e

CONTAINER="dovecot-test"
USER="test"
DIR="$(dirname "$0")"

count=0
for file in "$DIR"/testdata/*.eml; do
    name=$(basename "$file")
    echo "  $name"
    docker exec -i "$CONTAINER" doveadm save -u "$USER" -m INBOX < "$file"
    count=$((count + 1))
done

echo "Seeded $count emails into $CONTAINER"
