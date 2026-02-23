# Setup Guide

**📚 Documentation:** [README](../README.md) | [Setup](setup.md) | [Architecture](architecture.md) | [Operations](operations.md) | [Testing](testing_guide.md)

---

This guide covers everything you need to set up, configure, and deploy the EOS Voice RTC service.

## Prerequisites

### 1. Development Tools

You'll need the following tools installed on Windows 11 WSL2, Linux Ubuntu 22.04, or macOS 14+:

#### a. Bash

- On Windows WSL2 or Linux Ubuntu:

  ```
  bash --version

  GNU bash, version 5.1.16(1)-release (x86_64-pc-linux-gnu)
  ...
  ```

- On macOS:

  ```
  bash --version

  GNU bash, version 3.2.57(1)-release (arm64-apple-darwin23)
  ...
  ```

#### b. Make

- On Windows WSL2 or Linux Ubuntu:

  To install from the Ubuntu repository, run `sudo apt update && sudo apt install make`.

  ```
  make --version

  GNU Make 4.3
  ...
  ```

- On macOS:

  ```
  make --version

  GNU Make 3.81
  ...
  ```

#### c. Docker (Docker Desktop 4.30+/Docker Engine v23.0+)

- On Linux Ubuntu:

  1. To install from the Ubuntu repository, run `sudo apt update && sudo apt install docker.io docker-buildx docker-compose-v2`.
  2. Add your user to the `docker` group: `sudo usermod -aG docker $USER`.
  3. Log out and log back in to allow the changes to take effect.

- On Windows or macOS:

  Follow Docker's documentation on installing the Docker Desktop on [Windows](https://docs.docker.com/desktop/install/windows-install/) or [macOS](https://docs.docker.com/desktop/install/mac-install/).

  ```
  docker version

  ...
  Server: Docker Desktop
     Engine:
     Version:          24.0.5
  ...
  ```

#### d. Go v1.24

