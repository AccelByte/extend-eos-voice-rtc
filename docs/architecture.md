# Architecture Guide

**📚 Documentation:** [README](../README.md) | [Setup](setup.md) | [Architecture](architecture.md) | [Operations](operations.md) | [Testing](testing_guide.md)

---

This guide explains the technical architecture, design decisions, and key concepts of the EOS Voice RTC service.

## Architecture Overview

### Request Flow (Generate Token)

1. Client calls REST endpoint on the gateway (HTTP).
2. Gateway forwards to gRPC handler.
3. Auth interceptor validates bearer token and permission metadata.
4. Handler resolves user ID from token and validates party/session membership via AGS.
5. Handler maps AccelByte user ID to Epic PUID via Epic Connect, unless a `puid` is provided in the request.
6. Handler requests RTC room token from Epic and returns the token response.

### Core Components

The service consists of the following core components:

- **Gateway**: `pkg/common/gateway.go` wires REST to gRPC, and serves Swagger JSON/UI.
- **Auth**: `pkg/common/authServerInterceptor.go` enforces token + permission checks declared in proto.
- **EOS Voice Service**: `pkg/service/eosService.go` implements party/team/session token generation and revoke.
- **Session Validator**: `pkg/service/sessionValidator.go` checks party/session membership and team assignment.
- **Epic Client**: `pkg/service/epicClient.go` authenticates and calls Epic RTC + Connect endpoints.
- **Observability**: `main.go` sets up Prometheus metrics, OpenTelemetry tracing, and structured logs.

---

## PUID Resolution

The service supports two ways to identify Epic users when generating voice tokens, depending on your Epic integration model:

### Option 1: Automatic PUID Resolution via EOS Connect (Games using EOS SDK)

**When to use**: Your game uses Epic Online Services (EOS) SDK and players authenticate via AccelByte

**How it works**: 
- Omit `puid` parameter in token requests
- The service queries Epic Connect API to resolve AccelByte user ID → Epic PUID
- **Requires**: Users must link their AccelByte account to Epic using **EOS_Connect OpenID**

**Benefits**: 
- No need to pass PUID in every request
- Reduces client-side complexity

**Error**: Returns error code `40303` if Epic account not linked via EOS Connect

### Option 2: Explicit PUID (Games using Epic Account Services / Epic Launcher)

**When to use**: Your game uses Epic Account Services (EAS) or distributes via Epic Games Launcher

