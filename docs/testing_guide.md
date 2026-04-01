# EOS Voice RTC - Testing Guide

This guide provides instructions for manual end-to-end testing of the EOS Voice RTC Service Extension using Postman.

## Prerequisites

### 1. Service Setup
- **Service Running**: Ensure the service is running locally
  ```bash
  docker compose up --build
  ```
- **Service URL**: `http://localhost:8000`
- **Swagger UI**: `http://localhost:8000/eos-voice/apidocs/`

### 2. AccelByte Setup
You need the following AccelByte resources:

- **Client Credentials** (for user login)
  - `client_id` - OAuth client ID with password grant
  - `client_secret` - OAuth client secret

- **Admin Client Credentials** (for admin operations)
  - `admin_client_id` - OAuth client ID with client credentials grant
  - `admin_client_secret` - OAuth client secret
  - Required permissions:
    - `CUSTOM:ADMIN:NAMESPACE:{namespace}:VOICE [CREATE]` - for generating admin session tokens
    - `CUSTOM:ADMIN:NAMESPACE:{namespace}:VOICE [DELETE]` - for revoking tokens

- **Test User Account**
  - `user_email` - Email of test user
  - `user_password` - Password of test user
  - User must have Epic Games account linked
  - User must be in a party or game session

- **Test Resources**
  - `party_id` - Active party ID (user must be member)
  - `session_id` - Active game session ID (user must be member)

## Postman Setup

### Import Collection and Environment

1. **Import Collection**
   - Open Postman
   - Click **Import**
   - Select `postman/EOS-Voice-RTC-E2E-Testing.postman_collection.json`

2. **Import Environment**
   - Click **Import**
   - Select `postman/EOS-Voice-RTC.postman_environment.json`

3. **Configure Environment Variables**
   - Select "EOS Voice RTC - Environment" from dropdown
   - Click the eye icon to edit variables
   - Update the following values:

   ```
   base_url: https://test.accelbyte.io (or your AGS URL)
   service_url: http://localhost:8000
   client_id: <YOUR_CLIENT_ID>
   client_secret: <YOUR_CLIENT_SECRET>
   admin_client_id: <YOUR_ADMIN_CLIENT_ID>
   admin_client_secret: <YOUR_ADMIN_CLIENT_SECRET>
   user_email: <YOUR_TEST_USER_EMAIL>
   user_password: <YOUR_TEST_USER_PASSWORD>
   party_id: <YOUR_PARTY_ID>
   session_id: <YOUR_SESSION_ID>
   ```

## Test Flow

### Step 1: User Authentication

**Request:** `1.1 User Login (Email/Password)`

- **Method:** POST
- **Endpoint:** `{{base_url}}/iam/v3/oauth/token`
- **Auth:** Basic Auth (client_id:client_secret)
- **Body:**
  ```
  grant_type: password
  username: {{user_email}}
  password: {{user_password}}
  ```

