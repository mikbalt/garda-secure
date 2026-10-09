# shellcheck shell=sh
# Shared helpers for infra/scripts/*.sh (sourced, POSIX sh).

# Directory of infra/ (scripts live in infra/scripts).
INFRA_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

die()  { echo "ERROR: $*" >&2; exit 1; }
warn() { echo "WARN:  $*" >&2; }
info() { echo "==> $*"; }

# load_env FILE — export the KEY=VALUE lines of a compose .env file WITHOUT
# evaluating it as shell code (values may contain spaces, <, >, *, ...).
# Like docker compose: ${NAME} references to earlier keys are expanded,
# surrounding quotes are removed, and variables already set in the
# environment win over the file.
load_env() {
  _le_file=$1
  [ -f "$_le_file" ] || die "missing $_le_file (run: make env SERVER_HOST=<name-or-ip>)"
  _le_cr=$(printf '\r')
  while IFS= read -r _le_line || [ -n "$_le_line" ]; do
    _le_line=${_le_line%"$_le_cr"}
    case $_le_line in ''|'#'*|' '*) continue ;; esac
    case $_le_line in *=*) ;; *) continue ;; esac
    _le_key=${_le_line%%=*}
    _le_val=${_le_line#*=}
    case $_le_key in ''|[0-9]*|*[!A-Za-z0-9_]*) continue ;; esac
    eval "_le_set=\${$_le_key+x}"
    [ -n "$_le_set" ] && continue
    case $_le_val in
      \'*\') _le_val=${_le_val#\'}; _le_val=${_le_val%\'}; export "$_le_key=$_le_val"; continue ;;
      \"*\") _le_val=${_le_val#\"}; _le_val=${_le_val%\"} ;;
    esac
    _le_out=
    while :; do
      case $_le_val in
        *'${'*'}'*)
          _le_pre=${_le_val%%'${'*}
          _le_rest=${_le_val#*'${'}
          _le_name=${_le_rest%%'}'*}
          _le_val=${_le_rest#*'}'}
          case $_le_name in
            ''|[0-9]*|*[!A-Za-z0-9_]*) _le_out="$_le_out$_le_pre\${$_le_name}" ;;
            *) _le_rep=; eval "_le_rep=\${$_le_name-}"; _le_out="$_le_out$_le_pre$_le_rep" ;;
          esac
          ;;
        *) _le_out="$_le_out$_le_val"; break ;;
      esac
    done
    export "$_le_key=$_le_out"
  done < "$_le_file"
}

# is_ipv4 VALUE
is_ipv4() {
  printf '%s' "$1" | grep -Eq '^((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])$'
}

# is_hostname VALUE (RFC 1123 DNS name, no trailing dot)
is_hostname() {
  printf '%s' "$1" | grep -Eq '^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$' \
    && [ "${#1}" -le 253 ]
}

# compose — docker compose run from infra/ (reads infra/.env, incl. COMPOSE_FILE).
compose() {
  (cd "$INFRA_DIR" && docker compose "$@")
}
