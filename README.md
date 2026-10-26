# Advertiser

Monitors public iOS and Android store versions and posts release notes to Telegram subscribers.

`monitor/start` runs on the existing DigitalOcean Functions schedule. It reads the app lists and saved versions from Firebase Realtime Database, checks the Ukrainian storefront, and sends a notification when the version increases. iOS uses iTunes Lookup; Android reads the JSON embedded in the public Google Play page (`gl=UA`, `hl=uk`), with no Google Play credentials or scraper service.

`monitor/subscriber` handles the Telegram webhook. Add **@cropwiseappsbot** to a chat and send `/subscribe` or `/unsubscribe`.

## Configure monitored apps

The database uses these paths:

| Path | Value |
| --- | --- |
| `/apps` | Existing array of iOS bundle IDs |
| `/versions` | Existing iOS version map |
| `/androidApps` | Array of Android package names |
| `/androidVersions` | Android version map, maintained by the monitor |
| `/subscribers` | Shared array of Telegram chat IDs |

For example, set **only `/androidApps`** to this array in the Firebase console (do not replace the database root):

```json
[
  "com.nst.cropio",
  "com.nst.cropio.telematics",
  "com.nst.cropio.yield",
  "com.nst.cropio.weighings"
]
```

If `/androidApps` is absent, Android monitoring is disabled. The first successful check of a newly added app saves its current version without sending a notification. Later checks announce only higher versions. Existing iOS data requires no migration. Keys in both version maps omit dots, matching the existing iOS format.

The Android parser checks the package identity and requires a readable name and version. Missing data, `Varies with device`, HTTP errors, and versions unsupported by the existing semantic-version comparison are skipped without changing saved state. Other apps continue to be checked. Missing release notes are allowed; the update date is never used as a version fallback.

This observes the version advertised by the public Ukrainian storefront, not availability to every individual device or rollout cohort. Google can change the page format, so parsing failures appear in the function logs and are retried at the next scheduled run.

Versions are saved before sending notifications. If saving fails, that app is skipped for the current run. A failed Telegram send does not stop attempts to the remaining subscribers or trigger a repeat broadcast on the next run. Failed deliveries are logged and are not retried automatically.

## Configure secrets once

`project.yml` explicitly injects these runtime variables into both functions:

- `TLGRM_BOT_TOKEN`: the existing Telegram bot token.
- `GCP_CREDS_JSON_BASE64`: the existing Firebase service-account JSON, base64 encoded on one line.
- `FIREBASE_DATABASE_URL`: the Firebase Realtime Database URL for the deployment environment. Both functions require it; no database address is embedded in the code.

Keep the environment file outside repositories and temporary workspaces. Create it once:

```sh
mkdir -p "$HOME/.config/advertiser"
chmod 700 "$HOME/.config/advertiser"
cp -n .env.example "$HOME/.config/advertiser/production.env"
chmod 600 "$HOME/.config/advertiser/production.env"
```

Fill in the three values in `~/.config/advertiser/production.env`. The same file is reused by every checkout and survives removal of a temporary workspace. Keep one `NAME=value` assignment per line; the deploy script loads it as a shell file. Do not commit credentials or put their literal values in `project.yml`.

DigitalOcean deploys the configuration from `project.yml`; variables entered only in the dashboard should not be relied on to survive deployment. The `${VARIABLE}` references restore the values from the same local environment file on every deploy. No new service account is needed.

## Deploy

With `doctl` authenticated and connected to the intended Functions namespace, run:

```sh
./scripts/deploy.sh
```

The script automatically reads `~/.config/advertiser/production.env`, checks that both secrets and the database URL are nonempty before invoking `doctl`, then deploys using a remote build. It never prints the secret values. You can explicitly select another environment file:

```sh
./scripts/deploy.sh /path/to/another/environment.env
```

The underlying command is:

```sh
doctl serverless deploy . --env /path/to/production.env --remote-build
```

Use the script for the missing-secret checks and automatic reuse of the permanent environment file. The values only need to be entered once, or when rotated.

See DigitalOcean's [environment templating documentation](https://docs.digitalocean.com/products/functions/reference/project-configuration/#templating) and [deploy command reference](https://docs.digitalocean.com/reference/doctl/reference/serverless/deploy/).

## Verify

Offline tests use a reduced public-page fixture and local HTTP servers. They do not access Firebase or send real Telegram messages:

```sh
cd packages/monitor/start
go test ./...
```

To check all four public Android listings without database writes or notifications:

```sh
LIVE_GOOGLE_PLAY=1 go test -run '^TestGooglePlayLive$' -v
```