**Expected Response:**
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI...",
  "user_id": "a1b2c3d4e5f6...",
  "namespace": "your-namespace"
}
```

**Auto-Cached:**
- ✓ `user_access_token` - For voice token generation
- ✓ `user_id` - User identifier
- ✓ `namespace` - User's namespace

---

### Step 2: Generate Voice Tokens

#### 2.1 Party Voice Token

**Request:** `2.1 Generate Party Voice Token`

- **Method:** POST
- **Endpoint:** `{{service_url}}/eos-voice/public/party/{{party_id}}/token?hardMuted=false`
- **Auth:** Bearer Token (user_access_token)

**Expected Response:**
```json
{
  "channel_base_url": "wss://voice.epicgames.com",
  "token": "eyJ0eXAiOiJKV1QiLCJhbGci...",
  "channel_type": "PARTY",
  "room_id": "party-123:Voice"
}
```

**Auto-Cached:**
- ✓ `party_room_id` - For token revocation

**Possible Errors:**
- `403 - 40301`: User not in party
- `403 - 40303`: Epic account not linked
- `500 - 50002`: Epic RTC API error

---

#### 2.2 Session Voice Token

**Request:** `2.2 Generate Session Voice Token`

- **Method:** POST
- **Endpoint:** `{{service_url}}/eos-voice/public/session/{{session_id}}/token?hardMuted=false`
- **Auth:** Bearer Token (user_access_token)

**Expected Response:**
```json
{
  "channel_base_url": "wss://voice.epicgames.com",
  "token": "eyJ0eXAiOiJKV1QiLCJhbGci...",
  "channel_type": "SESSION",
  "room_id": "session-456:Voice"
}
```

**Auto-Cached:**
- ✓ `session_room_id` - For token revocation

**Possible Errors:**
- `403 - 40302`: User not in session
- `403 - 40303`: Epic account not linked
- `500 - 50002`: Epic RTC API error

---

#### 2.3 Team Voice Token

**Request:** `2.3 Generate Team Voice Token`

- **Method:** POST
- **Endpoint:** `{{service_url}}/eos-voice/public/session/{{session_id}}/team/token?hardMuted=false`
- **Auth:** Bearer Token (user_access_token)

**Expected Response:**
```json
{
  "channel_base_url": "wss://voice.epicgames.com",
  "token": "eyJ0eXAiOiJKV1QiLCJhbGci...",
  "channel_type": "TEAM",
  "room_id": "session-456:team-1"
}
```

**Auto-Cached:**
- ✓ `team_room_id` - For token revocation

**Possible Errors:**
- `403 - 40302`: User not in session
- `403 - 40304`: User not in team
- `403 - 40303`: Epic account not linked
- `500 - 50002`: Epic RTC API error

---

### Step 3: Admin Authentication

**Request:** `1.2 Admin Login (Client Credentials)`

- **Method:** POST
- **Endpoint:** `{{base_url}}/iam/v3/oauth/token`
- **Auth:** Basic Auth (admin_client_id:admin_client_secret)
- **Body:**
  ```
  grant_type: client_credentials
  ```

**Expected Response:**
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI...",
  "token_type": "Bearer"
}
```

**Auto-Cached:**
- ✓ `admin_access_token` - For admin operations

---

### Step 4: Revoke Voice Tokens

#### 4.1 Revoke Party Token

**Request:** `3.1 Revoke Party Voice Token`

- **Method:** POST
- **Endpoint:** `{{service_url}}/eos-voice/admin/room/{{party_room_id}}/token/revoke`
- **Auth:** Bearer Token (admin_access_token)
- **Body:**
  ```json
  {
    "userIds": ["{{user_id}}"]
  }
  ```

**Expected Response:**
- Status: `200 OK`
- Body: Empty `{}`

---

#### 4.2 Revoke Session Token

**Request:** `3.2 Revoke Session Voice Token`

- **Method:** POST
- **Endpoint:** `{{service_url}}/eos-voice/admin/room/{{session_room_id}}/token/revoke`
- **Auth:** Bearer Token (admin_access_token)
- **Body:**
  ```json
  {
    "userIds": ["{{user_id}}"]
  }
  ```

**Expected Response:**
- Status: `200 OK`
- Body: Empty `{}`

---

#### 4.3 Revoke Team Token

**Request:** `3.3 Revoke Team Voice Token`

- **Method:** POST
- **Endpoint:** `{{service_url}}/eos-voice/admin/room/{{team_room_id}}/token/revoke`
- **Auth:** Bearer Token (admin_access_token)
- **Body:**
  ```json
  {
    "userIds": ["{{user_id}}"]
  }
  ```

**Expected Response:**
- Status: `200 OK`
- Body: Empty `{}`

---

## Error Scenario Testing

The collection includes error scenario tests in folder `4. Error Scenarios`:

