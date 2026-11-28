#!/bin/sh
set -eu

stamp=${1:-}
case "$stamp" in
	[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]T[0-9][0-9][0-9][0-9][0-9][0-9]Z) ;;
	*) echo "Usage: restore.sh YYYYMMDDThhmmssZ" >&2; exit 2 ;;
esac

BACKUP_DIR=${BACKUP_DIR:-/backups}
ATTACHMENT_DIR=${ATTACHMENT_DIR:-/attachments}
db_file="$BACKUP_DIR/$stamp.dump"
files_file="$BACKUP_DIR/$stamp-attachments.tar.gz"
ready_file="$BACKUP_DIR/$stamp.complete"

if [ ! -f "$ready_file" ] || [ ! -f "$db_file" ] || [ ! -f "$files_file" ]; then
	echo "A complete backup set for $stamp was not found" >&2
	exit 1
fi
if [ "${CONFIRM_RESTORE:-}" != "YES" ]; then
	echo "This replaces database objects and overlays attachment files. Stop the app, back up current data, then rerun with CONFIRM_RESTORE=YES." >&2
	exit 2
fi

pg_restore --clean --if-exists --no-owner --no-acl --exit-on-error --dbname="$PGDATABASE" "$db_file"
tar -xzf "$files_file" -C "$ATTACHMENT_DIR" --no-same-owner
echo "Restore completed from $stamp"
