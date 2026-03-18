# Smart Clinic Bot — Admin API Reference

Base path: `/admin`
All responses are `Content-Type: application/json`.
All timestamps are ISO 8601 / RFC 3339 in UTC (e.g. `"2026-03-17T10:00:00Z"`).

---

## Authentication

The API uses short-lived JWT bearer tokens (HS256, 24-hour TTL). All endpoints except `POST /admin/auth/login` require the header:

```
Authorization: Bearer <access_token>
```

### Roles

| Role | What they can access |
|---|---|
| `super_admin` | Everything — all clinics, all users |
| `clinic_admin` | Only their own clinic's data |

---

## Error format

Every non-2xx response has this shape:

```json
{ "error": "human-readable message" }
```

### Status codes

| Code | Meaning |
|---|---|
| 200 | Success |
| 201 | Resource created |
| 400 | Validation / bad request body |
| 401 | Missing, expired, or invalid token |
| 403 | Authenticated but not allowed (wrong role or wrong clinic) |
| 404 | Resource not found |
| 409 | Unique constraint violated (slug, email) |
| 500 | Internal server error |

---

## Auth

### `POST /admin/auth/login`

Public endpoint — no token required.

**Request**

```json
{
  "email": "admin@example.com",
  "password": "yourpassword"
}
```

| Field | Type | Required |
|---|---|---|
| `email` | string | yes |
| `password` | string | yes |

**Response `200`**

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 86400
}
```

`expires_in` is always `86400` (seconds = 24 hours). Store the token and re-login when it expires.

**Errors**

| Status | Message | Cause |
|---|---|---|
| 400 | `email and password are required` | Either field is blank |
| 401 | `invalid credentials` | Wrong email, wrong password, or deactivated account |

> The 401 message is intentionally identical for wrong email and wrong password — the API does not reveal which one was wrong.

---

## Clinics

### `GET /admin/clinics` — super_admin only

List all clinics, newest first.

**Response `200`** — array of clinic objects

```json
[
  {
    "id": "018e2f3a-...",
    "name": "Bright Smile Delhi",
    "slug": "bright-smile-delhi",
    "address": "12 MG Road",
    "city": "Delhi",
    "phone": "+91-9999999999",
    "email": "hello@brightsmile.in",
    "is_active": true,
    "created_at": "2026-03-01T08:00:00Z"
  }
]
```

Fields `address`, `city`, `phone`, `email` are omitted when `null`.

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` — caller is not super_admin |

---

### `POST /admin/clinics` — super_admin only

Create a new clinic.

**Request**

```json
{
  "name": "Bright Smile Delhi",
  "slug": "bright-smile-delhi",
  "address": "12 MG Road",
  "city": "Delhi",
  "phone": "+91-9999999999",
  "email": "hello@brightsmile.in"
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | Display name |
| `slug` | string | yes | URL-safe unique handle, e.g. `bright-smile-delhi` |
| `address` | string | no | |
| `city` | string | no | |
| `phone` | string | no | |
| `email` | string | no | Clinic contact email |

**Response `201`** — created clinic object (same shape as list item above)

**Errors**

| Status | Message |
|---|---|
| 400 | `name and slug are required` |
| 403 | `forbidden` |
| 409 | `slug already exists` |

---

### `GET /admin/clinics/{clinic_id}`

Get a single clinic.
`clinic_admin` can only fetch their own clinic.

**Path params**

| Param | Description |
|---|---|
| `clinic_id` | UUID of the clinic |

**Response `200`** — clinic object

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` |
| 404 | `clinic not found` |

---

### `PUT /admin/clinics/{clinic_id}`

Update a clinic's details. `slug` is not updatable.
`clinic_admin` can only update their own clinic.

**Request**

```json
{
  "name": "Bright Smile Delhi (Updated)",
  "address": "14 MG Road",
  "city": "Delhi",
  "phone": "+91-8888888888",
  "email": "newcontact@brightsmile.in"
}
```

| Field | Type | Required |
|---|---|---|
| `name` | string | yes |
| `address` | string | no |
| `city` | string | no |
| `phone` | string | no |
| `email` | string | no |

**Response `200`** — updated clinic object

**Errors**

| Status | Message |
|---|---|
| 400 | `name is required` |
| 403 | `forbidden` |
| 404 | `clinic not found` |

---

## FAQs

All FAQ endpoints are scoped to a clinic. `clinic_admin` can only access their own clinic's FAQs.

### `GET /admin/clinics/{clinic_id}/faqs`

List all FAQs, newest first.

**Response `200`**