- Follow [Go's installation guide](https://go.dev/doc/install).

  ```
  go version

  go version go1.24.0 ...
  ```

#### e. Postman

- Use binary available [here](https://www.postman.com/downloads/)

#### f. extend-helper-cli

- Use the available binary from [extend-helper-cli](https://github.com/AccelByte/extend-helper-cli/releases).

> :exclamation: In macOS, you may use [Homebrew](https://brew.sh/) to easily install some of the tools above.

---

### 2. AccelByte Gaming Services (AGS) Setup

#### a. Base URL

- Sample URL for AGS Shared Cloud customers: `https://spaceshooter.prod.gamingservices.accelbyte.io`
- Sample URL for AGS Private Cloud customers:  `https://dev.accelbyte.io`

#### b. Create a Game Namespace

[Create a Game Namespace](https://docs.accelbyte.io/gaming-services/services/access/reference/namespaces/manage-your-namespaces/) if you don't have one yet. Keep the `Namespace ID`. Make sure this namespace is in active status.

#### c. Create OAuth Client for the Extend Service

[Create an OAuth Client for the Extend Service](https://docs.accelbyte.io/gaming-services/services/access/authorization/manage-access-control-for-applications/#create-an-iam-client) with confidential client type. This client is used by the service itself to validate tokens and call AGS APIs. Keep the `Client ID` and `Client Secret`.

**Required Permissions** (for both Private and Shared Cloud):

- For AGS Private Cloud customers:
  - `ADMIN:ROLE [READ]` to validate access token and permissions
  - `ADMIN:NAMESPACE:{namespace}:NAMESPACE [READ]` to validate access namespace
  - `ADMIN:NAMESPACE:{namespace}:SESSION [READ]` to validate game sessions
  - `ADMIN:NAMESPACE:{namespace}:PARTY [READ]` to validate parties
  - `ADMIN:NAMESPACE:{namespace}:NOTIFICATION [CREATE]` to send freeform notifications

- For AGS Shared Cloud customers:
  - IAM -> Roles (Read)
  - Basic -> Namespace (Read)
  - Session -> Game Session (Read)
  - Session -> Admin Party (Read)
  - Lobby -> Notification (Create)

---

### 3. Epic Games Developer Portal Setup

#### a. Create Epic Games Organization and Product

Create an Epic Games organization and product if you don't have one.

#### b. Create OAuth Client

[Create an OAuth Client](https://dev.epicgames.com/docs/epic-account-services/auth/auth-interface) with client credentials grant type. Keep the `Client ID` and `Client Secret`. Make sure voice and connect permission enabled.

#### c. Get Deployment ID

Get your `Deployment ID` from the Epic Games Developer Portal for the RTC product.

#### d. Account Linking Setup

This service supports two Epic integration models. Choose the one that matches your game's authentication flow:

##### **Option 1: EOS Connect (For games using EOS SDK with AccelByte authentication)**

If your game uses **Epic Online Services (EOS) SDK** and players authenticate via AccelByte, set up EOS Connect to link AccelByte → Epic:

**1. Configure AccelByte as OpenID Provider in Epic Developer Portal:**

- Open your product in the EOS Developer Portal: Product Settings → Identity Providers → Add Identity Provider.
- Fill the form with:
  - Identity Provider: OpenID
  - Description: AccelByte
  - Type: UserInfo Endpoint
  - UserInfo API Endpoint: https://<studio>.prod.gamingservices.accelbyte.io/iam/v3/public/users/me
  - HTTP Method: GET
  - AccountId: userId
  - DisplayName: displayName

**2. Implement Account Linking in Your Game:**

- Use the EOS Connect SDK to link users during login/registration
- Call `EOS_Connect_Login` with OpenID token from AccelByte IAM
- Epic will associate the AccelByte user ID with the Epic PUID

**3. Call Service Without PUID:**

- Omit `puid` parameter when calling token endpoints
- The service will automatically query Epic Connect API to resolve PUID
- If linking fails, you'll receive error code `40303` (Epic account not linked)

For detailed instructions, see:
- [Epic Connect Documentation](https://dev.epicgames.com/docs/game-services/connect)
- [EOS Connect OpenID Integration](https://dev.epicgames.com/docs/game-services/connect/connect-interface#openid-connect)

##### **Option 2: Epic Account Services (For games using Epic Launcher / EAS authentication)**

If your game uses **Epic Account Services (EAS)** or distributes via Epic Games Launcher:

**1. Link Epic Accounts to AccelByte:**

- Players authenticate via Epic first (Epic is primary identity)
- Link Epic account to AccelByte using [AccelByte Platform Linking](https://docs.accelbyte.io/gaming-services/services/access/iam/how-to/platform-accounts/link-platform-accounts/)
- This is the reverse direction: Epic → AccelByte

**2. Pass PUID Explicitly:**

- Your game client obtains the Epic PUID after Epic authentication
- Include `puid` parameter in all token requests: `POST /public/party/{party_id}/token?puid={epic_puid}`
- The service **cannot** query Epic to resolve PUID in this flow (EAS doesn't support this lookup)

**3. Benefits:**

- Reduces API calls to Epic (no PUID lookup needed)
- Better performance (one less API call per token request)
- Works with Epic Launcher authentication flow

##### **Which Option Should You Choose?**

- **Option 1 (EOS Connect)**: Games using EOS SDK, AccelByte as primary auth, want automatic PUID resolution
- **Option 2 (EAS + explicit PUID)**: Games using Epic Launcher/EAS, Epic as primary auth, reduces Epic API calls

**Account Linking Direction:**
- **EOS Connect (Option 1)**: AccelByte → Epic (AccelByte is primary identity)
- **EAS (Option 2)**: Epic → AccelByte (Epic is primary identity)

---

### 4. OAuth Client for Game Client/Server

Your game client or game server needs an OAuth client to call this service's endpoints. The required permissions depend on which endpoints you plan to use.

#### a. For Public Endpoints Only (User token - works in Shared & Private Cloud)

- Create an OAuth client with `password` grant type for user authentication
- No special permissions required beyond basic user permissions
- Users can call: `POST /public/party/{party_id}/token`, `POST /public/session/{session_id}/token`

#### b. For Admin Endpoints (Admin token - **Private Cloud Only**)

- Create an OAuth client with `client_credentials` grant type
- Add the following custom permissions:
  - `ADMIN:NAMESPACE:{namespace}:VOICE [CREATE]` - for `POST /admin/session/{session_id}/token`
  - `ADMIN:NAMESPACE:{namespace}:VOICE [DELETE]` - for `POST /admin/room/{room_id}/token/revoke`

> :warning: **Admin endpoints require AGS Private Cloud**: Custom permissions like `VOICE [CREATE]` and `VOICE [DELETE]` cannot be created in AGS Shared Cloud. Admin endpoints will only work in Private Cloud environments. If you're using Shared Cloud, use public endpoints instead.

---

## Environment Configuration

To be able to run this app, you will need to follow these setup steps.

### 1. Create Environment File

Create a docker compose `.env` file by copying the content of [.env.template](../.env.template) file.

> :warning: **The host OS environment variables have higher precedence compared to `.env` file variables**:
> If the variables in `.env` file do not seem to take effect properly, check if there are host OS environment variables with the same name. 
> See documentation about [docker compose environment variables precedence](https://docs.docker.com/compose/how-tos/environment-variables/envvars-precedence/) for more details.

### 2. Configure Environment Variables

Fill in the required environment variables in `.env` file as shown below.

```bash
# AccelByte Configuration
AB_BASE_URL=https://test.accelbyte.io         # Your AGS environment Base URL
AB_CLIENT_ID=xxxxxxxxxx                       # Service OAuth Client ID
AB_CLIENT_SECRET=xxxxxxxxxx                   # Service OAuth Client Secret
AB_NAMESPACE=xxxxxxxxxx                       # Namespace ID
PLUGIN_GRPC_SERVER_AUTH_ENABLED=true          # Enable auth validation (set false for dev only)
BASE_PATH=/eos-voice                          # Base path for the service endpoints

# Epic Games Configuration
EPIC_CLIENT_ID=xxxxxxxxxx                     # Epic OAuth Client ID
EPIC_CLIENT_SECRET=xxxxxxxxxx                 # Epic OAuth Client Secret
EPIC_DEPLOYMENT_ID=xxxxxxxxxx                 # Epic RTC Deployment ID
EPIC_BASE_URL=https://api.epicgames.dev       # Epic API base URL (default)

# Optional Configuration
REFRESH_INTERVAL=600                          # AccelByte IAM token validator refresh interval in seconds (default: 600); Epic OAuth token refreshes every 50 minutes independently
LOG_LEVEL=info                                # Log level: debug, info, warn, error
OTEL_EXPORTER_ZIPKIN_ENDPOINT=                # OpenTelemetry Zipkin endpoint (optional)
```

> :exclamation: **In this app, PLUGIN_GRPC_SERVER_AUTH_ENABLED is `true` by default**: If it is set to `false`, the endpoint `permission.action` and `permission.resource`  validation will be disabled and the endpoint can be accessed without a valid access token. This option is provided for development purpose only.

> :warning: **BASE_PATH must start with `/`**: The service uses this as the URL prefix for all endpoints. For example, `BASE_PATH=/eos-voice` results in endpoints like `/eos-voice/public/party/{party_id}/token`.

---

## Building

To build this app, use the following command.

```shell
make build
```

The build output will be available in `.output` directory.

### What Gets Built

- Go binary compiled from `main.go`
- Protocol buffer files generated from `pkg/proto/*.proto`
- gRPC Gateway files
- Swagger/OpenAPI specification

---

## Running

To (build and) run this app in a container, use the following command.

```shell
docker compose up --build
```

### Service Endpoints

Once running, the service will be available at:

- **gRPC Server**: `:6565`
- **HTTP Gateway**: `:8000`
- **Prometheus Metrics**: `:8080/metrics`
- **Swagger UI**: `http://localhost:8000{BASE_PATH}/apidocs/`
- **Swagger JSON**: `http://localhost:8000{BASE_PATH}/apidocs/api.json`

For example, with `BASE_PATH=/eos-voice`:
- Swagger UI: `http://localhost:8000/eos-voice/apidocs/`
- Health endpoint: `http://localhost:8000/eos-voice/v1/health`

---

## Deployment

After completing testing, the next step is to deploy your app to `AccelByte Gaming Services`.

### 1. Create an Extend Service Extension App

If you do not already have one, create a new [Extend Service Extension App](https://docs.accelbyte.io/gaming-services/services/extend/service-extension/getting-started-service-extension/#create-the-extend-app).

On the **App Detail** page, take note of the following values.
- `Namespace`
- `App Name`

Under the **Environment Configuration** section, set the required secrets and/or variables.

**Secrets:**
- `AB_CLIENT_ID` - AccelByte service OAuth client ID
- `AB_CLIENT_SECRET` - AccelByte service OAuth client secret
- `EPIC_CLIENT_ID` - Epic Games OAuth client ID
- `EPIC_CLIENT_SECRET` - Epic Games OAuth client secret
- `EPIC_DEPLOYMENT_ID` - Epic RTC deployment ID

**Variables:**
- `BASE_PATH` - Set to `/eos-voice` (or your preferred path)
- `EPIC_BASE_URL` - Set to `https://api.epicgames.dev` (or Epic sandbox URL)
- `PLUGIN_GRPC_SERVER_AUTH_ENABLED` - Set to `true`

### 2. Build and Push the Container Image

Use [extend-helper-cli](https://github.com/AccelByte/extend-helper-cli) to build and upload the container image.

```
extend-helper-cli image-upload --login --namespace <namespace> --app <app-name> --image-tag v0.0.1
```

> :warning: Run this command from your project directory. If you are in a different directory, add the `--work-dir <project-dir>` option to specify the correct path.

### 3. Deploy the Image

On the **App Detail** page:
- Click **Image Version History**
- Select the image you just pushed
- Click **Deploy Image**

---

## Next Steps

- **Test the service**: See [Testing Guide](testing_guide.md)
- **Understand the architecture**: See [Architecture Guide](architecture.md)
- **Monitor and troubleshoot**: See [Operations Guide](operations.md)
