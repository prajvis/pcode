// SPDX-License-Identifier: Apache-2.0

package execx

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	RunStatusActive = "PCODE_RUN_ACTIVE"
	RunStatusIdle   = "PCODE_RUN_IDLE"
)

var runSlugRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func RunSlug(value string) string {
	value = strings.TrimSpace(value)
	value = runSlugRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "default"
	}
	return value
}

func RunFileName(alias, title string) string {
	return alias + "-" + RunSlug(title) + ".pid"
}

func killTreeBash() string {
	return `
pcode_descendants() {
  _root="$1"
  _pids="$_root"
  _changed=1
  while [ "$_changed" = 1 ]; do
    _changed=0
    _list=$(ps -eo pid=,ppid= 2>/dev/null)
    while read -r _cpid _cppid; do
      [ -n "$_cpid" ] || continue
      case " $_pids " in
        *" $_cpid "*) continue ;;
      esac
      case " $_pids " in
        *" $_cppid "*)
          _pids="$_pids $_cpid"
          _changed=1
          ;;
      esac
    done <<EOF
$_list
EOF
  done
  echo "$_pids"
}

pcode_kill_tree() {
  _root="$1"
  [ -n "$_root" ] || return 0
  _saved=$(pcode_descendants "$_root")
  _pgids=""
  for _p in $_saved; do
    _pgid=$(ps -o pgid= -p "$_p" 2>/dev/null | tr -d ' ')
    if [ -n "$_pgid" ]; then
      _pgids="$_pgids $_pgid"
      kill -TERM -- "-$_pgid" 2>/dev/null || true
    fi
    kill -TERM "$_p" 2>/dev/null || true
  done
  sleep 1
  _left=$(pcode_descendants "$_root")
  for _p in $_saved $_left; do
    _pgid=$(ps -o pgid= -p "$_p" 2>/dev/null | tr -d ' ')
    if [ -n "$_pgid" ]; then
      _pgids="$_pgids $_pgid"
      kill -KILL -- "-$_pgid" 2>/dev/null || true
    fi
    kill -KILL "$_p" 2>/dev/null || true
  done
  for _g in $_pgids; do
    kill -KILL -- "-$_g" 2>/dev/null || true
  done
}
`
}

func WrapScript(alias, title, command string) string {
	name := RunFileName(alias, title)
	cmd := BashQuote(command)
	var b strings.Builder
	fmt.Fprintf(&b, "runfile=\"$HOME/.pcode/runs/%s\"\n", name)
	b.WriteString(`mkdir -p "$HOME/.pcode/runs"
if [ -f "$runfile" ]; then
  oldpid=$(tr -d ' \t\n\r' < "$runfile")
  if [ -n "$oldpid" ] && { kill -0 -- "-$oldpid" 2>/dev/null || kill -0 "$oldpid" 2>/dev/null; }; then
    echo "ai run already in progress" >&2
    exit 1
  fi
  rm -f "$runfile"
fi
`)
	b.WriteString(killTreeBash())
	b.WriteString("setsid bash -lc ")
	b.WriteString(cmd)
	b.WriteString(` </dev/null 3<&- &
pid=$!
printf '%s\n' "$pid" > "$runfile"
cleanup() { rm -f "$runfile"; }
trap cleanup EXIT
exec 3<&0
trap 'pcode_kill_tree "$pid"' USR1
(
  exec >/dev/null 2>&1
  cat <&3 >/dev/null
  kill -USR1 $$ 2>/dev/null || true
) >/dev/null 2>&1 &
wdpid=$!
wait "$pid"
st=$?
while [ "$st" -eq 138 ] && kill -0 "$pid" 2>/dev/null; do
  wait "$pid"
  st=$?
done
if [ -n "$wdpid" ]; then
  for _c in $(ps -o pid= --ppid "$wdpid" 2>/dev/null); do
    kill -KILL "$_c" 2>/dev/null || true
  done
  kill -TERM "$wdpid" 2>/dev/null || true
  kill -KILL "$wdpid" 2>/dev/null || true
fi
exit "$st"
`)
	return b.String()
}

func IsRunningScript(alias, title string) string {
	name := RunFileName(alias, title)
	return fmt.Sprintf(`runfile="$HOME/.pcode/runs/%s"
if [ -f "$runfile" ]; then
  oldpid=$(tr -d ' \t\n\r' < "$runfile")
  if [ -n "$oldpid" ] && { kill -0 -- "-$oldpid" 2>/dev/null || kill -0 "$oldpid" 2>/dev/null; }; then
    echo %s
    exit 0
  fi
fi
echo %s
`, name, RunStatusActive, RunStatusIdle)
}

func KillScript(alias, title string) string {
	name := RunFileName(alias, title)
	return killTreeBash() + fmt.Sprintf(`runfile="$HOME/.pcode/runs/%s"
if [ ! -f "$runfile" ]; then
  echo "no running ai job"
  exit 0
fi
pid=$(tr -d ' \t\n\r' < "$runfile")
if [ -z "$pid" ] || { ! kill -0 -- "-$pid" 2>/dev/null && ! kill -0 "$pid" 2>/dev/null; }; then
  rm -f "$runfile"
  echo "no running ai job"
  exit 0
fi
echo "sending SIGTERM to process group $pid"
pcode_kill_tree "$pid"
echo "sending SIGKILL to process group $pid"
rm -f "$runfile"
echo "stopped process group $pid"
`, name)
}