### 4.1 User Not in Party
Tests validation when user tries to get token for party they're not in.
- Expected: `403 Forbidden` with error code `40301`

### 4.2 User Not in Session
Tests validation when user tries to get token for session they're not in.
- Expected: `403 Forbidden` with error code `40302`

### 4.3 Missing Authorization
Tests authentication requirement.
- Expected: `401 Unauthorized`

---

## Health Check

**Request:** `5.1 Service Health Check`

- **Method:** GET
- **Endpoint:** `{{service_url}}/eos-voice/v1/health`
- **Auth:** None

**Expected Response:**
```json
{
  "status": "ok"
}
```

---

## Running the Complete Test Suite

To run all tests in sequence:

1. **Select Collection**: "EOS Voice RTC - E2E Testing"
2. **Click "Run"**: Opens Collection Runner
3. **Select Environment**: "EOS Voice RTC - Environment"
4. **Order**:
   - 1. Authentication
   - 2. Voice Token Generation
   - 3. Token Revocation (Admin)
   - 4. Error Scenarios
   - 5. Health Check
5. **Click "Run EOS Voice RTC - E2E Testing"**

**Expected Results:**
- All tests should pass ✓
- Console output shows cached variables
- Environment variables automatically populated

---

## Troubleshooting

### Service Not Running
```
Error: connect ECONNREFUSED 127.0.0.1:8000
```
**Solution:** Start the service with `docker compose up --build`

---

### Authentication Failed
```
401 Unauthorized
```
**Solution:** 
- Check client_id and client_secret are correct
- Ensure OAuth client has password grant enabled
- Verify user credentials

---

### User Not in Party/Session
```
403 Forbidden - Error Code: 40301 or 40302
```
**Solution:**
- User must join a party/session first
- Use AccelByte Admin Portal or SDK to create and join party/session
- Update `party_id` or `session_id` in environment

---

### Epic Account Not Linked
```
403 Forbidden - Error Code: 40303
```
**Solution:**
- Link Epic Games account to AccelByte user
- Use AccelByte IAM to link external account
- Verify Epic account connection

---

### Admin Permission Denied
```
403 Forbidden on revoke endpoint
```
**Solution:**
- Verify admin client has `CUSTOM:ADMIN:NAMESPACE:{namespace}:VOICE [DELETE]` permission
- Check namespace matches user's namespace
- Use correct admin_client_id/secret

---

## Test Data Examples

### Sample Party ID
```
party-0dd3c9e5be3842c7baa5eac47f77b471
```

### Sample Session ID
```
session-a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6
```

### Sample Room IDs (auto-generated)
```
Party Room:   party-0dd3c9e5be3842c7baa5eac47f77b471:Voice
Session Room: session-a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6:Voice
Team Room:    session-a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6:team-1
```

---

## API Response Reference

### Success Response Structure

**Voice Token Response:**
```json
{
  "channel_base_url": "wss://voice-server.epicgames.com",
  "token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "channel_type": "PARTY" | "SESSION" | "TEAM",
  "room_id": "party-id:Voice"
}
```

### Error Response Structure

**gRPC Error Response:**
```json
{
  "code": 7,
  "message": "You must join a party before calling this endpoint.",
  "details": []
}
```

**Error Codes:**
- `40301`: User not in party
- `40302`: User not in session
- `40303`: Epic account not linked
- `40304`: User not in team
- `50001`: Epic authentication failed
- `50002`: Epic RTC API error
- `50003`: AccelByte API error

---

## Next Steps

After successful testing:

1. **Integration Testing**: Test with real game client
2. **Load Testing**: Test with multiple concurrent users
3. **Token Expiration**: Test token refresh after ~60 minutes
4. **Epic Disconnect**: Test reconnection handling

---

## Support

For issues or questions:
- Check service logs: `docker compose logs -f`
- Review Swagger UI: `http://localhost:8000/eos-voice/apidocs/`
- Check implementation plan: `eos-voice-chat-plan.md`
