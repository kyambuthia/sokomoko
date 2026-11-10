#!/usr/bin/env bash
# Point the Asiliano demo at one image digest and restart its unit.
# The unit pulls that digest, migrates, and starts the app.
#
# Usage: deploy/rollout-demo.sh <registry>/<repository>@sha256:<digest>
set -euo pipefail

image=${1:?image reference pinned by digest}
region=${AWS_REGION:-eu-west-1}
instance=${DEPLOY_INSTANCE_ID:-i-0bec90e7573e98d76}
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)

case "$image" in
  *@sha256:*) ;;
  *) printf 'Image must be pinned by digest, for example registry/asiliano@sha256:...\n' >&2; exit 2 ;;
esac
# The reference is embedded in the remote command. Keep it to the ECR alphabet.
case "$image" in
  *[!A-Za-z0-9:._/@-]*) printf 'Image reference has unexpected characters\n' >&2; exit 2 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

tar -C "$root/deploy" -czf "$tmp/deploy.tgz" \
  Caddyfile compose.production.yml compose.ec2-demo.yml postgres-init-app-role.sh sokomoko-demo-deploy
files_b64=$(base64 -w0 "$tmp/deploy.tgz")

# One remote command: refresh the Compose files, pin the digest, restart.
# .env.production and .env.seed stay on the instance.
remote=$(cat <<EOF
set -euo pipefail
install -d -m 0700 /opt/sokomoko /opt/sokomoko/deploy
printf '%s' '${files_b64}' | base64 -d | tar -xz -C /opt/sokomoko/deploy
chmod 0755 /opt/sokomoko/deploy/postgres-init-app-role.sh
install -m 0755 /opt/sokomoko/deploy/sokomoko-demo-deploy /usr/local/bin/sokomoko-demo-deploy
IMAGE='${image}' python3 - <<'PY'
import os
from pathlib import Path
p = Path("/opt/sokomoko/deploy/.env.production")
image = os.environ["IMAGE"]
lines = p.read_text().splitlines() if p.exists() else []
found = False
for i, line in enumerate(lines):
    if line.startswith("SOKOMOKO_IMAGE="):
        lines[i] = "SOKOMOKO_IMAGE=" + image
        found = True
        break
if not found:
    lines.append("SOKOMOKO_IMAGE=" + image)
p.write_text("\\n".join(lines) + "\\n")
p.chmod(0o600)
print("release digest updated")
PY
cd /opt/sokomoko
docker compose --env-file deploy/.env.production -f deploy/compose.production.yml -f deploy/compose.ec2-demo.yml config --quiet
sudo systemctl restart sokomoko-demo.service
sudo systemctl show sokomoko-demo.service -p ActiveState -p SubState -p Result
EOF
)

python3 - "$remote" "$tmp/params.json" <<'PY'
import json
import sys
from pathlib import Path
remote, dest = sys.argv[1], sys.argv[2]
Path(dest).write_text(json.dumps({"commands": [remote]}))
PY

command_id=$(aws ssm send-command \
  --region "$region" \
  --instance-ids "$instance" \
  --document-name AWS-RunShellScript \
  --comment "Roll Sokomoko demo" \
  --timeout-seconds 900 \
  --parameters "file://${tmp}/params.json" \
  --query 'Command.CommandId' \
  --output text)
printf 'ssm command %s\n' "$command_id"

for _ in $(seq 1 90); do
  status=$(aws ssm get-command-invocation \
    --region "$region" \
    --command-id "$command_id" \
    --instance-id "$instance" \
    --query 'Status' \
    --output text 2>/dev/null || true)
  case "$status" in
    Success)
      aws ssm get-command-invocation \
        --region "$region" \
        --command-id "$command_id" \
        --instance-id "$instance" \
        --query 'StandardOutputContent' \
        --output text
      exit 0
      ;;
    Failed|Cancelled|TimedOut|Cancelling)
      aws ssm get-command-invocation \
        --region "$region" \
        --command-id "$command_id" \
        --instance-id "$instance" \
        --query '[StandardOutputContent, StandardErrorContent]' \
        --output text || true
      printf 'demo rollout %s\n' "$status" >&2
      exit 1
      ;;
  esac
  sleep 10
done

printf 'timed out waiting for ssm command %s\n' "$command_id" >&2
exit 1
