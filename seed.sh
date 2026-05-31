#!/bin/bash
set -e

CONTAINER="dovecot-test"
USER="test"
DIR="$(dirname "$0")"

for file in "$DIR"/testdata/plain.eml "$DIR"/testdata/html.eml "$DIR"/testdata/multipart.eml; do
    echo "Seeding $file..."
    docker exec -i "$CONTAINER" doveadm save -u "$USER" -m INBOX < "$file"
done

echo "Seeded 3 emails into $CONTAINER"
