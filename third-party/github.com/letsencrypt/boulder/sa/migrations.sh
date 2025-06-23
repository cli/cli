#!/usr/bin/env bash

set -eu

if type realpath >/dev/null 2>&1 ; then
  cd "$(realpath -- $(dirname -- "$0"))"
fi

# posix compliant escape sequence
esc=$'\033'"["
res="${esc}0m"

#
# Defaults
#
DB_NEXT_PATH="db-next"
DB_PATH="db"
OUTCOME="ERROR"
PROMOTE=()
RUN=()
DB=""

#
# Print Functions
#
function print_outcome() {
  if [ "${OUTCOME}" == OK ]
  then
    echo -e "${esc}0;32;1m${OUTCOME}${res}"
  else
    echo -e "${esc}0;31;1m${OUTCOME}${res}"
  fi
}

function print_usage_exit() {
  echo "${USAGE}"
  exit 0
}

# newline + bold magenta
function print_heading() {
  echo
  echo -e "${esc}0;34;1m${1}${res}"
}

# bold cyan
function print_moving() {
  local src=${1}
  local dest=${2}
  echo -e "moving:    ${esc}0;36;1m${src}${res}"
  echo -e "to:        ${esc}0;32;1m${dest}${res}"
}

# bold yellow
function print_unlinking() {
  echo -e "unlinking: ${esc}0;33;1m${1}${res}"
}

# bold magenta
function print_linking () {
  local from=${1}
  local to=${2}
  echo -e "linking:   ${esc}0;35;1m${from} ->${res}"
  echo -e "to:        ${esc}0;39;1m${to}${res}"
}

function check_arg() {
  if [ -z "${OPTARG}" ]
  then
    exit_msg "No arg for --${OPT} option, use: -h for help">&2
  fi
}

function print_migrations() {
  iter=1
  for file in "${migrations[@]}"
  do
    echo "${iter}) $(basename -- ${file})"
    iter=$(expr "${iter}" + 1)
  done
}

function exit_msg() {
  # complain to STDERR and exit with error
  echo "${*}" >&2
  exit 2
}

