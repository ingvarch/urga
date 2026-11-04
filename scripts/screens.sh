#!/bin/sh
# Takes the screens of urga that the README shows, docs/images/jobs.png and
# docs/images/client.png, from a cluster of its own: one Nomad agent in dev
# mode running the jobs in scripts/screens. urga runs in a terminal of a fixed
# size, and what the terminal shows is drawn into a picture.
#
# Needs go, nomad, tmux, curl and freeze. Takes about seven minutes: the
# charts of the client fill for six. CHART_WAIT sets that wait in seconds.
set -eu

cd "$(dirname "$0")/.."

cols=170
rows=43
addr=http://127.0.0.1:24646
chart_wait=${CHART_WAIT:-380}

work=$(mktemp -d)
tmux="tmux -L urga-screens -f /dev/null"
agent=

cleanup() {
	$tmux kill-server 2>/dev/null || true
	if [ -n "$agent" ]; then
		kill "$agent" 2>/dev/null || true
		wait "$agent" 2>/dev/null || true
	fi
	rm -rf "$work"
}
trap cleanup EXIT INT TERM

# until_ok runs a check every second until it passes, for at most a minute.
until_ok() {
	what=$1
	shift
	tries=0
	until "$@" >/dev/null 2>&1; do
		tries=$((tries + 1))
		if [ "$tries" -ge 60 ]; then
			echo "screens: gave up waiting for $what" >&2
			exit 1
		fi
		sleep 1
	done
}

shows() {
	$tmux capture-pane -p -t urga | grep -q "$1"
}

# shoot draws what the terminal shows into a picture. freeze draws with the
# font it carries only when rsvg-convert is not on the PATH: rsvg-convert
# ignores that font and draws the columns out of line.
shoot() {
	$tmux capture-pane -e -p -t urga >"$work/screen.ansi"
	env PATH=/usr/bin:/bin "$freeze" --language ansi \
		--output "$work/screen.png" --background "#1a1a26" --window=false \
		--border.radius 0 --margin 0 --padding 20 --font.size 14 \
		<"$work/screen.ansi" >/dev/null
	if command -v sips >/dev/null 2>&1; then
		sips --resampleWidth 2000 "$work/screen.png" --out "$1" >/dev/null
	else
		cp "$work/screen.png" "$1"
	fi
	echo "screens: wrote $1"
}

freeze=$(command -v freeze)

go build -o "$work/urga" ./cmd/urga

nomad agent -dev -bind 127.0.0.1 -config scripts/screens/agent.hcl >"$work/agent.log" 2>&1 &
agent=$!

export NOMAD_ADDR=$addr
until_ok "a ready client" sh -c "curl -sf $addr/v1/nodes | grep -q '\"Status\":\"ready\"'"

nomad namespace apply prod >/dev/null
nomad namespace apply staging >/dev/null
for job in scripts/screens/*.nomad.hcl; do
	nomad job run -detach "$job" >"$work/run.log" 2>&1 || {
		cat "$work/run.log" >&2
		exit 1
	}
done
until_ok "the failed allocation" sh -c "curl -sf '$addr/v1/allocations?namespace=*' | grep -q '\"ClientStatus\":\"failed\"'"

# urga gets an empty home, so the settings of whoever runs this do not change
# the pictures.
mkdir "$work/home"
$tmux new-session -d -s urga -x "$cols" -y "$rows" \
	"env -i PATH=/usr/bin:/bin HOME=$work/home TERM=xterm-256color COLORTERM=truecolor NOMAD_ADDR=$addr $work/urga"

until_ok "the jobs" shows "Jobs (all) \[6\]"
shoot docs/images/jobs.png

$tmux send-keys -t urga :
$tmux send-keys -t urga clients Enter
until_ok "the client list" shows "client-01"
$tmux send-keys -t urga Enter
until_ok "the client" shows "Client client-01"

# The newest allocation is listed first. Two rows down is one of web, the job
# that uses the CPU.
$tmux send-keys -t urga Down Down

echo "screens: waiting ${chart_wait}s for the charts to fill"
sleep "$chart_wait"
shoot docs/images/client.png
