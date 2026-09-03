#!/usr/bin/env bash
# `make ui-test`: fake AWS, seeded, with the real TUI driven through ttyd by Playwright.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

moto_image="${MOTO_IMAGE:-motoserver/moto:5.2.2}"
moto_port="${MOTO_PORT:-5055}"
ttyd_port="${TTYD_PORT:-7681}"
container="lazyaws-ui-test-moto"
# An IP, never a hostname: the SDK addresses an S3 bucket as a subdomain of a named host, and moto has no wildcard DNS.
endpoint="http://127.0.0.1:$moto_port"
region="${AWS_REGION:-eu-west-1}"

missing=()
for tool in docker ttyd aws curl; do
	command -v "$tool" >/dev/null || missing+=("$tool")
done
runtime="$(command -v bun || command -v node || true)"
[ -n "$runtime" ] || missing+=("bun or node")
if [ ${#missing[@]} -gt 0 ]; then
	echo "make ui-test needs: ${missing[*]}" >&2
	echo "install them (brew install ttyd; docker; awscli; bun) and run 'cd test/ui && bun install && bunx playwright install chromium'" >&2
	exit 1
fi
if [ ! -d "$here/node_modules/playwright" ]; then
	echo "playwright is not installed: cd test/ui && bun install && bunx playwright install chromium" >&2
	exit 1
fi

ttyd_pid=""
# A temporary HOME so the run reads the harness's own AWS profile and lazyaws config instead of the operator's.
fake_home="$(mktemp -d)"
cleanup() {
	[ -n "$ttyd_pid" ] && kill "$ttyd_pid" 2>/dev/null || true
	docker rm -f "$container" >/dev/null 2>&1 || true
	rm -rf "$fake_home"
}
trap cleanup EXIT HUP INT TERM

docker rm -f "$container" >/dev/null 2>&1 || true
docker run -d --name "$container" -p "$moto_port:5000" "$moto_image" >/dev/null
# Real ECS answers DescribeTasks with container cpu/memory as STRINGS; moto (5.2.2 and latest, checked 2026-08-28) echoes the registration integers, which aws-sdk-go-v2 strict-decoding rejects ("expected String, got json.Number") and every task-reading pane renders as unavailable.
# Patched here rather than tolerated in the app: the app's decoding matches the real API, and the fake is what is wrong. Drop when moto stringifies Container.cpu/memory upstream.
for _ in $(seq 1 20); do
	docker exec "$container" sed -i \
		-e 's/self.cpu = container_def.get("cpu")/self.cpu = str(container_def["cpu"]) if container_def.get("cpu") is not None else None/' \
		-e 's/self.memory = container_def.get("memory")/self.memory = str(container_def["memory"]) if container_def.get("memory") is not None else None/' \
		/moto/moto/ecs/models.py 2>/dev/null && break
	sleep 0.5
done
docker restart "$container" >/dev/null
for _ in $(seq 1 60); do
	curl -fsS "$endpoint/moto-api/" >/dev/null 2>&1 && break
	sleep 0.5
done
curl -fsS "$endpoint/moto-api/" >/dev/null || { echo "$moto_image did not come up on $endpoint" >&2; exit 1; }

AWS_ENDPOINT_URL="$endpoint" AWS_REGION="$region" bash "$here/seed.sh"

# AWS_PROFILE below and this section have to name the same profile, or the app starts degraded and loads nothing.
profile=ui-harness
mkdir -p "$fake_home/.aws"
cat >"$fake_home/.aws/config" <<CONF
[profile $profile]
region = $region
CONF

# One non-default movement chord makes the keys journey prove config reaches navigation handlers.
mkdir -p "$fake_home/.config/lazyaws"
cat >"$fake_home/.config/lazyaws/config.yml" <<CONF
keybindings:
  nav-down: n
CONF

go build -o "$here/.lazyaws" "$repo"

# ttyd owns the pty; the browser viewport is what sizes it (see harness.mjs).
# The journeys need --allow-writes: they drive the actions menus, which a default run does not offer because lazyaws starts read-only.
# The demo recording deliberately runs without it. It only browses, so the published GIF shows the [read-only] badge a first run really carries rather than a mode nobody starts in.
app_flags=--allow-writes
if [ "${DRIVER:-}" = demo.mjs ]; then
	app_flags=
fi

# XDG_CONFIG_HOME pinned because os.UserConfigDir answers ~/Library/Application Support on macOS, and the rebind config written above must resolve on every platform the harness runs on.
env -i HOME="$fake_home" XDG_CONFIG_HOME="$fake_home/.config" PATH="$PATH" TERM=xterm-256color \
	AWS_ENDPOINT_URL="$endpoint" AWS_REGION="$region" AWS_PROFILE="$profile" \
	AWS_ACCESS_KEY_ID=lazyaws-ui-test AWS_SECRET_ACCESS_KEY=lazyaws-ui-test \
	ttyd --writable --port "$ttyd_port" --interface 127.0.0.1 "$here/.lazyaws" ${app_flags} \
	>"$here/.ttyd.log" 2>&1 &
ttyd_pid=$!
for _ in $(seq 1 40); do
	curl -fsS "http://127.0.0.1:$ttyd_port" >/dev/null 2>&1 && break
	sleep 0.25
done
# Said here rather than left to the browser: a port already in use fails ttyd on startup, and "net::ERR_CONNECTION_REFUSED" does not name that.
curl -fsS "http://127.0.0.1:$ttyd_port" >/dev/null || { echo "ttyd did not come up on port $ttyd_port; see $here/.ttyd.log" >&2; exit 1; }

# The endpoint reaches the journeys too: proving a refresh key reached AWS means changing what AWS answers, which only the fake one may be asked to do.
# The dummy credentials go with it. Without them the CLI a journey shells out to has none of its own — HOME here is the operator's — and the call fails, or worse, signs with whatever profile the operator happens to have.
# DRIVER swaps the journey runner for another script over the same live stack; demo.mjs records the README GIF's frames this way.
TTYD_URL="http://127.0.0.1:$ttyd_port" AWS_ENDPOINT_URL="$endpoint" AWS_REGION="$region" \
	AWS_ACCESS_KEY_ID=lazyaws-ui-test AWS_SECRET_ACCESS_KEY=lazyaws-ui-test \
	"$runtime" "$here/${DRIVER:-run.mjs}" "$@"