**How it works**:
- Include `puid` parameter in token requests
- Players authenticate via Epic and link Epic → AccelByte (reverse direction)
- The service **cannot** query Epic to resolve PUID (EAS doesn't support this lookup)
- Your game client must obtain the Epic PUID and pass it explicitly

**Benefits**:
- Reduces API calls to Epic (no lookup needed)
- Works with EAS authentication flow
- Better performance (one less API call per request)

### Account Linking Direction

- **EOS Connect (Option 1)**: AccelByte → Epic (AccelByte is primary identity)
- **EAS (Option 2)**: Epic → AccelByte (Epic is primary identity)

For detailed instructions on EOS Connect integration, see [Epic Connect Documentation](https://dev.epicgames.com/docs/game-services/connect).

For setup instructions, see the [Setup Guide](setup.md#account-linking-setup).

---

## Authorization & Permissions

The service uses AccelByte IAM's permission-based authorization to secure admin endpoints:

### Public Endpoints (User Tokens - Works in Shared & Private Cloud)

- `POST /public/party/{party_id}/token` - No special permissions required
- `POST /public/session/{session_id}/token` - No special permissions required (use `team` and `session` query params to control which tokens are generated)

### Admin Endpoints

- `POST /admin/session/{session_id}/token` - Requires `CUSTOM:ADMIN:NAMESPACE:{namespace}:VOICE [CREATE]`
- `POST /admin/room/{room_id}/token/revoke` - Requires `CUSTOM:ADMIN:NAMESPACE:{namespace}:VOICE [DELETE]`

### How Authorization Works

1. **Token Validation**: The auth interceptor validates the Bearer token with AccelByte IAM
2. **Permission Check**: For admin endpoints, the interceptor verifies the token has the required permission
3. **User ID Extraction**: For public endpoints, the service extracts the user ID from the validated token's `sub` claim
4. **Membership Validation**: The service validates the user is a member of the party/session before generating tokens

### Security Notes

- Regular user tokens are automatically rejected from admin endpoints (403 Permission Denied)
- Only OAuth clients configured with the proper VOICE permissions can access admin endpoints
- Token validation is handled by AccelByte IAM SDK - no custom JWT validation logic
- Empty `sub` claim in tokens results in authentication failure for public endpoints

---

## Room ID Generation

The service generates room IDs using the following patterns:

- **Party Room**: `{party_id}:Voice`
- **Session Room**: `{session_id}:Voice`
- **Team Room**: `{session_id}:{team_id}`

These deterministic patterns ensure:
- Same room ID for same party/session across requests
- Easy room ID reconstruction for revocation
- Consistent voice channel behavior

---

## Admin Session Token Generation

The admin endpoint `POST /admin/session/{session_id}/token` supports generating voice tokens for all users in a session. It includes an `allow_pending_users` parameter to control which users receive tokens based on their session membership status.

### Session Member Status

AccelByte game sessions track member status using the `StatusV2` field with the following possible values:

**Active/Eligible Statuses:**
- `INVITED` - User has been invited but hasn't accepted the invitation yet
- `JOINED` - User has accepted and joined the session
- `CONNECTED` - User is actively connected to the session

**Inactive Statuses (always excluded):**
- `LEFT` - User left the session
- `KICKED` - User was kicked from the session
- `REJECTED` - User rejected the session invitation
- `DISCONNECTED`, `DROPPED`, `TIMEOUT`, `CANCELLED`, `TERMINATED` - Other inactive states

### The `allow_pending_users` Parameter

#### Default Behavior (`allow_pending_users=false`)

- Only generates tokens for users with status `JOINED` or `CONNECTED`
- Excludes users who have been invited but haven't accepted (`INVITED` status)
- This is the safer default to avoid pre-generating tokens for users who may never join

#### Include Pending Users (`allow_pending_users=true`)

- Generates tokens for users with status `INVITED`, `JOINED`, or `CONNECTED`
- Useful for pre-generating voice tokens before users accept the game session invitation
- Allows users to join voice chat as soon as they accept the invitation

### Request Examples

**Default: Only generate tokens for joined/connected users**

```bash
curl -X POST "https://your-domain/eos-voice/admin/session/abc123/token" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "team": true,
    "session": true,
    "hard_muted": false,
    "notify": true,
    "allow_pending_users": false
  }'
```

**Include pending/invited users**

```bash
curl -X POST "https://your-domain/eos-voice/admin/session/abc123/token" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "team": true,
    "session": true,
    "hard_muted": false,
    "notify": true,
    "allow_pending_users": true
  }'
```

### Error Messages

**When `allow_pending_users=false` and all members are invited:**

```json
{
  "code": 9,
  "message": "session has no valid members with status JOINED or CONNECTED. Set allow_pending_users=true to include users with status INVITED"
}
```

**When `allow_pending_users=true` and no eligible members exist:**

```json
{
  "code": 9,
  "message": "session has no valid members with status INVITED, JOINED, or CONNECTED"
}
```

### Use Cases

**Use `allow_pending_users=true` when:**
- You want to pre-generate voice tokens for all invited users
- Users should be able to join voice chat immediately upon accepting the session invitation
- You're implementing a "spectator" or "lobby" voice channel for invited users

**Use `allow_pending_users=false` (default) when:**
- Voice tokens should only be generated for users who have confirmed their participation
- You want to minimize token generation for users who may never join
- Security requirements dictate that only active session members should have voice access

---

## Architecture Decisions

### Fail-Fast Validation Pattern

The service validates all preconditions (party/session membership, Epic account linking) **before** calling Epic APIs. This approach:

- Reduces unnecessary Epic API calls
- Provides faster error feedback to users
- Lowers costs (Epic API rate limits)
- Improves overall system reliability

By validating party/session membership through AccelByte first, we can return errors like `40301` (user not in party) or `40302` (user not in session) immediately without making expensive calls to Epic's RTC API.

### Epic OAuth Token Caching

Epic OAuth tokens expire after ~60 minutes. The service implements an automatic token refresh mechanism:

- Automatically refreshes tokens every 50 minutes via background goroutine
- Uses thread-safe RWMutex for concurrent access
- Continues operation during refresh (no downtime)
- Logs token refresh events for monitoring

This ensures the service maintains a valid Epic OAuth token at all times without manual intervention or service interruptions.

### Room ID Determinism

Room IDs are deterministically generated using predictable patterns:

- Party: `{party_id}:Voice`
- Session: `{session_id}:Voice`
- Team: `{session_id}:{team_id}`

This ensures:

- **Consistency**: Same room ID for same party/session across requests
- **Easy Revocation**: Room IDs can be reconstructed for revocation without database lookups
- **Predictability**: Clients can predict room IDs if needed for debugging
- **No Collisions**: Different resource types (party vs session) won't collide

---

## Next Steps

- **Set up the service**: See [Setup Guide](setup.md)
- **Monitor and troubleshoot**: See [Operations Guide](operations.md)
- **Test the service**: See [Testing Guide](testing_guide.md)
