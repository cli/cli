#!/bin/sh

set -eu

usage() {
	printf 'usage: %s <demo.gif> <inspection-dir>\n' "$0" >&2
	exit 2
}

[ "$#" -eq 2 ] || usage
gif=$1
inspection_dir=$2

[ -s "$gif" ] || {
	printf 'GIF does not exist or is empty: %s\n' "$gif" >&2
	exit 1
}
[ ! -L "$gif" ] || {
	printf 'GIF must not be a symbolic link: %s\n' "$gif" >&2
	exit 1
}

script_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -P "$script_dir/../../../.." && pwd)
gif_dir=$(CDPATH= cd -P "$(dirname "$gif")" && pwd)
[ ! -e "$inspection_dir" ] || {
	printf 'Inspection directory already exists: %s\n' "$inspection_dir" >&2
	exit 1
}
inspection_parent=$(CDPATH= cd -P "$(dirname "$inspection_dir")" && pwd) || {
	printf 'Inspection parent directory does not exist: %s\n' "$(dirname "$inspection_dir")" >&2
	exit 1
}
inspection_dir="$inspection_parent/$(basename "$inspection_dir")"

case "$gif_dir/" in
	"$repo_root/"*)
		printf 'GIF must be outside the repository: %s\n' "$gif" >&2
		exit 1
		;;
esac
case "$inspection_dir/" in
	"$repo_root/"*)
		printf 'Inspection directory must be outside the repository: %s\n' "$inspection_dir" >&2
		exit 1
		;;
esac

mkdir "$inspection_dir"

probe=$(ffprobe -v error -select_streams v:0 \
	-show_entries stream=codec_name,width,height,nb_frames:format=format_name,duration,size \
	-of default=noprint_wrappers=1 "$gif")
printf '%s\n' "$probe"
printf '%s\n' "$probe" | grep -Eq '^codec_name=gif$' || {
	printf 'Expected GIF video codec: %s\n' "$gif" >&2
	exit 1
}
printf '%s\n' "$probe" | grep -Eq '^format_name=gif$' || {
	printf 'Expected GIF container: %s\n' "$gif" >&2
	exit 1
}

mkdir -p "$inspection_dir/sampled"
ffmpeg -v error -y -i "$gif" -frames:v 1 "$inspection_dir/first.png"
ffmpeg -v error -y -i "$gif" -update 1 "$inspection_dir/final.png"
ffmpeg -v error -y -i "$gif" -vf fps=1 \
	"$inspection_dir/sampled/frame-%04d.png"

mkdir -p "$inspection_dir/all"
ffmpeg -v error -y -i "$gif" "$inspection_dir/all/frame-%06d.png"

printf 'Inspection frames: %s\n' "$inspection_dir"
