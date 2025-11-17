# EOS Voice Integration

This service listens to AccelByte session and party events and mirrors membership into Epic Online Services (EOS) voice rooms. The following configuration is required before enabling the feature:

- `AB_NAMESPACE` – namespace containing the session and party data.
- `EOS_VOICE_DEPLOYMENT_ID` – deployment identifier used by your EOS title (required).
- `EOS_VOICE_CLIENT_ID` / `EOS_VOICE_CLIENT_SECRET` – OAuth credentials used to request access tokens (required).
- `EOS_VOICE_NOTIFICATION_TOPIC` – lobby freeform notification topic sent to players (default `EOS_VOICE`).

When active the handler performs the following actions:

1. `GameSessionCreated` events decode the base64 payload bundled in the message to obtain the latest session snapshot, group players per team room (`<sessionId>:<teamIndex>` or `<sessionId>:0`), and mint EOS tokens for every active player. `GameSessionEnded` re-fetches the snapshot and revokes every participant; intermediate lifecycle events are ignored to keep the app stateless.
2. Party voice uses a single room `<partyId>:Voice`. `PartyCreated` seeds tokens for the active roster, `PartyJoined` mints tokens for the user IDs carried in the event payload, and `PartyLeave` / `PartyKicked` revoke those user IDs. `PartyMembersChanged` is ignored.
3. The handler resolves EOS Product User IDs by calling the EOS Connect `Query External Accounts` API with `identityProviderId=openId` and the AccelByte user IDs in the batch; ensure each player has linked their EOS account accordingly. Calls to EOS Voice always target `https://api.epicgames.dev/rtc/` and OAuth tokens are minted from `https://api.epicgames.dev/epic/oauth/v2/token`, so no extra configuration is required for those endpoints.
4. The service performs the OAuth client-credentials flow automatically and caches tokens until near expiration.
5. On token creation, the handler sends a lobby freeform notification per user with the room identifier, `clientBaseUrl`, and a user-specific room token.
6. Leaving a party or ending the session revokes the participant via the EOS remove participant endpoint.

Because processing is stateless, replaying the same lifecycle event simply reissues or revokes tokens based on the event type—no manual resync is required.
