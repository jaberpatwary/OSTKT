#!/bin/sh
set -eu

: "${PGHOST:?PGHOST is required}"
: "${PGDATABASE:?PGDATABASE is required}"
: "${PGUSER:?PGUSER is required}"
: "${PGPASSWORD:?PGPASSWORD is required}"

BACKUP_DIR=${BACKUP_DIR:-/backups}
ATTACHMENT_DIR=${ATTACHMENT_DIR:-/attachments}
BACKUP_INTERVAL_SECONDS=${BACKUP_INTERVAL_SECONDS:-86400}
BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-30}

case "$BACKUP_INTERVAL_SECONDS:$BACKUP_RETENTION_DAYS" in
	*[!0-9:]*|:*) echo "backup interval and retention must be positive integers" >&2; exit 2 ;;
esac
if [ "$BACKUP_INTERVAL_SECONDS" -lt 60 ] || [ "$BACKUP_RETENTION_DAYS" -lt 1 ]; then
	echo "backup interval must be at least 60 seconds and retention at least 1 day" >&2
	exit 2
fi

mkdir -p "$BACKUP_DIR" "$ATTACHMENT_DIR"
echo "Backup worker ready; interval=${BACKUP_INTERVAL_SECONDS}s retention=${BACKUP_RETENTION_DAYS}d"

while :; do
	stamp=$(date -u +%Y%m%dT%H%M%SZ)
	db_tmp="$BACKUP_DIR/$stamp.dump.tmp"
	files_tmp="$BACKUP_DIR/$stamp-attachments.tar.gz.tmp"
	db_final="$BACKUP_DIR/$stamp.dump"
	files_final="$BACKUP_DIR/$stamp-attachments.tar.gz"
	ready="$BACKUP_DIR/$stamp.complete"

	if pg_dump --format=custom --no-owner --no-acl --file="$db_tmp" \
		&& pg_restore --list "$db_tmp" >/dev/null \
		&& tar -czf "$files_tmp" -C "$ATTACHMENT_DIR" . \
		&& tar -tzf "$files_tmp" >/dev/null; then
		mv "$db_tmp" "$db_final"
		mv "$files_tmp" "$files_final"
		: > "$ready"
		echo "Backup completed: $stamp"
	else
		rm -f "$db_tmp" "$files_tmp" "$db_final" "$files_final" "$ready"
		echo "Backup failed: $stamp" >&2
	fi

	find "$BACKUP_DIR" -type f -mtime "+$BACKUP_RETENTION_DAYS" -exec rm -f {} +
	sleep "$BACKUP_INTERVAL_SECONDS"
done