#
# Utility Functions
#
function get_promotable_migrations() {
  local migrations=()
  local migpath="${DB_NEXT_PATH}/${1}"
  for file in "${migpath}"/*.sql; do
    [[ -f "${file}" && ! -L "${file}" ]] || continue
    migrations+=("${file}")
  done
  if [[ "${migrations[@]}" ]]; then
    echo "${migrations[@]}"
  else
    exit_msg "There are no promotable migrations at path: "\"${migpath}\"""
  fi
}

function get_demotable_migrations() {
  local migrations=()
  local migpath="${DB_NEXT_PATH}/${1}"
  for file in "${migpath}"/*.sql; do
    [[ -L "${file}" ]] || continue
    migrations+=("${file}")
  done
  if [[ "${migrations[@]}" ]]; then
    echo "${migrations[@]}"
  else
    exit_msg "There are no demotable migrations at path: "\"${migpath}\"""
  fi
}

#
# CLI Parser
#
USAGE="$(cat -- <<-EOM

Usage:
  
  Boulder DB Migrations CLI

  Helper for listing, promoting, and demoting migration files

  ./$(basename "${0}") [OPTION]...
  -b  --db                  Name of the database, this is required (e.g. boulder_sa or incidents_sa)
  -n, --list-next           Lists migration files present in sa/db-next/<db>
  -c, --list-current        Lists migration files promoted from sa/db-next/<db> to sa/db/<db> 
  -p, --promote             Select and promote a migration from sa/db-next/<db> to sa/db/<db>
  -d, --demote              Select and demote a migration from sa/db/<db> to sa/db-next/<db>
  -h, --help                Shows this help message

EOM
)"

while getopts nchpd-:b:-: OPT; do
  if [ "$OPT" = - ]; then     # long option: reformulate OPT and OPTARG
    OPT="${OPTARG%%=*}"       # extract long option name
    OPTARG="${OPTARG#$OPT}"   # extract long option argument (may be empty)
    OPTARG="${OPTARG#=}"      # if long option argument, remove assigning `=`
  fi
  case "${OPT}" in
    b | db )                  check_arg; DB="${OPTARG}" ;;
    n | list-next )           RUN+=("list_next") ;;
    c | list-current )        RUN+=("list_current") ;;
    p | promote )             RUN+=("promote") ;;
    d | demote )              RUN+=("demote") ;;
    h | help )                print_usage_exit ;;
    ??* )                     exit_msg "Illegal option --${OPT}" ;;  # bad long option
    ? )                       exit 2 ;;  # bad short option (error reported via getopts)
  esac
done
shift $((OPTIND-1)) # remove parsed opts and args from $@ list

# On EXIT, trap and print outcome
trap "print_outcome" EXIT

[ -z "${DB}" ] && exit_msg "You must specify a database with flag -b \"foo\" or --db=\"foo\""

STEP="list_next"
if [[ "${RUN[@]}" =~ "${STEP}" ]] ; then
  print_heading "Next Migrations"
  migrations=($(get_promotable_migrations "${DB}"))
  print_migrations "${migrations[@]}"
fi

STEP="list_current"
if [[ "${RUN[@]}" =~ "${STEP}" ]] ; then
  print_heading "Current Migrations"
  migrations=($(get_demotable_migrations "${DB}"))
  print_migrations "${migrations[@]}"
fi

STEP="promote"
if [[ "${RUN[@]}" =~ "${STEP}" ]] ; then
  print_heading "Promote Migration"
  migrations=($(get_promotable_migrations "${DB}"))
  declare -a mig_index=()
  declare -A mig_file=()
  for i in "${!migrations[@]}"; do
    mig_index["$i"]="${migrations[$i]%% *}"
    mig_file["${mig_index[$i]}"]="${migrations[$i]#* }"
  done

  promote=""
  PS3='Which migration would you like to promote? (q to cancel): '
  
  select opt in "${mig_index[@]}"; do
    case "${opt}" in
      "") echo "Invalid option or cancelled, exiting..." ; break ;;
      *)  mig_file_path="${mig_file[$opt]}" ; break ;;
    esac
  done
  if [[ "${mig_file_path}" ]]
  then
    print_heading "Promoting Migration"
    promote_mig_name="$(basename -- "${mig_file_path}")"
    promoted_mig_file_path="${DB_PATH}/${DB}/${promote_mig_name}"
    symlink_relpath="$(realpath --relative-to=${DB_NEXT_PATH}/${DB} ${promoted_mig_file_path})"

    print_moving "${mig_file_path}" "${promoted_mig_file_path}"
    mv "${mig_file_path}" "${promoted_mig_file_path}"
    
    print_linking "${mig_file_path}" "${symlink_relpath}"
    ln -s "${symlink_relpath}" "${DB_NEXT_PATH}/${DB}"
  fi
fi

STEP="demote"
if [[ "${RUN[@]}" =~ "${STEP}" ]] ; then
  print_heading "Demote Migration"
  migrations=($(get_demotable_migrations "${DB}"))
  declare -a mig_index=()
  declare -A mig_file=()
  for i in "${!migrations[@]}"; do
    mig_index["$i"]="${migrations[$i]%% *}"
    mig_file["${mig_index[$i]}"]="${migrations[$i]#* }"
  done

  demote_mig=""
  PS3='Which migration would you like to demote? (q to cancel): '
  
  select opt in "${mig_index[@]}"; do
    case "${opt}" in
      "") echo "Invalid option or cancelled, exiting..." ; break ;;
      *)  mig_link_path="${mig_file[$opt]}" ; break ;;
    esac
  done
  if [[ "${mig_link_path}" ]]
  then
    print_heading "Demoting Migration"
    demote_mig_name="$(basename -- "${mig_link_path}")"
    demote_mig_from="${DB_PATH}/${DB}/${demote_mig_name}"

    print_unlinking "${mig_link_path}"
    rm "${mig_link_path}"
    print_moving "${demote_mig_from}" "${mig_link_path}"
    mv "${demote_mig_from}" "${mig_link_path}"
  fi
fi

OUTCOME="OK"