```json
[
  {
    "id": "018e2f3a-...",
    "clinic_id": "018e2f3a-...",
    "category": "general",
    "question": "What are your opening hours?",
    "answer": "Mon–Fri 9am–6pm, Sat 9am–2pm.",
    "created_at": "2026-03-01T08:00:00Z",
    "updated_at": "2026-03-01T08:00:00Z"
  }
]
```

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` |

---

### `POST /admin/clinics/{clinic_id}/faqs`

Create a FAQ. The content is automatically vectorized and stored for chatbot RAG search.

**Request**

```json
{
  "category": "general",
  "question": "What are your opening hours?",
  "answer": "Mon–Fri 9am–6pm, Sat 9am–2pm."
}
```

| Field | Type | Required | Default |
|---|---|---|---|
| `question` | string | yes | |
| `answer` | string | yes | |
| `category` | string | no | `"general"` |

**Response `201`** — created FAQ object (same shape as list item above)

> After the FAQ row is saved, the server calls the embedding API to vectorize `"Q: <question>\nA: <answer>"`. If embedding fails, the row is still saved and `201` is returned — the indexing failure is logged as a warning.

**Errors**

| Status | Message |
|---|---|
| 400 | `question and answer are required` |
| 403 | `forbidden` |

---

### `PUT /admin/clinics/{clinic_id}/faqs/{id}`

Update a FAQ. The old vector chunk is deleted and a fresh embedding is created.

**Request** — same shape as create

**Path params**

| Param | Description |
|---|---|
| `clinic_id` | UUID of the clinic |
| `id` | UUID of the FAQ |

**Response `200`** — updated FAQ object

**Errors**

| Status | Message |
|---|---|
| 400 | `question and answer are required` |
| 403 | `forbidden` |
| 404 | `faq not found` |

---

### `DELETE /admin/clinics/{clinic_id}/faqs/{id}`

Delete a FAQ and its vector chunks.

**Response `200`**

```json
{ "deleted": true }
```

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` |
| 404 | `faq not found` |

---

## Services

All service endpoints are scoped to a clinic. Same authorization rules as FAQs.

### `GET /admin/clinics/{clinic_id}/services`

List all services, newest first.

**Response `200`**

```json
[
  {
    "id": "018e2f3a-...",
    "clinic_id": "018e2f3a-...",
    "name": "Teeth Whitening",
    "category": "cosmetic",
    "description": "Professional in-chair whitening treatment.",
    "price_min": 3000.00,
    "price_max": 8000.00,
    "is_active": true,
    "created_at": "2026-03-01T08:00:00Z",
    "updated_at": "2026-03-01T08:00:00Z"
  }
]
```

Fields `description`, `price_min`, `price_max` are omitted when `null`.

---

### `POST /admin/clinics/{clinic_id}/services`

Create a service. Content is automatically vectorized.

**Request**

```json
{
  "name": "Teeth Whitening",
  "category": "cosmetic",
  "description": "Professional in-chair whitening treatment.",
  "price_min": 3000.00,
  "price_max": 8000.00
}
```

| Field | Type | Required | Default |
|---|---|---|---|
| `name` | string | yes | |
| `category` | string | no | `"general"` |
| `description` | string | no | |
| `price_min` | number | no | |
| `price_max` | number | no | |

**Response `201`** — created service object

**Errors**

| Status | Message |
|---|---|
| 400 | `name is required` |
| 403 | `forbidden` |

---

### `PUT /admin/clinics/{clinic_id}/services/{id}`

Update a service. Old vector chunk is replaced.

**Request** — same shape as create

**Response `200`** — updated service object

**Errors**

| Status | Message |
|---|---|
| 400 | `name is required` |
| 403 | `forbidden` |
| 404 | `service not found` |

---

### `DELETE /admin/clinics/{clinic_id}/services/{id}`

Delete a service and its vector chunks.

**Response `200`**

```json
{ "deleted": true }
```

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` |
| 404 | `service not found` |

---

## Doctors

All doctor endpoints are scoped to a clinic. Same authorization rules as FAQs.

### `GET /admin/clinics/{clinic_id}/doctors`

List all doctors, newest first.

**Response `200`**

```json
[
  {
    "id": "018e2f3a-...",
    "clinic_id": "018e2f3a-...",
    "name": "Priya Sharma",
    "specialization": "Orthodontist",
    "qualifications": ["BDS", "MDS"],
    "bio": "12 years of experience in orthodontics.",
    "available_days": ["Monday", "Wednesday", "Friday"],
    "languages": ["English", "Hindi"],
    "is_active": true,
    "created_at": "2026-03-01T08:00:00Z",
    "updated_at": "2026-03-01T08:00:00Z"
  }
]
```

Fields `specialization` and `bio` are omitted when `null`. `qualifications`, `available_days`, and `languages` are always arrays (empty `[]` if not set).

---

### `POST /admin/clinics/{clinic_id}/doctors`

Create a doctor. Content is automatically vectorized.

**Request**

```json
{
  "name": "Priya Sharma",
  "specialization": "Orthodontist",
  "qualifications": ["BDS", "MDS"],
  "bio": "12 years of experience in orthodontics.",
  "available_days": ["Monday", "Wednesday", "Friday"],
  "languages": ["English", "Hindi"]
}
```

| Field | Type | Required | Default |
|---|---|---|---|
| `name` | string | yes | |
| `specialization` | string | no | |
| `qualifications` | string[] | no | `[]` |
| `bio` | string | no | |
| `available_days` | string[] | no | `[]` |
| `languages` | string[] | no | `["English"]` |

**Response `201`** — created doctor object

**Errors**

| Status | Message |
|---|---|
| 400 | `name is required` |
| 403 | `forbidden` |

---

### `PUT /admin/clinics/{clinic_id}/doctors/{id}`

Update a doctor. Old vector chunk is replaced.

**Request** — same shape as create

**Response `200`** — updated doctor object

**Errors**

| Status | Message |
|---|---|
| 400 | `name is required` |
| 403 | `forbidden` |
| 404 | `doctor not found` |

---

### `DELETE /admin/clinics/{clinic_id}/doctors/{id}`

Delete a doctor and their vector chunks.

**Response `200`**

```json
{ "deleted": true }
```

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` |
| 404 | `doctor not found` |

