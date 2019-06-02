#!/bin/bash

if [[ $# -eq 0 ]] ; then
  echo "Usage: $0 <source dir> <target dir>" >&1
  exit 0
fi

## TODO larger help message

if [[ $# -ne 2 ]] ; then
  echo 'Exactly 2 arguments expected: source directory, target directory.' >&2
  exit 1
fi

echo -n 'Checking source... '
if [[ -d "$1" && -r "$1" && -x "$1" ]] ; then
  echo 'ok'
else
  echo "not accessible."
  echo "Specified source is not a directory or cannot be accessed: $1" >&2
  exit 1
fi

echo -n 'Checking destination... '
if [[ -d "$2" && -r "$2" && -x "$2" && -w "$2" ]] ; then
  echo 'ok'
else
  echo "not accessible."
  echo "Specified destination is not a directory or cannot be accessed: $2" >&2
  exit 1
fi

echo -n 'Checking rsync exists and is functioning... '
if ! rsync --version >/dev/null ; then
  echo 'rsync cannot be found or is not functioning.' >&2
  exit 1
fi
echo 'ok'

echo -n 'Checking rsync version... '
readonly rsync_version="$( rsync --version | grep 'version' | head -1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' )"
readonly rsync_version_major="$( echo "$rsync_version" | sed -e 's/\..*$//' )"
readonly rsync_version_minor="$( echo "$rsync_version" | sed -e 's/^[0-9]*\.//' -e 's/\.[0-9]*$//' )"

if [[ "$rsync_version_major" -ge 3 && "$rsync_version_minor" -ge 1 ]] ; then
  echo "$rsync_version, ok"
else
  echo "$rsync_version, too old."
  echo "Minimal supported rsync version is 3.1, found: $rsync_version" >&2
  exit 1
fi

readonly SRC="$( cd "$1" && pwd )"
readonly DST="$( cd "$2" && pwd )"
readonly backup_timestamp="$( date -u '+%Y-%m-%d_%H-%M-%S' )"

echo -n 'Checking for previous backup... '
if [[ -z "$( cd "$DST" && ls -1 )" ]] ; then
  echo 'none.'
  readonly previous_backup=''
else
  readonly previous_backup="$( cd "$DST" && find * -maxdepth 0 -type d | grep -E '[0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{2}-[0-9]{2}-[0-9]{2}' | sort -r | head -1 )"
  if [[ -z "$previous_backup" ]] ; then
    echo 'none.'
  else
    echo "$previous_backup."
  fi
fi

echo -n 'Preparing backup directory... '
readonly backup_dir="$DST/$backup_timestamp"
readonly log_file="$( cd "$DST" && pwd )/log_$backup_timestamp.txt"
if mkdir "$backup_dir" ; then
  echo 'ok'
else
  echo 'failed.'
  echo "Failed to create backup directory: $backup_dir" >&2
  exit 1
fi
CMD='rsync -rlpth --info=progress2'
if [[ -n "$previous_backup" ]] ; then
  CMD="$CMD --link-dest='$DST/$previous_backup'"
fi
CMD="$CMD --log-file='$log_file' '$( cd "$SRC" && pwd )/' '$( cd "$backup_dir" && pwd )/'"
echo 'Executing command:'
echo "$CMD"
## cannot just use the variable to run it because of escaping of quotes,
## have to use either eval or bash -c :
if bash -c "$CMD" </dev/null ; then
  echo 'Done.'
else
  echo -n 'Failed. ' >&2
  if [[ -f "$log_file" ]] ; then
    echo 'Last messages from the log file:' >&2
    tail -n 13 "$log_file" >&2
  else
    echo 'No log file found, console output should contain an error message from rsync.' >&2
  fi
  exit 1
fi
