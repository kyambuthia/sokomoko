#!/usr/bin/env bash
# Create the IAM role a trunk workflow assumes to push the demo image and
# restart the demo instance. Safe to run again: it updates the trust policy
# and the inline permissions.
#
# The default akyamz credentials cannot manage IAM. Sign in with the
# root-account profile first:
#
#   aws login --profile root-account
#   AWS_PROFILE=root-account bash deploy/github-oidc-role.sh
set -euo pipefail

account=139023234259
region=eu-west-1
provider_host=token.actions.githubusercontent.com
provider_arn="arn:aws:iam::${account}:oidc-provider/${provider_host}"
role_name=sokomoko-github-deploy
role_arn="arn:aws:iam::${account}:role/${role_name}"
instance_id=i-0bec90e7573e98d76
repository=asiliano

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
umask 077

if ! aws iam get-open-id-connect-provider --open-id-connect-provider-arn "$provider_arn" >/dev/null 2>&1; then
  aws iam create-open-id-connect-provider \
    --url "https://${provider_host}" \
    --client-id-list sts.amazonaws.com \
    --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1 1c58a3a8518e8759bf075b76b750d4f2df264fcd >/dev/null
  printf 'created OIDC provider %s\n' "$provider_arn"
else
  printf 'OIDC provider already exists\n'
fi

# Repositories created before 15 July 2026 keep the original subject claim
# unless immutable subjects are turned on. Accept both forms for trunk only.
cat >"$tmp/trust.json" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "${provider_arn}"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
          "token.actions.githubusercontent.com:sub": [
            "repo:kyambuthia/sokomoko:ref:refs/heads/trunk",
            "repo:kyambuthia@191156343/sokomoko@909808540:ref:refs/heads/trunk"
          ]
        }
      }
    }
  ]
}
EOF

if aws iam get-role --role-name "$role_name" >/dev/null 2>&1; then
  aws iam update-assume-role-policy --role-name "$role_name" --policy-document "file://${tmp}/trust.json" >/dev/null
  printf 'updated trust policy on %s\n' "$role_arn"
else
  aws iam create-role \
    --role-name "$role_name" \
    --assume-role-policy-document "file://${tmp}/trust.json" \
    --description "GitHub Actions deploys the Sokomoko trunk demo" \
    --max-session-duration 3600 >/dev/null
  printf 'created %s\n' "$role_arn"
fi

# GetCommandInvocation does not support a resource ARN, so that action is *.
cat >"$tmp/deploy.json" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EcrAuth",
      "Effect": "Allow",
      "Action": "ecr:GetAuthorizationToken",
      "Resource": "*"
    },
    {
      "Sid": "PushDemoImage",
      "Effect": "Allow",
      "Action": [
        "ecr:BatchCheckLayerAvailability",
        "ecr:BatchGetImage",
        "ecr:CompleteLayerUpload",
        "ecr:DescribeImages",
        "ecr:InitiateLayerUpload",
        "ecr:PutImage",
        "ecr:UploadLayerPart"
      ],
      "Resource": "arn:aws:ecr:${region}:${account}:repository/${repository}"
    },
    {
      "Sid": "RestartDemo",
      "Effect": "Allow",
      "Action": "ssm:SendCommand",
      "Resource": [
        "arn:aws:ec2:${region}:${account}:instance/${instance_id}",
        "arn:aws:ssm:${region}::document/AWS-RunShellScript"
      ]
    },
    {
      "Sid": "ReadRestartResult",
      "Effect": "Allow",
      "Action": [
        "ssm:GetCommandInvocation"
      ],
      "Resource": "*"
    }
  ]
}
EOF

aws iam put-role-policy \
  --role-name "$role_name" \
  --policy-name deploy \
  --policy-document "file://${tmp}/deploy.json" >/dev/null
printf 'role ready: %s\n' "$role_arn"