---

## Admin Users

All user management endpoints require `super_admin`.

### `GET /admin/users`

List all admin users, newest first. Password hashes are never returned.

**Response `200`**

```json
[
  {
    "id": "018e2f3a-...",
    "name": "Naveen",
    "email": "naveen@clinic.com",
    "role": "super_admin",
    "is_active": true,
    "last_login_at": "2026-03-17T09:30:00Z"
  },
  {
    "id": "018e2f3b-...",
    "name": "Receptionist",
    "email": "front@brightsmile.in",
    "role": "clinic_admin",
    "clinic_id": "018e2f3a-...",
    "is_active": true,
    "last_login_at": null
  }
]
```

`clinic_id` is omitted for `super_admin`. `last_login_at` is omitted if the user has never logged in.

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` |

---

### `POST /admin/users`

Create a new admin user.

**Request**

```json
{
  "name": "Front Desk",
  "email": "front@brightsmile.in",
  "password": "securepassword",
  "role": "clinic_admin",
  "clinic_id": "018e2f3a-..."
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | |
| `email` | string | yes | Must be unique |
| `password` | string | yes | Plain text — bcrypt-hashed before storage |
| `role` | string | yes | `super_admin` or `clinic_admin` |
| `clinic_id` | string (UUID) | required if `role == "clinic_admin"` | Omit or set `null` for `super_admin` |

**Response `201`**

```json
{
  "id": "018e2f3b-...",
  "name": "Front Desk",
  "email": "front@brightsmile.in",
  "role": "clinic_admin",
  "clinic_id": "018e2f3a-..."
}
```

**Errors**

| Status | Message |
|---|---|
| 400 | `name, email, password, and role are required` |
| 400 | `role must be super_admin or clinic_admin` |
| 400 | `clinic_id is required for clinic_admin` |
| 403 | `forbidden` |
| 409 | `email already exists` |

---

### `DELETE /admin/users/{id}`

Deactivate a user (sets `is_active = false`). The user can no longer log in but the record is preserved.

**Path params**

| Param | Description |
|---|---|
| `id` | UUID of the admin user |

**Response `200`**

```json
{ "deactivated": true }
```

**Errors**

| Status | Message |
|---|---|
| 403 | `forbidden` |
| 404 | `user not found` |

---

### `POST /admin/users/{id}/reset-password`

Set a new password for any admin user.

**Request**

```json
{ "password": "newsecurepassword" }
```

| Field | Type | Required |
|---|---|---|
| `password` | string | yes |

**Response `200`**

```json
{ "reset": true }
```

**Errors**

| Status | Message |
|---|---|
| 400 | `password is required` |
| 403 | `forbidden` |
| 404 | `user not found` |

---

## Quick-start flow for the dashboard

Below is the minimum sequence to get a clinic fully set up.

```
1. POST /admin/auth/login
   → save access_token, note expiry (24h)

2. POST /admin/clinics               (super_admin)
   → save clinic_id

3. POST /admin/clinics/{clinic_id}/faqs      (one per FAQ)
4. POST /admin/clinics/{clinic_id}/services  (one per service)
5. POST /admin/clinics/{clinic_id}/doctors   (one per doctor)

6. POST /admin/users                 (create a clinic_admin for day-to-day use)
```

After step 5 the chatbot's RAG pipeline can answer questions about the clinic's services, doctors, and FAQs.

---

## JWT token structure

The decoded payload looks like:

```json
{
  "admin_id": "018e2f3a-...",
  "email": "admin@example.com",
  "role": "super_admin",
  "clinic_id": null,
  "exp": 1742256000,
  "iat": 1742169600
}
```

For `clinic_admin` the `clinic_id` field contains the clinic UUID string. For `super_admin` the field is absent.

The dashboard can decode the JWT client-side (without verifying the signature) to read `role` and `clinic_id` and adjust the UI accordingly — e.g. skip the clinic-picker for a `clinic_admin`.

---

## Environment variables

| Variable | Purpose |
|---|---|
| `ADMIN_JWT_SECRET` | HS256 signing key — use a long random string (32+ chars) |
| `ADMIN_BOOTSTRAP_EMAIL` | Email for the first super_admin created on startup |
| `ADMIN_BOOTSTRAP_PASSWORD` | Password for the bootstrap super_admin |

The bootstrap super_admin is upserted on every server restart — changing `ADMIN_BOOTSTRAP_PASSWORD` in the environment and restarting the server will update the password.
