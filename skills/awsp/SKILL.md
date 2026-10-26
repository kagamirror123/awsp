---
name: awsp
description: Reach AWS accounts through the profiles in the local AWS config with awsp. Find the profile for an account or role, pass it to each aws command, and log in again when the SSO session has expired. Use whenever a task runs the aws CLI or SDK-based tools against an AWS account, or an AWS command fails with an SSO or credentials error.
---

# awsp

Each profile in the local AWS config reaches one account with one role.
awsp reads the config and the SSO token cache, and runs the browser login when a session has expired.
The human switches profiles in their own shell. You pick a profile for each command instead.

Use the awsp MCP tools when they are connected. Otherwise run the CLI; with `--json` it prints the same fields as the tools.

| Need | MCP tool | CLI |
|---|---|---|
| Find the profile for an account or role | `list_profiles` | `awsp list --json` |
| Check whether the SSO sessions are valid (local files, no network) | `auth_status` | `awsp status --json` |
| Confirm the account and role behind a profile | `whoami` | `awsp whoami <profile> --json` |
| Log in after the session has expired | `login` | `awsp login <profile> --json` |

## Pick a profile

- Match the account ID, role, or name from the task against `ssoAccountId`, `ssoRoleName` (or the account in `roleArn`), and `name` in the profile list.
- If the task does not say which account, use `currentProfile`, the profile the human selected in their shell. It is missing when you were not started from that shell; ask the human then.
- Look at the list before concluding that an account is unreachable. Do not ask the human to run read-only commands that one of the listed profiles can run.
- If no profile reaches the account, tell the human which account is missing. `configFile` shows which AWS config you read; it can be a different file from the human's.

## Run commands

- Pass `--profile <name>` to every `aws` command.
- For tools that take no `--profile` (Terraform, CDK, SDK scripts), prefix the command, as in `AWS_PROFILE=<name> terraform plan`.
- Set the profile on every command. Environment variables may not carry over from one of your commands to the next.
- Do not run `awsp <profile>` to switch. Switching is for the human's interactive shell.

## When the session has expired

Errors saying that the SSO session or token has expired or is invalid (for example `Token has expired and refresh failed`, `Session token not found or invalid`, `InvalidGrantException`) mean the human has to approve a new login in the browser.

1. Call `login` with the profile. It opens the browser on this machine and returns once the human approves.
2. If it returns `status: "pending"`, show `authorizationUrl` to the human when it is present, then call `login` again with the same profile. It joins the login already in progress instead of starting another one.
3. Run the failed command again.

Use this rather than `aws sso login`: the tool hands the URL back to you when the browser does not open, and never starts a second login for the same session.

With the CLI, `awsp login <profile> --json` blocks until the approval, for up to 5 minutes by default (`awsp login --timeout=<duration>` changes it). Give the command a longer timeout than that. If the browser cannot open, the URL is printed on stderr; run the command in the background if you can, so that you can show the URL while it waits.

On a remote or headless machine, the browser cannot reach the local callback. Pass `use_device_code: true` to `login` (or run `awsp login <profile> --use-device-code`) so the human can approve on any device with a code. Some organizations turn this off.

Do not use `awsp login --force`. It makes the human approve again while the session is still valid.

`auth_status` makes no network calls, so it is a cheap check before a long task. When `autoRefresh` is true, a short `remainingSeconds` is normal and needs no login.

## Boundaries

- The role in the config you were given is the permission boundary. When a command fails with AccessDenied, tell the human which profile and action failed. Do not retry with a more privileged profile, change `AWS_CONFIG_FILE`, or edit the AWS config unless the human asks.
- Do not read files under `~/.aws/sso/cache` or `~/.aws/cli/cache`, and do not export credentials (`aws configure export-credentials`). awsp never outputs token values.
- `awsp console [profile] [url]` opens the AWS console in the human's browser with that account and role. Suggest it when the human wants to look at something there; do not run it on your own.
